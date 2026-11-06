# AADR-002 — Auditoría y endurecimiento de rkt

| Campo | Valor |
|---|---|
| ID | AADR-002 |
| Estado | en curso |
| Fecha | 2026-11-05 |
| Base auditada | `4c0feb8` |
| Ámbito | seguridad, proxy, auth, deploy, operación, producto |
| Fase | implementación |
| Siguiente tarea | P2 restante: path routing, HTTP/3, email, upgrade, HA |
| Relacionados | [ARCHITECTURE](ARCHITECTURE.md), [CLI](CLI.md), [PRODUCT](../PRODUCT.md), [SPEC](SPEC.md) |

Este documento reemplaza al AADR-001 (checklist histórico ya cerrado). El
histórico completo queda en git: `git log -- docs/AADR.md`.

**AADR** = Architecture & API Decision Record. La checklist de la sección 8 es
la fuente de verdad del avance. Se actualiza en el mismo commit que cierra cada
tarea.

## 0. Contexto y método

Auditoría estática del PaaS sobre el commit `4c0feb8`, con la suite completa en
verde:

```text
go test ./... -count=1  →  38 paquetes ok
```

Se revisaron: proxy, servidores HTTP/TLS, auth, store SQLite, API, deploy,
supervisor, cron, alertas, CLI, instalador y CI.

No es un pentest. Los hallazgos salen de leer el código y se citan con
`archivo:línea`. Cuando falte explotación real, se dice: el impacto es el que el
código permite, no una suposición.

Criterio de prioridad:

- **P0** exposición de secretos, bypass de auth, pérdida de datos o caída
  evitable. Se corrige antes de exponer el servicio a internet.
- **P1** robustez y operación: lo que hoy funciona pero fallará en uso real
  (meses de uptime, picos, backups, límites).
- **P2** features de producto que faltan para competir con un PaaS moderno.

## 1. Veredicto

Sólido:

- Un solo camino de escritura de flota (`cfgfile`), validación antes de
  persistir y config viva inmutable con `Swap`.
- Deploy con clone/build/swap/rollback y auto-rollback tras crash.
- Sesiones bearer hasheadas, bcrypt, roles, fail-closed sin usuarios.
- Dashboard y API solo en loopback; webhook GitHub con HMAC; Job Objects para
  matar el árbol de procesos en Windows.
- Suite Go verde y CI con `-race` y cobertura mínima en paquetes blindados.

Riesgo principal: el borde público y la operación. Hay secretos filtrados por
API/export/argv, endpoints de setup sin auth, timeouts que rompen streaming,
control local sin credencial y ausencia de backup, poda y límites reales.

Regla de este AADR: ningún P0 se da por cerrado sin test de regresión que
falle antes del fix.

## 2. Convenciones

Estados de tarea:

- `[ ]` pendiente.
- `[ ] **EN CURSO**` trabajando ahora; añadir fecha y dueño.
- `[x]` hecha y verificada; añadir commit.
- `[-]` descartada; añadir motivo.

Definición de hecho:

- test de la lógica pura o del handler;
- `gofmt`, `go vet`, `go test ./...` y build;
- `tsc --noEmit`, `pnpm build` y guard i18n si toca web;
- la checklist y el registro de avance quedan actualizados en el mismo commit;
- si hay migración SQLite, es aditiva y probada sobre una DB existente.

## 3. Hallazgos P0

### SEC-01 — Fuga de secretos en `/api/v1/deployments`

**Evidencia.**

- `store.Deployment.EnvSnapshot` se serializa con `json:"env_snapshot"`
  (`internal/store/store.go:288`).
- Se llena con el env efectivo completo, incluidos globales
  (`internal/store/store.go:293-302`, `internal/app/service.go:699`).
- El handler no filtra por rol: cualquier sesión, incluido `viewer`, recibe el
  historial entero (`internal/api/deployments.go:13-25`).

**Impacto.** Tokens, passwords y variables globales quedan legibles por el rol
de solo lectura y viajan en cada listado. Contradice la promesa "secretos fuera
de listas".

**Decisión.** No persistir secretos por deploy. La trazabilidad se guarda como
nombres de claves o hash, nunca valores. Limpiar filas existentes en migración.

**Verificación.** Test que serialice `ListDeployments` y falle si el JSON
contiene valores de env; migración probada con filas antiguas.

### SEC-02 — `config export` escribe secretos en claro

**Evidencia.**

