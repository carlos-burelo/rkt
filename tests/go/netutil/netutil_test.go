package netutil

import (
	"net/http"
	"testing"
)

func TestPort(t *testing.T) {
	cases := []struct{ addr, def, want string }{
		{"", "80", "80"},
		{":80", "x", "80"},
		{"127.0.0.1:8080", "x", "8080"},
		{"[::1]:8090", "x", "8090"},
		{"localhost", "443", "443"},
	}
	for _, c := range cases {
		if got := Port(c.addr, c.def); got != c.want {
			t.Fatalf("Port(%q,%q)=%q quiere %q", c.addr, c.def, got, c.want)
		}
	}
}

func TestClientIP(t *testing.T) {
	r := &http.Request{RemoteAddr: "127.0.0.1:5555"}
	if got := ClientIP(r); got != "127.0.0.1" {
		t.Fatalf("ClientIP=%q", got)
	}
	r.RemoteAddr = "raro"
	if got := ClientIP(r); got != "raro" {
		t.Fatalf("ClientIP sin puerto=%q", got)
	}
}
