//go:build windows

package cli

import (
	"fmt"
	"log"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/windows"
)

func platformRequireAdmin() error {
	var sid *windows.SID
	err := windows.AllocateAndInitializeSid(&windows.SECURITY_NT_AUTHORITY, 2,
		windows.SECURITY_BUILTIN_DOMAIN_RID, windows.DOMAIN_ALIAS_RID_ADMINS,
		0, 0, 0, 0, 0, 0, &sid)
	if err != nil {
		return err
	}
	defer windows.FreeSid(sid)
	tok, err := windows.OpenCurrentProcessToken()
	if err != nil {
		return err
	}
	defer tok.Close()
	ok, err := tok.IsMember(sid)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("sin elevación")
	}
	return nil
}

func scOut(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("%s %v: %v: %s", name, args, err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// scQueryService devuelve la salida de `sc.exe qc <svc>`.
func scQueryService(svc string) (string, error) {
	return scOut("sc.exe", "qc", svc)
}

// scQueryState devuelve el código numérico de estado (1 stopped, 4 running).
func scQueryState(svc string) (int, error) {
	out, err := scOut("sc.exe", "query", svc)
	if err != nil && !strings.Contains(out, "STATE") && !strings.Contains(out, "ESTADO") {
		return 0, err
	}
	for _, line := range strings.Split(out, "\n") {
		up := strings.ToUpper(line)
		if !strings.Contains(up, "STATE") && !strings.Contains(up, "ESTADO") {
			continue
		}
		i := strings.Index(line, ":")
		if i < 0 {
			continue
		}
		for _, f := range strings.Fields(line[i+1:]) {
			if n, err := strconv.Atoi(f); err == nil {
				return n, nil
			}
		}
	}
	return 0, fmt.Errorf("sin estado en sc query %s", svc)
}

// setServiceBinpath apunta el servicio al slot (preserva el -config vigente).
func setServiceBinpath(svc, exe, cfg string) {
	bin := fmt.Sprintf(`"%s" -config "%s"`, exe, cfg)
	if _, err := scOut("sc.exe", "config", svc, "binPath=", bin); err != nil {
		log.Fatalf("sc config: %v", err)
	}
	fmt.Printf("binpath -> %s\n", bin)
}

// restartService para, espera STOPPED, arranca y espera RUNNING.
func restartService(svc string) {
	if st, _ := scQueryState(svc); st != svcStopped {
		if _, err := scOut("sc.exe", "stop", svc); err != nil {
			log.Fatalf("sc stop: %v", err)
		}
		if err := waitState(svc, svcStopped, 60*time.Second); err != nil {
			log.Fatalf("parando %s: %v", svc, err)
		}
	}
	if _, err := scOut("sc.exe", "start", svc); err != nil {
		log.Fatalf("sc start: %v", err)
	}
	if err := waitState(svc, svcRunning, 60*time.Second); err != nil {
		log.Fatalf("arrancando %s: %v", svc, err)
	}
}

func waitState(svc string, want int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if st, err := scQueryState(svc); err == nil && st == want {
			return nil
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("sin estado %d en %v", want, timeout)
}

// bestEffortRecovery programa reinicios del servicio ante crash (avisa, no falla).
func bestEffortRecovery(svc string) {
	if _, err := scOut("sc.exe", "failure", svc, "reset=", "86400", "actions=", "restart/60000/restart/60000//"); err != nil {
		fmt.Printf("aviso: sin recovery en %s: %v\n", svc, err)
		return
	}
	fmt.Printf("recovery programado en %s\n", svc)
}
