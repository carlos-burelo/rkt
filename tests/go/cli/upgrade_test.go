package cli

import (
	"strings"
	"testing"
)

func TestParseUpgradeVersion(t *testing.T) {
	ok := map[string]upgradeVersion{
		"v0.7.0":                 {core: [3]int{0, 7, 0}},
		"1.2.3":                  {core: [3]int{1, 2, 3}},
		"v0.7.0-beta.16":         {core: [3]int{0, 7, 0}, beta: true, preNum: 16},
		"v0.7.0-beta.16-1-gabcd": {core: [3]int{0, 7, 0}, beta: true, preNum: 16, commits: 1},
	}
	for in, want := range ok {
		got, err := parseUpgradeVersion(in)
		if err != nil {
			t.Fatalf("parse %q: %v", in, err)
		}
		if got.core != want.core || got.beta != want.beta || got.preNum != want.preNum || got.commits != want.commits {
			t.Fatalf("parse %q = %+v, quiere %+v", in, got, want)
		}
	}
	for _, in := range []string{"", "dev", "v1.2", "v1.2.x", "v1.2.3-rc.1", "v1.2.3-beta.x", "v1.2.3-beta.1-x"} {
		if _, err := parseUpgradeVersion(in); err == nil {
			t.Fatalf("parse %q debió fallar", in)
		}
	}
}

func TestCompareUpgradeVersion(t *testing.T) {
	must := func(s string) upgradeVersion {
		v, err := parseUpgradeVersion(s)
		if err != nil {
			t.Fatalf("parse %q: %v", s, err)
		}
		return v
	}
	cases := []struct {
		a, b string
		want int
	}{
		{"v0.7.0", "v0.7.0", 0},
		{"v0.7.1", "v0.7.0", 1},
		{"v0.7.0", "v0.8.0", -1},
		{"v0.7.0", "v0.7.0-beta.16", 1}, // estable > beta
		{"v0.7.0-beta.17", "v0.7.0-beta.16", 1},
		{"v0.7.0-beta.16-2-gabc", "v0.7.0-beta.16-1-gabc", 1},
		{"v1.0.0", "v0.9.9", 1},
	}
	for _, c := range cases {
		if got := compareUpgradeVersion(must(c.a), must(c.b)); got != c.want {
			t.Fatalf("compare %s vs %s = %d, quiere %d", c.a, c.b, got, c.want)
		}
		if got := compareUpgradeVersion(must(c.b), must(c.a)); got != -c.want {
			t.Fatalf("compare %s vs %s = %d, quiere %d", c.b, c.a, got, -c.want)
		}
	}
}

func TestSlotDirName(t *testing.T) {
	for _, v := range []string{"v0.7.0", "v0.7.0-beta.16-1-g5dafdb6", "v1.0.0+build"} {
		if _, err := slotDirName(v); err != nil {
			t.Fatalf("slot %q: %v", v, err)
		}
	}
	for _, v := range []string{"", "../x", "v1:2", `a\b`, "con espacio"} {
		if _, err := slotDirName(v); err == nil {
			t.Fatalf("slot %q debió fallar", v)
		}
	}
}

func TestParseServiceBinpath(t *testing.T) {
	es := "NOMBRE_SERVICIO: Rkt\n        TIPO               : 10  WIN32_OWN_PROCESS\n        NOMBRE_RUTA_BINARIO: \"C:\\Program Files\\rkt\\rkt.exe\" -config \"C:\\Program Files\\rkt\\rkt.db\"\n"
	exe, cfg, err := parseServiceBinpath(es)
	if err != nil || exe != `C:\Program Files\rkt\rkt.exe` || cfg != `C:\Program Files\rkt\rkt.db` {
		t.Fatalf("ES: exe=%q cfg=%q err=%v", exe, cfg, err)
	}
	en := "SERVICE_NAME: Rkt\n        BINARY_PATH_NAME   : \"C:\\r\\rkt.exe\" -config \"C:\\r\\rkt.db\"\n"
	exe, cfg, err = parseServiceBinpath(en)
	if err != nil || exe != `C:\r\rkt.exe` || cfg != `C:\r\rkt.db` {
		t.Fatalf("EN: exe=%q cfg=%q err=%v", exe, cfg, err)
	}
	for _, bad := range []string{"", "NOMBRE_RUTA_BINARIO: C:\\x.exe", "OTRA COSA: 1"} {
		if _, _, err := parseServiceBinpath(bad); err == nil {
			t.Fatalf("binpath %q debió fallar", bad)
		}
	}
}

func TestSplitArgs(t *testing.T) {
	got := splitArgs(`"C:\a b\rkt.exe" -config "C:\c\rkt.db"`)
	if len(got) != 3 || got[0] != `C:\a b\rkt.exe` || got[1] != "-config" || got[2] != `C:\c\rkt.db` {
		t.Fatalf("splitArgs = %q", strings.Join(got, "|"))
	}
}
