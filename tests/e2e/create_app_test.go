//go:build e2e

package e2e_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mxschmitt/playwright-go"
	"rkt/internal/store"
)

// Validación cliente del paso 2 (origen git por defecto): repo inválido,
// repo vacío, nombre inválido, auto-sugerencia y bloqueo de Continuar.
// Sin Red: nada válido llega a disparar la detección.
func TestCreateWizardValidacion(t *testing.T) {
	app := newBrowserApp(t)
	page := app.setup(t)
	gotoCreate(t, page, app.url)
	stepIs(t, page, "1")

	if err := page.Locator("button:has-text('Repositorio Git')").Click(); err != nil {
		t.Fatal(err)
	}
	clickNext(t, page)
	stepIs(t, page, "2")

	// Repo vacío: error y no avanza.
	clickNext(t, page)
	if err := page.GetByText("pega la URL del repositorio").WaitFor(); err != nil {
		t.Fatal(err)
	}
	stepIs(t, page, "2")

	// Repo con formato inválido: error sin Red.
	if err := page.GetByLabel("Repositorio").Fill("no-es-url"); err != nil {
		t.Fatal(err)
	}
	clickNext(t, page)
	if err := page.GetByText("usa https:// o git@").WaitFor(); err != nil {
		t.Fatal(err)
	}
	stepIs(t, page, "2")

	// Nombre auto-sugerido desde el repo (local, sin Red).
	if err := page.GetByLabel("Repositorio").Fill("https://github.com/o/MiApp.git"); err != nil {
		t.Fatal(err)
	}
	if v, err := nombreBox(t, page).InputValue(); err != nil || v != "miapp" {
		t.Fatalf("slug sugerido = %q, err = %v", v, err)
	}

	// Nombre inválido: error y no avanza (sin tocar Red).
	if err := nombreBox(t, page).Fill("MALA_NOMBRE!"); err != nil {
		t.Fatal(err)
	}
	clickNext(t, page)
	if err := page.GetByText("minúsculas, números y guiones").WaitFor(); err != nil {
		t.Fatal(err)
	}
	stepIs(t, page, "2")

	// Atrás conserva el origen elegido.
	clickBack(t, page)
	stepIs(t, page, "1")
}

// Origen manual de punta a punta sin compilar: validación, revisión,
// creación y navegación a la ficha.
func TestCreateManualCompleto(t *testing.T) {
	app := newBrowserApp(t)
	page := app.setup(t)
	gotoCreate(t, page, app.url)

	if err := page.Locator("button:has-text('Sin repositorio')").Click(); err != nil {
		t.Fatal(err)
	}
	clickNext(t, page)
	stepIs(t, page, "2")

	// Sin comando no avanza.
	clickNext(t, page)
	if err := page.GetByText("binario absoluto", playwright.PageGetByTextOptions{Exact: playwright.Bool(true)}).WaitFor(); err != nil {
		t.Fatal(err)
	}
	stepIs(t, page, "2")

	if err := page.GetByLabel("Comando").Fill(`C:\Windows\System32\cmd.exe`); err != nil {
		t.Fatal(err)
	}
	if err := nombreBox(t, page).Fill("man1"); err != nil {
		t.Fatal(err)
	}
	// El directorio vive en "avanzado": abrirlo no duplica el comando ya escrito.
	if err := page.Locator("button:has-text('avanzado')").Click(); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := page.GetByLabel("Directorio").Fill(dir); err != nil {
		t.Fatal(err)
	}
	if err := page.GetByLabel("Dominio").Fill("man1.test"); err != nil {
		t.Fatal(err)
	}
	clickNext(t, page)
	stepIs(t, page, "3")

	// La revisión muestra lo capturado.
	for _, want := range []string{"man1", `C:\Windows\System32\cmd.exe`, "man1.test"} {
		n, err := page.GetByText(want).Count()
		if err != nil || n == 0 {
			t.Fatalf("revisión sin %q", want)
		}
	}
	if err := page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Crear aplicación"}).Click(); err != nil {
		t.Fatal(err)
	}
	if err := page.WaitForURL("**/app/man1"); err != nil {
		t.Fatal(err)
	}
	if !appInDB(t, app, "man1") {
		t.Fatal("man1 no quedó en la DB")
	}
}

