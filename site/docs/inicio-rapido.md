---
title: Inicio rápido
---

# Inicio rápido (desarrollo)

Requisitos: Windows, Go 1.26+, Node 22+ y pnpm.

```powershell
pnpm --dir web install
pnpm --dir web build
go build -o bin/rkt.exe ./cmd/rkt
bin\rkt.exe -config rkt.local.db dev
```

- Dashboard: `http://127.0.0.1:8090`
- HTTP/HTTPS: `127.0.0.1:80` y `127.0.0.1:443`
- Dominios locales: `*.localhost` (resuelven solos)