- `ExportJSON` incluye `apps[].env` y `global_env`
  (`internal/config/config.go:311-321`).
- `cmdExport` escribe 0644 y sugiere "respaldo en git"
  (`internal/cli/manage.go:701-724`).

**Impacto.** Un respaldo en git sube secretos al repositorio y a su historial.
El modo 0644 no protege el archivo fuera del servidor.

**Decisión.** Export redactado por defecto (solo claves). `--include-secrets`
explícito, con 0600 y aviso. Añadir export cifrado (DPAPI/age) para backup real.

**Verificación.** Test: export por defecto no contiene valores; con flag los
contiene y el archivo queda 0600.

### SEC-03 — Token GitHub en argv y en env de máquina

**Evidencia.**

- `gitArgs` pasa `http.extraHeader=AUTHORIZATION: basic <b64>` como argumento
  (`internal/deploy/deploy.go:266-288`).
- `-GithubToken` lo guarda como variable de máquina
  (`scripts/install.ps1:45-49`).

**Impacto.** El token es legible desde la lista de procesos o desde el registro
por cuentas locales. Base64 no es cifrado.

**Decisión.** Pasar credenciales por env del proceso hijo
(`GIT_CONFIG_COUNT`, `GIT_CONFIG_KEY_n`, `GIT_CONFIG_VALUE_n`) o credential
helper por stdin. PAT en DPAPI/Credential Manager o solo GitHub App. No usar
env de máquina para secretos.

**Verificación.** Test que inspeccione los argumentos de `git` y falle si
contienen credenciales.

### SEC-04 — `/_rkt/setup` y `/_rkt/new-app` públicos sin auth ni `state`

**Evidencia.**

- Se montan en :80/:443 (`internal/app/service.go:250-281`,
  `internal/server/server.go:26-33`).
- `SetupHandler` canjea `code`/`installation_id` sin nonce
  (`internal/hook/hook.go:149-174`).

**Impacto.** Binding de una GitHub App ajena o CSRF de integración: un tercero
puede iniciar el flujo y hacer que la instancia guarde credenciales que no
controla el operador.

**Decisión.** `state` firmado one-time generado desde el dashboard con sesión
admin, TTL corto. Rechazar cualquier setup sin state válido. Rate limit por IP.

**Verificación.** Test: setup sin state → 403; state caducado → 403; state
válido → una sola vez.

### SEC-05 — Control loopback sin auth

**Evidencia.**

- `/healthz`, `/deploy`, `/restart`, `/build`, `/rollback` no piden credencial
  (`internal/health/health.go:54-57`).
- El listener es loopback (`internal/app/service.go:313-314`).

**Impacto.** Cualquier cuenta local alcanza `127.0.0.1:8090` y ejecuta
builds/deploys/restarts como SYSTEM.

**Decisión.** Token de control o comprobación de SID administrador. Separar el
listener admin del healthz público local. Los endpoints de acción dejan de ser
anónimos.

**Verificación.** Test: sin token → 401; con token de viewer → 403; admin → 200.

### SEC-06 — Spoof de `X-Forwarded-*`

**Evidencia.**

- El proxy conserva y añade `X-Forwarded-Proto` del cliente
  (`internal/proxy/proxy.go:50-59`).
- `httputil.ReverseProxy` concatena el `X-Forwarded-For` entrante.
- `X-Real-IP` se sobrescribe, pero el resto de cabeceras se confían.

**Impacto.** Apps que leen el primer valor ven IP o protocolo falsos: logs
mentirosos, cookies "secure" mal decididas, allowlists de app bypassables.

**Decisión.** Borrar `X-Forwarded-For`, `X-Forwarded-Proto`, `X-Forwarded-Host`
y `X-Real-IP` entrantes antes de escribir los del edge. Si se soporta un upstream
de confianza, declararlo explícitamente en config.

**Verificación.** Test de proxy: request con cabeceras falsas llega al backend
solo con las del edge.

### SEC-07 — ACME emite por cualquier subdominio de `*.dominio`

**Evidencia.** `HostPolicy` acepta todo subdominio bajo un wildcard de ruta
(`internal/acme/acme.go:25-43`).

**Impacto.** Un atacante pide miles de hostnames random y consume el rate limit
de Let's Encrypt, bloqueando la emisión de subdominios legítimos.

**Decisión.** Límite de emisión por host/ventana, allowlist opcional y precheck
DNS. DNS-01 queda como opción para wildcard real (ver FEAT-01).

