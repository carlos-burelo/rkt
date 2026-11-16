# Arquitectura

Reglas de diseño que el código debe cumplir. Si una capa necesita lógica de
otra, se mueve al hogar correcto; no se duplica.

## Capas y dirección de dependencias

```text
cmd/rkt
  ├─ internal/cli          comandos, servicio Windows y salida de operador
  └─ internal/app          composición del runtime (Program)
       ├─ internal/api      transporte HTTP /api/v1
       ├─ internal/health   transporte HTTP loopback /healthz
       ├─ internal/mcpserver transporte MCP stdio
       ├─ internal/cfgfile  mutaciones de flota + carga/guardado
       │    ├─ internal/store   SQLite
       │    └─ internal/config  modelo, validación, Live
       └─ dominio: proxy, supervisor, deploy, discover, github,
          buildpack, acme, devca, weblog, accesslog, alerts, domains,
          cronjobs, ...
```

- `cmd` solo cablea. `cli` interpreta comandos y habla con el servicio o
  SQLite. `app` compone y orquesta el runtime; ninguno expone reglas de negocio.
- Los transportes (`api`, `health`, `mcpserver`) parsean, autorizan y llaman a
  `cfgfile` o al servicio. Nunca reimplementan mutaciones de flota.
- `cfgfile` es el único camino de escritura de apps/rutas/env. Valida antes de
  persistir (`SaveFleet`) y las operaciones parciales son atómicas.
- `store` solo persiste. `config` solo modela, valida y publica la config viva.
- Los paquetes de dominio no importan transportes ni `app`.

## Hogares únicos

| Concepto | Hogar |
|---|---|
| Modelo, defaults y validación | `internal/config` |
| Copia editable de la config viva | `config.Config.Clone` |
| Config viva con swap atómico | `config.Live` |
| Escritura de flota | `internal/cfgfile` |
| Alta de app (inferencia + rutas) | `cfgfile.CreateAndAdd` |
| Patch/borrado de app, rutas, env | `cfgfile/mutate.go` |
| Vistas para API/MCP | `internal/appview` |
| Puerto de listen y IP de cliente | `internal/netutil` |
| Validación de ajustes de flota | `internal/settings` |
| Dominio por defecto local/público | `internal/domains` |
| Resolución DNS de dominios | `internal/dnscheck` |
| Pipeline deploy/build | `internal/deploy` |
| Procesos y reinicios | `internal/supervisor` |
| Tareas programadas por app | `internal/cronjobs` |
| Historial y eventos | `internal/store` |
| Autorización: roles, ops y tabla única | `internal/authz` (API, MCP-HTTP, MCP-stdio y healthz deciden aquí) |
| Postgres gestionado (una base, esquema por app) | `internal/pg` (SQL puro + servicio; `store` persiste `pg_apps`, `api/pg.go` expone) |
| Layout del home (slots, current/previous, lanzador, DB, logs) | `internal/logx` (canónico; `install.ps1` lo espeja) |

## Base inmutable (contratos on-disk y de servicio)

Lo que sigue no se cambia sin migración explícita. El server en limpio
parte de aquí:

1. **Un solo hogar**: `<InstallDir>` (defecto `C:\Program Files\rkt`).
   `rkt.db` (ancla), `data/rkt.db` (SQLite real), `versions/<ver>/rkt.exe`
   (slots inmutables), `rkt.exe` (lanzador = copia del slot activo, PATH),
   `current.txt`/`previous.txt`/`VERSION.txt`, `logs/`, `secrets/`,
   `apps/`, `certs/`, `data/backups/`. Canónico en `internal/logx`.
2. **Servicio `Rkt` (LocalSystem, auto) con binpath al slot**:
   `"<slot>\rkt.exe" -config "<home>\rkt.db"`. Nunca se escribe el exe en
   uso: upgrade = publicar al costado + flip + gate + rollback auto.
   Recovery: 3 reinicios ante crash (`sc.exe failure`).
3. **SQLite aditiva**: esquema solo crece (`migrations[]`, `user_version`);
   tablas nuevas con `IF NOT EXISTS`. Destructivo = `backup`/`restore`.
