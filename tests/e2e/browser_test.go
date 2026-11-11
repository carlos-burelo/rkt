//go:build e2e

package e2e_test

import "testing"

func TestBrowserSetupAndLogin(t *testing.T) {
	app := newBrowserApp(t)
	app.setup(t)
	app.login(t, "jefe", adminPassword)
}
