package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"
)

func version() string {
	b, err := os.ReadFile("VERSION")
	if err != nil {
		return "sin-version"
	}
	if s := strings.TrimSpace(string(b)); s != "" {
		return s
	}
	return "vacia"
}

func main() {
	v := version()
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, v)
	})
	_ = http.ListenAndServe("127.0.0.1:"+port, nil)
}
