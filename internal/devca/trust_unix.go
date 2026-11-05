//go:build !windows

package devca

import (
	"fmt"
	"runtime"
)

// EnsureTrusted no instala la CA en el trust store del sistema: cada
// distribución lo gestiona distinto y suele requerir root. Se devuelve un
// error accionable; el servicio solo avisa y sigue.
func (c *CA) EnsureTrusted() error {
	return fmt.Errorf("CA local en %s (%s): expórtala con SSL_CERT_FILE=%s o impórtala al trust store del sistema",
		c.certPath, runtime.GOOS, c.certPath)
}
