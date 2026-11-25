# site/ — landing + docs públicas

Astro estático. Fuente pública: `site/docs/*.md` (ruta `/docs/*`).
La spec interna de dev sigue en `/docs` del repo y no se publica aquí.

```powershell
pnpm install          # workspace raíz (web + site + shots)
pnpm --dir site dev   # http://localhost:4321
pnpm --dir site build # sale a site/dist/
```

No se embebe en `rkt.exe` (`web/dist` sí). Deploy: Pages/Cloudflare con `site/dist`.
