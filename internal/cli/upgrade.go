// Comando upgrade: actualiza el binario del servicio por slots versionados.
//
// Layout: <home>/versions/<ver>/rkt.exe (inmutable por versión),
// <home>/rkt.exe = copia del slot activo (para el PATH),
// current.txt/previous.txt/VERSION.txt en <home>.
// Upgrade = publicar al costado (nunca se escribe un exe en uso) +
// flip de binpath del servicio + restart + gate + rollback auto.
// Si el CLI corre desde el lanzador, se re-ejecuta desde el slot staged
// antes de tocar nada.
package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"rkt/internal/build"
	"rkt/internal/logx"
)

const (
	upgradeService = "Rkt"
	upgradeRepo    = "carlos-burelo/rkt"
	upgradeAsset   = "rkt-windows-amd64.exe"
	upgradeTimeout = 120 * time.Second
)

const (
	svcStopped = 1
	svcRunning = 4
)

// upgradeVersion es un tag vA.B.C[-beta.N][-M-gHASH] parseado.
type upgradeVersion struct {
	core    [3]int
	beta    bool
	preNum  int
	commits int
	raw     string
}

func parseUpgradeVersion(s string) (upgradeVersion, error) {
	v := upgradeVersion{raw: s}
	t := strings.TrimSpace(strings.TrimPrefix(s, "v"))
	rest := ""
	if i := strings.Index(t, "-"); i >= 0 {
		rest = t[i+1:]
		t = t[:i]
	}
	parts := strings.Split(t, ".")
	if len(parts) != 3 {
		return v, fmt.Errorf("versión sin núcleo A.B.C: %s", s)
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return v, fmt.Errorf("versión sin núcleo A.B.C: %s", s)
		}
		v.core[i] = n
	}
	if rest != "" {
		pre := rest
		if i := strings.Index(rest, "-"); i >= 0 {
			pre = rest[:i]
			tail := rest[i+1:] // M-gHASH
			if j := strings.Index(tail, "-"); j >= 0 {
				tail = tail[:j]
			}
			n, err := strconv.Atoi(tail)
			if err != nil {
				return v, fmt.Errorf("versión con sufijo inválido: %s", s)
			}
			v.commits = n
		}
		low := strings.ToLower(pre)
		if strings.HasPrefix(low, "beta") {
			v.beta = true
			n, err := strconv.Atoi(strings.TrimPrefix(strings.TrimPrefix(low, "beta"), "."))
			if err != nil {
				return v, fmt.Errorf("versión con beta inválido: %s", s)
			}
			v.preNum = n
		} else {
			return v, fmt.Errorf("pre-release no soportado: %s", s)
		}
	}
	return v, nil
}

// compareUpgradeVersion: -1 si a<b, 0 igual, 1 si a>b. Estable > beta.
func compareUpgradeVersion(a, b upgradeVersion) int {
	for i := 0; i < 3; i++ {
		if a.core[i] != b.core[i] {
			if a.core[i] < b.core[i] {
				return -1
			}
			return 1
		}
	}
	if a.beta != b.beta {
		if a.beta {
			return -1
		}
		return 1
	}
	if a.preNum != b.preNum {
		if a.preNum < b.preNum {
			return -1
		}
		return 1
	}
	if a.commits != b.commits {
		if a.commits < b.commits {
			return -1
		}
		return 1
	}
	return 0
}

// slotDirName valida que la versión sirva como nombre de directorio.
func slotDirName(v string) (string, error) {
	if v == "" {
		return "", fmt.Errorf("versión vacía")
	}
	for _, r := range v {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-' || r == '+') {
			return "", fmt.Errorf("versión con carácter inválido: %s", v)
		}
	}
	return v, nil
}

