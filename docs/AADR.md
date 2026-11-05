# AADR — Expansión por módulos de rkt

| Campo | Valor |
|---|---|
| ID | AADR-001 |
| Estado | aceptado |
| Fecha | 2026-11-05 |
| Ámbito | backend, API, dashboard, MCP |
| Fase activa | AADR completo |
| Siguiente tarea | revisión final y mantenimiento |
| Ampliación | SRV, PROTO, BLD y MOCK cerrados |
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

La ampliación SRV/PROTO/BLD/MOCK está aprobada, pero **no se inicia** hasta
cerrar las fases P1 y P2. Este documento la registra para no perder alcance ni
mezclarla con el trabajo activo.

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
apps usuarias, Kubernetes, agentes remotos, agregación multi-servidor, túneles
TCP crudos y métricas de visitantes únicos o bounce rate. Si no se mide, no se
muestra.

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

### D12. rkt es una instancia única de VPS, no un panel multi-servidor

**Contexto.** El producto está pensado para un solo servidor propio. Añadir
agentes remotos, clustering o federación multiplica la superficie sin aportar
al caso de uso.

**Decisión.** La monitorización del servidor es local y de solo lectura para la
propia instancia: CPU, memoria, swap, disco, red, load average, uptime,
temperatura y firewall. No hay agentes remotos ni agregación de varias
máquinas. Si un dato no está disponible en la plataforma, se omite con un
estado honesto; no se estima.

**Consecuencias.**

- Un único endpoint `/api/v1/server` alimenta el módulo, con snapshot e
  histórico corto en memoria.
- Temperatura y firewall son capacidades opcionales por plataforma.
- El alcance multi-servidor queda fuera de misión, no como deuda.

### D13. Protocolos: HTTP/HTTPS, WS/WSS, SSE y upgrades

**Contexto.** Un PaaS que solo enruta peticiones HTTP completas rompe apps con
WebSocket, SSE o upgrades. El proxy actual usa `httputil.ReverseProxy`; hay que
demostrar y cubrir el resto de protocolos, no asumirlos.

**Decisión.** El proxy soporta y prueba: HTTP/1.1, HTTP/2 cuando el listener
TLS lo permite, WebSocket y WSS mediante upgrade/hijack, SSE sin buffering y
targets `http://`, `https://`, `ws://` y `wss://`. `ws`/`wss` se normalizan al
esquema HTTP equivalente para el transporte y conservan la semántica de
upgrade. Los túneles crudos TCP quedan fuera hasta que exista un caso de uso.

**Consecuencias.**

- La validación de rutas acepta los cuatro esquemas.
- Un WebSocket no se corta por timeouts de respuesta ni por compresión.
- Hay test de eco WS/WSS y de SSE con flush.
- Un target no soportado devuelve error accionable, nunca un 502 opaco.

### D14. Mockdata en español neutro y nombres neutros

**Contexto.** Las capturas son documentación viva. Nombres regionales o copias
de marcas ajenas hacen que el producto parezca una demo local y no un PaaS
sobrio.

**Decisión.** Los fixtures usan español neutro, nombres de persona neutros y
nombres de app descriptivos en español cuando aporten (`tienda`, `api`,
`documentacion`, `facturacion`, `panel`). Nada de `xavi`, `mira`, `tomas` ni
referencias regionales. Los datos deben ser verosímiles, pero nunca fingir
métricas que el backend no produce.

**Consecuencias.**

- `scripts/shots/mock.mjs` y cualquier fixture comparten un vocabulario.
- Las capturas se regeneran tras cambiar nombres para evitar mezclas.
- Los dominios de ejemplo usan `ejemplo.com` o el `base_domain` local.

### D15. Buildpack como ventaja competitiva

**Contexto.** rkt ya detecta Node, Python, Go y estáticos. Para superar a los
buildpacks existentes necesita detección amplia, builds deterministas, caché,
logs claros y monorepos, sin ejecutar `npm`.

**Decisión.** El buildpack es un módulo de primera clase: detección por
manifiestos y lockfiles, frameworks modernos, monorepos, gestores `pnpm`,
`yarn` y `bun` (nunca `npm`), pasos de build declarativos, caché de
dependencias, salida estructurada y tests por matriz. La detección explica por
qué eligió un plan; si no puede, falla con una acción concreta.

**Consecuencias.**

