// Package github: auth vía GitHub App (JWT + installation token cacheado).
// Sin dependencias externas: RS256 a mano (header.payload.firma).
package github

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// Manager miente tokens de instalación (1h) con cache y refresh.
type Manager struct {
	appID          int64
	installationID int64
	key            *rsa.PrivateKey
	apiBase        string

	mu     sync.Mutex
	token  string
	expiry time.Time
	client *http.Client
}

// New lee la clave PEM (PKCS1 o PKCS8). apiBase para tests.
func New(appID, installationID int64, keyFile, apiBase string) (*Manager, error) {
	data, err := os.ReadFile(keyFile)
	if err != nil {
		return nil, fmt.Errorf("leer key_file: %w", err)
	}
	key, err := parseKey(data)
	if err != nil {
		return nil, err
	}
	if apiBase == "" {
		apiBase = "https://api.github.com"
	}
	return &Manager{
		appID: appID, installationID: installationID, key: key,
		apiBase: strings.TrimRight(apiBase, "/"),
		client:  &http.Client{Timeout: 15 * time.Second},
	}, nil
}

func parseKey(data []byte) (*rsa.PrivateKey, error) {
	b, _ := pem.Decode(data)
	if b == nil {
		return nil, fmt.Errorf("PEM inválido")
	}
	if k, err := x509.ParsePKCS1PrivateKey(b.Bytes); err == nil {
		return k, nil
	}
	k, err := x509.ParsePKCS8PrivateKey(b.Bytes)
	if err != nil {
		return nil, fmt.Errorf("clave no es RSA PKCS1/PKCS8: %w", err)
	}
	rsaKey, ok := k.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("clave no es RSA")
	}
	return rsaKey, nil
}

// Token devuelve installation token válido (cacheado hasta 5min antes).
func (m *Manager) Token() (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.token != "" && time.Now().Add(5*time.Minute).Before(m.expiry) {
		return m.token, nil
	}
	jwt, err := m.mintJWT()
	if err != nil {
		return "", err
	}
	req, _ := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/app/installations/%d/access_tokens", m.apiBase, m.installationID), nil)
	req.Header.Set("Authorization", "Bearer "+jwt)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := m.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("access_tokens: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("access_tokens: %s", resp.Status)
	}
	var out struct {
		Token     string `json:"token"`
		ExpiresAt string `json:"expires_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	exp, err := time.Parse(time.RFC3339, out.ExpiresAt)
	if err != nil {
		return "", fmt.Errorf("expires_at: %w", err)
	}
	m.token, m.expiry = out.Token, exp
	return out.Token, nil
}

func (m *Manager) mintJWT() (string, error) {
	now := time.Now()
	header := b64(`{"alg":"RS256","typ":"JWT"}`)
	payload := b64(fmt.Sprintf(`{"iat":%d,"exp":%d,"iss":%d}`,
		now.Add(-time.Minute).Unix(), now.Add(9*time.Minute).Unix(), m.appID))
	h := sha256.Sum256([]byte(header + "." + payload))
	sig, err := rsa.SignPKCS1v15(rand.Reader, m.key, crypto.SHA256, h[:])
	if err != nil {
		return "", err
	}
	return header + "." + payload + "." + b64raw(sig), nil
}

func b64(s string) string    { return base64.RawURLEncoding.EncodeToString([]byte(s)) }
func b64raw(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

// ParseRepo extrae owner/repo de URL https (con o sin .git).
func ParseRepo(url string) (string, string, bool) {
	s := strings.TrimSuffix(strings.TrimRight(strings.TrimSpace(url), "/"), ".git")
	parts := strings.SplitN(s, "://", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	segs := strings.Split(parts[1], "/")
	if len(segs) != 3 {
		return "", "", false
	}
	host, owner, repo := segs[0], segs[1], segs[2]
	if !strings.Contains(host, ".") || owner == "" || repo == "" ||
		strings.ContainsAny(owner+repo, " :@") {
		return "", "", false
	}
	return owner, repo, true
}

// CommitSHA sha del ref en GitHub API (repo público o con token).
func (m *Manager) CommitSHA(owner, repo, ref, token string) (string, error) {
	req, _ := http.NewRequest(http.MethodGet,
		fmt.Sprintf("%s/repos/%s/%s/commits/%s", m.apiBase, owner, repo, ref), nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("commits: %s", resp.Status)
	}
	var out struct {
		SHA string `json:"sha"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if out.SHA == "" {
		return "", fmt.Errorf("sin sha en respuesta")
	}
	return out.SHA, nil
}