// splitArgs parte una línea de comando respetando comillas dobles.
func splitArgs(s string) []string {
	var out []string
	var cur strings.Builder
	inQ := false
	flush := func() {
		if cur.Len() > 0 {
			out = append(out, cur.String())
			cur.Reset()
		}
	}
	for _, r := range s {
		switch {
		case r == '"':
			inQ = !inQ
		case (r == ' ' || r == '\t') && !inQ:
			flush()
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	return out
}

// parseServiceBinpath extrae exe y -config de `sc.exe qc` (ES o EN).
func parseServiceBinpath(qc string) (exe, cfg string, err error) {
	for _, line := range strings.Split(qc, "\n") {
		up := strings.ToUpper(line)
		if !strings.Contains(up, "RUTA_BINARIO") && !strings.Contains(up, "BINARY_PATH") && !strings.Contains(up, "BINARY_PATH_NAME") {
			continue
		}
		i := strings.Index(line, ":")
		if i < 0 {
			continue
		}
		toks := splitArgs(strings.TrimSpace(line[i+1:]))
		for j, t := range toks {
			if strings.EqualFold(t, "-config") && j+1 < len(toks) {
				return toks[0], toks[j+1], nil
			}
		}
		return "", "", fmt.Errorf("binpath sin -config: %s", strings.TrimSpace(line))
	}
	return "", "", fmt.Errorf("sc qc sin ruta binaria")
}

// readSlotFile lee un puntero de slot (vacío si no existe).
func readSlotFile(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// copyFileRetry copia con reintentos (el lanzador puede estar en uso).
func copyFileRetry(dst, src string) error {
	var err error
	for i := 0; i < 5; i++ {
		var in *os.File
		in, err = os.Open(src)
		if err != nil {
			return err
		}
		var out *os.File
		out, err = os.Create(dst)
		if err != nil {
			in.Close()
			return err
		}
		_, err = io.Copy(out, in)
		in.Close()
		out.Close()
		if err == nil {
			return nil
		}
		time.Sleep(2 * time.Second)
	}
	return err
}

// runSlotVersion ejecuta `<exe> version` y devuelve su salida.
func runSlotVersion(exe string) (string, error) {
	out, err := exec.Command(exe, "version").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

type githubRelease struct {
	TagName   string `json:"tag_name"`
	Draft     bool   `json:"draft"`
	CreatedAt string `json:"created_at"`
}

// resolveLatestRelease devuelve el tag más nuevo no-draft (incluye betas).
func resolveLatestRelease() (string, error) {
	req, _ := http.NewRequest("GET", "https://api.github.com/repos/"+upgradeRepo+"/releases?per_page=100", nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	c := &http.Client{Timeout: 60 * time.Second}
	resp, err := c.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("github releases: %s", resp.Status)
	}
	var rels []githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rels); err != nil {
		return "", err
	}
	type cand struct {
		tag, created string
	}
	var list []cand
	for _, r := range rels {
		if r.Draft || !strings.HasPrefix(r.TagName, "v") {
			continue
		}
		list = append(list, cand{r.TagName, r.CreatedAt})
	}
	if len(list) == 0 {
		return "", fmt.Errorf("sin releases publicados")
	}
	sort.Slice(list, func(i, j int) bool { return list[i].created > list[j].created })
	return list[0].tag, nil
}

// downloadAsset baja el exe del release y verifica checksum si se publica.
func downloadAsset(version, dest string) error {
	base := "https://github.com/" + upgradeRepo + "/releases/download/" + version + "/"
	fmt.Printf("descargando %s%s...\n", base, upgradeAsset)
	if err := downloadURL(base+upgradeAsset, dest); err != nil {
		return err
	}
	sumURL := base + "checksums.txt"
	sums, err := downloadBytes(sumURL)
	if err != nil {
		fmt.Printf("aviso: sin checksums (%v), se instala sin verificar\n", err)
		return nil
	}
	var want string
	for _, line := range strings.Split(string(sums), "\n") {
		f := strings.Fields(strings.TrimSpace(line))
		if len(f) == 2 && f[1] == upgradeAsset {
			want = strings.ToUpper(f[0])
		}
	}
	if want == "" {
		fmt.Println("aviso: sin línea de checksum, se instala sin verificar")
		return nil
	}
	raw, err := os.ReadFile(dest)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(raw)
	if got := strings.ToUpper(hex.EncodeToString(sum[:])); got != want {
		os.Remove(dest)
		return fmt.Errorf("checksum inválido en %s", upgradeAsset)
	}
	fmt.Println("checksum OK")
	return nil
}

func downloadURL(url, dest string) error {
	b, err := downloadBytes(url)
	if err != nil {
		return err
	}
	return os.WriteFile(dest, b, 0o644)
}

func downloadBytes(url string) ([]byte, error) {
	c := &http.Client{Timeout: 10 * time.Minute}
	resp, err := c.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("%s: %s", url, resp.Status)
	}
	var total int64
	var buf []byte
	tmp := make([]byte, 1<<20)
	for {
		n, err := resp.Body.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
			total += int64(n)
			if total%(5<<20) < 1<<20 {
				fmt.Printf("  %.1f MB...\n", float64(total)/(1<<20))
			}
		}
		if err != nil {
			if err == io.EOF {
				return buf, nil
			}
			return nil, err
		}
	}
}

// waitServiceVersion espera servicio RUNNING + /healthz con la versión diana.
func waitServiceVersion(healthzAddr, want string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	c := &http.Client{Timeout: 10 * time.Second}
	var last string
	for time.Now().Before(deadline) {
		st, err := scQueryState(upgradeService)
		if err == nil && st == svcRunning {
			resp, err := c.Get("http://" + healthzAddr + "/healthz")
			if err == nil {
				var v struct {
					Version string `json:"version"`
				}
				if json.NewDecoder(resp.Body).Decode(&v) == nil && v.Version != "" {
					last = v.Version
					resp.Body.Close()
					if v.Version == want {
						return nil
					}
				} else {
					resp.Body.Close()
				}
			}
		}
		time.Sleep(3 * time.Second)
	}
	if last == "" {
		return fmt.Errorf("el servicio no responde /healthz en %v", timeout)
	}
	return fmt.Errorf("sigue en %s, quería %s", last, want)
}

// pruneSlots conserva current+previous y borra el resto.
func pruneSlots(home, keep1, keep2 string) {
	dirs, err := os.ReadDir(logx.VersionsDir(home))
	if err != nil {
		return
	}
	for _, d := range dirs {
		if !d.IsDir() || d.Name() == keep1 || d.Name() == keep2 {
			continue
		}
		p := logx.SlotDir(home, d.Name())
		if err := os.RemoveAll(p); err != nil {
			fmt.Printf("aviso: no pude podar %s: %v\n", d.Name(), err)
			continue
		}
		fmt.Printf("podado %s\n", d.Name())
	}
}

func cmdUpgrade(args []string, cfgPath string) {
	fs := flag.NewFlagSet("upgrade", flag.ContinueOnError)
	verFlag := fs.String("version", "", "versión diana (default: último release)")
	localFlag := fs.String("local", "", "exe local a publicar como slot (dev)")
	rollbackFlag := fs.Bool("rollback", false, "volver al slot previo (previous.txt)")
	forceFlag := fs.Bool("force", false, "permite misma versión o downgrade")
	checkFlag := fs.Bool("check", false, "solo muestra actual vs diana")
	stagedFlag := fs.Bool("staged", false, "interno: ya corre desde el slot staged")
	timeoutFlag := fs.Int("timeout", 120, "tope del gate de salud en segundos")
	if err := fs.Parse(args); err != nil {
		log.Fatalf("uso: rkt upgrade [--version X] [--local exe] [--rollback] [--force] [--check]")
	}
	if runtime.GOOS != "windows" {
		log.Fatal("rkt upgrade: solo Windows")
	}
	if err := platformRequireAdmin(); err != nil {
		log.Fatalf("rkt upgrade: %v (ejecuta como Administrador)", err)
	}
	if *rollbackFlag {
		upgradeRollback(cfgPath, time.Duration(*timeoutFlag)*time.Second)
		return
	}

	qc, err := scQueryService(upgradeService)
	if err != nil {
		log.Fatalf("rkt upgrade: sin servicio %s (%v); instala primero", upgradeService, err)
	}
	curExe, curCfg, err := parseServiceBinpath(qc)
	if err != nil {
		log.Fatalf("rkt upgrade: %v", err)
	}
	_ = curExe
	home := logx.HomeDir(curCfg)
	fmt.Printf("servicio %s, home %s\n", upgradeService, home)

	var target string
	switch {
	case *localFlag != "":
		target, err = runSlotVersion(*localFlag)
		if err != nil {
			log.Fatalf("rkt upgrade: --local inválido: %v", err)
		}
	case *verFlag != "":
		target = strings.TrimSpace(*verFlag)
	default:
		target, err = resolveLatestRelease()
		if err != nil {
			log.Fatalf("rkt upgrade: no pude resolver último release: %v", err)
		}
	}
	if _, err := slotDirName(target); err != nil {
		log.Fatalf("rkt upgrade: %v", err)
	}
	fmt.Printf("actual: %s, diana: %s (binario: %s)\n", build.Version, target, curExe)
	if *checkFlag {
		return
	}

	curV, curErr := parseUpgradeVersion(build.Version)
	tgtV, err := parseUpgradeVersion(target)
	if err != nil {
		log.Fatalf("rkt upgrade: diana ilegible: %v", err)
	}
	if curErr == nil && !*forceFlag {
		if c := compareUpgradeVersion(tgtV, curV); c == 0 {
			fmt.Printf("ya estás en %s, nada que hacer\n", target)
			return
		} else if c < 0 {
			log.Fatalf("rkt upgrade: %s < %s (downgrade: usa --force)", target, build.Version)
		}
	}

	self, _ := os.Executable()
	selfSame := sameFile(self, logx.SlotExe(home, target))
	if !*stagedFlag && !selfSame {
		stageUpgradeSlot(home, target, *localFlag)
		reexecUpgradeSlot(home, target, curCfg, args)
		return // el hijo hace el flip
	}

	flipUpgradeSlot(home, target, curCfg, time.Duration(*timeoutFlag)*time.Second)
}

// stageUpgradeSlot publica el exe diana en versions/<target>/.
func stageUpgradeSlot(home, target, local string) {
	slot := logx.SlotDir(home, target)
	exe := logx.SlotExe(home, target)
	if _, err := os.Stat(exe); err == nil {
		if v, err := runSlotVersion(exe); err == nil && v == target {
			fmt.Printf("slot %s ya publicado\n", target)
			return
		}
		os.Remove(exe)
	}
	if err := os.MkdirAll(slot, 0o755); err != nil {
		log.Fatalf("rkt upgrade: %v", err)
	}
	if local != "" {
		raw, err := os.ReadFile(local)
		if err != nil {
			log.Fatalf("rkt upgrade: --local: %v", err)
		}
		if err := os.WriteFile(exe, raw, 0o644); err != nil {
			log.Fatalf("rkt upgrade: %v", err)
		}
	} else {
		if err := downloadAsset(target, exe); err != nil {
			log.Fatalf("rkt upgrade: %v", err)
		}
	}
	if v, err := runSlotVersion(exe); err != nil || (v != target && local == "") {
		log.Fatalf("rkt upgrade: el staged no reporta %s (dio %q, %v)", target, v, err)
	}
	fmt.Printf("slot %s listo\n", target)
}

// reexecUpgradeSlot re-ejecuta el CLI desde el slot y propaga su código.
func reexecUpgradeSlot(home, target, curCfg string, args []string) {
	exe := logx.SlotExe(home, target)
	abs, _ := filepath.Abs(curCfg)
	nargs := append([]string{"-config", abs, "upgrade", "--staged"}, args...)
	cmd := exec.Command(exe, nargs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			os.Exit(ee.ExitCode())
		}
		log.Fatalf("rkt upgrade: re-exec: %v", err)
	}
}