- `internal/buildpack` y `internal/discover` comparten una matriz única.
- Cada framework soportado tiene fixture y test de plan.
- El build log muestra pasos, tiempos y causa de fallo.
- No se promete Dockerfile, Nix ni builds arbitrarios sin diseño previo.

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
| `/api/v1/server` | GET | viewer | Snapshot local: CPU, RAM, swap, load, uptime, disco, red, temperatura y firewall. |
| `/api/v1/server/history` | GET | viewer | Serie corta en memoria (`?hours=1..24`). |
| `/api/v1/server/processes` | GET | viewer | Top procesos por CPU/RAM de la instancia (solo lectura). |
| `/api/v1/proxy/capabilities` | GET | viewer | Protocolos soportados y límites reales (HTTP/1.1, HTTP/2, WS, WSS, SSE). |
| `/api/v1/buildpack/frameworks` | GET | viewer | Matriz de frameworks, gestores y requisitos que el detector reconoce. |
| `/api/v1/detect` | GET | operator | Se amplía: explica el plan elegido y los manifiestos que lo activaron. |

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

- [x] **DOM-01** Inventariar usos de `domain` singular y `SetAppDomain` en
  API, CLI, MCP y web; dejar la lista en el registro de avance.
- [x] **DOM-02** Cambiar `cfgfile.AppPatch` a `domains *[]string`; eliminar
  `domain *string` y actualizar el JSON.
- [x] **DOM-03** Implementar `cfgfile.SetAppDomains(cp, name, hosts)`:
  normalizar, deduplicar, validar, reemplazar el conjunto del target en una
  sola ruta y eliminar si la lista queda vacía.
- [x] **DOM-04** Borrar `SetAppDomain` y migrar todos los llamadores. Sin
  wrapper de compatibilidad.
- [x] **DOM-05** `CreateAndAdd` acepta `domains []string`; API POST y flujo de
  creación envían la lista.
- [x] **DOM-06** Tests de `SetAppDomains`: reemplazo, vacío, duplicado, host
  inválido, host ya usado por otro target y rutas ajenas intactas.
- [x] **DOM-07** Handler PUT `/apps/{name}` acepta `domains`; validación 400
  accionable; test de handler.
- [x] **DOM-08** `appview` devuelve `domains` en orden estable y documenta
  "primero = principal visual".
- [x] **DOM-09** UI Ajustes de app: editor de lista con añadir/quitar,
  `DomainSuggest` añade y aviso de que quitar elimina la ruta.
- [x] **DOM-10** UI cabecera y overview usan la lista; "Abrir" usa el primer
  host; no se persiste un `primary`.
- [x] **DOM-11** Eliminar textos y claves i18n del dominio singular; añadir
  claves de lista, vacío y principal.
- [x] **DOM-12** Actualizar CLI y MCP que editen dominio para usar el conjunto;
  test de paridad.
- [x] **DOM-13** Test de integración: PUT con dos hosts, GET refleja ambos,
  PUT con lista vacía retira las rutas.
- [x] **DOM-14** Nuevo `GET /api/v1/domains/{host}` con ruta, app dueña, TLS,
  DNS y `checked_at`.
- [x] **DOM-15** Absorber `/domains/{host}/tls` en el detalle y borrar el
  endpoint viejo; actualizar `TLSBadge`.
- [x] **DOM-16** Paquete `internal/dnscheck` con resolver inyectable: A, AAAA,
  CNAME, timeout de 2s y error accionable.
- [x] **DOM-17** Regla de expectativa DNS: localhost/test/base → `127.0.0.1`;
  dominio propio usa `public_ip`; sin `public_ip` solo se informa la
  resolución, sin veredicto falso.
- [x] **DOM-18** Handler de detalle compone ruta, app por target, TLS y DNS;
  `checked_at` en UTC.
- [x] **DOM-19** Drawer de dominio desde tabla y mapa: registros, TLS, target,
  app y último chequeo.
- [x] **DOM-20** Aviso de expiración TLS: ≤14 días warn, vencido bad; visible
  en Dominios y Resumen.
- [x] **DOM-21** Preflight en "nueva ruta": consulta el detalle DNS antes de
  guardar, muestra aviso y no bloquea zonas locales.
- [x] **DOM-22** Renovación: mostrar "automática" y fecha. Sin botón si ACME
  no permite forzar; si se añade, `POST /domains/{host}/renew` con test.
- [x] **DOM-23** Tests de handler y detalle con resolver/TLS falsos; sin red.
- [x] **DOM-24** Hook único `useProxyStatus` con refresco y caché corta;
  elimina el fetch duplicado de Routes y Proxy.
- [x] **DOM-25** Fusionar Proxy como pestaña "Diagnóstico" en Dominios;
  `/proxy` redirige; quitar el ítem de navegación y actualizar la paleta.
- [x] **DOM-26** Capturas: Dominios, Diagnóstico, drawer de dominio y móvil.

### 5.2 ENV — Entorno

**Prioridad:** P1. **Depende de:** D3.

- [x] **ENV-01** Auditar endpoints y UI actuales; fijar claves vs valores.
- [x] **ENV-02** `cfgfile.ApplyEnvPatch(scope, set, unset)` atómico para global
  y app.
