# rkt

[![release](https://img.shields.io/github/v/release/carlos-burelo/rkt-proxy)](https://github.com/carlos-burelo/rkt-proxy/releases/latest)
[![go](https://img.shields.io/badge/go-1.26-blue)](https://go.dev)
[![platform](https://img.shields.io/badge/platform-windows--amd64-blue)](https://github.com/carlos-burelo/rkt-proxy)
[![license](https://img.shields.io/github/license/carlos-burelo/rkt-proxy)](LICENSE)

PaaS para **Windows Server** en un binario Go (`rkt.exe`): dominios con TLS
automático, apps supervisadas y deploys desde Git. Repo: `carlos-burelo/rkt-proxy`
(el módulo Go conserva ese path por estabilidad).

```text
Internet --:443/80--> rkt --http--> 127.0.0.1:3000 (app1)
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

El instalador descarga el `.exe` del último release (verifica checksum),
abre `80/443`, registra el dir en el `PATH` e instala/arranca el servicio.
La configuración vive en SQLite (`logs/rkt.db`), no en archivos YAML: se
edita desde el dashboard o con el CLI (`rkt settings`, `rkt routes`, `rkt apps`).

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
restart (`config recargada` en log). Config inválida = se conserva la anterior.
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
rkt -config ... logs -app app1 -n 30
curl.exe http://127.0.0.1:8090/healthz  # incluye "apps": running/pid/restarts
```

## Restart sin cortes

```powershell
rkt -config ... restart app1
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
error al cargar (fail fast, no typo silencioso). Secretos: `rkt.db` y
`.env` en disco con ACL solo SYSTEM+Admins:

```powershell
icacls "C:\Program Files\rkt-proxy\rkt.db" /inheritance:r /grant:r "SYSTEM:(F)" "Administradores:(F)"
icacls "C:\apps\app1\.env" /inheritance:r /grant:r "SYSTEM:(F)" "Administradores:(F)"
```

## Deploy desde git (repos privados incluidos)

Por app, con un PAT (`contents:read`, fine-grained a esos repos) como
**variable de máquina** (la ve el servicio SYSTEM - nunca en la config):

```yaml
apps:
  - name: app1
    dir: "C:\\apps\\app1"
    deploy:
      repo: "https://github.com/tuorg/app1.git"
      ref: "main"                 # rama, tag o SHA (SHA = clon completo)
      token_env: "GITHUB_TOKEN"   # default; solo para URLs http(s)
      build: ["pnpm install", "pnpm run build"]  # sintaxis cmd.exe
```

```powershell
# una vez (persiste en máquina):
& ([scriptblock]::Create((irm https://raw.githubusercontent.com/carlos-burelo/rkt-proxy/main/scripts/install.ps1))) -GithubToken "ghp_..."

rkt -config ... deploy app1
```

Flujo (todo en el servicio): preflight (git, token, `dir`) → clone
`--depth 1` a staging → build → **para la app, swap atómico, arranca**
(imprescindible en Windows: el rename se bloquea con handles abiertos) →
healthz hasta sana (30s) → rollback automático si falla (devuelve backup +
reinicia). Log del build en `logs/deploys/<app>-<ts>.log`. Sin red en tests:
`repo` local (ruta) no pide token.

## Auto-deploy con GitHub App (push-to-deploy)

```yaml
github:
  app_id: 123456
  app_slug: "mi-app-rkt"
  key_file: "C:\\ProgramData\\rkt-proxy\\github-app.pem"
  poll_interval: 60s   # mín 10s
  # webhook_secret: "..."  # push instantáneo (ver abajo)
```

**Vincular en el browser** (cero PAT, cero YAML a mano):

```powershell
rkt github init --name rkt-deployer --domain tu-dominio.coatl.in --org dacyti-gsi
```

Abre la URL que imprime (form con manifest oficial ya rellenado: permisos
mínimos + webhook + setup). Pulsa Create en GitHub; al aprobar, GitHub vuelve
a `/_rkt/setup`, rkt canjea el code, guarda PEM + settings y redirige directo
a la instalación. **Instala eligiendo All repositories**: GitHub no permite
forzar la selección desde el manifest. La App se crea pública (`public: true`)
porque una App privada solo puede instalarse en la cuenta dueña; así puedes
elegir tu cuenta personal o una organización donde seas admin. Al terminar,
setup te devuelve al panel (`/settings?github=ok`).
Ajustes → GitHub App muestra el alcance y el botón para cambiarlo. Repo nuevo
= app + deploy solos. (Sin `--org` = App personal.) En desarrollo local no
hace falta dominio: el manifest usa `https://localhost` (CA local) o
`http://localhost:<puerto>`, sin webhook; el auto-deploy va por polling. Cada
poll compara `HEAD` local vs remoto y despliega solo si cambió (tokens de 1h
cacheados; repos http usan App, resto `token_env`). Sin inbound: puro outbound
a `api.github.com`.

## Webhook instantáneo (opcional)

Con `webhook_secret` rkt sirve `POST /_rkt/hook/github` en :80/:443
(fuera del routing, como `/.well-known`). GitHub firma con HMAC-SHA256:

```yaml
github:
  # ...
  webhook_secret: "cambia-esto-por-64-hex"
```

```powershell
# genera secreto:
[BitConverter]::ToString((1..32 | ForEach-Object { Get-Random -Max 256 })).Replace("-","").ToLower()
```

En la App: webhook URL `https://tu-dominio/_rkt/hook/github`, secret igual,
evento `push`. El manifest solo admite eventos estándar (doc GitHub):
`installation*` se rechazan ahí. Para altas/bajas instantáneas, suscríbelos
manual en App settings (si no, el poll las detecta igual). Solo despliega
ramas trackeadas con `auto: true` (SHA pineado nunca); responde `202` y
despliega en fondo (log en servicio + `logs/deploys/`). Sin secreto da 404.

## Auto-discovery (repo nuevo = app viva sola)

Cada poll revisa repos instalados de la App. Repo no registrado + preset
conocido (`package.json` con/sin next, `requirements.txt`) = app creada
(nombre slug, dominio `<repo>.<base_domain>`, puerto libre del pool,
binario de settings) + route + deploy inmediato:

```powershell
rkt settings set base_domain=coatl.in
rkt settings set node_bin="C:\Program Files\nodejs\node.exe"
# port_pool default 3000-3999; ajústalo con settings set port_pool=...
```

Stack raro = warn con el `apps add` manual (nada a medias).

## CLI de gestión (adiós config a mano)

`rkt help` lista todo. Códigos: `0` ok, `1` error runtime, `2` mal uso.
`--json` global para `status`, `deployments`, `apps list`, `routes list`.

```powershell
rkt apps add --name app2 --repo https://github.com/o/app2.git --domain app2.coatl.in --port 3002 --command C:\nodejs\node.exe --preset next --auto
rkt apps list
rkt routes list
rkt routes add --host x.coatl.in --host y.coatl.in --target http://127.0.0.1:4000
rkt env set app2 CLAVE=valor
rkt settings set base_domain=coatl.in
rkt config export respaldo.json
rkt config import respaldo.json
rkt status
rkt deploy app2
rkt rollback app2
rkt deployments list --app app2
```

La flota (apps/routes/settings) vive en SQLite (`<dir-config>/logs/rkt.db`);
el YAML conserva solo infra estable. Toda mutación valida + sube versión
(hot-reload la aplica); `apps`/`env` avisan restart. `config export` vuelca
todo (incluye settings) para respaldo en git; `import` lo restaura.
`deployments list [-n 20] [--app x]` lee el historial (deployments + crashes).

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

## Seguridad

Capas activas: HostPolicy (solo tus dominios emiten cert), WAF `deny_paths`,
rate-limit por IP, `max_body_mb` (default 10, `413` + acota chunked),
recovery anti-panic (500 sin tumbar), HMAC en webhook (sin secreto = 404),
timeouts de servidor, `/_rkt/*` reservado fuera del routing.

```yaml
max_body_mb: 10   # 0 = sin tope
```

Fronteras de confianza (léeme):

- **Deploy = código del repo corriendo como SYSTEM.** Solo repos tuyos;
  el swap exige parar la app (Windows) y el rollback es automático.
- **Secretos**: `webhook_secret` (mín 16, rechazado si corto), PAT/App key y
  `.env` con ACL SYSTEM+Admins (el instalador blinda `rkt.db`).
  Rota PATs con fecha de expiración; prefiere GitHub App (tokens de 1h).
- **healthz es loopback** por default; si lo expones, `/restart` y `/deploy`
  quedan sin auth — no lo hagas.

## MCP para agentes (`rkt mcp`)

Servidor MCP por stdio: `status`, `logs`, `deployments_list`, `restart`,
`rollback`, `deploy`. Lee local, actúa vía servicio. Límite: opera, no
configura (eso es del operador por CLI).

```jsonc
// opencode.json (V2)
{"mcp": {"servers": {"rkt": {"type": "local",
  "command": ["C:\\Program Files\\rkt-proxy\\rkt.exe", "-config",
              "C:\\Program Files\\rkt-proxy\\rkt.db", "mcp"]}}},
 "plugins": []}
```

```powershell
# Claude Code
claude mcp add rkt -- C:\Program` Files\rkt-proxy\rkt.exe -config "C:\Program Files\rkt-proxy\rkt.db" mcp
```

## Logs

El servicio escribe en `<dir-config>/logs/rkt.log` (rota a `.1` a los
10 MB). Desde cualquier terminal (el instalador agrega el dir al `PATH`):

```powershell
rkt -config "C:\Program Files\rkt-proxy\rkt.db" logs -n 50
rkt -config "C:\Program Files\rkt-proxy\rkt.db" logs -f
```

## Servicio Windows (manual)

```powershell
.\bin\rkt.exe -config C:\ruta\rkt.db install
sc.exe start Rkt
sc.exe query Rkt
sc.exe stop Rkt
.\bin\rkt.exe uninstall
```

## Troubleshooting

| Síntoma | Causa probable | Fix |
|---|---|---|
| `bind: ... forbidden` en `:80` | IIS (`W3SVC`) u otro servicio ocupa el puerto | `sc.exe stop W3SVC; Set-Service W3SVC -StartupType Disabled` |
| `ERR_SSL_PROTOCOL_ERROR` | Emisión fallida: sin `:80` público no hay challenge | DNS + firewall/SG + `rkt logs` para el error ACME |
| Navegador no confía pero `curl` sí | Cert staging en uso o caché TLS vieja | `acme_staging: false` + restart (+ ventana privada) |
| `STOP_PENDING` tras `sc stop` | Conexiones colgadas | `taskkill /F /IM rkt.exe` y `sc start` |
| Escaneos (`/.env`, `/php-cgi`…) en logs | Bots de internet, normal en IP pública | Nada urgente; roadmap: rate-limit |

## Desarrollo local (sin VPS)

```powershell
rkt dev
```

Corre en foreground con los **mismos puertos que producción**:
`127.0.0.1:80` (HTTP) y `127.0.0.1:443` (HTTPS), redirección 80→443 y
rate-limit off. El TLS usa una **CA propia** (`logs/devca/ca.crt`, se
instala sola en el almacén Root del usuario, sin admin); ACME no puede
emitir para `.localhost`. Dominios `.localhost` resuelven solos; para otros
agrega `127.0.0.1 app1.test` a `C:\Windows\System32\drivers\etc\hosts`
(una vez, como admin). Abre `https://app1.test` — la misma URL que en prod.

Flags: `--http 127.0.0.1:80` y `--https 127.0.0.1:443` (vacío desactiva TLS).
`listen_https` vacío en cualquier config desactiva TLS. Firefox no usa el
almacén de Windows: importa `logs/devca/ca.crt` a mano si lo usas.

## Desarrollo del proyecto

```powershell
go mod tidy
go vet ./...
go test ./...
go build -o bin/rkt.exe ./cmd/rkt-proxy
.\bin\rkt.exe -config rkt.db run
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
internal/hook/              webhook GitHub (HMAC) -> deploy
internal/deploy/            pull+build+swap+restart+rollback
internal/discover/          auto-registro desde repos instalados
internal/github/            GitHub App: JWT + tokens + API
internal/dotenv/            parser .env + expansión
internal/cfgfile/           bootstrap YAML + snapshot + export/import
internal/store/             SQLite: historial + flota (apps/routes/settings)
configs/config.example.yaml ejemplo canónico
scripts/install.ps1         instalador prod (irm|iex) y updater
.github/workflows/release.yml CI: vet+test+build+release en cada tag v*
```

## Licencia

[MIT](LICENSE) — Carlos Burelo.