// Dev externo: validación de carpeta, filtro numérico del puerto,
// revisión sin-supervisión y creación sin build ni track.
func TestCreateExternalCompleto(t *testing.T) {
	app := newBrowserApp(t)
	page := app.setup(t)
	gotoCreate(t, page, app.url)

	if err := page.Locator("button:has-text('Dev externo')").Click(); err != nil {
		t.Fatal(err)
	}
	clickNext(t, page)
	stepIs(t, page, "2")

	// Sin carpeta no avanza.
	clickNext(t, page)
	if err := page.GetByText("ruta absoluta del proyecto").WaitFor(); err != nil {
		t.Fatal(err)
	}
	stepIs(t, page, "2")

	dir := t.TempDir()
	if err := nombreBox(t, page).Fill("ext1"); err != nil {
		t.Fatal(err)
	}
	if err := repoDirBox(t, page).Fill(dir); err != nil {
		t.Fatal(err)
	}
	// El puerto solo acepta dígitos.
	if err := page.Locator("button:has-text('avanzado')").Click(); err != nil {
		t.Fatal(err)
	}
	portInput := page.GetByLabel("Puerto")
	if err := portInput.Fill("ab12cd"); err != nil {
		t.Fatal(err)
	}
	if v, err := portInput.InputValue(); err != nil || v != "12" {
		t.Fatalf("puerto filtrado = %q, err = %v", v, err)
	}
	if err := portInput.Fill("3210"); err != nil {
		t.Fatal(err)
	}

	clickNext(t, page)
	stepIs(t, page, "3")
	if err := page.GetByText("sin build, sin deploy").First().WaitFor(); err != nil {
		t.Fatal(err)
	}
	if err := page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Crear aplicación"}).Click(); err != nil {
		t.Fatal(err)
	}
	if err := page.WaitForURL("**/app/ext1"); err != nil {
		t.Fatal(err)
	}
	if !appInDB(t, app, "ext1") {
		t.Fatal("ext1 no quedó en la DB")
	}
}

// Directorio local con proyecto estático: la detección corre al entrar al
// paso 3, se apaga la compilación y la creación no construye nada.
func TestCreateLocalSinBuild(t *testing.T) {
	app := newBrowserApp(t)
	page := app.setup(t)
	gotoCreate(t, page, app.url)

	if err := page.Locator("button:has-text('Directorio local')").Click(); err != nil {
		t.Fatal(err)
	}
	clickNext(t, page)
	stepIs(t, page, "2")

	// Sin carpeta no avanza.
	clickNext(t, page)
	if err := page.GetByText("ruta absoluta del proyecto").WaitFor(); err != nil {
		t.Fatal(err)
	}
	stepIs(t, page, "2")

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "miweb"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "miweb", "index.html"), []byte("<h1>h</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := page.GetByLabel("Directorio").Fill(filepath.Join(dir, "miweb")); err != nil {
		t.Fatal(err)
	}
	clickNext(t, page)
	stepIs(t, page, "3")

	// La detección local termina: "analizando" desaparece del DOM.
	analyzing := page.GetByText("analizando")
	if err := analyzing.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateDetached, Timeout: playwright.Float(15000)}); err != nil {
		t.Fatalf("detección no terminó: %v", err)
	}

	// Sin compilar: creación directa a la ficha, sin seguimiento.
	box := page.GetByRole("checkbox")
	if n, _ := box.Count(); n != 1 {
		t.Fatalf("checkboxes = %d, quiere 1", n)
	}
	if err := box.Uncheck(); err != nil {
		t.Fatal(err)
	}
	if err := page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Crear aplicación"}).Click(); err != nil {
		t.Fatal(err)
	}
	if err := page.WaitForURL("**/app/miweb"); err != nil {
		t.Fatal(err)
	}
	if !appInDB(t, app, "miweb") {
		t.Fatal("miweb no quedó en la DB")
	}
}

