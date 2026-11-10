# Pruebas

Todas las fuentes de pruebas viven aquí:

- `go/`: pruebas Go por paquete. `run.go` las monta con `go -overlay` en los paquetes de `internal/`, así pueden probar también funciones no exportadas. `tests/go` es un módulo separado para que `go test ./...` no las compile como paquetes independientes.
- `web/`: pruebas unitarias Vitest del dashboard.
- `e2e/`: navegador real, API real y SQLite temporal con Playwright Go.

Desde la raíz:

```sh
go run ./tests/run.go test ./... -count=1
go run ./tests/run.go vet ./...
pnpm --dir web test
pnpm --dir web build
go run github.com/mxschmitt/playwright-go/cmd/playwright@v0.6401.0 install chromium
go test -tags=e2e ./tests/e2e -count=1
```

`go test ./...` compila el proyecto, pero para ejecutar las pruebas Go centralizadas se usa `tests/run.go`. El directorio `web/dist` debe existir antes de la suite de navegador. La URL histórica `github.com/playwright-community/playwright-go` redirige al repositorio cuyo `go.mod` declara `github.com/mxschmitt/playwright-go`; ese es el import usado aquí.
