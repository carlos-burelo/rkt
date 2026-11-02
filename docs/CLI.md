# Referencia CLI de `rkt`

Sintaxis: `rkt [-config <ruta>] [--json] <comando> [args]`.

- `-config` default `config.yaml` (relativa al `.exe` cuando corre como servicio).
- `--json` en comandos de lectura: salida JSON en vez de tabla.
- Códigos de salida: `0` ok · `1` error de runtime · `2` mal uso.

## Servicio

| Comando | Efecto |
|---|---|
| `install` | Registra el servicio Windows `Rkt` (auto-arranque) |
| `uninstall` | Detiene y elimina el servicio |
| `start` / `stop` | Arranca / detiene el servicio |
| `run` | Foreground con Ctrl+C limpio (apaga servidores + apps) |
| `dev [--port 8080]` | Foreground local: genera `rkt.dev.yaml` (HTTP alto, sin TLS) si falta y corre |
| `version` | Versión del binario (`dev` en builds locales) |
| `mcp` | Servidor MCP stdio para agentes (ver abajo) |
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
| `apps add [repo] [flags]` | Wizard si faltan flags: infiere nombre, preset, puerto, dominio, comando. Flags = override. Ej: `apps add https://github.com/o/app2.git` |
| `apps rm <name>` | Elimina app (avisa revisar routes huérfanas) |
| `routes list [--json]` | `hosts, target` |
| `routes add --host (repetible) --target <url>` | Agrega ruta (hot) |
| `routes rm <host>` | Quita host (borra la ruta si queda vacía) |
| `env list <app>` | Vars inline (`KEY=valor` por línea) |
| `env set <app> CLAVE=valor` | Escribe (+expande `${}`) |
| `env unset <app> CLAVE` | Borra |
| `settings list` | `node_bin, python_bin, base_domain, port_pool` |
| `settings set CLAVE=valor` | Escribe setting |
| `config export [file]` | Vuelca YAML completo (+`x-settings`) para git |
| `config import <file>` | Restaura flota + settings (valida antes) |
| `github login` | URL para instalar la App en browser |
| `github init --name --domain [--org]` | URL del form manifest para crear la App |
| `help` | Esta referencia resumida |

`--preset next`: args `.next\standalone\server.js`, build
`npm ci --omit=dev` + `npm run build`, `PORT=<port>`.
`--preset node`: `server.js` + `npm ci --omit=dev`.
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

## MCP (`rkt mcp`)

Servidor [Model Context Protocol](https://modelcontextprotocol.io) por stdio.
Monitoreo: `status`, `logs` (`n`, `app`), `deployments_list` (`n`, `app`),
`events_list` (`n`, `kind`, `app`), `version`.
Pipeline: `apps_add` (repo→app inferida), `env_set` (solo escritura),
`discover` (registro inmediato), `deploy_check` (preflight JSON),
`deployment_logs` (tail del build), `wait_healthy` (gate post-deploy),
`restart`, `rollback`, `deploy`.
Límite: no muta config fuera de apps/env, ni toca el servicio Windows.

```jsonc
// opencode.json
{"mcp": {"rkt": {"type": "local",
  "command": ["C:\\Program Files\\rkt-proxy\\rkt.exe", "-config",
              "C:\\Program Files\\rkt-proxy\\config.yaml", "mcp"],
  "enabled": true}}}
```
