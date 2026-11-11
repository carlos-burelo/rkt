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
