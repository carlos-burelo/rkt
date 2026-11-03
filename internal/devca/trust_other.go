//go:build !windows

package devca

// EnsureTrusted: fuera de Windows la confianza se gestiona aparte.
func (c *CA) EnsureTrusted() error { return nil }