**Verificación.** Test de política: host fuera de allowlist no dispara emisión;
contador de emisiones por ventana respetado.

### REL-01 — Timeouts del listener público rompen subidas y streaming

**Evidencia.**

- `ReadTimeout: 15s` cubre el body completo del request.
- `WriteTimeout: 60s` corta SSE y vídeo
  (`internal/server/server.go:48-55,76-82`).
- El proxy soporta SSE/WS con flush inmediato
  (`internal/proxy/proxy.go:62-64`).

**Impacto.** Uploads de varios MB fallan en conexiones lentas. Un SSE o vídeo de
más de 60 s muere aunque el backend esté sano.

**Decisión.** `ReadHeaderTimeout` corto, sin `ReadTimeout`/`WriteTimeout`
globales en el proxy y deadlines por ruta/tipo. Mantener `IdleTimeout`.

**Verificación.** Test de upload lento y test SSE que supere 60 s.

### BUG-01 — `rkt settings set` no valida

**Evidencia.**

- Escribe directo `db.SetSetting` sin `settings.Validate`
  (`internal/cli/manage.go:670-681`).
- `store.SetSetting` no valida (`internal/store/fleet.go:58-67`).
- La API sí valida (`internal/api/settings.go:32-45`).

**Impacto.** Claves desconocidas o valores inválidos llegan a SQLite. El
siguiente arranque puede fallar y el export los ignora porque solo recorre
claves conocidas.

**Decisión.** Un solo camino: clave conocida + `settings.Validate`, igual que la
API. Error accionable si la clave no existe.

**Verificación.** Test CLI: `port_pool=basura` → error; clave inventada → error;
valor válido → persiste.

### BUG-02 — Deploy ignora `health_path`

**Evidencia.**

- `WaitAppHTTP` hardcodea `/` (`internal/deploy/deploy.go:153-158`).
- El supervisor sondea `HealthPath` (`internal/app/service.go:434-458`).

**Impacto.** Apps con health en otra ruta hacen rollback falso tras un deploy
correcto; o al revés, pasan el check de `/` sin estar listas.

**Decisión.** `WaitAppHTTP` usa `app.HealthPath`. Un solo criterio de salud para
supervisor, deploy y dashboard.

**Verificación.** Test con health en `/healthz` y `/` devolviendo 500: el deploy
debe aceptar la app.

## 4. Hallazgos P1 — fortificar lo existente

### SEC-08 — Auth y auditoría

- Sin MFA/TOTP ni tokens de API con alcance para CI. Solo sesión de usuario
  (`internal/store/auth.go`).
- Sesión fija de 30 días, sin idle timeout ni rotación (`store/auth.go:26`).
- No hay purga global de sesiones expiradas.
- Cookie sin `Secure` (`internal/api/auth.go:195-199,237-241`); aceptable en
  loopback HTTP, pero bloquea exponer el panel.
- Auditoría solo de acciones de usuario; deploy, rutas, env, apps y settings no
  se auditan (`internal/api/auth.go:230-424`).
- El token vuelve en el body del login además de la cookie httpOnly.

Decisión: tokens de API con rol y TTL, MFA opcional para admin, auditoría de
toda mutación, purga programada de expiradas y `Secure` condicionado a TLS.

### SEC-09 — WAF y rate limit

- Rate limit global por IP: un cliente pesado en una app agota el bucket de las
  demás (`internal/ratelimit/ratelimit.go:26-99`).
- Sin `Retry-After`, sin allowlist de IPs.
- WAF son globs sobre el path crudo (`internal/waf/waf.go:15-25`).
- `/x/.env`, `//`, `..` y paths anidados esquivan los defaults
  (`internal/config/config.go:222-231`).

Decisión: límites por host/app, allowlist, `Retry-After`; normalizar path antes
del match y añadir reglas por método/IP.

### OPS-01 — Cabeceras de seguridad y compresión

No hay HSTS, `X-Content-Type-Options`, `Referrer-Policy`, CSP del SPA ni
compresión gzip/brotli. Decisión: middleware de cabeceras con defaults seguros
y compresión opt-in por respuesta.

### OPS-02 — Shutdown ordenado y deploy sin caída

- Kill duro sin shutdown ordenado
  (`internal/supervisor/supervisor.go:604-617`,
  `internal/supervisor/job_windows.go:53-57`).
- Deploy es stop→swap→start (`internal/deploy/deploy.go:106-116`); requests en
  vuelo mueren.

