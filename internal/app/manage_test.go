package app

import (
	"strings"
	"testing"
)

func TestGithubInitURL(t *testing.T) {
	got := githubInitURL("app1.coatl.in", "rkt-deployer", "dacyti-gsi")
	want := "https://app1.coatl.in/_rkt/new-app?name=rkt-deployer&domain=app1.coatl.in&org=dacyti-gsi"
	if got != want {
		t.Fatalf("got %q quiere %q", got, want)
	}
	got = githubInitURL("vps.test", "mi app", "")
	if !strings.HasPrefix(got, "https://vps.test/_rkt/new-app?name=mi+app&domain=vps.test") || strings.Contains(got, "org=") {
		t.Fatalf("escape/org mal: %q", got)
	}
}
