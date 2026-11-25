package logx

import (
	"path/filepath"
	"testing"
)

func TestHomeLayout(t *testing.T) {
	home := filepath.Join("C:", "rkt")
	if got := VersionsDir(home); got != filepath.Join(home, "versions") {
		t.Fatalf("VersionsDir = %s", got)
	}
	if got := SlotExe(home, "v0.7.0"); got != filepath.Join(home, "versions", "v0.7.0", "rkt.exe") {
		t.Fatalf("SlotExe = %s", got)
	}
	for _, tc := range []struct {
		got, want string
	}{
		{CurrentFile(home), filepath.Join(home, "current.txt")},
		{PreviousFile(home), filepath.Join(home, "previous.txt")},
		{VersionFile(home), filepath.Join(home, "VERSION.txt")},
		{LauncherPath(home), filepath.Join(home, "rkt.exe")},
		{DBPath(filepath.Join(home, "rkt.db")), filepath.Join(home, "data", "rkt.db")},
	} {
		if tc.got != tc.want {
			t.Fatalf("got %s, quiere %s", tc.got, tc.want)
		}
	}
}