- [x] **ENV-03** Validación canónica de claves `[A-Z_][A-Z0-9_]*`; reservadas
  (`PORT`, etc.) con error accionable.
- [x] **ENV-04** `GET /api/v1/env` devuelve solo claves (global y por app),
  rol operator.
- [x] **ENV-05** `GET /env/global/{key}` y `GET /apps/{name}/env/{key}` revelan
  un valor; operator; jamás en listas.
- [x] **ENV-06** `PATCH /env/global` con `set`/`unset`; elimina PUT/DELETE por
  clave.
- [x] **ENV-07** `PATCH /apps/{name}/env` con `set`/`unset`; elimina
  PUT/DELETE por clave.
- [x] **ENV-08** `POST /env/global/import` y `/apps/{name}/env/import` con
  `{raw}`, parseado por `internal/dotenv`.
- [x] **ENV-09** Tests de `ApplyEnvPatch`: set, unset, mixto, clave inválida y
  fallo atómico sin cambios.
- [x] **ENV-10** Tests de handlers: roles, claves, revelado e importación.
- [x] **ENV-11** UI global: editar valor en sitio, eliminar, revelar/copiar y
  guardar patch.
- [x] **ENV-12** UI global: importar `.env` pegado o archivo, previsualizar
  claves y aplicar.
- [x] **ENV-13** UI por app: editar en sitio e importar; heredadas vs propias.
- [x] **ENV-14** Banner "aplicado al reiniciar" con acción de reinicio por app;
  nunca reinicio automático.
- [x] **ENV-15** `EnvAll` carga claves en una sola llamada; sin N+1; valores
  solo al revelar.
- [x] **ENV-16** Limpiar claves i18n duplicadas y textos viejos.
- [x] **ENV-17** Tests TS del parser de import si queda lógica cliente; si no,
  eliminar el parser cliente y probar solo el backend.
- [x] **ENV-18** Capturas: entorno global, importación, app env y móvil.

### 5.3 SET — Ajustes

**Prioridad:** P1. **Depende de:** D4.

- [x] **SET-01** Inventariar `KnownSettings`: tipo, rango, efecto y reinicio,
  en una tabla dentro de esta spec.
- [x] **SET-02** Mover claves y validación a `internal/settings`; `store` solo
  persiste.
- [x] **SET-03** Validadores: `base_domain` host, `port_pool` `n-m`, cuotas
  numéricas ≥0, bins ruta absoluta y `site_name` no vacío.
- [x] **SET-04** Añadir `public_ip` opcional para preflight DNS; validar IP.
- [x] **SET-05** API PUT valida con `settings.Validate`; test y error
  accionable.
- [x] **SET-06** UI agrupa red, proceso, despliegue, cuotas y GitHub; una
  descripción por clave.
- [x] **SET-07** Indicador "requiere reinicio" por clave; sin reinicio
  automático.
- [x] **SET-08** Acción "restaurar valor por defecto" con confirmación.
- [x] **SET-09** No exponer secretos ni claves internas en la tabla.
- [x] **SET-10** Reemplazar etiquetas crudas (`env_file`, `min_uptime`, `repo`,
  `ref`, `build`) por términos canónicos.
- [x] **SET-11** Tests de validación y handler.
- [x] **SET-12** Capturas: ajustes agrupados, edición y error.

### 5.4 APP — Aplicaciones y ciclo de vida

**Prioridad:** P1. **Depende de:** D5, D1.

- [x] **APP-01** Extraer `canSaveApp(form, meta)` puro y corregir: externas y
  estáticas guardan sin `command`.
- [x] **APP-02** Test TS de `canSaveApp` (requiere X-01).
- [x] **APP-03** Añadir `disabled bool` a `config.App`, validación y snapshot;
  el supervisor no arranca apps deshabilitadas.
- [x] **APP-04** Acciones canónicas `start`/`stop` en el servicio; API
  `POST /apps/{name}/start|stop`, CLI y MCP; tests.
- [x] **APP-05** UI: detener/iniciar con confirmación y estado "detenida"; no
  duplicar con restart.
- [x] **APP-06** Añadir `health_path` (default `/`); la sonda usa el path;
  validación y test.
- [x] **APP-07** UI de `health_path` con descripción.
- [x] **APP-08** Editor de pasos de build (añadir, quitar, ordenar) en ajustes
  y creación; validación.
- [x] **APP-09** Ajustes agrupados red/proceso/despliegue; campos condicionales
  para externas/estáticas.
- [x] **APP-10** Borrado: redirigir a `/`, mostrar rutas retiradas y evitar
  rutas ambiguas.
- [x] **APP-11** Cabecera con lista de dominios y avisos TLS/DNS; primer host
  como principal visual.
