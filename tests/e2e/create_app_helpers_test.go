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
