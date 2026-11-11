# Checklist beta

Lo automatizado vive en `tests/` (Go por paquete vía `tests/run.go`,
navegador real con `go test -tags=e2e`, Vitest, shots sin errores de
consola). Esto es lo que NO se puede automatizar en CI y debe pasarse
a mano antes de cada beta, en una máquina Windows limpia.

## Instalación y servicio

- [ ] `rkt install` registra el servicio `Rkt` (auto-arranque) y muestra el dashboard.
- [ ] Reinicio del equipo: el servicio arranca solo y las apps vuelven a servir sin tocar nada.
- [ ] `rkt stop` detiene servicio + apps (ningún `node`/`python` huérfano en el administrador de tareas).
- [ ] `rkt start` lo levanta todo de nuevo.
- [ ] `rkt uninstall` limpia el servicio; `rkt run` funciona en foreground con Ctrl+C limpio.

## TLS real (staging)

- [ ] Con `acme_staging` activo y un dominio apuntando a la máquina, el
  certificado staging se emite solo al primer request (ver `server/doctor`).
- [ ] `storage/dist` separa `staging/` de `prod/`: un cert staging jamás se sirve como prod.
- [ ] Renovar (borrar cache + request) emite de nuevo sin rate-limit.

## Actualización entre betas

- [ ] Parar servicio, sustituir el binario, arrancar: `PRAGMA user_version`
  sube solo y la flota (apps, rutas, usuarios, deployments) sigue intacta.
- [ ] `rkt backup` antes y `rkt restore` después recuperan todo en una
  máquina distinta (misma versión).

## Flujo operador

- [ ] Crear app Next desde el dashboard → deploy → log vivo → corte sin caída → rollback.
- [ ] `rkt deploy` y `rkt rollback` desde CLI contra el servicio en marcha.
- [ ] Webhook de GitHub (`/hook`) dispara deploy con push a main.
- [ ] Cron real ejecuta y deja historial; alertas llegan al canal configurado.

## Estrategias de enrutado (nuevo en esta beta)

Tres formas de exponer una app, configurables desde Dominios (visual:
tarjeta por modo con ejemplo vivo):

- [ ] **Host**: `app.midominio.com` → app (comportamiento clásico, sin cambios).
- [ ] **Path**: `http://IP/app` → app (recorta `/app`; prefijo más largo gana;
  `/app2` no caza `/app`).
- [ ] **Puerto**: `http://IP:3000` → app (listener propio por puerto;
  cambiar puertos exige reinicio — el dashboard lo avisa).
- [ ] Puertos reservados (80/443/dashboard/healthz) se rechazan al guardar.
- [ ] Host desconocido → 404; path sin match → cae a la ruta del host.

## Setup por pasos + doctor integrado

- [ ] Primera visita abre el stepper: admin → red (dominio/IP, puertos) →
  toolchains (bins) → GitHub App (omitible) → resumen.
- [ ] Pasos no obligatorios se omiten y quedan pendientes visibles.
- [ ] El doctor corre dentro del setup: lo que bloquee el arranque ofrece
  reparación (fijar bin, abrir puerto) en el mismo paso.
- [ ] `/_rkt/new-app` y `/_rkt/setup` documentados (qué permiso pide cada
  scope y por qué); instalar pide todo (orgs + repos) y el usuario
  desmarca conscientemente lo que no quiera (nada queda huérfano en
  silencio: el pre-check avisa antes de crear/desplegar).

## MCP en prod

- [ ] Token con alcance (lectura / deploy / admin) en vez de todo-o-nada.
- [ ] Allowlist de tools por token; `config_import` y `users_*` fuera del
  perfil de solo-lectura.
- [ ] Límite de ritmo en `/mcp` y auditoría por herramienta.
