# Arquitectura

Reglas de diseño que el código debe cumplir. Si una capa necesita lógica de
otra, se mueve al hogar correcto; no se duplica.

## Capas y dirección de dependencias

```text
cmd/rkt-proxy
  ├─ internal/cli          comandos, servicio Windows y salida de operador
  └─ internal/app          composición del runtime (Program)
       ├─ internal/api      transporte HTTP /api/v1
       ├─ internal/health   transporte HTTP loopback /healthz
       ├─ internal/mcpserver transporte MCP stdio
       ├─ internal/cfgfile  mutaciones de flota + carga/guardado
       │    ├─ internal/store   SQLite
       │    └─ internal/config  modelo, validación, Live
       └─ dominio: proxy, supervisor, deploy, discover, github,
          buildpack, acme, devca, weblog, accesslog, alerts, ...
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
| Resolución DNS de dominios | `internal/dnscheck` |
| Pipeline deploy/build | `internal/deploy` |
| Procesos y reinicios | `internal/supervisor` |
| Historial y eventos | `internal/store` |

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

Estado: Windows es la plataforma soportada. Linux compila, pasa la suite
completa en Ubuntu/WSL y CI ejecuta build+vet+tests más un job con `-race`.
Limitaciones conscientes en Linux: sin tope de RAM (Job Objects) y CA de
desarrollo manual.

## Protocolos

El proxy acepta targets `http`, `https`, `ws` y `wss`; `ws`/`wss` se
normalizan a `http`/`https` para el transporte y conservan el upgrade. Soporta
HTTP/1.1, HTTP/2 cuando el listener TLS lo permite, WebSocket/WSS y SSE con
flush inmediato. TCP crudo queda fuera de alcance.

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

## Estado del refactor

Hecho:

- `internal/cli` separado de `internal/app` (comandos vs runtime).
- `internal/api` dividido por recurso: `api.go` queda en ~190 líneas
  (Deps, NewHandler, helpers); handlers en `apps.go`, `routes.go`,
  `deployments.go`, `analytics.go`, `auth.go`, `env.go`, `settings.go`,
  `github.go`, `alerts.go`, `status.go`, `domains.go`.
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

## Deuda pendiente (orden sugerido)

1. **Confirmar `-race` en CI.** El job está añadido, pero no se pudo ejecutar
   localmente (sin CGO/gcc); la primera pasada del runner es la validación.
2. **E2E del dashboard.** Hoy hay `tsc`, build y capturas; falta un runner de
   navegador para regresiones de interacción.
