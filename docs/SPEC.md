# SPEC — dashboard rkt estilo Vercel (inspiración, no clon)

Fuente: 8 capturas vercel.com/coatl (Team Overview, Deployments, Analytics,
Project Overview, Env Vars, Domains, Settings, Logs). Marca, iconos y
Geist NO se copian. Se copian patrones UX + densidad.

Estado hoy: auth sesiones, apps, deployments+drawer, logs tail, routes,
settings KV, users, ⌘K, toasts, PWA offline. Este spec cierra la brecha.

## 0. Principios

- Un endpoint = una feature; UI compone (`/api/v1`, JSON `{error}`).
- Roles: viewer lee, operator muta, admin usuarios (ya existe).
- Secretos jamás en listas (env solo detalle operator, ya existe).
- Nada que Vercel hace con edge (ISR, Fluid CPU) se finge: se adapta
  a lo que rkt mide (access.log + events + supervisor).
- Cada fase termina verde: gofmt/vet/test/build + smoke + commit átomo.

## 1. Layout global (base, casi listo)

Existe: sidebar, topbar móvil, ⌘K, toasts, skeletons, offline, install.
Falta:
- [ ] 1.1 Topbar desktop: breadcrumb contexto (Team/App), botón Create New
      (menu: App desde repo/URL, Domain), usuario+rol+salir.
- [ ] 1.2 Sidebar secciones team: Overview, Deployments, Logs, Analytics,
      Observability, Domains, Environment Variables, Settings, Users.
      Contexto app (al elegir app): Overview, Deployments, Logs,
      Analytics, Env, Domains(app), Settings(app). Sin iconos copiados:
      glyphs propios (▲ ● ◆ ⌘ ▤ ⬡).
- [ ] 1.3 ⌘K extiende: ir a settings/users/routes, filtrar por estado,
      acción directa restart/deploy/rollback (existe), reciente primero.
- [ ] 1.4 “New repository detected” (captura 1): tarjeta cuando discovery
      registra app sola. Backend existe (`discover.Run` → registrados);
      falta `GET /api/v1/discovery` (últimas auto-registradas desde events
      kind=discover) + botón Import (= abre app). Si no hay, la tarjeta
      no se muestra (nada fingido).

## 2. Team Overview (`/`)

Captura 1: Usage + Alerts + Recent Previews + Projects.
- [ ] 2.1 Projects: fila = dot estado, nombre, dominio primario (primer
      host de routes que apunte a su puerto), último deploy (msg=rev,
      repo, fecha), menú (abrir, logs, restart). Buscar filtra (existe
      global). Sin avatares: inicial en círculo.
- [ ] 2.2 Usage (30d, desde access.log): Requests, Bandwidth (bytes),
      Error rate (5xx %), P95 latency. Barras vs límite local configurable
      (settings `quota_*`, solo visual, sin cobro). Backend:
      `GET /api/v1/usage?days=30` (parser + rollup diario en memoria con
      caché 5min; access.log rota a .1 → lee ambos).
- [ ] 2.3 Alerts: tarjeta vacía honesta (“sin alertas”) + crear alerta
      (fase 8). Nada de “Upgrade to Pro”.
- [ ] 2.4 Recent: últimos 5 deployments con estado (reusa tabla).

## 3. Project Overview (`/app/:name`)

Captura 4: Production Deployment + checklist + observability + analytics.
- [ ] 3.1 Production Deployment: último deploy ok (o running): rev,
      domains (hosts que apuntan a la app, con copiar), Status, Created
      (fecha + actor = usuario que disparó deploy; deploy manual guarda
      `triggered_by` — migración deployments v7), Source (repo@ref si hay).
      Botones: Visit (abre dominio), Instant Rollback (existe API),
      Redeploy (POST deploy, existe).
- [ ] 3.2 Production Checklist (computado, nada manual): repo conectado
      (deploy!=nil), dominio custom (host != base_domain), primer deploy
      ok, env production (≥1 var), health corriendo. x/y real.
- [ ] 3.3 Observability 6h mini: requests + errores (access.log por host
      de la app) + restarts (events app) con sparklines SVG propios.
- [ ] 3.4 Deployment Settings colapsable: muestra build/ref/auto + link
      a app settings. Sin “Recommendations” inventadas.

## 4. Deployments (team y por app)