Decisión: intento de cierre ordenado con timeout (CTRL_BREAK/CTRL_C en Windows)
y swap con readiness gate y solape. Sin prometer cero downtime hasta tener test
que lo demuestre.

### OPS-03 — Límites y cuotas reales

- RAM por proceso, no agregada por Job Object.
- Sin CPU, sin cuota de disco efectiva.
- `quota_reqs` está muerto; `quota_bytes` solo pinta alertas
  (`internal/app/background.go:145`).

Decisión: aplicar o borrar. Nada de settings decorativos. Si se quedan, se
aplican en supervisor/proxy con límites medibles.

### OPS-04 — Backup y restore de SQLite

No hay backup. `config export` no respalda usuarios, historial ni sesiones.
Decisión: `VACUUM INTO` programado, `rkt backup/restore`, verificación de
integridad y retención. Backup probado restaurando en una DB limpia.

### OPS-05 — Retención y rotación de logs

- Rotación solo al abrir el archivo, 10 MB y una sola `.1`
  (`internal/logx/logx.go:109-112`, `internal/accesslog/accesslog.go:27-29`,
  `internal/supervisor/supervisor.go:585-590`).
- `PruneDeployments`/`PruneEvents` existen pero no se programan
  (`internal/store/prune.go`); solo MCP manual
  (`internal/mcpserver/tools_more.go:942`).

Decisión: rotación en caliente por tamaño, retención por tipo
(app/access/deploy/cron) y poda programada con configuración explícita.

### OPS-06 — Observabilidad y alertas

- Sin `/metrics` Prometheus, request IDs ni trazas; solo `/healthz` JSON
  loopback.
- Canales de alerta solo webhook genérico
  (`internal/app/background.go:164-203`).
- Sin alertas de expiración TLS, deploy fallido ni disco.

Decisión: `/metrics` loopback, histogramas de latencia, request ID, alertas de
certificado/deploy y canales Slack/email además del webhook.

### PLAT-01 — Build sin sandbox

Los builds de repos corren como SYSTEM, sin CPU/RAM/red limitada y con secretos
en el env (`internal/deploy/deploy.go:317-341`). Decisión: usuario de build
dedicado, entorno mínimo, límites y timeout por paso.

### PLAT-02 — Coste por request en el proxy

Hay un dial TCP extra por request y se crea un `ReverseProxy` por request
(`internal/proxy/proxy.go:82-97`). Decisión: sacar el dial del camino caliente,
pool de proxies y circuit breaker con singleflight.

### PLAT-03 — Paridad CLI

Faltan `cron`, `alerts`, `domains`, `audit`, `backup`, `doctor` y `tls`. Decisión:
misma API y mismo contrato para CLI, dashboard y MCP. Un endpoint = una feature.

### CI-01 — Cadena de suministro y pruebas

- Exe sin firma; checksum del mismo canal.
- Actions sin pinear SHA.
- Sin `govulncheck` ni `staticcheck`.
- Web con 2 tests unitarios y sin E2E.

Decisión: firmar el binario, SBOM, actions por SHA, `govulncheck`,
`staticcheck` y runner E2E del dashboard.

## 5. Features P2 — producto

| ID | Feature | Motivo |
|---|---|---|
| FEAT-01 | Certificados propios y DNS-01 | Wildcard real y dominios con cert externo |
| FEAT-02 | Rutas por path, rewrites, redirects y cabeceras | Hoy solo Host; no hay reglas por path |
| FEAT-03 | Basic auth/IP allowlist por app y modo mantenimiento | Proteger previews y apps internas |
| FEAT-04 | Hooks de release, cola de deploys y rollback por SHA | Migraciones y operación predecible |
| FEAT-05 | Volúmenes/directorio persistente por app | Hoy el swap borra uploads y datos locales |
| FEAT-06 | HTTP/3 | Latencia en redes móviles |
| FEAT-07 | Canales Slack/email nativos | Alertas accionables sin webhook propio |
| FEAT-08 | `rkt upgrade`, `doctor`, SBOM y firma | Operación y confianza en el instalador |
| FEAT-09 | HA/multi-nodo | Estratégico; no encaja con el producto actual de un binario |

## 6. Fuera de alcance por diseño

No son deuda: multi-tenant, billing, preview environments, CDN, Kubernetes,
TCP crudo, agregación multi-servidor y métricas de visitantes únicos o bounce
rate. Si no se mide, no se muestra.

## 7. Plan por sprints

### Sprint 1 — Secretos y validación (S)

