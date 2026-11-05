# AADR — Expansión por módulos de rkt

| Campo | Valor |
|---|---|
| ID | AADR-001 |
| Estado | aceptado |
| Fecha | 2026-11-05 |
| Ámbito | backend, API, dashboard, MCP |
| Fase activa | DOM — Dominios y Proxy |
| Siguiente tarea | DOM-01 |
| Relacionados | [SPEC](SPEC.md), [ARCHITECTURE](ARCHITECTURE.md), [CLI](CLI.md) |

**AADR** = Architecture & API Decision Record. Este documento fija las
decisiones de arquitectura y el contrato de API de la expansión por módulos.
La checklist es la fuente de verdad del avance: se actualiza en el mismo
commit que cierra cada tarea.

## 0. Objetivo

Convertir el dashboard de rkt en el plano de control real de un PaaS
self-host: cada dato sale del backend (SQLite, `access.log`, git, ACME,
supervisor), cada concepto tiene un solo hogar, el español es neutro y sobrio,
el tema claro/oscuro es consistente y la UX resiste uso diario.

No se inventan métricas, estados, botones ni integraciones. Si una capacidad
no existe, se dice o se omite; no se simula.

## 1. Convenciones

### Estados de tarea

- `[ ]` pendiente.
- `[ ] **EN CURSO**` trabajando ahora; añadir fecha y dueño.
- `[x]` hecha y verificada; añadir commit.
- `[-]` descartada; añadir motivo. No se deja ambigua.

### Protocolo de actualización