Captura 2: filtros + tabla rica.
- [ ] 4.1 Filtros: texto (existe), estado (all/ok/running/failed), app.
      Chips removibles. Sin Author/Environment (no hay multi-env: rkt
      es production-only; decirlo, no fingir environments).
- [ ] 4.2 Fila: dot, app, rev corta + copiar, status + duración,
      ref (branch del deploy, existe en bloque deploy), fecha, actor.
- [ ] 4.3 Drawer (existe) + timeline (existe) + build log (existe) +
      falta: variables del deploy (snapshot env usado — guardar
      `env_snapshot` en deployments v7 al correr deploy), botón
      Redeploy aquí también.
- [ ] 4.4 Redeploy de ID (no solo de app): `POST /api/v1/deployments/:id/retry`
      (re-corre mismo rev). Migración: deployments guarda `ref+rev`.

## 5. Logs (el salto grande)

Captura 8: búsqueda, live, timeline, niveles, facetas.
Hoy: tail por archivo + follow. Fases:
- [ ] 5.1 Search en tail cargado (cliente, regex) + Live toggle (poll
      2s vs pausa) + descargar (.log).
- [ ] 5.2 Niveles: parsea `level=`/`WARN|ERROR` de líneas slog del
      rkt.log; contadores Info/Warning/Error. Líneas sin nivel = Info.
- [ ] 5.3 Facetas cliente: app (existe selector), texto. Servidor después.
- [ ] 5.4 Timeline histograma (24 buckets desde access.log para app;
      clic fija rango). NO se promete: Source/Route/Method/Branch/
      DeploymentID por línea (nuestros logs no traen esos campos).
