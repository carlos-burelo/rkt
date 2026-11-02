// Package hook: webhook GitHub -> auto-deploy instantáneo.
// POST /_rkt/hook/github con X-Hub-Signature-256 (HMAC del secreto).
// Solo eventos push a ramas trackeadas con auto:true. Responde 202
// y despliega en fondo (GitHub exige respuesta <10s).
package hook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"rkt-proxy/internal/config"
	"rkt-proxy/internal/github"
)

// Handler sirve el webhook. deployFn(name) hace el deploy en fondo.
func NewHandler(live *config.Live, secret string, deployFn func(name string), log *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				log.Error("hook: panic recuperado", "v", v)
				http.Error(w, "error interno", http.StatusInternalServerError)
			}
		}()
		if r.URL.Path != "/_rkt/hook/github" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "solo POST", http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
		if err != nil {
			http.Error(w, "body inválido", http.StatusBadRequest)
			return
		}
		if !validSignature(r.Header.Get("X-Hub-Signature-256"), body, secret) {
			log.Warn("hook: firma inválida", "remote", r.RemoteAddr)
			http.Error(w, "firma inválida", http.StatusUnauthorized)
			return
		}
		if r.Header.Get("X-GitHub-Event") != "push" {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		var ev struct {
			Ref        string `json:"ref"`
			Deleted    bool   `json:"deleted"`
			Repository struct {
				FullName string `json:"full_name"`
			} `json:"repository"`
		}
		if err := json.Unmarshal(body, &ev); err != nil || ev.Deleted {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		branch := strings.TrimPrefix(ev.Ref, "refs/heads/")
		matched := false
		for _, name := range matchApps(live.Get(), ev.Repository.FullName, branch) {
			matched = true
			log.Info("hook: push detectado, desplegando", "app", name, "rev", branch)
			go deployFn(name)
		}
		if !matched {
			log.Info("hook: push sin app (repo/rama no trackeados)",
				"repo", ev.Repository.FullName, "ref", ev.Ref)
		}
		w.WriteHeader(http.StatusAccepted)
	})
}

// matchApps apps con auto:true cuyo repo/rama coinciden. SHA pineado nunca.
func matchApps(cfg *config.Config, fullName, branch string) []string {
	out := []string{}
	for i := range cfg.Apps {
		a := &cfg.Apps[i]
		if a.Deploy == nil || !a.Deploy.Auto || branch == "" {
			continue
		}
		owner, repo, ok := github.ParseRepo(a.Deploy.Repo)
		if !ok {
			continue
		}
		if strings.EqualFold(owner+"/"+repo, fullName) && a.Deploy.Ref == branch {
			out = append(out, a.Name)
		}
	}
	return out
}

func validSignature(header string, body []byte, secret string) bool {
	if secret == "" {
		return false
	}
	const prefix = "sha256="
	if !strings.HasPrefix(header, prefix) {
		return false
	}
	got, err := hex.DecodeString(strings.TrimPrefix(header, prefix))
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hmac.Equal(got, mac.Sum(nil))
}
