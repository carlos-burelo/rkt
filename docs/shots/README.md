# Capturas del dashboard

Las imágenes de `docs/shots/` se generan con Chromium headless (Playwright).
Hay dos modos:

## Datos ficticios (recomendado para documentación)

No requiere rkt ni una base de datos con datos reales. Usa `mock.mjs`, que
intercepta `/api/v1/**` con una flota de ejemplo (`checkout`, `api-core`,
`edge-preview`, etc.).

```powershell
pnpm --dir scripts/shots install
pnpm --dir web dev --port 5173 --host 127.0.0.1
pnpm --dir scripts/shots mock
```

## Entorno real

Requiere el servicio corriendo y un usuario con acceso.

```powershell
$env:RKT_URL = "http://127.0.0.1:8090"
$env:RKT_USER = "admin"
$env:RKT_PASS = "tu-password-largo"
pnpm --dir scripts/shots install
pnpm --dir scripts/shots shots
```

- Login por UI; la cookie queda en el contexto del navegador.
- Captura rutas fijas y las tabs de la primera app.
- `users` solo si el rol es admin.
- Salida: `docs/shots/<ruta>.png` (full page, 1600×900).
- El modo mock registra errores de consola y respuestas HTTP 4xx/5xx; úsalos
  como control de calidad. Una corrida limpia termina en «sin errores de
  consola».
