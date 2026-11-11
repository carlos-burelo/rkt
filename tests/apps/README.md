# Apps demo para pruebas reales (sin mocks)

Fixtures mínimas con comportamiento observable. Los tests las copian a un
temporal, las convierten en repo git (`init + commit`) y corren el pipeline
de verdad: **clone + build + canario + espera HTTP + corte + rollback**.

## demo-py

Python stdlib, sin dependencias (arranque ~1s).

- `app.py`: sirve en `127.0.0.1:$PORT` el contenido del archivo `VERSION`.
- `requirements.txt`: vacío (existe para que el buildpack detecte `python`).
- `VERSION`: `v1`. El test la cambia a `v2`, commitea y redespliega:
  el proxy debe pasar de servir `v1` a `v2` sin caídas.
- Build real usado por los tests: `python -m py_compile app.py`
  (falla con syntax error, pasa con código sano).

## demo-go

Go stdlib, sin dependencias externas.

- `main.go`: igual que `demo-py` pero en Go (lee `VERSION`, sirve `$PORT`).
- `go.mod`: módulo propio `demo-go` (anidado: el `go ./...` de la raíz
  lo ignora, igual que `tests/go`).
- `VERSION`: `v1`.
- Build real usado por los tests: `go build -trimpath -o app[.exe] .`.
- Arranque real: `./app` (`.\app.exe` en Windows), relativo al slot con
  separador para que el supervisor lo resuelva en el dir del canario
  (los slots `.a`/`.b` rotan con el corte; absoluto no sirve).
- El `Start` del plan Go usa la misma forma (`./app`); `discover` lo
  acepta sin exigirlo en PATH porque el binario lo genera el build.
- A propósito NO se usa `go run .` como arranque supervisado: genera un
  proceso nieto fuera del job del canario adoptado y el `Stop` se cuelga.

## demo-static

- `index.html`: marcador `demo-estatica-v1`.
- Sin proceso: sirve para `buildpack.DetectDir` real (`static`, `.`).
  El deploy de estáticos corre como `rkt __static <dir>`, no como canario
  de app; por eso aquí solo se prueba detección, no pipeline.

## Familia Node (detección real sobre archivos, sin red ni install)

| fixture | kind | arranque / salida |
|---|---|---|
| `demo-node-server` | `node` | `pnpm run start` (flujo real: `node --check` + `node server.js`) |
| `demo-react-vite` | `vite` | estático `dist` |
| `demo-next` | `next` | `pnpm exec next start -H 127.0.0.1` |
| `demo-sveltekit` | `sveltekit` | `node build` |
| `demo-nuxt` | `nuxt` | `node .output/server/index.mjs` |
| `demo-remix` | `remix` | `pnpm run start` |
| `demo-astro` | `astro` | estático `dist` |
| `demo-node-serve` | `node` | `pnpm exec serve dist` |
| `demo-node-static` | `static` | `.` (package.json sin scripts + index.html) |

Solo `demo-node-server` corre el pipeline (Node stdlib, sin
dependencias). Los frameworks con build (`next build`, `vite build`…)
requieren `pnpm install` con red y quedan en detección: el plan que el
test afirma es el mismo que `discover`/`apps add` consume.

## Con red (lo que revienta en prod)

- `demo-next` (+`app/layout.tsx`, `app/page.tsx`, `tsconfig.json`,
  typescript en devDeps): `pnpm install` + `pnpm run build` dentro del
  pipeline y canario del server compilado. El `tsconfig.json` y los
  `@types` van en el fixture para no depender del auto-setup de Next
  en mitad del build.
- `demo-react-vite` (+`pnpm-workspace.yaml` con `allowBuilds`):
  pnpm v10+ bloquea los postinstall (`ERR_PNPM_IGNORED_BUILDS`,
  esbuild) y `pnpm install` muere sin esa declaración; es la misma
  que una app real necesita. `pnpm install` + `pnpm run build` y dos
  versiones servidas con el preview del `dist` (v1→v2 con corte).
- Ambos arrancan con `node` directo sobre el bin del framework
  (`node_modules/next/dist/bin/next`, `node_modules/vite/bin/vite.js`):
  proceso único, supervisado limpio.

> Nota: los arranques con wrapper (`pnpm run/exec`, `go run`) generan
> un nieto antes de que el canario adoptado entre al job object; ese
> nieto sobrevive al `Stop` (retiene puerto y pipe) y cuelga
> `killCurrent`. Por eso los fixtures usan el binario/proceso final.
> Arreglar el supervisor (job desde `StartCanary`) queda pendiente.

## Reglas

- Nada de red en build/arranque (solo stdlib). Los tests deben pasar
  sin internet.
- Nada específico de OS en el código (el `PORT` llega por entorno).
- Los fixtures nunca se mutan: cada test copia a `t.TempDir()`.
