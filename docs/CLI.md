# Referencia CLI de `rkt`

Sintaxis: `rkt [-config <ruta>] [--json] <comando> [args]`.

- `-config` default `rkt.db`: ancla del home; data, secrets, logs, apps y certs cuelgan de `<dir>`.
  `dev` usa `rkt.local.db` cuando no se pasa `-config`.
- `--json` en comandos de lectura: salida JSON en vez de tabla.
- Códigos de salida: `0` ok · `1` error de runtime · `2` mal uso.

## Servicio

| Comando | Efecto |
|---|---|
| `install` | Registra el servicio Windows `Rkt` (auto-arranque) |
| `uninstall` | Detiene y elimina el servicio |
| `start` / `stop` | Arranca / detiene el servicio |
| `run` | Foreground con Ctrl+C limpio (apaga servidores + apps) |
| `dev [--http a:80] [--https a:443]` | Foreground local con puertos de prod y TLS de CA propia (`.localhost` funciona sin hosts); `--https ""` desactiva TLS |
| `version` | Versión del binario (`dev` en builds locales) |
| `upgrade [--version X|--local exe] [--rollback] [--force] [--check]` | Upgrade por slots: publica `versions/<ver>` al costado, flip de binpath + gate + rollback auto. Nunca escribe un exe en uso |
| `mcp` | Servidor MCP stdio para agentes (ver abajo) |
| `hosts list\|add\|rm <host>` | DNS local en el archivo hosts (admin) |
| `help` | Esta referencia resumida |

Sin args en terminal interactiva imprime ayuda; sin args como servicio arranca.

## Desarrollo local (hot reload)

Dos procesos: backend Go + Vite con HMR. El frontend cambia al instante;
el backend se reconstruye y reinicia a mano.

```powershell
# 1. Backend (API + dashboard embebido en :8090, apps en :18080)
.\bin\rkt.exe -config rkt.dev.db dev --http=127.0.0.1:18080 --https=

# 2. Frontend con HMR (http://127.0.0.1:5173, /api proxieado a :8090)
pnpm --dir web dev
```

Cambios web: recarga sola. Cambios Go: `go build -o bin/rkt.exe ./cmd/rkt`
y reinicia el proceso del paso 1.

## Operación (contra el servicio en marcha)

Requieren `healthz` activado (default `127.0.0.1:8090`, solo loopback).