// flipUpgradeSlot cambia binpath, reinicia, gatea y confirma (o revierte).
func flipUpgradeSlot(home, target, curCfg string, timeout time.Duration) {
	newExe := logx.SlotExe(home, target)
	if _, err := os.Stat(newExe); err != nil {
		log.Fatalf("rkt upgrade: falta slot %s (corre sin --staged)", target)
	}
	old := readSlotFile(logx.CurrentFile(home))
	if old == "" {
		old = build.Version // primera vez con slots: el vigente
	}
	fmt.Printf("flip %s -> %s\n", old, target)
	if err := os.WriteFile(logx.PreviousFile(home), []byte(old), 0o644); err != nil {
		log.Fatalf("rkt upgrade: %v", err)
	}
	setServiceBinpath(upgradeService, newExe, curCfg)
	restartService(upgradeService)

	healthz := healthzAddr(curCfg)
	if err := waitServiceVersion(healthz, target, timeout); err != nil {
		fmt.Printf("gate falló: %v; revirtiendo a %s\n", err, old)
		if _, serr := os.Stat(logx.SlotExe(home, old)); serr != nil {
			log.Fatalf("rkt upgrade: rollback imposible (falta slot %s): %v", old, serr)
		}
		setServiceBinpath(upgradeService, logx.SlotExe(home, old), curCfg)
		restartService(upgradeService)
		if verr := waitServiceVersion(healthz, old, timeout/2); verr != nil {
			log.Fatalf("rkt upgrade: rollback falló (%v); servicio en estado manual, revisa sc.exe query %s", verr, upgradeService)
		}
		refreshLauncher(home, old)
		log.Fatalf("rkt upgrade: revertido a %s", old)
	}
	fmt.Printf("servicio en %s sano\n", target)
	if err := os.WriteFile(logx.CurrentFile(home), []byte(target), 0o644); err != nil {
		log.Fatalf("rkt upgrade: %v", err)
	}
	if err := os.WriteFile(logx.VersionFile(home), []byte(target), 0o644); err != nil {
		log.Fatalf("rkt upgrade: %v", err)
	}
	bestEffortRecovery(upgradeService)
	refreshLauncher(home, target)
	pruneSlots(home, target, old)
	fmt.Printf("upgrade a %s completo\n", target)
}

