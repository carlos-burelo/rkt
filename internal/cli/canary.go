// Comando canary: sube un rkt.exe de desarrollo a un servidor remoto sin
// pasar por git/release. `push` SOLO sube; aplicar es explícito (--apply o
// `canary apply`) y delega en el self-update con gate de salud y rollback.
package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// canaryCall request autenticado al API remoto; devuelve cuerpo y status.
func canaryCall(method, base, key, path string, body io.Reader, size int64, hdr map[string]string) (int, string, error) {
	req, err := http.NewRequest(method, strings.TrimRight(base, "/")+path, body)
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	if body != nil {
		req.ContentLength = size
		req.Header.Set("Content-Type", "application/octet-stream")
	}
	resp, err := (&http.Client{Timeout: 15 * time.Minute}).Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return resp.StatusCode, strings.TrimSpace(string(raw)), nil
}

func fileSHA256(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	return hex.EncodeToString(h.Sum(nil)), n, err
}

func cmdCanary(args []string) {
	fs := flag.NewFlagSet("canary", flag.ContinueOnError)
	to := fs.String("to", os.Getenv("RKT_URL"), "URL del servidor (o RKT_URL), ej: https://panel.midominio.com")
	key := fs.String("key", os.Getenv("RKT_KEY"), "token de API admin (o RKT_KEY)")
	label := fs.String("label", "", "etiqueta del canary (ej: git describe)")
	apply := fs.Bool("apply", false, "push: aplicar tras subir (self-update con rollback)")
	if len(args) == 0 {
		log.Fatal("uso: rkt canary push <rkt.exe> [--label X] [--apply] | list | apply <id> | rollback  (--to URL --key TOKEN)")
	}
	sub := args[0]
	rest := args[1:]
	// flags pueden ir antes o después del argumento posicional
	var pos []string
	for len(rest) > 0 {
		if err := fs.Parse(rest); err != nil {
			log.Fatalf("rkt canary: %v", err)
		}
		if fs.NArg() == 0 {
			break
		}
		pos = append(pos, fs.Arg(0))
		rest = fs.Args()[1:]
	}
	if *to == "" || *key == "" {
		log.Fatal("rkt canary: falta --to y --key (o RKT_URL / RKT_KEY)")
	}
	if u, err := url.Parse(*to); err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		log.Fatalf("rkt canary: --to inválido: %q", *to)
	}
	if strings.HasPrefix(*to, "http://") {
		fmt.Fprintln(os.Stderr, "aviso: http:// envía el token y el binario sin cifrar; usa https")
	}

	switch sub {
	case "push":
		if len(pos) != 1 {
			log.Fatal("uso: rkt canary push <rkt.exe>")
		}
		sum, size, err := fileSHA256(pos[0])
		if err != nil {
			log.Fatalf("rkt canary: %v", err)
		}
		f, err := os.Open(pos[0])
		if err != nil {
			log.Fatalf("rkt canary: %v", err)
		}
		defer f.Close()
		fmt.Printf("subiendo %s (%d MB, sha256 %s…)\n", pos[0], size>>20, sum[:12])
		code, body, err := canaryCall(http.MethodPut, *to, *key, "/api/v1/canary?label="+url.QueryEscape(*label), f, size,
			map[string]string{"X-Rkt-Sha256": sum})
		if err != nil || code != http.StatusCreated {
			log.Fatalf("rkt canary: subida falló (http %d): %v %s", code, err, body)
		}
		fmt.Println(body)
		if *apply {
			id := extractID(body)
			if id == "" {
				log.Fatal("rkt canary: no pude leer el id subido; aplica con `rkt canary apply <id>`")
			}
			canaryApplyCmd(*to, *key, id)
		}
	case "list":
		code, body, err := canaryCall(http.MethodGet, *to, *key, "/api/v1/canary", nil, 0, nil)
		if err != nil || code != http.StatusOK {
			log.Fatalf("rkt canary: http %d: %v %s", code, err, body)
		}
		fmt.Println(body)
	case "apply":
		if len(pos) != 1 {
			log.Fatal("uso: rkt canary apply <id>")
		}
		canaryApplyCmd(*to, *key, pos[0])
	case "rollback":
		code, body, err := canaryCall(http.MethodPost, *to, *key, "/api/v1/canary/rollback", nil, 0, nil)
		if err != nil || code != http.StatusOK {
			log.Fatalf("rkt canary: rollback falló (http %d): %v %s", code, err, body)
		}
		fmt.Println(body)
	default:
		log.Fatalf("rkt canary: subcomando %q desconocido", sub)
	}
}

func canaryApplyCmd(to, key, id string) {
	code, body, err := canaryCall(http.MethodPost, to, key, "/api/v1/canary/"+url.PathEscape(id)+"/apply", nil, 0, nil)
	if err != nil || code != http.StatusOK {
		log.Fatalf("rkt canary: apply falló (http %d): %v %s", code, err, body)
	}
	fmt.Println(body)
}

// extractID saca "id":"..." del JSON de subida sin atar el CLI al struct.
func extractID(body string) string {
	const k = `"id":"`
	i := strings.Index(body, k)
	if i < 0 {
		return ""
	}
	rest := body[i+len(k):]
	if j := strings.IndexByte(rest, '"'); j > 0 {
		return rest[:j]
	}
	return ""
}