- [-] **APP-12** Clonar app: no se implementa en esta fase. Duplicar apps
  arrastra secretos y rutas; requiere diseño aparte. Sin botón falso.
- [x] **APP-13** Tests de cfgfile/servicio y API para start/stop/health.
- [x] **APP-14** Capturas: ajustes externa/estática, detenida, health y build
  steps.

### 5.5 OBS — Observabilidad

**Prioridad:** P2. **Depende de:** D6.

- [x] **OBS-01** Documentar atribución host→app y campos reales de `weblog`.
- [x] **OBS-02** Función pura de series: req/err por bucket, p50/p95/p99 y
  clases 2xx/3xx/4xx/5xx.
- [x] **OBS-03** `GET /observability` añade `by_app`, `classes`, `latency` y
  `events`; caché de 1 min.
- [x] **OBS-04** Tests de agregación con entradas sintéticas y ventanas.
- [x] **OBS-05** UI: serie con ejes y leyenda, tarjetas total/error/p95 y tabla
  por app.
- [x] **OBS-06** Superponer eventos de deploy, caída y reinicio en la serie.
- [x] **OBS-07** Empty state honesto si no hay `access.log`; sin RUM.
- [x] **OBS-08** Enlazar app/host al detalle de dominio y a logs filtrados.
- [x] **OBS-09** i18n y textos sobrios.
- [x] **OBS-10** Capturas: observabilidad, rangos 24h/7d y móvil.

### 5.6 ALR — Alertas

**Prioridad:** P2. **Depende de:** D7.

- [x] **ALR-01** Migración: `alert_channels`, `enabled/channel_id` en
  `alert_rules`, `alert_fires` y `alert_deliveries`.
- [x] **ALR-02** Reglas nuevas: `latency_p95`, `memory_mb`, `disk_pct` y
  `down_min`; evaluador puro y tests.
- [x] **ALR-03** CRUD de reglas: editar, activar/desactivar y borrar; API
  `PUT /alerts/{id}`.
- [x] **ALR-04** Canales webhook: CRUD, URL http(s), timeout y headers
  permitidos; test con `httptest`.
- [x] **ALR-05** Outbox con reintentos/backoff y estado; worker en el tick; sin
  entrega fire-and-forget.
- [x] **ALR-06** `POST /alerts/{id}/test` dispara prueba sin contar historial
  real.
- [x] **ALR-07** Historial `GET /alerts/{id}/fires` y estado resuelto.
- [x] **ALR-08** Evento `kind=alert` incluye regla, valor, umbral, ventana y
  resultado de entrega.
- [x] **ALR-09** UI: editar regla, toggle, probar, historial, selector de canal
  y estado de entrega.
- [x] **ALR-10** Validar reglas: app existente o `*`, umbral >0 y ventana >0.
- [x] **ALR-11** Seguridad: secretos fuera de listas, timeout, límite de
  payload y bloqueo de metadata/link-local en webhooks.
- [x] **ALR-12** Tests de store, evaluador y outbox.
- [x] **ALR-13** Email solo si hay SMTP configurado; si no, no mostrar canal
  falso.
- [x] **ALR-14** Capturas: alertas, edición, historial y canal.

### 5.7 USR — Usuarios, sesiones y auditoría

**Prioridad:** P2. **Depende de:** D8.

- [x] **USR-01** Migración: `users.updated_at`, `users.last_login`,
  `users.disabled` e índice de sesiones.
- [x] **USR-02** Cambio de contraseña propio con actual+nueva; bcrypt; invalida
  otras sesiones.
- [x] **USR-03** Reset por admin; invalida sesiones del usuario.
- [x] **USR-04** Listar sesiones propias y por admin; borrar por id; marcar la
  actual.
- [x] **USR-05** Actualizar `last_login` al entrar y mostrarlo.
- [x] **USR-06** Deshabilitar usuario y revocar sesiones; impedir borrar o
  deshabilitar el último admin.
- [x] **USR-07** Auditoría mínima: `audit(actor, action, target, ip, ts)` para
  auth/usuarios; API admin.
- [x] **USR-08** UI: contraseña, sesiones, deshabilitar, último acceso y
  auditoría.
- [x] **USR-09** Tests de store/auth y roles.
- [-] **USR-10** El rate limit de login ya existe; cambio/reset exigen sesión
  válida y no pasan por el throttle. No se duplica un segundo limitador.
- [x] **USR-11** i18n y mensajes de seguridad claros.
- [x] **USR-12** Capturas: usuarios, sesiones y cambio de contraseña.

### 5.8 CI — Pipeline

**Prioridad:** P3. **Depende de:** D9.

- [x] **CI-01** Confirmar `beginOp` por app y documentar el 409 "app ocupada";
  la UI explica y ofrece esperar/reintentar.
