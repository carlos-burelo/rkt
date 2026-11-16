package logx

import (
	"os"
	"path/filepath"
	"testing"
)

func writeLog(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestClearLogsTruncaYBorraBackups(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "rkt.db")
	logs := DirFor(cfgPath)
	writeLog(t, filepath.Join(logs, "rkt.log"), "11111")
	writeLog(t, filepath.Join(logs, "rkt.log.1"), "22")
	writeLog(t, filepath.Join(logs, "access.log"), "333")
	writeLog(t, filepath.Join(logs, "apps", "w.log"), "4444")
	writeLog(t, filepath.Join(logs, "deploys", "1.log"), "5")

	cleared, err := ClearLogs(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int64{
		filepath.Join(logs, "rkt.log"):          5,
		filepath.Join(logs, "rkt.log.1"):        2,
		filepath.Join(logs, "access.log"):       3,
		filepath.Join(logs, "apps", "w.log"):    4,
		filepath.Join(logs, "deploys", "1.log"): 1,
	}
	if len(cleared) != len(want) {
		t.Fatalf("archivos = %v, quiere %d", cleared, len(want))
	}
	for p, n := range want {
		if cleared[p] != n {
			t.Fatalf("%s = %d, quiere %d", p, cleared[p], n)
		}
	}
	// Los .log quedan truncados; los .1 borrados.
	for _, p := range []string{
		filepath.Join(logs, "rkt.log"),
		filepath.Join(logs, "access.log"),
		filepath.Join(logs, "apps", "w.log"),
		filepath.Join(logs, "deploys", "1.log"),
	} {
		if st, err := os.Stat(p); err != nil || st.Size() != 0 {
			t.Fatalf("%s no quedó vacío", p)
		}
	}
	if _, err := os.Stat(filepath.Join(logs, "rkt.log.1")); !os.IsNotExist(err) {
		t.Fatal("backup .1 no borrado")
	}
}

func TestClearLogsSinLogsNoFalla(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "rkt.db")
	cleared, err := ClearLogs(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(cleared) != 0 {
		t.Fatalf("sin logs debe devolver vacío: %v", cleared)
	}
}
