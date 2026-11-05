// Package platform: diferencias de sistema operativo en un solo lugar.
// El resto del backend no pregunta por runtime.GOOS ni hardcodea rutas.
package platform

import (
	"os/exec"
	"runtime"
)

// DefaultAppsDir directorio padre de apps gestionadas.
func DefaultAppsDir() string {
	if runtime.GOOS == "windows" {
		return `C:\apps`
	}
	return "/srv/rkt/apps"
}

// DefaultCertsDir cache ACME por defecto.
func DefaultCertsDir() string {
	if runtime.GOOS == "windows" {
		return `C:\ProgramData\rkt-proxy\certs`
	}
	return "/var/lib/rkt/certs"
}

// Shell devuelve el comando para ejecutar un paso de build.
func Shell(step string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.Command("cmd.exe", "/C", step)
	}
	return exec.Command("sh", "-c", step)
}

// OpenBrowser abre una URL (best effort). El caller decide si ignora el error.
func OpenBrowser(url string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		return exec.Command("open", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
}
