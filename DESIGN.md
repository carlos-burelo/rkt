# DESIGN.md — dashboard rkt

Sistema visual del dashboard embebido. Superficie **Operate**: el operador
completa una tarea, no contempla una marca. La identidad vive en la precisión
de los detalles, no en decoración.

## Mundo

Terminal intermodal: tinta de puerto sobre grafito azulado. Rail de tinta
profunda, suelo acero frío, hairlines de 1px en vez de sombras, esquinas de
2px, micro-etiquetas monoespaciadas. Naranja-señal solo en la acción primaria
y la selección. Semáforo solo donde hay estado. En claro: papel técnico,
grafito y la misma señal; no es una inversión mecánica de colores.

Firma: fila-manifiesto con tira de estado, bahía (puerto), destino
(dominio/TLS) y revisión; tablero de salidas con duraciones tabulares.

## Tokens

Definidos en `web/src/index.css` (`:root` oscuro, `.light` claro). Dos capas:
cada familia tiene un valor canónico y los alias (`--primary`, `--ring`,
`--sidebar-*`, `--chart-*`, `--destructive`) lo consumen; ningún alias repite
un literal. Tema `system`/`light`/`dark`, persistido en `localStorage`
(`rkt-theme`) y aplicado antes del primer paint. Toggle en el rail y en el
panel de acceso.

| Token | Rol |
|---|---|
| `--background` / `--ink` | suelo / fondo profundo de rail y overlays |
| `--card` | panel de contenido |
| `--field` | fondo de inputs y buscadores |
| `--border` / `--input` | hairlines 1px / borde de control |
| `--foreground` / `--muted-foreground` | texto primario / secundario |
| `--signal` | naranja de acción primaria, selección y foco |
| `--ok` / `--warn` / `--bad` / `--info` | estado y severidad |
| `--console` / `--console-raised` | placa de consola / cabecera y chips |
| `--console-border` | hairline interna de consola |
| `--console-foreground` / `--console-muted` | texto / metadatos en consola |
| `--console-signal` … `--console-info` | semáforo legible sobre consola |
| `--wash` / `--floor-top` / `--floor-bottom` | lavado y degradado del suelo |

Valores canónicos (`oklch(L C h)`):

| Familia | Oscuro | Claro |
|---|---|---|
| señal | `0.72 0.185 52` | `0.53 0.16 50` |
| ok | `0.74 0.12 165` | `0.50 0.13 165` |
| warn | `0.78 0.14 78` | `0.51 0.13 78` |
| bad | `0.66 0.2 25` | `0.53 0.2 25` |
| info | `0.72 0.08 230` | `0.50 0.1 230` |
| suelo | `0.155 0.012 245` | `0.982 0.004 95` |
| panel | `0.19 0.012 245` | `0.995 0.002 95` |
| tinta (rail/scrim) | `0.085 0.014 250` | `0.22 0.012 250` |
| consola | `0.125 0.012 250` | igual |

Reglas de contraste: texto y estados ≥4.5:1 sobre suelo, panel y sus tintes
al 10–15 %. En claro los estados son tinta (L 0.50–0.53); en oscuro son
luminosos. La consola es una placa de máquina idéntica en ambos temas: no
hereda `--ink` ni `--foreground`, y por eso el registro conserva la misma
legibilidad al alternar tema. Selección, caret, foco y scrollbars derivan de
los mismos tokens.

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
- Pintar consolas o bloques de log con `--ink` translúcido y texto heredado:
  en claro el resultado es gris medio con tinta ilegible. Usar `.console` y
  sus tokens.
- Duplicar el valor de `--signal` o del semáforo en alias, gráficos o
  selectores locales (`::selection`, `@keyframes`): siempre `var()` o
  `color-mix()` sobre el token.
