package canary

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// LogPath salida del último upgrade lanzado (la consulta canary_status).
func LogPath(home string) string { return filepath.Join(home, "logs", "upgrade.log") }

// Launch ejecuta `rkt -config <cfg> upgrade <args...>` desacoplado del
// proceso actual: el upgrade reinicia el servicio (y por tanto a quien lo
// lanzó), así que el hijo debe sobrevivir. Salida a LogPath.
func Launch(cfgPath, home string, args ...string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(LogPath(home)), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(LogPath(home), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, append([]string{"-config", cfgPath, "upgrade"}, args...)...)
	cmd.Stdout, cmd.Stderr = out, out
	detach(cmd)
	if err := cmd.Start(); err != nil {
		out.Close()
		return fmt.Errorf("lanzar upgrade: %w", err)
	}
	go func() { _ = cmd.Wait(); out.Close() }()
	return nil
}
