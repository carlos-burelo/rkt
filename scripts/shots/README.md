# Shots del dashboard (`docs/shots/`)

Captura todas las vistas con Chromium headless (Playwright usa el
cache de browsers local, sin descargas). Requiere el servicio corriendo.

```powershell
$env:RKT_URL = "http://127.0.0.1:8090"  # default
$env:RKT_USER = "admin"
$env:RKT_PASS = "tu-password-largo"
pnpm --dir scripts/shots install
pnpm --dir scripts/shots shots
```

- Login por API (`/api/v1/auth/login`), cookie en contexto.
- Rutas fijas + tabs de la primera app (si hay apps).
- `users` solo si el rol es admin.
- Imprime errores de consola por página (QA gratis).
- Salida: `docs/shots/<ruta>.png` (full page, 1600x900).
