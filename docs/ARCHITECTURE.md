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
| Pipeline deploy/build | `internal/deploy` |
| Procesos y reinicios | `internal/supervisor` |
| Historial y eventos | `internal/store` |

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
- `internal/api` dividido por recurso: `apps.go`, `routes.go`,
  `deployments.go`, `analytics.go`; el resto sigue en `api.go`.
- Mutaciones de flota canónicas en `cfgfile/mutate.go`.
- Carrera del supervisor corregida (`m.apps` bajo lock).

## Deuda pendiente (orden sugerido)

1. **Terminar de dividir `api.go`.** Extraer `auth.go` (middleware, login,
   usuarios) y `env.go`/`settings.go`/`alerts.go`. El resto del paquete ya
   sigue el patrón por recurso.
2. **Unificar parseo de puerto/listen.** Hoy hay copias en `api.listenPort` y
   `app.setupOrigin`; extraer una función de red compartida.
3. **Web.** `CreateDialog.tsx` concentra estado de 3 pasos + tracking: extraer
   pasos a componentes con props explícitas y helpers de slug a un módulo.
4. **Cobertura de concurrencia.** Añadir pruebas con `-race` en CI (requiere
   CGO) para supervisor, API y watcher.
