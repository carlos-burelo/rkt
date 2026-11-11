# GitHub App: auto-deploy al hacer push

La GitHub App le da a rkt tres cosas: clonar repos privados (token
por instalación), desplegar solo al hacer push a la rama (webhook)
y registrar repos nuevos solos (sondeo + discovery).

## Puesta en marcha (dos caminos)

**Dashboard** (recomendado): Ajustes → GitHub → *Conectar*. El backend
genera `GET /_rkt/new-app?...&state=<one-time>` (`setup_url`).

**CLI**: `rkt github init --name rkt-deployer --domain TU-DOMINIO [--org MI-ORG]`.
El CLI acuña el `state` en la DB local (30 min, un solo uso) y abre el
form. Sin `state` el form responde 400 y el callback 403.

El form hace auto-submit del manifest a `github.com/.../apps/new` y
GitHub redirige a `/_rkt/setup?code=...&state=...`, que:

1. Consume el state (403 si falta o caducó → reinicia el flujo).
2. Convierte el `code` en credenciales (`POST /app-manifests/{code}/conversions`),
   guarda el PEM y `github_{app_id,key_file,webhook_secret,app_slug}`.
3. Redirige a `https://github.com/apps/{slug}/installations/new`.

Ahí **instalas la App eligiendo repos**: *All repositories* o una
selección. GitHub vuelve a
`/_rkt/setup?setup_action=install&installation_id=...`: rkt verifica la
instalación, guarda `install_url`, `repository_selection`
(`all`/`selected`), `repo_count` y dispara discovery + auto-deploy.

Para otra organización, repite con `?org=MI-ORG` en el setup
(`GET /api/v1/github?org=...` lo añade) o `rkt github init --org ...`.
`rkt github login` abre solo la pantalla de instalación (para una App
ya creada a mano; necesita `github.app_slug` en el YAML).

## Permisos que pide (y por qué)

El template vive en `internal/hook/hook.go` (`manifestJSON`):

| Permiso / evento | Valor | Para qué |
|---|---|---|
| `contents` | read | clonar el repo (incluye privados) |
| `metadata` | read | obligatorio de GitHub |
| `push` | evento | desplegar al hacer push a la rama (solo con dominio público) |
| webhook | `https://TU-DOMINIO/_rkt/hook/github` | recibir el push (solo con dominio público) |
| `public: true` | — | una App privada solo se instala en la cuenta dueña; pública cubre orgs donde eres admin |

En local (`localhost`/`127.0.0.1` sin TLS) se omiten webhook y
eventos: GitHub no puede llamarte; todo funciona por sondeo
(mínimo 10s).

## Alcance: todo vs seleccionado

GitHub pregunta *All repositories* o una selección. rkt pide todo a
propósito: si deniegas un repo u org, **no hay huérfanos**:

- el repo no aparece en `GET /api/v1/github/repos` ni en discovery,
- `deploy`/`retry` fallan con `instalación para o/r: ... (revisa que
  la App esté instalada con acceso a todos los repos)`,
- el push no llega (sin webhook para ese repo).

La tarjeta de GitHub muestra `all`/`selected`, instalaciones y el
`install_url` para ampliar el acceso. Si revocas todo, rkt lo detecta
(`ErrAppGone`) y limpia settings + PEM; las apps creadas **no** se
borran solas. En el setup del sistema, la casilla de omisión exige
confirmar que entiendes estas consecuencias.

## Operativa

- **Poll**: cada `poll_interval` (defecto 10s, mín 10s) comprueba HEAD
  vs live y despliega; `discover` cada 5 min registra repos con `auto`.
- **Webhook**: `POST /_rkt/hook/github` (firma `X-Hub-Signature-256`
  con `webhook_secret`); push en la rama con `auto` → deploy en fondo.
- **Eventos `installation*`**: solo disparan discovery; no borran nada.
- **Setup states**: tabla `setup_states`, un solo uso, TTL 30 min;
  caducados se purgan.

## Problemas típicos

| Síntoma | Causa | Arreglo |
|---|---|---|
| `403 Setup no autorizado` en `/_rkt/setup` | state caducado/consumido | reinicia el flujo (Ajustes o CLI) |
| `Faltan datos` en `/_rkt/new-app` | falta `?name=&domain=&state=` | actualiza rkt; usa el link del dashboard o `github init` |
| `instalación para o/r ...` al desplegar | repo/org sin acceso | abre el `install_url` de la tarjeta y amplía |
| No despliega al hacer push en local | esperado sin webhook público | sondeo cada 10s; revisa `deployments` |
| Webhook 4xx en GitHub | `webhook_secret` cambiado | repite el setup (sobrescribe settings + PEM) |
