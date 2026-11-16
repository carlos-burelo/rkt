//go:build e2e

package e2e_test

import (
	"strings"
	"testing"

	"github.com/mxschmitt/playwright-go"
)

// Vaciar logs desde Registros (admin, doble clic de confirmación):
// toastea el conteo y la consola queda vacía.
func TestLogsClearDesdeUI(t *testing.T) {
	app := newBrowserApp(t)
	page := app.setup(t)
	msgs := []string{}
	page.On("response", func(r playwright.Response) {
		if strings.Contains(r.Request().URL(), "/api/") {
			body, _ := r.Text()
			if len(body) > 200 {
				body = body[:200]
			}
			msgs = append(msgs, r.Request().Method()+" "+r.Request().URL()+": "+string(r.StatusText())+" "+body)
		}
	})
	if _, err := page.Goto(app.url + "/logs"); err != nil {
		t.Fatal(err)
	}
	if err := page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "vaciar logs"}).Click(); err != nil {
		t.Fatal(err)
	}
	if err := page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "clic otra vez para vaciar"}).Click(); err != nil {
		t.Fatal(err)
	}
	if err := page.GetByText("logs vaciados:").WaitFor(); err != nil {
		t.Fatalf("sin toast de confirmación: %v\nconsola: %v", err, msgs)
	}
}