4. **Datos sobreviven** a upgrade y a desinstala (el MSI/script solo quita
   lo que instaló; `nuke.ps1` es el único borrado total).
5. **Identidad del producto**: módulo `rkt`, binario `rkt.exe`,
   repo `carlos-burelo/rkt`, servicio `Rkt`.

## Plataformas

`internal/platform` concentra defaults y diferencias de OS. El backend no
pregunta por `runtime.GOOS` fuera de ese paquete ni de los wrappers con build
tags.| Pieza | Windows | Linux/Unix |
|---|---|---|
| Procesos hijos | Job Object | process group (`Setpgid`) |
| Límite RAM | Job Object | no disponible (degradado) |
| Métricas de proceso | Win32 | `/proc/<pid>` |
| Pasos de build | `cmd.exe /C` | `sh -c` |
| Hosts locales | `%SystemRoot%\...\hosts` | `/etc/hosts` |
| CA de desarrollo | Root del usuario | manual (`SSL_CERT_FILE`) |
| Abrir navegador | `rundll32` | `xdg-open` / `open` |

Estado: Windows es la única plataforma objetivo (hasta nuevo aviso).
Quedan rutas Linux/Unix como best-effort sin validar; CI no las corre.

## Protocolos

El proxy acepta targets `http`, `https`, `ws` y `wss`; `ws`/`wss` se
normalizan a `http`/`https` para el transporte y conservan el upgrade. Soporta
HTTP/1.1, HTTP/2 cuando el listener TLS lo permite, WebSocket/WSS y SSE con
flush inmediato. TCP crudo queda fuera de alcance.

## Puertos y dominios (reglas de operación)

1. **Todo backend escucha en `loopback:puerto` interno.** Lo asigna rkt
   del pool (`port_pool`); el usuario no lo elige (solo override avanzado).
2. **El usuario nunca marca ese puerto: entra por dominio.** El puerto del
   proxy es de la instancia, no de la app (`:18080` en dev, `:80/:443`
   en producción).
3. **Cada corte blue-green publica en un puerto libre nuevo y re-enruta;**
   el anterior se libera. El puerto de creación caduca en el primer deploy.
4. **Solo las apps externas exigen puerto**: es el proceso del usuario y
   rkt debe saber dónde escucha. Ahí sí es input obligatorio y visible.
5. **Público vs local solo cambia dominio+TLS** (`<app>.localhost` sin
   nada vs `<app>.<base>` con ACME). El modelo de puertos es el mismo.

## Invariantes

1. **Validar antes de persistir.** Ninguna escritura inválida llega a SQLite.
2. **Config viva inmutable.** Se lee sin lock; toda mutación es clonar → mutar
   la copia → validar → `Swap`.
3. **Un camino por operación.** API, CLI y MCP comparten la misma función de
   dominio. Si aparece una segunda, se borra la anterior.
4. **Handlers finos.** Parseo, auth y respuesta en el handler; reglas en el
   paquete de dominio.
5. **Sin estado dual.** SQLite es la única fuente de flota; no hay YAML
   paralelo ni flags legacy.
6. **Errores accionables.** Qué falló + comando/ajuste exacto que lo corrige.
7. **Postgres gestionado, no servido.** rkt no corre el servidor: el operador
   instala el cluster y rkt aprovisiona encima (una base, un esquema por app,
   rol propio o `rkt_apps` compartido). Trust solo en loopback (`pg_hba` con
   bloque gestionado + `pg_reload_conf`); `DATABASE_URL` sin password, el
   superuser vive en settings y sale enmascarado. Destructivo con
   confirmación en los tres frentes (UI dos clics, CLI prompt/`--yes`,
   MCP `confirm`).

## Estado del refactor

Hecho:

- `internal/cli` separado de `internal/app` (comandos vs runtime).
- `internal/api` dividido por recurso: `api.go` queda en ~290 líneas
  (Deps, NewHandler, helpers); handlers en `apps.go`, `routes.go`,
  `deployments.go`, `analytics.go`, `auth.go`, `env.go`, `settings.go`,
  `github.go`, `alerts.go`, `status.go`, `domains.go`, `cron.go`.
