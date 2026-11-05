# DESIGN.md — dashboard rkt

Sistema visual del dashboard embebido. Superficie **Operate**: el operador
completa una tarea, no contempla una marca. La identidad vive en la precisión
de los detalles, no en decoración.

## Mundo

Terminal intermodal: tinta de puerto sobre grafito azulado. Rail de tinta
profunda, suelo acero frío, hairlines de 1px en vez de sombras, esquinas de
2px, micro-etiquetas monoespaciadas. Naranja-señal solo en la acción primaria
y la selección. Semáforo solo donde hay estado.

Firma: fila-manifiesto con tira de estado, bahía (puerto), destino
(dominio/TLS) y revisión; tablero de salidas con duraciones tabulares.

## Tokens

Definidos en `web/src/index.css` (`:root`). Tema único oscuro; no hay modo
claro ni multi-entorno.

| Token | Rol |
|---|---|
| `--background` / `--ink` | suelo / fondo profundo de rail y overlays |
| `--card` | panel de contenido |
| `--field` | fondo de inputs y buscadores |
| `--border` / `--input` | hairlines 1px / borde de control |
| `--foreground` / `--muted-foreground` | texto primario / secundario |
| `--signal` | naranja de acción primaria, selección y foco |
| `--ok` / `--warn` / `--bad` / `--info` | estado y severidad |
| `--paper` | contraste puntual sobre tinta |

Reglas: el naranja no se usa como decoración; el estado no se usa como acento.
Los filtros activos usan `--secondary` + subrayado de 1px `--signal`, nunca un
bloque naranja (el CTA debe seguir siendo el único bloque sólido).

## Tipografía

- **Inter Variable** para UI, títulos, botones y prosa.
- **JetBrains Mono Variable** para datos, códigos, cifras y micro-etiquetas.
- Escala fija: 24px h1, 14px h2 de sección, 13px dato principal de fila,
  11–12px columnas y valores, 10px micro-etiqueta, 9px folio.
- Cifras siempre `tabular-nums` (`.tnum`). Cabeceras con `text-wrap: balance`.
- Prosa de descripción limitada a ~78ch; tablas pueden ser densas.

## Layout

- Rail fijo de 15.5rem, colapsable a iconos; contenido con header sticky de
  3rem y `max-w-[1480px]`.
- Overview: métricas de flota a ancho completo, manifiesto de unidades +
  movimientos en columna principal, rail lateral de 330px con salidas, notas,
  entrante, medidor y totales.
- Responsive estructural: columnas de tabla se ocultan y se resumen en la
  celda de unidad; tabs de app con scroll horizontal; sidebar como sheet.
- Una sola página puede scrollear; los headers de tablas largas son sticky
  bajo el header global (`sticky top-12`).

## i18n

- Textos en `web/src/shared/i18n/messages/es/<dominio>.ts`, un archivo por
  dominio. El agregador es `messages/es.ts`.
- Componentes: `const { t, tn } = useI18n()`; helpers no-React: `import { t }`.
- `t(clave, vars)` interpola `{var}`; `tn(clave, n)` pluraliza con
  `Intl.PluralRules` (`.one` / `.other`).
- Español neutro: sin voseo ni regionalismos; "tú" o impersonal.
- Copy operativo: app, despliegue, caída, estado, configuración. Sin metáforas
  visibles ("varado", "salida", "manifiesto", "terminal"), sin claims de
  posicionamiento ("sin X inventados", "lo que no se mide…") y sin adjetivos
  de marketing. Estados y hechos medidos, no promesas.
- `node scripts/i18n-check.mjs` falla si una clave usada no existe o está
  duplicada. CI lo ejecuta junto a `tsc`.

## Componentes canónicos

- `PageHeader` (`shared/components/patterns.tsx`): título + regla de señal +
  descripción + meta/acciones.
- `Section`: bloque con cabecera hairline y descripción opcional.
- `Stat` / `StatGrid`: celda de libro, número tabular, ventana de medición
  honesta; no es una tarjeta con sombra.
- `Segmented`: filtros; activo subrayado, no relleno de acento.
- `Meter`: barra de 6px con `role="progressbar"` y valor textual.
- `StateRail` + `Dot`: estado por color **y forma** (sólida, discontinua,
  rota, tenue; cuadrado, diamante, círculo, hueco).
- `StatusBadge`: etiqueta textual del estado, obligatoria en móvil y en
  fichas; nunca depender solo del color.
- `ConfirmButton`: borrado en dos pasos (armar → confirmar, auto-desarme a
  4s). Toda acción irreversible de fila lo usa.
- `FormDialog`: único patrón de creación/edición; el formulario no vive en la
  página.

## Estados

| Estado | Color | Forma | Etiqueta |
|---|---|---|---|
| running / ok | `--ok` | cuadrado sólido, tira sólida | en marcha / correcto |
| degraded | `--warn` | diamante, tira discontinua | degradado |
| failed | `--bad` | círculo, tira rota | fallida |
| stopped | muted | cuadrado hueco, tira tenue | detenida |

Toda fila interactiva tiene hover, foco visible y activo. Los fallos se
ordenan primero y reciben un tinte de fondo sutil, no un borde grueso. El
Resumen abre con una banda accionable cuando hay unidades varadas o
degradadas.

## Movimiento

140–250ms, ease-out; solo estado (llegada de fila, pulso de running, blink de
señal). Sin coreografías de carga. `prefers-reduced-motion` desactiva todo.

## Accesibilidad

- Foco visible de 2px con `--ring`, offset 2.
- Filas de manifiesto y despliegues: `role="row"`, celdas, `tabIndex=0`,
  Enter/Espacio abre; acciones internas detienen la propagación.
- `Err` es `role="alert"`; los skeletons marcan `aria-busy`.
- Contraste de texto secundario ≥4.5:1 sobre el suelo; los tamaños de dato
  principal no bajan de 12–13px.
- El estado nunca se comunica solo por color: siempre hay etiqueta o forma.

## Evidencia

`docs/shots/` se genera con `scripts/shots/mock.mjs` (API ficticia, sin datos
reales) y Playwright. El mock cubre auth, flota, deploys, logs SSE, analytics,
ajustes y GitHub para que las capturas no muestren errores de consola.

## No hacer

- Clonar Vercel, Geist, iconos ajenos o sus assets.
- Tarjetas anidadas, sombras difusas, gradientes de texto.
- Métricas que rkt no mide (visitantes únicos, bounce, benchmarks).
- Acciones destructivas prominentes y sin confirmación.