// Atrás conserva origen y lo capturado.
func TestCreateAtrasConserva(t *testing.T) {
	app := newBrowserApp(t)
	page := app.setup(t)
	gotoCreate(t, page, app.url)

	if err := page.Locator("button:has-text('Dev externo')").Click(); err != nil {
		t.Fatal(err)
	}
	clickNext(t, page)
	stepIs(t, page, "2")

	dir := t.TempDir()
	if err := nombreBox(t, page).Fill("back1"); err != nil {
		t.Fatal(err)
	}
	if err := repoDirBox(t, page).Fill(dir); err != nil {
		t.Fatal(err)
	}
	if err := page.GetByLabel("Puerto").Fill("4321"); err != nil {
		t.Fatal(err)
	}
	clickNext(t, page)
	stepIs(t, page, "3")

	clickBack(t, page)
	stepIs(t, page, "2")
	if v, err := nombreBox(t, page).InputValue(); err != nil || v != "back1" {
		t.Fatalf("nombre no conservado: %q %v", v, err)
	}
	if v, err := repoDirBox(t, page).InputValue(); err != nil || v != dir {
		t.Fatalf("dir no conservado: %q %v", v, err)
	}

	clickBack(t, page)
	stepIs(t, page, "1")
	pressed, err := page.Locator("button:has-text('Dev externo')").GetAttribute("aria-pressed")
	if err != nil || pressed != "true" {
		t.Fatalf("origen no conservado: %q %v", pressed, err)
	}
}

// Viewer: abre el wizard pero el servidor deniega la creación (403).
// No navega y no persiste nada.
func TestCreateViewerSinPermiso(t *testing.T) {
	app := newBrowserApp(t)
	app.setup(t)
	if err := app.db.CreateUser("miron", "password-larga-1", store.RoleViewer); err != nil {
		t.Fatal(err)
	}
	page := app.login(t, "miron", "password-larga-1")
	gotoCreate(t, page, app.url)
	stepIs(t, page, "1")

	if err := page.Locator("button:has-text('Dev externo')").Click(); err != nil {
		t.Fatal(err)
	}
	clickNext(t, page)
	stepIs(t, page, "2")

	dir := t.TempDir()
	if err := nombreBox(t, page).Fill("noview"); err != nil {
		t.Fatal(err)
	}
	if err := repoDirBox(t, page).Fill(dir); err != nil {
		t.Fatal(err)
	}
	if err := page.GetByLabel("Puerto").Fill("4322"); err != nil {
		t.Fatal(err)
	}
	clickNext(t, page)
	stepIs(t, page, "3")
	if err := page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Crear aplicación"}).Click(); err != nil {
		t.Fatal(err)
	}
	if err := page.WaitForURL("**/apps/new", playwright.PageWaitForURLOptions{Timeout: playwright.Float(5000)}); err != nil {
		t.Fatalf("el viewer no debió salir del wizard: %v", err)
	}
	if appInDB(t, app, "noview") {
		t.Fatal("el viewer logró crear app")
	}
}

// Picker de GitHub sin App configurada: panel vacío determinista (sin Red;
// el backend responde [] cuando no hay manager).
func TestCreateGithubPickerVacio(t *testing.T) {
	app := newBrowserApp(t)
	page := app.setup(t)
	gotoCreate(t, page, app.url)
	clickNext(t, page)
	stepIs(t, page, "2")

	if err := page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "GitHub"}).Click(); err != nil {
		t.Fatal(err)
	}
	if err := page.GetByText("Sin repositorios").First().WaitFor(); err != nil {
		t.Fatal(err)
	}
}