- [x] **CI-02** Cancelación real: contexto por deploy,
  `POST /deployments/{id}/cancel` y test.
- [-] **CI-03** Cola persistente: no se implementa hasta que exista demanda
  real. El 409 "app ocupada" es accionable y no se finge una cola.
- [x] **CI-04** Parsear pasos `step:` del log a checklist con tiempos; función
  pura y tests.
- [x] **CI-05** UI: en curso, cola, cancelar, pasos y enlace al log.
- [x] **CI-06** Reintentar usa el mismo `rev` y respeta la cancelación.
- [x] **CI-07** Tests de estados y cancelación.
- [x] **CI-08** Capturas: CI con cola, cancelación y pasos.

### 5.9 LOG — Registros

**Prioridad:** P3. **Depende de:** base `f7edccb`.

- [x] **LOG-01** Toggle de ajuste de línea (truncar vs envolver) independiente
  de compacto.
- [x] **LOG-02** Resaltar coincidencias de búsqueda sin inyectar HTML.
- [x] **LOG-03** Copiar línea cruda y selección.
- [x] **LOG-04** Alternar hora absoluta/relativa, persistido.
- [x] **LOG-05** Contexto ±N alrededor de coincidencias cuando hay búsqueda.
- [x] **LOG-06** Tests TS de `format.ts`: JSON, logfmt, plano y agrupación.
- [x] **LOG-07** Teclado y accesibilidad: filtros, pausa y foco en consola.
- [x] **LOG-08** Capturas: compacto, ajuste, búsqueda resaltada y contexto.

### 5.10 OVR — Resumen

**Prioridad:** P3. **Depende de:** DOM-20, OBS, ALR.

- [x] **OVR-01** Consumir `/usage` para requests, bytes, error rate y p95 con
  barras de cuota.
- [x] **OVR-02** Avisos TLS próximos a vencer desde el detalle de dominios.
- [x] **OVR-03** Alertas abiertas desde eventos/fires.
- [x] **OVR-04** Tasa de éxito de despliegues de 7 días desde SQLite.
- [x] **OVR-05** Apps caídas y reinicios desde status/observability.
- [x] **OVR-06** Jerarquía y empty states; no duplicar tablas enteras.
- [x] **OVR-07** Capturas y tests de datos derivados.

### 5.11 SRV — Servidor y monitorización local

**Prioridad:** P1.5. **Depende de:** D12. **Alcance:** una sola instancia VPS.

- [x] **SRV-01** Inventariar colectores disponibles por plataforma y decidir
  qué se mide, qué es opcional y qué se omite.
- [x] **SRV-02** Crear `internal/servermetrics` con `Snapshot` tipado: CPU
  total/por núcleo, load 1/5/15, RAM, swap, uptime, discos, red, temperatura y
  firewall.
- [x] **SRV-03** Colector Windows: CPU, memoria, discos, red, uptime y
  temperatura (best effort); firewall de solo lectura.
- [x] **SRV-04** Colector Linux: `/proc/stat`, `/proc/meminfo`,
  `/proc/loadavg`, `/proc/uptime`, `/proc/net/dev`, `statfs`,
  `/sys/class/thermal` y nft/iptables de solo lectura.
- [x] **SRV-05** Cálculo de CPU por delta con reloj inyectable; test.
- [x] **SRV-06** Parseo de memoria/swap con fixtures; test.
- [x] **SRV-07** Discos por punto de montaje, ignorando pseudo-filesystems;
  test.
- [x] **SRV-08** Tasas de red por delta; test.
- [x] **SRV-09** Temperatura opcional: si el sensor no existe, estado vacío
  honesto; nunca valor inventado.
- [x] **SRV-10** Firewall de solo lectura: estado, puertos y reglas visibles;
  sin mutación desde el dashboard.
- [x] **SRV-11** Top procesos por CPU/RAM con `procstat`/`appusage`, acotado y
  sin bloquear el request.
- [x] **SRV-12** Histórico en memoria de 24h con muestras de 1 min y límite
  duro; sin persistir series en SQLite.
- [x] **SRV-13** Endpoints `/server`, `/server/history` y
  `/server/processes`; cache corta y rol viewer.
- [x] **SRV-14** Módulo "Servidor" en el dashboard: placas, sparklines, discos,
  temperatura, red y firewall.
- [x] **SRV-15** Alertas nuevas: `cpu_pct`, `mem_pct`, `disk_pct`, `temp_c`;
  evaluador con datos del snapshot.
- [x] **SRV-16** Enlazar alertas de servidor con el mismo outbox de canales.
- [x] **SRV-17** Empty states por plataforma: "sensor no disponible" y "firewall
  no legible" sin romper la vista.