- [ ] 5.5 Runtime Logs vs Build Logs: tab que alterna app.log y último
      deploys/*.log (endpoints existen).

## 6. Analytics (por app, desde access.log)

Captura 3. Sin “Enable”: siempre on (datos locales).
- [ ] 6.1 `GET /api/v1/analytics?app=&days=7`: requests, bytes,
      errores %, p50/p95 (dur=), top rutas (path sin query, top 10),
      top referers (top 5), serie diaria (para sparkline/chart SVG
      propio, sin lib de charts).
- [ ] 6.2 UI: 4 stat cards + chart SVG + tablas top. Rango 24h/7d/30d.
- [ ] 6.3 NO: Visitors únicos reales (IP+UA es aproximación y se dice),
      Bounce Rate (sin concepto de sesión web: se omite, no se inventa),
      “Demo Data”.

## 7. Observability (team)

Captura 4 (cards) a nivel flota.
- [ ] 7.1 `GET /api/v1/observability?hours=6`: serie requests/min,
      errores/min (access.log), restarts (events kinds died/restarted),
      apps caídas ahora (supervisor). Sparklines SVG.
- [ ] 7.2 Speed Insights: NO como producto (necesita RUM en apps
      usuarias). Se ofrece p50/p95 por app desde access.log dentro de
      Analytics. Nombre honesto: “Latency”, no “Speed Insights”.

## 8. Alerts (motor mínimo real)

Captura 1 (Alerts vacía con upsell → nosotros: funcional).
- [ ] 8.1 `alert_rules` en DB (migración): {app|*, kind: failed|error_rate,
      threshold, window_min}. Evaluación en el poll existente
      (autoDeploy tick): al disparar → `events(kind=alert)` + log.
- [ ] 8.2 UI: lista/crear/borrar (operator), tarjeta en overview con
      alertas abiertas (events kind=alert). Sin email/webhook fase 1
      (el evento + dashboard bastan; notificar es fase posterior).

## 9. Domains

Captura 6. Sin registrar/comprar (somos PaaS con DNS externo).
- [ ] 9.1 Team: tabla host → target + app inferida (puerto→app),
      buscar, agregar (existe), borrar (existe), copiar.
- [ ] 9.2 Por dominio: estado TLS (del manager: existe cert? expira? —
      exponer vía `GET /api/v1/domains/:host/tls`), target editable
      (POST route ya existe; PUT = del+add atómico en API nueva o
      endpoint dedicado).
- [ ] 9.3 NO: Buy/Transfer (externo), “Third Party” labels (no sabemos
      registrar: se omite).

## 10. Environment Variables

Captura 5. rkt hoy: inline por app.
- [ ] 10.1 Vista agrupada por app (todas las apps, buscador global,
      badge Secret siempre + mostrar/ocultar por fila con estado local,
      copiar). CRUD existe por app; esta página lo agrega.
- [ ] 10.2 Shared fase 2 (NO ahora): `shared_env` en DB + fusión en
      arranque supervisor. Spec aparte cuando se pida; la UI deja el
      tab “Shared” deshabilitado con tooltip, no roto.
- [ ] 10.3 NO: Environments Preview/Production (una sola flota),
      “Last Updated” por var (no se guarda; se omite, no se inventa).

## 11. Settings

Captura 7 (project settings con 20 secciones → recorte honesto).
- Team Settings (existe KV): agrupar con descripción + restart
  requerido donde aplique. Agregar `site_name` (reemplaza “▲ rkt”
  hardcode) y `quota_*` para Usage.
- App Settings (nuevo, `PUT /api/v1/apps/:name` operator):
  command/args/dir/restart_delay/min_uptime/memory_mb (validado por
  `ValidateApp`), deploy {repo, ref, build[], auto}. Danger zone:
  `DELETE /api/v1/apps/:name` (operator + confirm doble; borra app,
  routes huérfanas y para supervisor vía restart requerido).
- NO: Avatar upload, Passport, Sandboxes, Cron, Microfrontends,
  Drains, Tracing, Networking, Flags, Storage (fuera de misión;
  cada uno diría “no soportado” en vez de página vacía: no se crean).

## 12. CI (vista pipeline)

Sidebar Vercel lo trae; rkt ya hace CI (clone+build+swap).
- [ ] 12.1 Página CI = deployments en curso + últimos con estado y
      duración + link a build log (reusa deployments + filtro
      status=running). Sin backend nuevo fase 1.
- [ ] 12.2 Fase 2: pasos del build parseados del log (líneas `step:`)
      como checklist. Requiere que deploy loguee pasos con prefijo
      estable (cambio en deploy.go, atom aparte).

## 13. Fuera de alcance (decisión, no deuda)

Connect/Git import UI (CLI `apps add` + discovery cubren; browser
linking existe en hook), Integrations, Storage, Flags, Agent/AI,
CDN/Firewall propios (WAF y rate-limit existen como config, se
exponen solo lectura en Settings avanzada fase tardía), Teams/
billing/roles más allá de admin/operator/viewer.

## 14. Backend necesario (inventario)

Existe: auth, users, apps RO, acciones, env, routes, settings KV,
deployments list, deplog, logs tail, events, status.
Nuevo:
- [ ] B1 `triggered_by` + `env_snapshot` en deployments (migración v7;
      deploy.go los escribe; rollback los hereda).
- [ ] B2 `GET /api/v1/discovery` (events kind=discover recientes).
- [ ] B3 `GET /api/v1/usage?days=` (rollup access.log, caché 5min).
- [ ] B4 `GET /api/v1/analytics?app&days=` + `GET /api/v1/observability?hours=`
      (parser access.log dedicado `internal/weblog`, testeable puro).
- [ ] B5 `PUT /api/v1/apps/:name` + `DELETE /api/v1/apps/:name`
      (operator, doble confirm en UI).
- [ ] B6 `POST /api/v1/deployments/:id/retry` (mismo rev).
- [ ] B7 `GET /api/v1/domains/:host/tls` (manager: existe/expira).
- [ ] B8 `alert_rules` CRUD + evaluador en tick + events kind=alert.
- [ ] B9 `site_name`, `quota_*` en KnownSettings (con docs).

## 15. Orden de construcción (átomos)

1. B1 migración + escritura (deploy) — base de todo lo creíble.
2. B4 weblog + tests puros (sin UI).
3. B3 usage + overview Usage/Projects (2.1, 2.2).
4. App overview 3.x + B5 app settings + DELETE.
5. Deployments 4.x + B6 retry (+B2 tarjeta discovery).
6. Logs 5.x por fases.
7. Analytics 6.x + Observability 7.1.
8. Domains 9.x + B7 TLS.
9. Env agrupado 10.1 (+ flag Shared deshabilitado).
10. Settings 11.x (site_name, quotas).
11. CI 12.1 (solo vista).
12. Alerts B8 + UI 8.2.

Aceptación por átomo: endpoint con test + UI que lo usa + build
ambos + smoke + commit. Nada sin dueña (todo item tiene backend
o dice explícito qué reusa).
