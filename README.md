# rkt-proxy

[![release](https://img.shields.io/github/v/release/carlos-burelo/rkt-proxy)](https://github.com/carlos-burelo/rkt-proxy/releases/latest)
[![go](https://img.shields.io/badge/go-1.26-blue)](https://go.dev)
[![platform](https://img.shields.io/badge/platform-windows--amd64-blue)](https://github.com/carlos-burelo/rkt-proxy)
[![license](https://img.shields.io/github/license/carlos-burelo/rkt-proxy)](LICENSE)

Reverse proxy para **Windows Server** escrito en Go. Asigna dominios y
subdominios a puertos locales, con **TLS automático vía Let's Encrypt**
(emisión + renovación sin intervención).

```text
Internet --:443/80--> rkt-proxy --http--> 127.0.0.1:3000 (app1)
                                    \---> 127.0.0.1:8080 (app2)
```

## Características

- Routing por header `Host`: coincidencia exacta primero, luego `*.dominio`.
- Certificados Let's Encrypt automáticos (desafíos `tls-alpn-01` + `http-01`
  con fallback), cache separada staging/prod, renovación automática.
- Redirect `80 → 443` configurable; WebSocket soportado.
- Corre como **servicio Windows** (auto-arranque) o en foreground.
- **Rate-limit por IP** (token bucket) delante del backend: exceso → `429`.
- **WAF-lite**: `deny_paths` con globs (hot-reload) → `403` sin gastar tokens.
- **`/healthz` en loopback dedicado** con alcance real de backends (`ok/degraded`).
- **Access log** combinado en archivo, separado del operativo.
- Instalador prod de una línea (`irm … | iex`): binario, config, firewall,
  PATH y servicio. Updates versionados con backup y rollback automático.
- Logs a archivo + subcomando `logs -n/-f` (el stdout se pierde en servicios).
- Headers `X-Forwarded-For` (auto), `X-Forwarded-Proto`, `X-Forwarded-Host`,
  `X-Real-IP`. Sin ruta → `404`; backend caído → `502`.

## Instalación en producción

Requisitos previos:

| Requisito | Detalle |
|---|---|
| DNS | Registro `A` de cada dominio/subdominio → IP pública del VPS |
| Red | Puertos `80` y `443` abiertos (firewall Windows + grupo de seguridad cloud) |
| Apps | Corriendo en sus puertos locales (`127.0.0.1:PUERTO`) |
| Permisos | PowerShell como Administrador |

```powershell
irm https://raw.githubusercontent.com/carlos-burelo/rkt-proxy/main/scripts/install.ps1 | iex
```

El instalador descarga el `.exe` del último release (verifica checksum), crea
`config.yaml` si no existe, abre `80/443`, registra el dir en el `PATH` e
instala/arranca el servicio. Después edita la config y reinicia:

```powershell
notepad "C:\Program Files\rkt-proxy\config.yaml"
sc.exe stop RktProxy; sc.exe start RktProxy
```

## Configuración

```yaml
listen_http: ":80"
listen_https: ":443"

acme_email: "admin@tu-dominio.com"   # avisos LE; vale cualquier email
acme_staging: false                  # true para probar sin gastar rate-limit
acme_cache_dir: "C:\\ProgramData\\rkt-proxy\\certs"

redirect_http_to_https: true

routes:
  - hosts: ["app1.tu-dominio.com"]
    target: "http://127.0.0.1:3000"
  - hosts: ["app2.tu-dominio.com", "www.tu-dominio.com"]
    target: "http://127.0.0.1:8080"
  - hosts: ["*.tu-dominio.com"]      # resto de subs a este backend
    target: "http://127.0.0.1:4000"
```

Reglas de routing:

- `hosts` es la única forma canónica (lista, sin singular/plural duales).
- Gana la coincidencia **exacta** sobre el wildcard.
- `*.dominio` cubre subdominios, **no** la base `dominio` (listala explícita).
- Comparación case-insensitive; host duplicado = error al arrancar.
- El primer request a cada dominio emite su cert (tarda segundos); quedan en
  `acme_cache_dir` (`prod/` y `staging/` separados) y renuevan solos.

## Salud y límites

Hot-reload: editar `routes` o `redirect_http_to_https` aplica en ≤10s sin
restart (`config recargada` en log). YAML inválido = se conserva la anterior.
Cambios de listeners, ACME, healthz o rate-limit avisan `requieren restart`.
```powershell
curl.exe http://127.0.0.1:8090/healthz
# {"status":"ok","version":"v0.2.0","routes":2,"backends":[{"target":"http://127.0.0.1:3000","ok":true}]}
```

`status` es `degraded` si algún backend no acepta TCP. El rate-limit
(`requests_per_second` + `burst` por IP) responde `429` sin tocar el backend
y loguea `rate limit` con la IP. Defaults: `20 rps / burst 40`.

## WAF-lite y access log

`deny_paths` niega con `403` (globs estilo `path.Match`, hot-reload) antes
de rate-limit y backend. Defaults contra escaneos ubicuos:

```yaml
deny_paths: ["/.env", "/.git/*", "/.DS_Store", "/php-cgi*", "/php-cgi/*", "/cgi-bin/*", "/info.php", "/wp-*"]
```

Cada respuesta (incluidos 403/429/502) queda en `<dir-config>/logs/access.log`
en formato combinado + `host/target/dur`.

## Mini-PaaS: apps supervisadas

```yaml
apps:
  - name: app1
    command: "C:\\nodejs\\node.exe"
    args: ["server.js"]
    dir: "C:\\apps\\app1"
    env: { PORT: "3000" }
    restart_delay: 5s
    min_uptime: 10s
    memory_mb: 512
```

rkt levanta cada app al arrancar, guarda su salida en
`<dir-config>/logs/apps/<name>.log` y la reinicia si muere. Backoff
exponencial por crash rápido (base → tope 5m); vivir `min_uptime` resetea
backoff y no cuenta crash. `memory_mb` (Job Object, Windows) niega commits
sobre el tope: el runtime muere con OOM y rkt reinicia (0 = sin límite).
(tope: 5 muertes/minuto → `failed` hasta el próximo restart). Quitar una app
de la config + restart la detiene. `sc stop` / `Ctrl+C` matan las apps
(limpio, sin huérfanos). Cambios en `apps` exigen restart
(hot-reload solo cubre `routes` y `redirect`).

```powershell
rkt-proxy -config ... logs -app app1 -n 30
curl.exe http://127.0.0.1:8090/healthz  # incluye "apps": running/pid/restarts
```

## Restart sin cortes

```powershell
rkt-proxy -config ... restart app1
```

Reinicio supervisado: resetea contadores crash, mata la actual y arranca
al instante (salta el delay). Durante el hueco (proceso booteando) el proxy
**espera hasta `restart_grace`** reintentando TCP en vez de dar 502: cero
requests fallidos, solo latencia. Funciona con cualquier runtime sin
cooperación (sin cluster no hay handoff perfecto: un request justo en el
instante del kill puede caer; haz tus handlers idempotentes).

Nota Windows: el servicio corre como SYSTEM — usa rutas absolutas en
`command` (no ve tu PATH de usuario) y valida `dir` al cargar.

## Env vars sin fragilidad

```yaml
apps:
  - name: app1
    env_file: "C:\\apps\\app1\\.env"   # KEY=valor, # comentarios, export opcional
    env:
      PORT: "3000"
      DATABASE_URL: "postgres://${DB_USER}:${DB_PASS}@db:5432/app"
```

Capas en orden: **OS → `env_file` → `env:`** (cada una gana a la anterior).
`${VAR}`/`$VAR` expanden contra OS+file+inline-anteriores (ordenado,
determinista); indefinida = `""`. Archivo inexistente o línea malformada =
error al cargar (fail fast, no typo silencioso). Secretos: `config.yaml` y
`.env` en disco con ACL solo SYSTEM+Admins:

```powershell
icacls "C:\Program Files\rkt-proxy\config.yaml" /inheritance:r /grant:r "SYSTEM:(F)" "Administradores:(F)"
icacls "C:\apps\app1\.env" /inheritance:r /grant:r "SYSTEM:(F)" "Administradores:(F)"
```

Ejemplo listo en [`configs/config.example.yaml`](configs/config.example.yaml).

## Actualizar y rollback

Sin auto-update silencioso: cada deploy es explícito y versionado. Sacar una
versión = `git tag vX.Y.Z && git push origin vX.Y.Z` (CI testea, compila y
publica el release). Aplicarla = re-correr el instalador:

```powershell
# último release (salta si ya instalado, backup + rollback auto incluidos)
irm https://raw.githubusercontent.com/carlos-burelo/rkt-proxy/main/scripts/install.ps1 | iex

# pin de versión / rollback / forzar
$s = irm https://raw.githubusercontent.com/carlos-burelo/rkt-proxy/main/scripts/install.ps1
& ([scriptblock]::Create($s)) -Version v0.1.7
& ([scriptblock]::Create($s)) -Rollback
& ([scriptblock]::Create($s)) -Force
```

## Logs

El servicio escribe en `<dir-config>/logs/rkt-proxy.log` (rota a `.1` a los
10 MB). Desde cualquier terminal (el instalador agrega el dir al `PATH`):

```powershell
rkt-proxy -config "C:\Program Files\rkt-proxy\config.yaml" logs -n 50
rkt-proxy -config "C:\Program Files\rkt-proxy\config.yaml" logs -f
```

## Servicio Windows (manual)

```powershell
.\bin\rkt-proxy.exe -config C:\ruta\config.yaml install
sc.exe start RktProxy
sc.exe query RktProxy
sc.exe stop RktProxy
.\bin\rkt-proxy.exe uninstall
```

## Troubleshooting

| Síntoma | Causa probable | Fix |
|---|---|---|
| `bind: ... forbidden` en `:80` | IIS (`W3SVC`) u otro servicio ocupa el puerto | `sc.exe stop W3SVC; Set-Service W3SVC -StartupType Disabled` |
| `ERR_SSL_PROTOCOL_ERROR` | Emisión fallida: sin `:80` público no hay challenge | DNS + firewall/SG + `rkt-proxy logs` para el error ACME |
| Navegador no confía pero `curl` sí | Cert staging en uso o caché TLS vieja | `acme_staging: false` + restart (+ ventana privada) |
| `STOP_PENDING` tras `sc stop` | Conexiones colgadas | `taskkill /F /IM rkt-proxy.exe` y `sc start` |
| Escaneos (`/.env`, `/php-cgi`…) en logs | Bots de internet, normal en IP pública | Nada urgente; roadmap: rate-limit |

## Desarrollo

```powershell
go mod tidy
go vet ./...
go test ./...
go build -o bin/rkt-proxy.exe ./cmd/rkt-proxy
.\bin\rkt-proxy.exe -config config.yaml run
```

```text
cmd/rkt-proxy/main.go       entrypoint (flags, ~20 líneas)
internal/app/               ciclo de vida del servicio + subcomandos CLI
internal/config/            carga/validación YAML + match por Host
internal/proxy/             ReverseProxy (headers, 404/502)
internal/acme/              Manager autocert + HostPolicy + cache staging/prod
internal/server/            HTTP (challenge+redirect) y HTTPS (TLS+ALPN)
internal/logx/              logger consola+archivo y resolución de rutas
internal/ratelimit/         token bucket por IP + middleware 429
internal/waf/               deny_paths globs + middleware 403
internal/accesslog/         access log combinado a archivo
internal/health/            /healthz con chequeo TCP de backends
configs/config.example.yaml ejemplo canónico
scripts/install.ps1         instalador prod (irm|iex) y updater
.github/workflows/release.yml CI: vet+test+build+release en cada tag v*
```

## Licencia

[MIT](LICENSE) — Carlos Burelo.
