# Referencia CLI de `rkt`

Sintaxis: `rkt [-config <ruta>] [--json] <comando> [args]`.

- `-config` default `rkt.db`: ancla de datos; DB y logs viven en `<dir>/logs`.
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
| `mcp` | Servidor MCP stdio para agentes (ver abajo) |
| `hosts list\|add\|rm <host>` | DNS local en el archivo hosts (admin) |
| `help` | Esta referencia resumida |

Sin args en terminal interactiva imprime ayuda; sin args como servicio arranca.

## Operación (contra el servicio en marcha)

Requieren `healthz` activado (default `127.0.0.1:8090`, solo loopback).

| Comando | Efecto |
|---|---|
| `status [--json]` | Salud: versión, backends (ok/fallo), apps (running/pid/restarts/failed) |
| `logs [-n 50] [-f] [-app nombre]` | Últimas N líneas; `-f` sigue; `-app` lee `logs/apps/<nombre>.log` |
| `restart <app>` | Mata y arranca al instante (salta delay, resetea crashloop, revive `failed`) |
| `deploy <app>` | Clone + build + swap atómico + restart + health 30s + rollback auto |
| `rollback <app>` | Devuelve `live.prev` a `live` + restart + health |
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
| `routes list [--json]` | `hosts, target` |
| `routes add --host (repetible) --target <url>` | Agrega ruta (hot). Target `http(s)://` o `ws(s)://`; ws/wss se normalizan y conservan el upgrade |
| `routes rm <host>` | Quita host (borra la ruta si queda vacía) |
| `env list <app>` | Vars inline (`KEY=valor` por línea) |
| `env set <app> CLAVE=valor` | Escribe (+expande `${}`) |
| `env unset <app> CLAVE` | Borra |
| `env global` | Lista las variables de toda la flota |
| `env global set CLAVE=valor` | Escribe una global (las apps la heredan) |
| `env global unset CLAVE` | Borra una global |
| `settings list` | `node_bin, python_bin, base_domain, port_pool, site_name, quota_reqs, quota_bytes, public_ip, retention_*, dashboard_host, dashboard_listen` |
| `settings set CLAVE=valor` | Escribe setting (valida clave y valor) |
| `users list` | Usuarios dashboard (`name, role, created_at`) |
| `users add <nombre> [--role admin]` | Alta con password sin eco (el primero se fuerza a admin) |
| `users del <nombre>` | Baja (protege último admin, mata sesiones) |
| `users passwd <nombre>` | Rota password e invalida sesiones |
| `config export [file] [--include-secrets]` | JSON redactado por defecto; `--include-secrets` para respaldo restaurable |
| `config import <file>` | Restaura flota + settings (valida antes) |
| `backup [file]` | Copia consistente de SQLite con `VACUUM INTO` |
| `restore <file>` | Restaura SQLite (servicio detenido) |
| `doctor` | Diagnóstico de config, DB, servicio, puertos y git |
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

## Dashboard (`/` + `/api/v1/`)

SPA embebida servida en la raíz del listener healthz (loopback). Auth por
credenciales: crea el primer admin con `users add`, entra en
`/`, sesión por cookie httpOnly (30d). Roles:
viewer lee, operator muta, admin gestiona usuarios.

### Acceso externo

Dos caminos, ambos con login:

- **Recomendado:** `settings set dashboard_host=rkt.tudominio.com` y DNS A
  al servidor. Se sirve en 80/443 con TLS automático. Si hay `base_domain`
  y no defines `dashboard_host`, el default es `rkt.<base_domain>`; usa
  `dashboard_host=off` para desactivarlo.
- **Puerto directo:** `settings set dashboard_listen=0.0.0.0:8090` y
  reinicia el servicio (`sc.exe restart Rkt`). Abre el puerto 8090 en el
  firewall. Este listener es HTTP sin TLS: úsalo solo en red de confianza.

El listener de healthz/métricas/acciones sigue siendo loopback. La consola
externa solo sirve SPA + `/api/v1`.

Endpoints principales (todos salvo login exigen sesión):
`GET status|deployments|events|logs|apps|routes|settings|users|alerts|env|audit|cron`,
`GET apps/:name|deployments/:id/log|domains/:host|domains/preflight|domains/suggest|usage|analytics|observability|server`,
`POST auth/login|auth/logout|users|routes|apps|apps/:n/restart|apps/:n/start|apps/:n/stop|deploy|rollback|deployments/:id/retry|deployments/:id/cancel|alerts|alerts/:id/test|cron/:app/:job/run`,
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

## MCP (`rkt mcp`)

Servidor [Model Context Protocol](https://modelcontextprotocol.io) por stdio.
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
  "command": ["C:\\Program Files\\rkt-proxy\\rkt.exe", "-config",
              "C:\\Program Files\\rkt-proxy\\rkt.db", "mcp"]}}},
 "plugins": []}
```