1. Antes de tocar código, marcar la tarea como `EN CURSO`.
2. Al cerrar, ejecutar la verificación de la sección 8.
3. Marcar `[x]` con el hash del commit.
4. Añadir una línea al [registro de avance](#10-registro-de-avance).
5. Si cambia el alcance, editar esta spec en el mismo commit. No dejar la
   checklist desincronizada.

### Definición de hecho

Una tarea solo está hecha si:

- tiene test de la lógica pura o del handler cuando aplica;
- `gofmt`, `go vet`, `go test ./...` y build pasan si toca Go;
- `tsc --noEmit`, `i18n-check` y `pnpm build` pasan si toca web;
- no introduce textos crudos ni claves i18n huérfanas;
- los datos nuevos son reales y tienen origen identificable;
- la UI tiene captura actualizada si cambia una vista;
- la checklist y el registro de avance quedan actualizados.

### Roles

- `viewer`: lectura.
- `operator`: mutaciones de flota, despliegues, rutas, entorno y alertas.
- `admin`: usuarios, sesiones y auditoría.

Los secretos nunca viajan en listas. Los valores se revelan por clave y solo a
`operator`.

## 2. Contexto y línea base

### Ya construido

- [x] Diff de archivos entre revisiones de un despliegue (`01e227e`).
- [x] Resumen del historial de despliegues (`01e227e`).
- [x] Logs estructurados compactos, niveles cortos y agrupación (`f7edccb`).
- [x] i18n español neutro y guard de claves (`c88b4c7`).
- [x] Tema claro/oscuro/sistema persistente (`55d19de`).
- [x] Ficha de app como centro de control (`d4df5d0`).
- [x] Capturas y smoke visual sin errores de consola (`0ea9252`).

### Hallazgos que justifican esta spec

- `SetAppDomain` asume un dominio por app y borra los alias, pero la UI y la
  API exponen `domains[]`. El modelo no es canónico.
- `AppSettings` no permite guardar apps externas o estáticas porque exige
  `command` no vacío.
- El entorno global solo permite crear y borrar; editar un valor obliga a
  borrar y volver a crear.
- `EnvAll` hace una petición por app (N+1) y devuelve valores en la lista.
- Ajustes es una tabla clave-valor sin tipos, grupos, rangos ni estado de
  reinicio.
- Alertas solo evalúa `failed` y `error_rate`; no edita, no desactiva, no
  prueba y no entrega notificaciones.
- Usuarios no permite cambiar contraseña, listar o revocar sesiones, ni
  deshabilitar cuentas.
- CI reutiliza Despliegues; no cancela ni muestra pasos con tiempos.
- Observabilidad solo muestra dos series de barras; no atribuye tráfico por
  app ni separa clases de estado o latencia.

### Fuera de alcance

Multi-tenant, billing, compra de dominios, CDN, WAF con UI propia, RUM en las
apps usuarias, Kubernetes, métricas de visitantes únicos o bounce rate. Si no
se mide, no se muestra.

## 3. Decisiones de arquitectura

### D1. Rutas es la fuente única de hosts de una app

**Contexto.** Hoy el dominio vive en `routes` y también en un campo singular
del patch de app. Editar la app reemplaza todos los hosts de su target.

**Decisión.** `routes` sigue siendo la única fuente de host→target. La app no
guarda un campo `domain`. El patch de app acepta `domains []string` y
`cfgfile.SetAppDomains` reemplaza el conjunto de hosts del target en una sola
ruta atómica. El primer host es el principal solo para mostrar; no se persiste
un flag `primary`.

**Consecuencias.**

- Se elimina `SetAppDomain` y el campo `Domain`. Sin wrapper de compatibilidad.
- Quitar un host en Ajustes elimina su ruta; la UI lo advierte.
- Crear app acepta una lista desde el inicio.
- Las rutas manuales siguen siendo válidas; Ajustes de app ve el conjunto
  resultante.

### D2. Proxy es diagnóstico dentro de Dominios

**Contexto.** Dominios y Proxy leen el mismo endpoint y muestran la misma
relación host→target desde dos ángulos. La navegación duplica el concepto.

**Decisión.** Dominios es el módulo canónico con pestañas `Rutas` y
`Diagnóstico`. El contenido de Proxy pasa a `Diagnóstico`. `/proxy` redirige a
la pestaña. Un solo hook `useProxyStatus` alimenta ambas vistas.

**Consecuencias.**

- Un ítem menos en la navegación y un solo fetch.
- Los enlaces antiguos a `/proxy` siguen funcionando por redirección.
- La tabla de rutas sigue siendo el lugar de edición; diagnóstico es lectura.

### D3. Entorno: claves en listas, valores por clave, patch atómico

**Contexto.** Las listas devuelven valores y la escritura es por clave. Eso
dificulta editar, importar y razonar sobre secretos.

**Decisión.** Las listas devuelven solo claves y metadatos. El valor se revela
con `GET` por clave y rol `operator`. La escritura es un `PATCH` multi-clave
(`set` + `unset`) atómico. La importación de `.env` usa el parser canónico
`internal/dotenv` en el backend.

**Consecuencias.**

- No hay `PUT`/`DELETE` por clave conviviendo con `PATCH`.
- El cliente no reimplementa el parser de `.env`.
- `EnvAll` carga claves en una llamada y valores solo al revelar.
- Tras guardar se avisa "aplicado al reiniciar"; no se reinicia solo.

### D4. Ajustes tipados y validados en un paquete propio

**Contexto.** `KnownSettings` vive en `store`, que solo debería persistir. La
API acepta cualquier valor para una clave conocida.

**Decisión.** Las claves, tipos, rangos y validación viven en
`internal/settings`. `store` persiste. La UI agrupa por red, proceso,
despliegue y cuotas, y muestra qué requiere reinicio.

**Consecuencias.**

- Un valor inválido no llega a SQLite.
- `public_ip` se añade como setting opcional para el preflight DNS.
- No se muestran valores por defecto inventados; si no hay valor, se dice.

### D5. Ciclo de vida de app canónico

**Contexto.** Solo existe reiniciar. No hay forma de dejar una app detenida de
forma persistente ni de configurar su sonda de salud.

**Decisión.** `config.App` gana `disabled bool` y `health_path string`
(default `/`). `start`/`stop` son acciones canónicas del servicio y se exponen
igual en API, CLI y MCP. `stop` persiste `disabled` y detiene el proceso;
`start` lo limpia y arranca. Reiniciar no cambia `disabled`.

**Consecuencias.**

- El supervisor ignora apps deshabilitadas.
- La UI muestra "detenida" y no confunde con "caída".
- La sonda usa `health_path`; si falla, el estado es degradado, no inventado.

### D6. Observabilidad derivada, sin RUM

**Contexto.** `access.log` tiene host, path, estado, duración y referer.
`events` tiene despliegues, caídas y reinicios. No hay RUM dentro de las apps.

**Decisión.** Toda la observabilidad se deriva de `access.log`, `events` y el
supervisor. Se añaden series por bucket, clases 2xx/3xx/4xx/5xx, p50/p95/p99 y
atribución host→app. No se muestran visitantes únicos ni bounce rate.

**Consecuencias.**

- Cada cifra se puede reconstruir desde archivos locales.
- Los buckets vacíos son cero real, no dato simulado.
- Si no hay `access.log`, el estado vacío lo dice.

### D7. Alertas con canales y outbox

**Contexto.** El evaluador actual solo escribe un evento. No hay forma de
editar, desactivar, probar ni notificar.

**Decisión.** `alert_rules` gana `enabled` y `channel_id`. Se añaden
`alert_channels`, `alert_fires` y `alert_deliveries` (outbox con reintentos).
El worker de entrega corre en el tick de fondo. Webhook primero; email solo si
hay SMTP configurado, sin canal falso.

**Consecuencias.**

- La entrega no es fire-and-forget.
- Cada disparo conserva valor, umbral y ventana.
- Los canales se validan y tienen timeout.

### D8. Sesiones revocables y auditoría mínima

**Contexto.** Hay tabla `sessions`, pero no se listan ni se revocan. No hay
cambio de contraseña ni historial de acciones.

**Decisión.** Se añaden cambio/reset de contraseña, listado y revocación de
sesiones, `last_login`, deshabilitar usuario y auditoría de auth/usuarios.
Nunca se borra o deshabilita el último admin.

**Consecuencias.**

- Cambiar contraseña invalida las demás sesiones del usuario.
- La UI distingue sesión actual.
- La auditoría empieza por auth/usuarios; se extiende después sin duplicar.

### D9. CI honesta

**Contexto.** `beginOp` ya impide dos operaciones simultáneas sobre la misma
app. No existe cancelación ni cola.

**Decisión.** Se añade cancelación real con contexto por despliegue. La cola
solo se implementa si es persistente y con worker FIFO por app; mientras no
exista, la UI explica el 409 "app ocupada" y no muestra una cola fingida.

**Consecuencias.**

- Cancelar detiene el proceso y marca el despliegue como cancelado.
- Reintentar usa el mismo `rev` y respeta la cancelación.
- Los pasos se muestran desde marcas estables del log, no desde texto adivinado.

### D10. Paridad MCP mediante servicios canónicos

**Contexto.** API, CLI y MCP deben compartir reglas. MCP ya usa funciones de
dominio, pero las nuevas capacidades pueden quedarse solo en HTTP.

**Decisión.** Cada mutación nueva se expone en MCP con la misma función de
servicio o `cfgfile`. Si una capacidad es solo UI (por ejemplo, un drawer), no
se inventa tool. Las lecturas nuevas se añaden cuando aportan operación.

**Consecuencias.**

- No hay lógica duplicada en `mcpserver`.
- La matriz de paridad se revisa en cada módulo.

### D11. UI, i18n y tema

**Decisión.** Cada módulo reutiliza `PageHeader`, `Section`, `Stat`, tablas y
`FormDialog`. Los textos nuevos van a diccionarios por dominio. Los códigos
cortos (`INF/WRN/ERR`) se mantienen; los nombres largos quedan en `aria-label`
o `title`. Toda vista nueva tiene captura en claro y oscuro cuando aporte.

**Consecuencias.**

- No se añaden librerías de UI ni de charts.
- No se copian assets ajenos.
- La densidad y la jerarquía mandan sobre el adorno.

## 4. Contratos de API

### 4.1 Nuevos o cambiados

| Endpoint | Método | Rol | Sustituye / nota |
|---|---|---|---|
| `/api/v1/apps` | POST | operator | Body `domains []string` en vez de `domain string`. |
| `/api/v1/apps/{name}` | PUT | operator | Body `domains []string` en vez de `domain string`. |
| `/api/v1/domains/{host}` | GET | viewer | Detalle: ruta, app, TLS, DNS y `checked_at`. |
| `/api/v1/domains/{host}/tls` | GET | viewer | Se elimina; su dato pasa al detalle. |
| `/api/v1/env` | GET | operator | Solo claves: `{global:[], apps:{app:[]}}`. |
| `/api/v1/env/global/{key}` | GET | operator | Revela un valor. |
| `/api/v1/apps/{name}/env/{key}` | GET | operator | Revela un valor. |
| `/api/v1/env/global` | PATCH | operator | `{set:{}, unset:[]}` atómico. |
| `/api/v1/apps/{name}/env` | PATCH | operator | `{set:{}, unset:[]}` atómico. |
| `/api/v1/env/global/import` | POST | operator | `{raw}` parseado con `internal/dotenv`. |
| `/api/v1/apps/{name}/env/import` | POST | operator | `{raw}` parseado con `internal/dotenv`. |
| `/api/v1/apps/{name}/start` | POST | operator | Limpia `disabled` y arranca. |
| `/api/v1/apps/{name}/stop` | POST | operator | Persiste `disabled` y detiene. |
| `/api/v1/users/me/password` | PUT | autenticado | `{current,next}`; invalida otras sesiones. |
| `/api/v1/users/{name}/password` | PUT | admin | Reset; invalida sesiones del usuario. |
| `/api/v1/users/me/sessions` | GET | autenticado | Lista sesiones propias. |
| `/api/v1/users/{name}/sessions` | GET | admin | Lista sesiones del usuario. |
| `/api/v1/users/me/sessions/{id}` | DELETE | autenticado | Revoca una sesión propia. |
| `/api/v1/users/{name}/sessions/{id}` | DELETE | admin | Revoca una sesión. |
| `/api/v1/users/{name}` | PUT | admin | `{role?,disabled?}`. |
| `/api/v1/alerts/{id}` | PUT | operator | Editar regla, activar/desactivar. |
| `/api/v1/alerts/{id}/test` | POST | operator | Disparo de prueba sin historial real. |
| `/api/v1/alerts/{id}/fires` | GET | viewer | Historial de disparos. |
| `/api/v1/alert-channels` | GET/POST | operator | Canales de notificación. |
| `/api/v1/alert-channels/{id}` | PUT/DELETE | operator | Editar/borrar canal. |
| `/api/v1/deployments/{id}/cancel` | POST | operator | Cancelación real con contexto. |
| `/api/v1/observability` | GET | viewer | Añade `by_app`, `classes`, `latency`, `events`. |
| `/api/v1/settings` | PUT | operator | Valida con `settings.Validate`; error accionable. |

### 4.2 Se mantienen

- `GET /api/v1/status`, `GET /api/v1/deployments`, `GET /api/v1/events`,
  `GET /api/v1/logs`, `GET /api/v1/logs/stream`.
- `GET /api/v1/proxy` como fuente de diagnóstico.
- `GET /api/v1/usage`, `GET /api/v1/analytics`.
- `GET/POST/PUT/DELETE /api/v1/routes` y zonas/hosts.
- `GET/POST/DELETE /api/v1/fs` y `GET /api/v1/detect`.

### 4.3 Reglas de contrato

- JSON `{error}` uniforme; errores accionables.
- `404` para recurso inexistente, `403` por rol, `409` por conflicto de
  operación, `422` por validación de dominio/revisión.
- Nada de valores de entorno en listas.
- Nada de `any` en respuestas del cliente.

## 5. Checklist por módulo

### 5.1 DOM — Dominios y Proxy

**Prioridad:** P1. **Depende de:** D1, D2.

- [ ] **DOM-01** Inventariar usos de `domain` singular y `SetAppDomain` en
  API, CLI, MCP y web; dejar la lista en el registro de avance.
- [ ] **DOM-02** Cambiar `cfgfile.AppPatch` a `domains *[]string`; eliminar
  `domain *string` y actualizar el JSON.
- [ ] **DOM-03** Implementar `cfgfile.SetAppDomains(cp, name, hosts)`:
  normalizar, deduplicar, validar, reemplazar el conjunto del target en una
  sola ruta y eliminar si la lista queda vacía.
- [ ] **DOM-04** Borrar `SetAppDomain` y migrar todos los llamadores. Sin
  wrapper de compatibilidad.
- [ ] **DOM-05** `CreateAndAdd` acepta `domains []string`; API POST y flujo de
  creación envían la lista.
- [ ] **DOM-06** Tests de `SetAppDomains`: reemplazo, vacío, duplicado, host
  inválido, host ya usado por otro target y rutas ajenas intactas.
- [ ] **DOM-07** Handler PUT `/apps/{name}` acepta `domains`; validación 400
  accionable; test de handler.
- [ ] **DOM-08** `appview` devuelve `domains` en orden estable y documenta
  "primero = principal visual".
- [ ] **DOM-09** UI Ajustes de app: editor de lista con añadir/quitar,
  `DomainSuggest` añade y aviso de que quitar elimina la ruta.
- [ ] **DOM-10** UI cabecera y overview usan la lista; "Abrir" usa el primer
  host; no se persiste un `primary`.
- [ ] **DOM-11** Eliminar textos y claves i18n del dominio singular; añadir
  claves de lista, vacío y principal.
- [ ] **DOM-12** Actualizar CLI y MCP que editen dominio para usar el conjunto;
  test de paridad.
- [ ] **DOM-13** Test de integración: PUT con dos hosts, GET refleja ambos,
  PUT con lista vacía retira las rutas.
- [ ] **DOM-14** Nuevo `GET /api/v1/domains/{host}` con ruta, app dueña, TLS,
  DNS y `checked_at`.
- [ ] **DOM-15** Absorber `/domains/{host}/tls` en el detalle y borrar el
  endpoint viejo; actualizar `TLSBadge`.
- [ ] **DOM-16** Paquete `internal/dnscheck` con resolver inyectable: A, AAAA,
  CNAME, timeout de 2s y error accionable.
- [ ] **DOM-17** Regla de expectativa DNS: localhost/test/base → `127.0.0.1`;
  dominio propio usa `public_ip`; sin `public_ip` solo se informa la
  resolución, sin veredicto falso.
- [ ] **DOM-18** Handler de detalle compone ruta, app por target, TLS y DNS;
  `checked_at` en UTC.
- [ ] **DOM-19** Drawer de dominio desde tabla y mapa: registros, TLS, target,
  app y último chequeo.
- [ ] **DOM-20** Aviso de expiración TLS: ≤14 días warn, vencido bad; visible
  en Dominios y Resumen.
- [ ] **DOM-21** Preflight en "nueva ruta": consulta el detalle DNS antes de
  guardar, muestra aviso y no bloquea zonas locales.
- [ ] **DOM-22** Renovación: mostrar "automática" y fecha. Sin botón si ACME
  no permite forzar; si se añade, `POST /domains/{host}/renew` con test.
- [ ] **DOM-23** Tests de handler y detalle con resolver/TLS falsos; sin red.
- [ ] **DOM-24** Hook único `useProxyStatus` con refresco y caché corta;
  elimina el fetch duplicado de Routes y Proxy.
- [ ] **DOM-25** Fusionar Proxy como pestaña "Diagnóstico" en Dominios;
  `/proxy` redirige; quitar el ítem de navegación y actualizar la paleta.
- [ ] **DOM-26** Capturas: Dominios, Diagnóstico, drawer de dominio y móvil.

### 5.2 ENV — Entorno

**Prioridad:** P1. **Depende de:** D3.

- [ ] **ENV-01** Auditar endpoints y UI actuales; fijar claves vs valores.
- [ ] **ENV-02** `cfgfile.ApplyEnvPatch(scope, set, unset)` atómico para global
  y app.
- [ ] **ENV-03** Validación canónica de claves `[A-Z_][A-Z0-9_]*`; reservadas
  (`PORT`, etc.) con error accionable.
- [ ] **ENV-04** `GET /api/v1/env` devuelve solo claves (global y por app),
  rol operator.
- [ ] **ENV-05** `GET /env/global/{key}` y `GET /apps/{name}/env/{key}` revelan
  un valor; operator; jamás en listas.
- [ ] **ENV-06** `PATCH /env/global` con `set`/`unset`; elimina PUT/DELETE por
  clave.
- [ ] **ENV-07** `PATCH /apps/{name}/env` con `set`/`unset`; elimina
  PUT/DELETE por clave.
- [ ] **ENV-08** `POST /env/global/import` y `/apps/{name}/env/import` con
  `{raw}`, parseado por `internal/dotenv`.
- [ ] **ENV-09** Tests de `ApplyEnvPatch`: set, unset, mixto, clave inválida y
  fallo atómico sin cambios.
- [ ] **ENV-10** Tests de handlers: roles, claves, revelado e importación.
- [ ] **ENV-11** UI global: editar valor en sitio, eliminar, revelar/copiar y
  guardar patch.
- [ ] **ENV-12** UI global: importar `.env` pegado o archivo, previsualizar
  claves y aplicar.
- [ ] **ENV-13** UI por app: editar en sitio e importar; heredadas vs propias.
- [ ] **ENV-14** Banner "aplicado al reiniciar" con acción de reinicio por app;
  nunca reinicio automático.
- [ ] **ENV-15** `EnvAll` carga claves en una sola llamada; sin N+1; valores
  solo al revelar.
- [ ] **ENV-16** Limpiar claves i18n duplicadas y textos viejos.
- [ ] **ENV-17** Tests TS del parser de import si queda lógica cliente; si no,
  eliminar el parser cliente y probar solo el backend.
- [ ] **ENV-18** Capturas: entorno global, importación, app env y móvil.

### 5.3 SET — Ajustes

**Prioridad:** P1. **Depende de:** D4.

- [ ] **SET-01** Inventariar `KnownSettings`: tipo, rango, efecto y reinicio,
  en una tabla dentro de esta spec.
- [ ] **SET-02** Mover claves y validación a `internal/settings`; `store` solo
  persiste.
- [ ] **SET-03** Validadores: `base_domain` host, `port_pool` `n-m`, cuotas
  numéricas ≥0, bins ruta absoluta y `site_name` no vacío.
- [ ] **SET-04** Añadir `public_ip` opcional para preflight DNS; validar IP.
- [ ] **SET-05** API PUT valida con `settings.Validate`; test y error
  accionable.
- [ ] **SET-06** UI agrupa red, proceso, despliegue, cuotas y GitHub; una
  descripción por clave.
- [ ] **SET-07** Indicador "requiere reinicio" por clave; sin reinicio
  automático.
- [ ] **SET-08** Acción "restaurar valor por defecto" con confirmación.
- [ ] **SET-09** No exponer secretos ni claves internas en la tabla.
- [ ] **SET-10** Reemplazar etiquetas crudas (`env_file`, `min_uptime`, `repo`,
  `ref`, `build`) por términos canónicos.
- [ ] **SET-11** Tests de validación y handler.
- [ ] **SET-12** Capturas: ajustes agrupados, edición y error.

### 5.4 APP — Aplicaciones y ciclo de vida

**Prioridad:** P1. **Depende de:** D5, D1.

- [ ] **APP-01** Extraer `canSaveApp(form, meta)` puro y corregir: externas y
  estáticas guardan sin `command`.
- [ ] **APP-02** Test TS de `canSaveApp` (requiere X-01).
- [ ] **APP-03** Añadir `disabled bool` a `config.App`, validación y snapshot;
  el supervisor no arranca apps deshabilitadas.
- [ ] **APP-04** Acciones canónicas `start`/`stop` en el servicio; API
  `POST /apps/{name}/start|stop`, CLI y MCP; tests.
- [ ] **APP-05** UI: detener/iniciar con confirmación y estado "detenida"; no
  duplicar con restart.
- [ ] **APP-06** Añadir `health_path` (default `/`); la sonda usa el path;
  validación y test.
- [ ] **APP-07** UI de `health_path` con descripción.
- [ ] **APP-08** Editor de pasos de build (añadir, quitar, ordenar) en ajustes
  y creación; validación.
- [ ] **APP-09** Ajustes agrupados red/proceso/despliegue; campos condicionales
  para externas/estáticas.
- [ ] **APP-10** Borrado: redirigir a `/`, mostrar rutas retiradas y evitar
  rutas ambiguas.
- [ ] **APP-11** Cabecera con lista de dominios y avisos TLS/DNS; primer host
  como principal visual.
- [ ] **APP-12** Clonar app: decidir sí/no; si no, documentarlo como fuera de
  alcance. No dejar botón falso.
- [ ] **APP-13** Tests de cfgfile/servicio y API para start/stop/health.
- [ ] **APP-14** Capturas: ajustes externa/estática, detenida, health y build
  steps.

### 5.5 OBS — Observabilidad

**Prioridad:** P2. **Depende de:** D6.

- [ ] **OBS-01** Documentar atribución host→app y campos reales de `weblog`.
- [ ] **OBS-02** Función pura de series: req/err por bucket, p50/p95/p99 y
  clases 2xx/3xx/4xx/5xx.
- [ ] **OBS-03** `GET /observability` añade `by_app`, `classes`, `latency` y
  `events`; caché de 1 min.
- [ ] **OBS-04** Tests de agregación con entradas sintéticas y ventanas.
- [ ] **OBS-05** UI: serie con ejes y leyenda, tarjetas total/error/p95 y tabla
  por app.
- [ ] **OBS-06** Superponer eventos de deploy, caída y reinicio en la serie.
- [ ] **OBS-07** Empty state honesto si no hay `access.log`; sin RUM.
- [ ] **OBS-08** Enlazar app/host al detalle de dominio y a logs filtrados.
- [ ] **OBS-09** i18n y textos sobrios.
- [ ] **OBS-10** Capturas: observabilidad, rangos 24h/7d y móvil.

### 5.6 ALR — Alertas

**Prioridad:** P2. **Depende de:** D7.

- [ ] **ALR-01** Migración: `alert_channels`, `enabled/channel_id` en
  `alert_rules`, `alert_fires` y `alert_deliveries`.
- [ ] **ALR-02** Reglas nuevas: `latency_p95`, `memory_mb`, `disk_pct` y
  `down_min`; evaluador puro y tests.
- [ ] **ALR-03** CRUD de reglas: editar, activar/desactivar y borrar; API
  `PUT /alerts/{id}`.
- [ ] **ALR-04** Canales webhook: CRUD, URL http(s), timeout y headers
  permitidos; test con `httptest`.
- [ ] **ALR-05** Outbox con reintentos/backoff y estado; worker en el tick; sin
  entrega fire-and-forget.
- [ ] **ALR-06** `POST /alerts/{id}/test` dispara prueba sin contar historial
  real.
- [ ] **ALR-07** Historial `GET /alerts/{id}/fires` y estado resuelto.
- [ ] **ALR-08** Evento `kind=alert` incluye regla, valor, umbral, ventana y
  resultado de entrega.
- [ ] **ALR-09** UI: editar regla, toggle, probar, historial, selector de canal
  y estado de entrega.
- [ ] **ALR-10** Validar reglas: app existente o `*`, umbral >0 y ventana >0.
- [ ] **ALR-11** Seguridad: secretos fuera de listas, timeout, límite de
  payload y redirects controlados.
- [ ] **ALR-12** Tests de store, evaluador y outbox.
- [ ] **ALR-13** Email solo si hay SMTP configurado; si no, no mostrar canal
  falso.
- [ ] **ALR-14** Capturas: alertas, edición, historial y canal.

### 5.7 USR — Usuarios, sesiones y auditoría

**Prioridad:** P2. **Depende de:** D8.

- [ ] **USR-01** Migración: `users.updated_at`, `users.last_login`,
  `users.disabled` e índice de sesiones.
- [ ] **USR-02** Cambio de contraseña propio con actual+nueva; bcrypt; invalida
  otras sesiones.
- [ ] **USR-03** Reset por admin; invalida sesiones del usuario.
- [ ] **USR-04** Listar sesiones propias y por admin; borrar por id; marcar la
  actual.
- [ ] **USR-05** Actualizar `last_login` al entrar y mostrarlo.
- [ ] **USR-06** Deshabilitar usuario y revocar sesiones; impedir borrar o
  deshabilitar el último admin.
- [ ] **USR-07** Auditoría mínima: `audit(actor, action, target, ip, ts)` para
  auth/usuarios; API admin.
- [ ] **USR-08** UI: contraseña, sesiones, deshabilitar, último acceso y
  auditoría.
- [ ] **USR-09** Tests de store/auth y roles.
- [ ] **USR-10** Cubrir cambio/reset con el rate limit existente.
- [ ] **USR-11** i18n y mensajes de seguridad claros.
- [ ] **USR-12** Capturas: usuarios, sesiones y cambio de contraseña.

### 5.8 CI — Pipeline

**Prioridad:** P3. **Depende de:** D9.

- [ ] **CI-01** Confirmar `beginOp` por app y documentar el 409 "app ocupada";
  la UI explica y ofrece esperar/reintentar.
- [ ] **CI-02** Cancelación real: contexto por deploy,
  `POST /deployments/{id}/cancel` y test.
- [ ] **CI-03** Cola persistente solo si se implementa: estado `queued`,
  worker FIFO por app y cancelación. Sin cola fingida.
- [ ] **CI-04** Parsear pasos `step:` del log a checklist con tiempos; función
  pura y tests.
- [ ] **CI-05** UI: en curso, cola, cancelar, pasos y enlace al log.
- [ ] **CI-06** Reintentar usa el mismo `rev` y respeta la cancelación.
- [ ] **CI-07** Tests de estados y cancelación.
- [ ] **CI-08** Capturas: CI con cola, cancelación y pasos.

### 5.9 LOG — Registros

**Prioridad:** P3. **Depende de:** base `f7edccb`.

- [ ] **LOG-01** Toggle de ajuste de línea (truncar vs envolver) independiente
  de compacto.
- [ ] **LOG-02** Resaltar coincidencias de búsqueda sin inyectar HTML.
- [ ] **LOG-03** Copiar línea cruda y selección.
- [ ] **LOG-04** Alternar hora absoluta/relativa, persistido.
- [ ] **LOG-05** Contexto ±N alrededor de coincidencias cuando hay búsqueda.
- [ ] **LOG-06** Tests TS de `format.ts`: JSON, logfmt, plano y agrupación.
- [ ] **LOG-07** Teclado y accesibilidad: filtros, pausa y foco en consola.
- [ ] **LOG-08** Capturas: compacto, ajuste, búsqueda resaltada y contexto.

### 5.10 OVR — Resumen

**Prioridad:** P3. **Depende de:** DOM-20, OBS, ALR.

- [ ] **OVR-01** Consumir `/usage` para requests, bytes, error rate y p95 con
  barras de cuota.
- [ ] **OVR-02** Avisos TLS próximos a vencer desde el detalle de dominios.
- [ ] **OVR-03** Alertas abiertas desde eventos/fires.
- [ ] **OVR-04** Tasa de éxito de despliegues de 7 días desde SQLite.
- [ ] **OVR-05** Apps caídas y reinicios desde status/observability.
- [ ] **OVR-06** Jerarquía y empty states; no duplicar tablas enteras.
- [ ] **OVR-07** Capturas y tests de datos derivados.

### 5.11 X — Transversal

- [ ] **X-01** Añadir Vitest a `web` (`pnpm --dir web test`) y cubrir
  `format.ts`, `canSaveApp` y validadores puros.
- [ ] **X-02** CI ejecuta Vitest, i18n-check, build y smoke de capturas.
- [ ] **X-03** Convertir capturas en smoke con aserciones: sin errores de
  consola y elementos clave presentes.
- [ ] **X-04** Matriz de paridad MCP: cada mutación nueva con tool y mismo
  servicio; tests.
- [ ] **X-05** i18n guard cubre los namespaces nuevos; cero textos crudos.
- [ ] **X-06** Capturas light/dark de cada módulo nuevo.
- [ ] **X-07** Actualizar `ARCHITECTURE.md` (hogares `dnscheck`, `settings`,
  alertas), `CLI.md` y README.
- [ ] **X-08** Revisión de seguridad: secretos fuera de listas/logs, roles,
  timeouts y SSRF de webhooks.
- [ ] **X-09** Performance: caché DNS/TLS, entorno sin N+1 y series en una
  pasada.
- [ ] **X-10** Commits atómicos y checklist actualizada en el mismo commit;
  sin push.

## 6. Orden de construcción

1. **Fase 0:** X-01 y X-02 (red de tests) en paralelo con DOM.
2. **Fase 1:** DOM completo. Es el módulo acordado y fija el modelo de
   dominios.
3. **Fase 2:** ENV y SET. Comparten escritura atómica y validación.
4. **Fase 3:** APP. Depende del modelo de dominios y de `disabled`.
5. **Fase 4:** OBS y ALR. Comparten `weblog`, eventos y tick de fondo.
6. **Fase 5:** USR. Independiente, pero toca auth y sesiones.
7. **Fase 6:** CI, LOG y OVR. Mejoras sobre lo ya construido.

Cada fase termina verde y con commit atómico. No se abre la siguiente fase con
tests rojos.

## 7. Riesgos y decisiones abiertas

| Riesgo | Mitigación |
|---|---|
| El cambio `domain` → `domains` rompe API/CLI/MCP | Migración en un solo commit, tests de integración y sin wrapper dual. |
| `public_ip` desconocida en dominios propios | Sin valor, el preflight informa resolución y no emite veredicto falso. |
| ACME no expone emisor o renovación forzada | Mostrar solo `managed/expires`; no añadir botón hasta que exista soporte real. |
| Outbox de alertas duplica entregas | Idempotencia por `fire_id+channel_id`, reintentos con backoff y estado persistido. |
| `stop` persistido confunde con caída | Estado "detenida" explícito en UI; `start` limpia `disabled`; reiniciar no lo cambia. |
| Cola de CI añade complejidad | Solo se implementa persistente; mientras tanto, 409 accionable y sin UI de cola. |
| Valores de entorno en listas | Listas solo con claves; revelado por clave y rol operator. |
| Start/stop y restart con caminos duplicados | Una sola función de servicio; API/CLI/MCP solo la llaman. |

## 8. Verificación

Backend:

```text
gofmt -l .
go vet ./...
go test ./...
go build ./...
```

Web:

```text
pnpm --dir web exec tsc --noEmit
node scripts/i18n-check.mjs
pnpm --dir web build
pnpm --dir web test        # desde X-01
```

Smoke visual:

```text
pnpm --dir web exec vite --port 5173
node scripts/shots/mock.mjs
```

Criterios:

- sin errores de consola ni respuestas ≥400 en el mock;
- capturas revisadas en dark y light cuando aplique;
- `web/dist/.gitkeep` restaurado tras el build;
- sin `npm`; siempre `pnpm`.

## 9. Matriz MCP

| Capacidad | API | MCP | Estado |
|---|---|---|---|
| Leer apps/estado | GET `/status`, `/apps` | tools actuales | existente |
| Desplegar/reiniciar/rollback | POST acciones | tools actuales | existente |
| Diff de despliegue | GET `/apps/{name}/diff` | pendiente | DOM/APP |
| Dominios (conjunto) | PUT `/apps/{name}` | añadir tool | DOM-12 |
| Detalle de dominio | GET `/domains/{host}` | añadir tool de lectura | DOM-14 |
| Entorno (claves) | GET `/env` | `app_env_keys` | existente |
| Entorno (mutar) | PATCH/import | decidir tool operator | ENV |
| Alertas (editar/probar) | PUT/POST | añadir tools | ALR |
| Sesiones/usuarios | API admin | no exponer al inicio | USR |
| Cancelar deploy | POST `/deployments/{id}/cancel` | añadir tool | CI |

## 10. Registro de avance

| Fecha | Tarea | Commit | Verificación |
|---|---|---|---|
| 2026-11-05 | Línea base: diff, resumen y logs compactos | `01e227e`, `f7edccb` | `go test ./...`, `tsc`, build, capturas |
| 2026-11-05 | Spec creada (este documento) | `812da8c` | revisión manual |

Al cerrar una tarea, añadir fila con fecha, ID, commit y comandos ejecutados.
No borrar filas anteriores.

## 11. Anexos

### A. Inventario de settings

| Clave | Tipo | Validación | Efecto |
|---|---|---|---|
| `node_bin` | ruta | absoluta y existente | builds Node |
| `python_bin` | ruta | absoluta y existente | builds Python |
| `base_domain` | host | host válido | dominios y discovery |
| `port_pool` | rango | `n-m`, 1..65535 | alta de apps |
| `site_name` | texto | no vacío | marca visible |
| `quota_reqs` | entero | ≥0 | barras de uso |
| `quota_bytes` | entero | ≥0 | barras de uso |
| `public_ip` | IP | IPv4/IPv6 | preflight DNS |

### B. Inventario de dominios (a completar en DOM-01)

| Lugar | Campo/función | Cambio |
|---|---|---|
| `internal/cfgfile/mutate.go` | `AppPatch.Domain`, `SetAppDomain` | reemplazar por `Domains` y `SetAppDomains` |
| `internal/api/apps.go` | create/update | aceptar `domains` |
| `internal/appview` | `domains[]` | orden estable |
| `web/src/features/apps/AppSettings.tsx` | input singular | editor de lista |
| `web/src/features/apps/AppOverview.tsx` | `domains[0]` | lista |
| `internal/mcpserver` | patch de app | conjunto |
| `internal/cli` | comando de app | conjunto |