- [x] **SRV-18** i18n español neutro para todas las métricas y unidades.
- [x] **SRV-19** Tests de colectores y handlers; snapshot determinista con
  fixtures.
- [x] **SRV-20** Capturas desktop, móvil y light/dark del módulo Servidor.

### 5.12 PROTO — Protocolos y upgrades

**Prioridad:** P1.5. **Depende de:** D13.

- [x] **PROTO-01** Inventariar el proxy actual: `ReverseProxy`, timeouts,
  compresión, flush y manejo de `Connection: Upgrade`.
- [x] **PROTO-02** Aceptar y normalizar targets `http`, `https`, `ws` y `wss`
  en validación de rutas.
- [x] **PROTO-03** Test de eco WebSocket a través del proxy, incluyendo
  mensajes binarios y cierre limpio.
- [x] **PROTO-04** Test WSS con la CA local y certificado de desarrollo.
- [x] **PROTO-05** Test SSE: eventos llegan sin buffering y el cliente puede
  cancelar.
- [x] **PROTO-06** Verificar HTTP/2 en listener TLS y ALPN; documentar límites.
- [x] **PROTO-07** Preservar cabeceras de upgrade y no comprimir respuestas de
  upgrade.
- [x] **PROTO-08** Ajustar timeouts e idle connections para conexiones largas
  sin afectar peticiones normales.
- [x] **PROTO-09** UI de rutas: aceptar los cuatro esquemas, mostrarlos y
  explicar el destino.
- [x] **PROTO-10** `GET /proxy/capabilities` con protocolos y límites reales.
- [x] **PROTO-11** Errores accionables para esquema no soportado o upgrade
  rechazado; nunca un 502 opaco.
- [x] **PROTO-12** Documentar ejemplos de app con WebSocket y SSE.
- [x] **PROTO-13** Prueba de carga ligera con conexiones WS concurrentes.
- [x] **PROTO-14** Capturas o demo de app WS/SSE funcionando a través de rkt.

### 5.13 BLD — Buildpack

**Prioridad:** P1.5. **Depende de:** D15.

- [x] **BLD-01** Inventariar `internal/buildpack` y `internal/discover`: qué
  detecta hoy, con qué evidencia y qué promete.
- [x] **BLD-02** Modelo `FrameworkPlan` con `kind`, `evidence`, `install`,
  `build`, `start`, `staticDir` y `nodeVersion`.
- [x] **BLD-03** Gestores Node: `pnpm`, `yarn` y `bun` por lockfile; nunca
  `npm`; normalización canónica y tests.
- [x] **BLD-04** Frameworks: Next standalone, Nuxt, Astro, Vite, SvelteKit,
  Remix, SolidStart, Go, Python (FastAPI/Django/Flask), Rust y Java básico.
- [x] **BLD-05** Monorepos: detectar workspace, app concreta y filtro de build;
  no asumir la raíz.
- [x] **BLD-06** Salidas estáticas: `dist`, `build`, `out`, `.output/public`,
  `public`; servir con el binario.
- [x] **BLD-07** Build determinista: `NODE_ENV`, `CI=1`, sin prompts, sin
  telemetría y sin `npm`.
- [-] **BLD-08** Caché custom: pnpm/yarn/bun ya mantienen store global por
  lockfile. No se añade una segunda caché sin medir el beneficio.
- [x] **BLD-09** Log de build con pasos, tiempos y causa exacta del fallo.
- [x] **BLD-10** Errores de detección con manifiesto faltante y comando
  sugerido.
- [x] **BLD-11** Fixtures por framework y test de matriz (detección + plan).
- [x] **BLD-12** `GET /detect` devuelve evidencia y explicación del plan.
- [x] **BLD-13** UI de creación muestra evidencia y permite editar pasos.
- [x] **BLD-14** Versión de Node desde `.nvmrc`, `engines` o default seguro.
- [x] **BLD-15** Python: `requirements.txt`, `pyproject`, `uv` y `poetry` con
  entorno aislado.
- [x] **BLD-16** Go: flags de build, `-trimpath`, binario esperado y test.
- [-] **BLD-17** El build conserva el env de la app porque los frameworks lo
  necesitan (variables públicas y de build). Los secretos no se imprimen y no
  viajan en argumentos; una allowlist rígida rompería builds válidos.
- [x] **BLD-18** Documentar matriz soportada y límites; actualizar capturas.

### 5.14 MOCK — Fixtures y capturas

**Prioridad:** P1.5. **Depende de:** D14.

- [x] **MOCK-01** Inventariar nombres, dominios y textos de
  `scripts/shots/mock.mjs`.
- [x] **MOCK-02** Sustituir nombres regionales por nombres neutros.
- [x] **MOCK-03** Usar apps descriptivas: `tienda`, `api`, `documentacion`,
  `facturacion`, `panel`.