| Comando | Efecto |
|---|---|
| `status [--json]` | Salud: versión, backends (ok/fallo), apps (running/pid/restarts/failed) |
| `logs [-n 50] [-f] [-app nombre]` | Últimas N líneas; `-f` sigue; `-app` lee `logs/apps/<nombre>.log` |
| `logs clear [--yes]` | Vacía rkt.log, access.log, apps/*.log y deploys/*.log (+ .1). Sin `--yes` pide `si` |
| `restart <app>` | Mata y arranca al instante (salta delay, resetea crashloop, revive `failed`) |
| `deploy <app>` | Clone + build en slot inactivo + canario + corte de ruta (zero downtime) + rollback auto |
| `rollback <app>` | Corta al slot inactivo (versión anterior) con blue-green + health |
| `deployments [-n 20] [--app x] [--json]` | Historial SQLite (`id, app, status, rev, started_at`) |

`deploy`/`rollback`/`restart`/`deployments`/`status` hablan HTTP a healthz;
si el servicio no responde, fallan con mensaje (exit 1).

## Gestión (flota en SQLite)

Toda mutación valida antes de persistir. `routes` aplica por hot-reload
(≤10s); `apps`/`env`/`settings` estructurales avisan restart.

| Comando | Efecto |
|---|---|
| `apps list [--json]` | `name, command, dir` |
| `apps add [repo] [flags]` | Wizard si faltan flags: infiere nombre, preset, puerto, dominio, comando. Flags = override. Sin `public_ip` el dominio por defecto es `<app>.localhost`; en público exige `base_domain` o `--domain`. Ej: `apps add https://github.com/o/app2.git` |
| `apps add --external --name dev --dir C:\repo [--domain dev.localhost] [--port 3010]` | Registra app externa: solo proxy (sin build/deploy/supervisor). El dev server lo corrés vos. Sin dominio usa el defecto local/público. |
| `apps rm <name>` | Elimina app (avisa revisar routes huérfanas) |
| `routes list [--json]` | `hosts, target, path?` |
| `routes add [--host H]... [--path /app] --target <url>` | Agrega ruta (hot). Sin hosts vale para cualquier host/IP; `--path` recorta el prefijo al proxear. Target `http(s)://` o `ws(s)://`; ws/wss se normalizan y conservan el upgrade |
| `routes rm <host> \| [--host H] [--path P]` | Quita host (borra la ruta si queda vacía) o rutas enteras por selector |
| `env list <app>` | Vars inline (`KEY=valor` por línea) |
| `env set <app> CLAVE=valor` | Escribe (+expande `${}`) |
| `env unset <app> CLAVE` | Borra |
| `env global` | Lista las variables de toda la flota |
| `env global set CLAVE=valor` | Escribe una global (las apps la heredan) |
| `env global unset CLAVE` | Borra una global |
| `settings list` | `node_bin, python_bin, base_domain, port_pool, site_name, quota_reqs, quota_bytes, public_ip, retention_*, dashboard_host, dashboard_listen` |
| `settings set CLAVE=valor` | Escribe setting (valida clave y valor) |
| `users list` | Usuarios dashboard (`name, role, created_at`) |
| `users add <nombre> [--role admin] [--password-stdin]` | Alta con password sin eco (el primero se fuerza a admin) |
| `users del <nombre>` | Baja (protege último admin, mata sesiones) |
| `users passwd <nombre>` | Rota password e invalida sesiones |
| `config export [file] [--include-secrets]` | JSON redactado por defecto; `--include-secrets` para respaldo restaurable |
| `config import <file>` | Restaura flota + settings (valida antes) |
| `backup [file]` | Copia consistente de SQLite con `VACUUM INTO` |
| `restore <file>` | Restaura SQLite (servicio detenido) |
| `pg status` | Postgres gestionado: versión y base (sin secretos) |
| `pg list` | Bases por app: esquema, rol, modo |
| `pg provision <app> [--mode dedicated\|shared]` | Crea esquema+rol e inyecta `DATABASE_URL` (sin password) en el env |
| `pg tables <app>` | Tablas del esquema |
| `pg rows <app> <tabla> [--n 100] [--page 1] [--order col] [--dir asc\|desc] [--filter col:op:val]... [--search q]` | Filas con total filtrado (ops: eq neq gt gte lt lte like ilike in isnull isnotnull) |
| `pg schema <app> [tabla]` | Inspector: columnas (tipo/nulo/default/PK), RLS, policies y conteo |
| `pg insert <app> <tabla> '{"col":valor}' [--yes]` | Inserta una fila (pide confirmar) |
| `pg update <app> <tabla> '<pk-json>' '<patch-json>' [--yes]` | Actualiza por PK exacta (pide confirmar) |
| `pg delete <app> <tabla> (--pk '<json>' \| --filter col:op:val...) [--yes]` | Borra por PK o filtros; sin ninguno no borra (pide confirmar) |
| `pg create-table <app> <tabla> '[{"name":"id","type":"uuid","pk":true}]' [--yes]` | Crea tabla (tipos cerrados, pide confirmar) |
| `pg add-column <app> <tabla> <col> <tipo> [--not-null] [--default ...] [--yes]` | Añade columna (pide confirmar) |
| `pg drop-table <app> <tabla> [--yes]` | Borra tabla (pide confirmar, los datos se pierden) |
| `pg drop-column <app> <tabla> <col> [--yes]` | Quita columna (pide confirmar) |
| `pg query <app> <sql> [--yes]` | Una sentencia SQL en el esquema (pide confirmar) |
| `pg backup <app>` | Vuelca el esquema a `data/backups/pg-<app>-<ts>.dump` (-Fc) |
| `pg restore <app> <dump> [--yes]` | Restaura el dump sobre la app (pide confirmar, sobrescribe) |
| `pg drop <app> [--yes]` | Borra esquema+rol (pide confirmar, los datos se pierden) |
| `doctor [--json]` | Entorno: toolchains con versión, node_bin/python_bin, pool con hueco, DB, servicio, puertos y git (warn = aviso, fail = bloqueante) |
| `github login` | URL para instalar la App en browser |
| `github init --name --domain [--org]` | URL del form manifest para crear la App |
| `help` | Esta referencia resumida |

`--preset next`: args `.next\standalone\server.js`, build
`pnpm install` + `pnpm run build`, `PORT=<port>`.
`--preset node`: `server.js` + `pnpm install`.
`--preset python`: `-u app.py`.

## Salidas JSON (schemas)

`status`:
```json
{"status":"ok","version":"v0.7.0","routes":1,
 "backends":[{"target":"http://127.0.0.1:3000","ok":true}],
 "apps":[{"name":"web","running":true,"pid":123,"restarts":0}]}
```

`deployments list`:
```json
[{"id":3,"app":"web","rev":"abc1234","status":"ok",
  "started_at":"2026-11-02T06:00:00Z","finished_at":"2026-11-02T06:01:00Z"}]
```

`apps list` / `routes list`: arrays de objetos con los campos de la tabla.
Vacío = `[]` (nunca `null` en listas).

`doctor` / `GET /api/v1/server/doctor`:
```json
[{"name":"git","state":"ok|warn|fail","detail":"git version 2.43.0","hint":"..."}]
```
`state: fail` bloquea (git, pool, DB, bins mal configurados);
`warn` avisa (toolchain ausente, *_bin sin fijar).

`GET /api/v1/waf` (operator): reglas, bloqueos y baneos activos.
```json
{"deny_paths": ["/*.php"], "deny_methods": ["TRACE"], "blocked": [{"at": "...", "ip": "...", "host": "...", "method": "GET", "path": "/.env", "rule": "/.env*"}], "banned": [{"ip": "...", "hits": 23, "until": "..."}]}
```
`PUT /api/v1/waf` reemplaza listas (ausente = conserva).
`DELETE /api/v1/waf/bans?ip=` levanta un bloqueo.

## Dashboard (`/` + `/api/v1/`)

SPA embebida servida en la raíz del listener healthz (loopback). Auth por
credenciales: crea el primer admin con `users add`, entra en
`/`, sesión por cookie httpOnly (30d). Roles:
viewer lee, operator muta, admin gestiona usuarios.

### Acceso externo

Dos caminos, ambos con login:

- **Recomendado:** `settings set dashboard_host=rkt.tudominio.com` y DNS A
  al servidor. Con IP pública (`public_ip`), `listen_https` defaultea a
  `:443` solo (desactívalo con `settings set listen_https=off`). Sin IP
  pública, TLS sigue desactivado. Reinicia el servicio tras cambiar
  listeners. Si hay `base_domain` y no defines `dashboard_host`, el
  default es `rkt.<base_domain>`; usa `dashboard_host=off` para
  desactivarlo.
- **Puerto directo:** `settings set dashboard_listen=0.0.0.0:8090` y
  reinicia el servicio (`sc.exe restart Rkt`). Abre el puerto 8090 en el
  firewall. Este listener es HTTP sin TLS: úsalo solo en red de confianza.

El listener de healthz/métricas/acciones sigue siendo loopback. La consola
externa solo sirve SPA + `/api/v1` + `/mcp`.

### MCP remoto

`POST /mcp` habla MCP Streamable HTTP. Requiere una API key Bearer
(`rkt_...`, se generan en Ajustes → MCP remoto, se muestran una sola
vez) o sesión de navegador con Origin propio (misma defensa CSRF que la API).

**Alcances por rol** (se elige al crear la key; tabla única en
`internal/authz`: vale igual para API, MCP-HTTP, MCP-stdio y healthz):

| Alcance | Puede |
|---|---|
| lectura (viewer) | consultar estado, logs, deployments, métricas, zonas, dominios, detectar planes, diff, env-keys, long-polls |
| operar (operator) | lo anterior + deploy, rollback, retry, restart, stop/start, build, apps, env (valores), rutas, alertas, discover, settings |
| admin | lo anterior + usuarios, tokens, `config_import`, `prune`, `hosts_*`, auditoría |

Herramientas sin entrada en la tabla exigen `admin` (fail-closed).
`/mcp` lleva rate-limit (8 rps, burst 32) contra agotamiento.
`rkt mcp` (stdio, canal local) aplica la misma tabla: sin `control.token`
legible es viewer; con él, operator. Nunca admin por stdio.
Los streams SSE aceptan `?ticket=` (POST `/api/v1/stream/ticket`, un uso,
60s) para clientes sin cookie como `EventSource`.

Endpoints principales (todos salvo login exigen sesión):
`GET status|deployments|events|logs|apps|routes|settings|users|alerts|env|audit|cron`,
`GET apps/:name|deployments/:id/log|domains/:host|domains/preflight|domains/suggest|usage|analytics|observability|server`,
`POST auth/login|auth/logout|users|routes|apps|apps/:n/restart|apps/:n/start|apps/:n/stop|deploy|rollback|deployments/:id/retry|deployments/:id/cancel|alerts|alerts/:id/test|cron/:app/:job/run|stream/ticket`,
`PUT apps/:name|settings|users/:n/password|users/:n/sessions/:id|cron/:app/:job`,
`DELETE users/:n|users/:n/sessions/:id|routes?host|apps/:n|alerts/:id|alert-channels/:id|cron/:app/:job`.
Errores: JSON `{error}` (401 sin sesión, 403 sin rol).

Páginas: Overview (métricas, usage, alertas, discovery, actividad),
Deployments (filtros, drawer con build log + timeline + pasos CI),
App (production, checklist, tráfico, settings, env, danger),
Logs (runtime/build, search, niveles, live, histograma),
Analytics, Observability, Domains (TLS), Environment (agrupado),
Settings, Users, CI, Alerts, Tareas (cron command/http).
⌘K paleta, toasts, PWA offline.

## Repos privados (git)

Orden único de acceso al clonar: **1) GitHub App** instalada en el dueño del repo (preferida: pide *All repositories* en tu cuenta y en cada org) → **2) credencial guardada** (repo exacto > `owner/*` > `*`) → **3) token de entorno heredado** → **4) anónimo**. Que la App no cubra un repo no es error: se prueba el siguiente mecanismo.

```
rkt git cred add --name ci --kind pat --match "acme/*" --secret-file token.txt   # PAT (repo / contents:read)
rkt git cred add --name web --kind ssh --match acme/web                          # genera deploy key; registra la pública en GitHub
rkt git cred list | rm <nombre>
```

- Los secretos viven en `secrets/git/` (0600); la API, el MCP y los logs solo muestran nombre, tipo, patrón y huella. Pásalos por `--secret-file` o `RKT_GIT_SECRET`, nunca por argv.
- **Revalidar la App** (`POST /api/v1/github/revalidate`, MCP `github_revalidate`, botón en Ajustes): relee instalaciones, marca las que quedaron en *selected* y, si hay un PAT con `read:org`, detecta tus orgs nuevas donde falta instalar la App (con el enlace directo).
- Probar un repo: `GET /api/v1/git/access?repo=` o MCP `git_access_check` (dice qué mecanismo se usa y prueba `git ls-remote`).
- MCP: `git_creds_list`, `git_cred_add` (ssh sin secreto = la privada nunca viaja), `git_cred_remove`, `git_access_check`, `github_revalidate`.
- El auto-deploy por sondeo requiere la App o un PAT; una deploy key SSH despliega manual o por webhook.

## Canary: subir un build sin git

Para probar un `rkt.exe` de desarrollo en un servidor sin pasar por commit/CI/release.
**Subir no actualiza**: el binario queda guardado (máx. 3, 256 MB) y aplicarlo es un paso aparte.

```
rkt canary push .inkt.exe --to https://panel.midominio.com --key $RKT_KEY --label (git describe)
rkt canary list   --to ... --key ...
rkt canary apply <id> --to ... --key ...     # self-update: gate de salud + rollback auto
rkt canary rollback --to ... --key ...       # vuelve al slot previo
```

- Requiere token **admin** (`RKT_URL` / `RKT_KEY` como alternativa a `--to` / `--key`). Usa https: el token y el binario viajan por ahí.
- La subida verifica `sha256` y que sea un ejecutable del SO. `--apply` en `push` sube y aplica en un paso.
- Aplicar lanza `rkt upgrade --local <canary> --force` desacoplado; el servicio se reinicia (~1 min) y su log queda en `logs/upgrade.log`.
- API: `GET|PUT /api/v1/canary`, `POST /api/v1/canary/{id}/apply`, `DELETE /api/v1/canary/{id}`, `POST /api/v1/canary/rollback`.
- MCP: `canary_list`, `canary_status`, `canary_fetch` (el servidor descarga de una URL https con sha256), `canary_apply`, `canary_discard`, `canary_rollback`. Un agente no puede pasar 60 MB como argumento: sube con `rkt canary push` o `canary_fetch`.
- Solo Windows para aplicar (igual que `rkt upgrade`).

## Dominios, correo y ataques (API + MCP)

- **Guía DNS**: `GET /api/v1/domains/guide?host=` y MCP `domain_guide` (con `wait_seconds` espera la propagación). Devuelve el registro A exacto, estado real y pasos.
- **Entorno de GitHub App**: `github_setup_check` / el wizard detectan IP pública sin dominio o A que no apunta al servidor y bloquean con el comando que lo arregla (`rkt set base_domain <dominio>`).
- **Alertas por correo**: `rkt set smtp_host/smtp_from [smtp_user smtp_pass smtp_security]` y un canal `mailto:a@x.com`. Prueba: `POST /api/v1/alert-channels/{id}/test` o MCP `alert_channel_test`.
- **Mapa de ataques**: `GET /api/v1/security/attacks?hours=` y MCP `attacks_summary`. Países con `POST /api/v1/security/geoip` / `geoip_update` (descarga DB-IP lite, CC BY 4.0).

## MCP (`rkt mcp`)

Servidor [Model Context Protocol](https://modelcontextprotocol.io) por stdio.
Mismo gate que el remoto (`internal/authz`): sin `control.token` legible
es viewer; con él, operator. Nunca admin por stdio (usuarios, tokens,
`config_import`, `prune` y `hosts_*` exigen dashboard/API).
Monitoreo: `status`, `logs` (`n`, `app`), `deployments_list` (`n`, `app`),
`events_list` (`n`, `kind`, `app`), `version`.
Flota: `apps_list` (dominio, puerto, estático, deploy, `dns_ok`),
`routes_list` (host→target con `dns_ok`), `zones_list` (localhost/test/base
con apex y subdominios), `detect_plan` (framework + install/build/start).
Pipeline: `apps_add` (repo/dir→app inferida), `app_update` (dominio, puerto,
comando, args, dir, estático, env_file, memoria), `build` (compila un dir
local y registra deployment), `env_set` (solo escritura),
`discover` (registro inmediato), `deploy_check` (preflight JSON),
`deployment_logs` (tail del build), `wait_healthy` (gate post-deploy),
`restart`, `rollback`, `deploy`, `retry`, `apps_remove` (baja con rutas),
`start`, `stop` (detención persistente).
Postgres: `pg_status|list|tables|rows|schema` (lectura; rows con
order/dir/filters/search), `pg_provision|backup` (escritura),
`pg_insert` (exige `confirm=insert`), `pg_update` (exige `confirm=update`),
`pg_delete` (exige `confirm=delete`; pk o filtros, sin ninguno no borra),
`pg_query` (exige `confirm=query`), `pg_restore`
(exige `confirm=restore`), `pg_drop` (exige `confirm=drop`).

Catálogo extendido (control total): `app_get`, `app_env_keys`, `env_delete`,
`env_global_list|set|delete` (variables de toda la flota; la app las anula),
`routes_add|update|remove`, `domains_suggest`, `verify_domain` (DNS + HTTP
real por Host), `hosts_list|add|remove`, `deployment_get`, `deploy_wait`,
`git_status`, `diff_rev`, `disk_usage`, `log_search`, `usage_summary`,
`proxy_map`, `backends_probe`, `settings_list|set`, `alerts_list|add|remove`,
`discover_status`, `users_list|add|remove|passwd`, `config_export|import`
(import exige `confirm=replace` y conserva Env de apps existentes si el JSON
no lo trae), `prune` (exige `confirm=prune`).

Monitoreo en vivo: `metrics_now` (estado/pid/RSS por app + tráfico 5m),
`app_usage` (CPU%, RSS/pico/virtual, I/O, handles/hilos, disco del proyecto,
log y tráfico real de la ventana), `process_stats` (RSS + %CPU), `traffic_rate`
(rps/errores/bytes/p50-p99 en ventana), `watch_status` (long-poll hasta
healthy/unhealthy/…),
`watch_events` (eventos nuevos al instante), `watch_deployment` (espera un
deploy nuevo), `tail_follow` (sigue un log y espera regex).
Límite: no toca el servicio Windows; los cambios de manifiesto se aplican
en caliente en ≤10s (supervisor.Apply).

```jsonc
// opencode.json (V2)
{"mcp": {"servers": {"rkt": {"type": "local",
  "command": ["C:\\rkt\\rkt.exe", "-config",
              "C:\\rkt\\rkt.db", "mcp"]}}},
 "plugins": []}
```