- SEC-01, SEC-02, SEC-03.
- BUG-01, BUG-02.
- Test de regresión por hallazgo.

Salida: ningún endpoint/export/argv filtra secretos; settings no puede
corromper la DB; deploy respeta `health_path`.

### Sprint 2 — Borde público (M)

- SEC-04, SEC-05, SEC-06, SEC-07.
- REL-01.
- OPS-01.

Salida: setup con state, control local con auth, cabeceras del edge
sobrescritas, SSE/uploads sin corte artificial, HSTS/nosniff/CSP.

### Sprint 3 — Operación (M)

- OPS-04, OPS-05.
- OPS-06 (métricas y alertas base).
- SEC-08 (auditoría de mutaciones y purga de sesiones).

Salida: backup/restore probado, poda y rotación activas, `/metrics` y auditoría
completa.

### Sprint 4 — Robustez (L)

- SEC-08 (tokens de API, MFA, cookie Secure).
- SEC-09.
- OPS-02, OPS-03.
- PLAT-01, PLAT-02.

Salida: límites reales aplicados, shutdown ordenado, deploy con readiness y
proxy sin dial por request.

### Sprint 5 — Producto (L)

- FEAT-01 a FEAT-05.
- FEAT-07, FEAT-08.
- CI-01.

Salida: certificados propios, rutas avanzadas, protección por app, hooks de
release, volúmenes, alertas nativas y cadena de suministro firmada.

## 8. Checklist verificable

### P0

- [x] SEC-01 · `env_snapshot` fuera del JSON y de la DB; migración v15
  (`e62f682`).
- [ ] **PARCIAL** SEC-02 · export redactado + `--include-secrets` 0600 hecho;
  falta cifrado del export (`e62f682`).
- [x] SEC-03 · token fuera de argv (`GIT_CONFIG_*`) y del env de máquina;
  instalador usa archivo con ACL (`e62f682`).
- [ ] **PARCIAL** SEC-04 · state one-time con hash en DB y consumo único;
  falta rate limit del endpoint de setup (`e62f682`).
- [x] SEC-05 · token de control en acciones loopback (`e62f682`).
- [x] SEC-06 · `X-Forwarded-*`/`X-Real-IP` entrantes borrados (`e62f682`).
- [x] SEC-07 · límite de emisión ACME + precheck DNS opcional (`e62f682`).
- [x] REL-01 · `ReadHeaderTimeout`, sin WriteTimeout global (`e62f682`).
- [x] BUG-01 · `settings set` valida clave y valor (`e62f682`).
- [x] BUG-02 · deploy usa `health_path` (`e62f682`).

### P1

- [x] SEC-08 · tokens de API, idle timeout, purga, auditoría total y cookie
  Secure. **MFA excluido por decisión del operador** (`c64426a`).
- [x] SEC-09 · rate limit por host/app, allowlist, `Retry-After`, WAF
  normalizado (`c64426a`).
- [x] OPS-01 · cabeceras de seguridad, CSP y gzip (`c64426a`).
- [ ] **PARCIAL** OPS-02 · shutdown ordenado con CTRL_BREAK/SIGTERM y timeout;
  deploy sigue stop→swap→start sin solape azul/verde (`c64426a`).
- [ ] **PARCIAL** OPS-03 · RAM agregada del Job Object; CPU y cuotas de
  disco/requests siguen sin enforcement (`c64426a`).
- [x] OPS-04 · `rkt backup/restore` con `VACUUM INTO` y verificación
  (`c64426a`).
- [x] OPS-05 · rotación viva y poda programada de historial/logs
  (`c64426a`).
- [ ] **PARCIAL** OPS-06 · `/metrics`, request ID, alertas cert/deploy y
  Slack/Discord; falta email (`c64426a`).
- [ ] **PARCIAL** PLAT-01 · env saneado y timeout de build/release; falta
  usuario de build dedicado y límites CPU/red (`c64426a`).
- [ ] **PARCIAL** PLAT-02 · sin dial por request y pool; falta circuit breaker
  (`c64426a`).
- [ ] **PARCIAL** PLAT-03 · `audit`, `alerts`, `cron` (list), `domains`,
  `backup`, `doctor`; faltan `tls` y `cron run` (`c64426a`).
- [ ] CI-01 · firma, SBOM, actions por SHA, `govulncheck`, `staticcheck`, E2E.

### P2

- [ ] **PARCIAL** FEAT-01 · certificados propios por host/wildcard; falta
  DNS-01 (`c64426a`).