// upgradeRollback vuelve al slot de previous.txt.
func upgradeRollback(cfgPath string, timeout time.Duration) {
	qc, err := scQueryService(upgradeService)
	if err != nil {
		log.Fatalf("rkt upgrade: sin servicio %s (%v)", upgradeService, err)
	}
	_, curCfg, err := parseServiceBinpath(qc)
	if err != nil {
		log.Fatalf("rkt upgrade: %v", err)
	}
	home := logx.HomeDir(curCfg)
	prev := readSlotFile(logx.PreviousFile(home))
	if prev == "" {
		log.Fatal("rkt upgrade: sin previous.txt, nada a lo que volver")
	}
	cur := readSlotFile(logx.CurrentFile(home))
	fmt.Printf("rollback %s -> %s\n", cur, prev)
	setServiceBinpath(upgradeService, logx.SlotExe(home, prev), curCfg)
	restartService(upgradeService)
	if err := waitServiceVersion(healthzAddr(curCfg), prev, timeout); err != nil {
		log.Fatalf("rkt upgrade: rollback falló: %v", err)
	}
	os.WriteFile(logx.CurrentFile(home), []byte(prev), 0o644)
	os.WriteFile(logx.PreviousFile(home), []byte(cur), 0o644)
	os.WriteFile(logx.VersionFile(home), []byte(prev), 0o644)
	bestEffortRecovery(upgradeService)
	refreshLauncher(home, prev)
	fmt.Printf("rollback a %s completo\n", prev)
}

// refreshLauncher copia el slot activo al lanzador (PATH). Avisa, no falla.
func refreshLauncher(home, ver string) {
	dst := logx.LauncherPath(home)
	self, _ := os.Executable()
	if sameFile(self, dst) {
		fmt.Println("aviso: el lanzador está en uso; se refresca en la próxima invocación")
		return
	}
	if err := copyFileRetry(dst, logx.SlotExe(home, ver)); err != nil {
		fmt.Printf("aviso: no pude refrescar %s: %v\n", dst, err)
		return
	}
	fmt.Printf("lanzador %s actualizado\n", dst)
}

// healthzAddr lee el listen del healthz desde la config fusionada.
func healthzAddr(cfgPath string) string {
	cfg, db := merged(cfgPath)
	if db != nil {
		db.Close()
	}
	if cfg != nil && cfg.Healthz.Listen != "" {
		return cfg.Healthz.Listen
	}
	return "127.0.0.1:8090"
}

func sameFile(a, b string) bool {
	aa, err1 := filepath.Abs(a)
	bb, err2 := filepath.Abs(b)
	if err1 != nil || err2 != nil {
		return false
	}
	return strings.EqualFold(aa, bb)
}
