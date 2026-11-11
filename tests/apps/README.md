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

## Reglas

- Nada de red en build/arranque (solo stdlib). Los tests deben pasar
  sin internet.
- Nada específico de OS en el código (el `PORT` llega por entorno).
- Los fixtures nunca se mutan: cada test copia a `t.TempDir()`.