- [ ] FEAT-02 · rutas por path, rewrites, redirects y cabeceras.
- [x] FEAT-03 · basic auth/IP allowlist por app y mantenimiento (`c64426a`).
- [ ] **PARCIAL** FEAT-04 · hooks `release`; faltan cola de deploys y rollback
  por SHA (`c64426a`).
- [x] FEAT-05 · `persist` sobrevive al swap de deploy (`c64426a`).
- [ ] FEAT-06 · HTTP/3.
- [ ] **PARCIAL** FEAT-07 · Slack/Discord; falta email SMTP (`c64426a`).
- [ ] **PARCIAL** FEAT-08 · `doctor`; faltan `upgrade`, SBOM y firma
  (`c64426a`).
- [ ] FEAT-09 · HA/multi-nodo (decisión estratégica).

## 9. Registro de avance

| Fecha | ID | Commit | Nota |
|---|---|---|---|
| 2026-11-05 | AADR-002 | — | Auditoría inicial sobre `4c0feb8`. Sin tareas cerradas. |
| 2026-11-05 | P0 | `e62f682` | Cerrados SEC-01..07, REL-01, BUG-01, BUG-02. |
| 2026-11-05 | P1/P2 | `c64426a` | Auth, edge, operación, deploy, CLI y features P2 viables. |

## 10. Estado de implementación (2026-11-05)

### Cerrado y verde

- **P0 completo:** SEC-01 a SEC-07, REL-01, BUG-01, BUG-02.
- **SEC-09:** rate limit por host + allowlist, `Retry-After`, WAF con
  normalización y sufijos anidados.
- **OPS-01:** HSTS condicionado a TLS, `nosniff`, `Referrer-Policy`, CSP del
  dashboard y gzip opt-in.
- **OPS-04:** `VACUUM INTO` programado, `rkt backup`, `rkt restore` con
  verificación de integridad y retención.
- **OPS-05:** rotación viva (log operativo, access log, logs de app), poda
  programada de deployments/events/logs y retención configurable.
- **PLAT-02:** sin dial TCP por request, pool de `ReverseProxy` y reintento
  de dial durante `restart_grace`.
- **FEAT-03:** basic auth por app, allowlist CIDR y modo mantenimiento.
- **FEAT-05:** `persist` copia data/uploads/sqlite al staging antes del swap.
- **CLI:** `backup`, `restore`, `doctor`, `audit`, `alerts`, `cron` (list),
  `domains`; `settings set` valida clave y valor.

### Parcial

- **SEC-08:** tokens de API, idle timeout de 7 días, purga de sesiones,
  auditoría automática de toda mutación y cookie `Secure` si hay TLS.
  **MFA/TOTP excluido por decisión del operador.**
- **OPS-02:** cierre ordenado con CTRL_BREAK/SIGTERM y timeout; el deploy
  sigue siendo stop→swap→start, sin solape azul/verde.
- **OPS-03:** tope de RAM agregado del Job Object; cuotas de disco/requests
  siguen sin aplicarse (solo alertas/uso).
- **OPS-06:** `/metrics`, request ID, alertas `cert_days`/`deploy_failed`,
  Slack/Discord; email pendiente.
- **PLAT-01:** entorno saneado (no hereda secretos de máquina) y timeout de
  build/release; falta usuario de build dedicado y límites CPU/red.
- **PLAT-03:** falta `rkt tls` y ejecución de cron por CLI.
- **FEAT-01:** certificados propios por host/wildcard; DNS-01 pendiente.
- **FEAT-04:** hooks `release`; cola de deploys y rollback por SHA pendientes.
- **FEAT-07:** Slack/Discord; email SMTP pendiente.
- **FEAT-08:** `doctor`; `upgrade`, SBOM y firma pendientes.

### Pendiente (requiere otra iteración)

- FEAT-02 rutas por path, rewrites, redirects y cabeceras por regla.
- FEAT-06 HTTP/3.
- FEAT-07 email SMTP con gestión de secreto.
- FEAT-08 `rkt upgrade`, SBOM y firma del binario.
- FEAT-09 HA/multi-nodo: incompatible con el diseño actual de binario único;
  requiere AADR propio.
- CI-01: `govulncheck`, `staticcheck`, E2E del dashboard, firma y actions
  pineadas por SHA.

Ninguna tarea pendiente se considera cerrada. La checklist de la sección 8
queda como referencia; este estado manda hasta que se marquen una a una.