- [x] **MOCK-04** Revisar todo el copy de fixtures en español neutro.
- [x] **MOCK-05** Dominios de ejemplo `ejemplo.com` o base local.
- [x] **MOCK-06** Extraer fixtures compartidos para capturas y tests.
- [x] **MOCK-07** Eliminar regionalismos y lenguaje de marketing.
- [x] **MOCK-08** Regenerar todas las capturas y revisar dark/light/móvil.
- [x] **MOCK-09** Asegurar que ninguna vista usa datos fuera del fixture.
- [x] **MOCK-10** Guard que detecte nombres prohibidos en fixtures.

### 5.15 X — Transversal

- [x] **X-01** Añadir Vitest a `web` (`pnpm --dir web test`) y cubrir
  `format.ts`, `canSaveApp` y validadores puros.
- [x] **X-02** CI ejecuta Vitest, i18n-check, build y smoke de capturas.
- [x] **X-03** Convertir capturas en smoke con aserciones: sin errores de
  consola y elementos clave presentes.
- [x] **X-04** Matriz de paridad MCP: cada mutación nueva con tool y mismo
  servicio; tests.
- [x] **X-05** i18n guard cubre los namespaces nuevos; cero textos crudos.
- [x] **X-06** Capturas light/dark de cada módulo nuevo.
- [x] **X-07** Actualizar `ARCHITECTURE.md` (hogares `dnscheck`, `settings`,
  alertas), `CLI.md` y README.
- [x] **X-08** Revisión de seguridad: secretos fuera de listas/logs, roles,
  timeouts y bloqueo SSRF de webhooks.
- [x] **X-09** Performance: caché DNS/TLS, entorno sin N+1 y series en una
  pasada.
- [x] **X-10** Commits atómicos y checklist actualizada en el mismo commit;
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
8. **Fase 7 (posterior, no iniciar sin cerrar P1):** SRV, PROTO y BLD.
   MOCK se ejecuta junto a las capturas de cada fase.
9. **Fase 8:** MOCK completo y regeneración global de capturas.

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
| Sensores de temperatura no disponibles en VPS | Mostrar "sensor no disponible"; nunca estimar. |
| Firewall ilegible sin privilegios | Lectura best effort; estado honesto y sin acciones de escritura. |
| WebSocket roto por timeouts/compresión | Tests WS/WSS/SSE y configuración explícita de upgrade. |
| Buildpack con detección ambigua | Evidencia obligatoria y error accionable; matriz de fixtures. |
| Fixtures regionales o de marcas ajenas | Guard de nombres prohibidos y revisión de capturas. |

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
| Servidor local | GET `/server` | añadir tool de lectura | SRV |
| Buildpack | GET `/buildpack/frameworks`, `/detect` | añadir tool de lectura | BLD |
| Capacidades proxy | GET `/proxy/capabilities` | añadir tool de lectura | PROTO |

## 10. Registro de avance

| Fecha | Tarea | Commit | Verificación |
|---|---|---|---|
| 2026-11-05 | Línea base: diff, resumen y logs compactos | `01e227e`, `f7edccb` | `go test ./...`, `tsc`, build, capturas |
| 2026-11-05 | Spec creada (este documento) | `812da8c` | revisión manual |
| 2026-11-05 | AADR ampliado: servidor local, protocolos, buildpack y fixtures neutros | `fe31663` | revisión manual |
| 2026-11-05 | Bloque P1/P2: 114 tareas marcadas | `272ec58` | `go test ./...`, `tsc`, Vitest, build, capturas |
| 2026-11-05 | Brechas: dominios externos, borrado, observabilidad, webhooks y tiempos | `a3700a9` | `go test ./internal/{api,deploy,cli}`, `tsc`, i18n |
| 2026-11-05 | Checklist actualizada tras el bloque P1/P2 | este commit | revisión manual |
| 2026-11-05 | Cierre P1/P2: tests de dominios/usuarios/alertas, contexto de logs, eventos y capturas light | `601fe5e` | `go test ./...`, `tsc`, Vitest, capturas |
| 2026-11-05 | X-04: paridad MCP de entorno y alertas | `58aa1ff` | `go test ./internal/mcpserver`, `tsc` |
| 2026-11-05 | Gate de cobertura: store 90%, hosts 98% y tests de `WithTx` | `2eeaebc` | bucle de cobertura local |
| 2026-11-05 | SRV completo: colector local, API, vista, alertas y capturas | `1f30244` | `go test ./internal/servermetrics ./internal/api`, `tsc`, capturas |
| 2026-11-05 | PROTO completo: ws/wss, SSE, HTTP/2, capacidades y tests | `d3c53ff`, `dbb1c6c` | `go test ./internal/proxy ./internal/config`, `tsc` |
| 2026-11-05 | BLD parcial: evidencia, gestores, Python/Rust/SolidStart, env determinista y UI | `42434d2` | `go test ./internal/buildpack ./internal/discover ./internal/deploy`, `tsc` |
| 2026-11-05 | BLD final: Java, salidas estáticas, versión Node y caché decidida | `350575e` | `go test ./internal/buildpack ./internal/discover` |
| 2026-11-05 | MOCK: nombres neutros, dominios ejemplo.com y guard de fixtures | `350575e` | `node scripts/fixtures-check.mjs`, capturas |
| 2026-11-05 | MOCK-06: fixtures extraídos a módulo compartido | este commit | `node --check`, `fixtures-check`, capturas |

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

