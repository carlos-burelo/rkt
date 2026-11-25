//go:build e2e

package e2e_test

import (
	"testing"

	"github.com/mxschmitt/playwright-go"
)

func gotoCreate(t *testing.T, page playwright.Page, base string) {
	t.Helper()
	if _, err := page.Goto(base + "/apps/new"); err != nil {
		t.Fatal(err)
	}
	if err := page.GetByText("Nueva app").First().WaitFor(); err != nil {
		t.Fatal(err)
	}
}

func stepIs(t *testing.T, page playwright.Page, n string) {
	t.Helper()
	if err := page.GetByText("paso " + n + " de 3").First().WaitFor(); err != nil {
		t.Fatalf("no llegó a paso %s", n)
	}
}

func clickNext(t *testing.T, page playwright.Page) {
	t.Helper()
	if err := page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Continuar"}).Click(); err != nil {
		t.Fatal(err)
	}
}

func clickBack(t *testing.T, page playwright.Page) {
	t.Helper()
	if err := page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Atrás"}).Click(); err != nil {
		t.Fatal(err)
	}
}

// nombreBox devuelve el campo Nombre del wizard por placeholder exacto,
// no por label: el hint de Dominio ("vacío = nombre…") hace que GetByLabel
// (substring) resuelva a 2 elementos, y otros placeholders contienen
// "mi-app" ("apps/mi-app", "…\mi-app"). Un solo hogar para el selector.
func nombreBox(t *testing.T, page playwright.Page) playwright.Locator {
	t.Helper()
	return page.GetByPlaceholder("mi-app", playwright.PageGetByPlaceholderOptions{Exact: playwright.Bool(true)})
}

// repoDirBox devuelve el campo de carpeta del wizard por label exacto:
// el copy ya cambió una vez ("repositorio" -> "proyecto") y el substring
// es frágil. Un solo hogar para el selector.
func repoDirBox(t *testing.T, page playwright.Page) playwright.Locator {
	t.Helper()
	return page.GetByLabel("Carpeta del proyecto", playwright.PageGetByLabelOptions{Exact: playwright.Bool(true)})
}

func appInDB(t *testing.T, app *browserApp, name string) bool {
	t.Helper()
	fleet, err := app.db.LoadFleet()
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range fleet.Apps {
		if a.Name == name {
			return true
		}
	}
	return false
}
