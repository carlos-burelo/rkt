// Comando git: credenciales para repos privados fuera de la GitHub App.
// Opera directo sobre la DB y el directorio de secretos locales.
package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"rkt/internal/gitcred"
	"rkt/internal/logx"
	"rkt/internal/store"
)

func cmdGit(args []string, cfgPath string) {
	if len(args) < 2 || args[0] != "cred" {
		log.Fatal("uso: rkt git cred add --name N --kind pat|ssh [--match owner/*] [--secret-file F | env RKT_GIT_SECRET] | list | rm <nombre>")
	}
	db, err := store.Open(logx.DBPath(cfgPath))
	if err != nil {
		log.Fatalf("rkt git: %v", err)
	}
	defer db.Close()
	secrets := logx.SecretsDir(cfgPath)

	switch args[1] {
	case "list":
		list, err := db.ListGitCreds()
		if err != nil {
			log.Fatalf("rkt git: %v", err)
		}
		raw, _ := json.MarshalIndent(list, "", "  ")
		fmt.Println(string(raw))
	case "rm":
		if len(args) != 3 {
			log.Fatal("uso: rkt git cred rm <nombre>")
		}
		if err := gitcred.Remove(db, secrets, args[2]); err != nil {
			log.Fatalf("rkt git: %v", err)
		}
		auditLocal(cfgPath, "git cred rm", args[2])
		fmt.Println("credencial borrada")
	case "add":
		fs := flag.NewFlagSet("git cred add", flag.ContinueOnError)
		name := fs.String("name", "", "nombre de la credencial")
		kind := fs.String("kind", "", "pat | ssh")
		match := fs.String("match", "*", "repos: * | owner/* | owner/repo")
		secretFile := fs.String("secret-file", "", "archivo con el token (pat) o la llave privada (ssh); ssh sin esto genera una")
		if err := fs.Parse(args[2:]); err != nil {
			log.Fatal("uso: rkt git cred add --name N --kind pat|ssh [--match owner/*] [--secret-file F]")
		}
		secret := os.Getenv("RKT_GIT_SECRET") // nunca por argv: se vería en la lista de procesos
		if *secretFile != "" {
			b, err := os.ReadFile(*secretFile)
			if err != nil {
				log.Fatalf("rkt git: %v", err)
			}
			secret = string(b)
		}
		res, err := gitcred.Add(db, secrets, gitcred.AddReq{Name: *name, Kind: *kind, Match: *match, Secret: strings.TrimSpace(secret)})
		if err != nil {
			log.Fatalf("rkt git: %v", err)
		}
		auditLocal(cfgPath, "git cred add", *name)
		fmt.Printf("credencial %q guardada (%s, aplica a %s)\n", res.Cred.Name, res.Cred.Kind, res.Cred.Match)
		if res.DeployKey != "" {
			fmt.Printf("\nllave pública (regístrala como Deploy key del repo en GitHub):\n%s\nhuella: %s\n", res.DeployKey, res.Cred.Fingerprint)
		}
		fmt.Println("\nsiguiente:", res.Next)
	default:
		log.Fatalf("rkt git cred: subcomando %q desconocido", args[1])
	}
}