### B. Inventario de dominios (DOM-01 completado)

| Lugar | Campo/función | Cambio |
|---|---|---|
| `internal/cfgfile/mutate.go` | `AppPatch.Domain`, `SetAppDomain` | reemplazar por `Domains` y `SetAppDomains` |
| `internal/api/apps.go` | create/update | aceptar `domains` |
| `internal/appview` | `domains[]` | orden estable |
| `web/src/features/apps/AppSettings.tsx` | input singular | editor de lista |
| `web/src/features/apps/AppOverview.tsx` | `domains[0]` | lista |
| `internal/mcpserver` | patch de app | conjunto |
| `internal/cli` | comando de app | conjunto |

### C. Métricas del servidor (SRV)

| Grupo | Datos | Fuente |
|---|---|---|
| CPU | uso total, por núcleo, load 1/5/15 | `GetSystemTimes` / `/proc/stat` + `/proc/loadavg` |
| Memoria | total, usada, disponible, caché, swap | `GlobalMemoryStatusEx` / `/proc/meminfo` |
| Disco | por montaje: total, libre, usado, % | `GetDiskFreeSpaceEx` / `statfs` |
| Red | bytes y paquetes rx/tx, tasa por segundo | `GetIfTable2` / `/proc/net/dev` |
| Tiempo | uptime del host | API del SO / `/proc/uptime` |
| Temperatura | °C por sensor, si existe | WMI térmico / `/sys/class/thermal` |
| Firewall | estado, puertos y reglas de solo lectura | API del SO / nft/iptables |
| Procesos | top por CPU/RAM, PID y nombre | `procstat` |

Sin agentes remotos. Sin multi-servidor. Sin valores estimados.

### D. Matriz inicial del buildpack (BLD)

| Familia | Detección | Salida / arranque |
|---|---|---|
| Next.js | `next.config.*`, `package.json` | standalone + `server.js` |
| Nuxt | `nuxt.config.*` | `.output/server/index.mjs` |
| Astro | `astro.config.*` | `dist/` estático o SSR |
| Vite | `vite.config.*` | `dist/` estático |
| SvelteKit | `svelte.config.*` | `build/` o adapter-node |
| Remix | `remix.config.*` / `@remix-run` | `build/` + server |
| SolidStart | `app.config.*` / `solid-start` | `.output/` |
| Go | `go.mod` | binario `cmd/*` o raíz |
| Python | `pyproject.toml`, `requirements.txt`, `uv.lock`, `poetry.lock` | ASGI/WSGI o script |
| Rust | `Cargo.toml` | binario release |
| Java | `pom.xml`, `build.gradle*` | jar ejecutable |

La matriz se amplía solo con fixture y test. Un plan sin evidencia no se elige.

### E. Protocolos soportados (PROTO)

| Protocolo | Estado objetivo | Verificación |
|---|---|---|
| HTTP/1.1 | obligatorio | tests actuales + smoke |
| HTTPS | obligatorio | TLS local/ACME |
| HTTP/2 | cuando el listener TLS lo permita | ALPN y test |
| WebSocket (`ws`) | obligatorio | eco binario/texto |
| WebSocket seguro (`wss`) | obligatorio | eco con CA local |
| SSE | obligatorio | eventos sin buffering |
| TCP crudo | fuera de alcance | decisión explícita |

### F. Atribución de observabilidad

| Dato | Origen | Regla |
|---|---|---|
| Peticiones/errores | `access.log` | `status >= 500` cuenta como error. |
| Latencia | `dur=` del access log | p50/p95/p99 por bucket y por app. |
| App dueña | `routes` | target `http://127.0.0.1:PORT` → app con ese `PORT`. |
| Reinicios/caídas | `events` | kinds `died`, `restarted`, `failed`. |
| Clases | status HTTP | 2xx, 3xx, 4xx, 5xx. |

Sin RUM ni métricas dentro de las apps. Si no hay `access.log`, la vista lo
dice y no inventa series.
