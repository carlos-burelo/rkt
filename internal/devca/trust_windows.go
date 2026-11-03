//go:build windows

package devca

import (
	"fmt"
	"syscall"
	"unsafe"
)

// La CA se instala con Crypt32 directo, no con certutil: la ruta de certutil
// abre el diálogo "Advertencia de seguridad" y cuelga un proceso sin UI.
var (
	crypt32       = syscall.NewLazyDLL("crypt32.dll")
	certOpenStore = crypt32.NewProc("CertOpenStore")
	certAddEnc    = crypt32.NewProc("CertAddEncodedCertificateToStore")
	certClose     = crypt32.NewProc("CertCloseStore")
)

const (
	storeProvSystem        = 10
	systemStoreCurrentUser = 0x00010000
	encodingX509PKCS7      = 0x00010001
	storeAddNew            = 1 // falla con CRYPT_E_EXISTS si ya está
	cryptEExists           = syscall.Errno(0x80092005)
)

func openRootStore() (uintptr, error) {
	name, _ := syscall.UTF16PtrFromString("Root")
	h, _, err := certOpenStore.Call(storeProvSystem, 0, 0, systemStoreCurrentUser, uintptr(unsafe.Pointer(name)))
	if h == 0 {
		return 0, fmt.Errorf("abrir Root del usuario: %w", err)
	}
	return h, nil
}

// EnsureTrusted instala la CA en Root del usuario (sin admin, sin diálogo).
// Idempotente: CERT_STORE_ADD_NEW devuelve CRYPT_E_EXISTS si ya estaba.
func (c *CA) EnsureTrusted() error {
	h, err := openRootStore()
	if err != nil {
		return err
	}
	defer certClose.Call(h, 0)
	ok, _, cerr := certAddEnc.Call(h, encodingX509PKCS7, uintptr(unsafe.Pointer(&c.cert.Raw[0])), uintptr(len(c.cert.Raw)), storeAddNew, 0)
	if ok != 0 {
		return nil
	}
	if cerr == cryptEExists {
		return nil
	}
	return fmt.Errorf("CertAddEncodedCertificateToStore: %w; importa %s a mano", cerr, c.certPath)
}