- Mutaciones de flota canónicas en `cfgfile/mutate.go`.
- Carrera del supervisor corregida (`m.apps` bajo lock).
- Parseo de listen y RemoteAddr unificado en `internal/netutil`.
- Escrituras de flota con read-modify-write transaccional (`BEGIN IMMEDIATE`)
  vía `cfgfile.MutateFleet`/`MutateInfra`/`ReplaceAll`; API, CLI y MCP comparten
  el mismo camino y no se pisan entre procesos.
- Migraciones SQLite atómicas por transacción.
- `internal/app` dividido: `service.go` (runtime), `github.go`, `background.go`.
- `CreateDialog.tsx` reducido a estado/orquestación (~330 líneas):
  helpers en `createDialog.model.tsx`, pasos 1–3 en `CreateSteps.tsx` y
  tracking en `CreateTrack.tsx`.
- Suite completa validada en Linux (WSL Ubuntu) y CI con job `-race`.
- `Program.Stop` sincronizado con el arranque (`stateMu`) para el detector.
- Dominios de app como conjunto de hosts en rutas; `cfgfile.SetAppDomains`
  reemplaza el conjunto y el detalle DNS/TLS vive en `internal/dnscheck`.
- Entorno con listas de claves, revelado por clave y `ApplyEnvPatch` atómico.
- Ajustes tipados en `internal/settings`; `store` solo persiste.
- Apps con `disabled` persistido y `health_path`; `start`/`stop` canónicos.
- Alertas con canales, historial y outbox de entregas.
- Sesiones revocables, cambio/reset de contraseña y auditoría mínima.
- Despliegues cancelables con contexto; pasos con duración en el log.
- Dominios por defecto según modo de red: local usa `<app>.localhost`; público
  exige `base_domain` o un dominio explícito. Hogar: `internal/domains`.
- Tareas cron por app con dos tipos: `command` (shell/env/dir) y `http`
  (GET/POST al puerto local). Historial en SQLite y log por ejecución.
- Marca canónica en `web/public/icons/mark.svg`; favicon e iconos PWA se
  generan desde ese SVG con `scripts/shots/icons.mjs`.
- Un solo verificador: `internal/authz` con tabla única op→rol; API, MCP-HTTP,
  MCP-stdio y healthz deciden ahí. Sesiones valen en `/mcp`, streams con
  `?ticket=` de un uso, stdio con gate (local=operator, nunca admin).
- Grupo `rkt-operators` (RX en hogar/logs, nunca secrets/data): operadores leen
  logs sin ser admins; la gestión va por dashboard/API.
- Upgrade por slots: `versions/<ver>` inmutables + `rkt upgrade` (flip de
  binpath + gate + rollback auto); `install.ps1` instala al slot.
- Journal de cortes: intent `started` antes de persistir, `done` tras adoptar;
  el arranque marca `aborted` lo pendiente (deriva DB-vs-runtime visible).
- Canario con reintento ante puerto ocupado (TOCTOU del efímero) y reubicación
  de puertos grabados ocupados al arrancar/reiniciar (con retarget de rutas).
- Deriva visible: `pending_restart` en `/api/v1/status`, SSE y banner del dashboard.
- Auditoría completa del CLI local (actor = usuario del SO); healthz ya no
  necesita auditar (cada edge audita el suyo).
- Job `race-win` en CI (supervisor, proxy, store, hub, api con `-race`).

## Deuda pendiente

Vacía a propósito: lo listado arriba era la deuda y ya está hecho.
Lo único que falta es validación viva en un servidor real (instalador por
slots, `rkt upgrade` e2e, ACLs de operadores, SSE en navegador real), que no se
puede hacer desde esta máquina sin admin: el vps en limpio es el terreno.
E2E de navegador (`-tags e2e`, Playwright + Chromium) cubre smoke de
navegación, gating por rol y el wizard de alta completo (validación,
revisión, creación por origen y 403 de viewer).
