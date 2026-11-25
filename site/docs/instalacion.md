---
title: Instalación
---

# Instalación en producción

Requisitos: registro DNS `A`, puertos `80/443` abiertos y PowerShell como administrador.

```powershell
irm https://rkt.twwk.cc/install.ps1 -OutFile $env:TEMP\rkt-install.ps1; & $env:TEMP\rkt-install.ps1
```

Actualizar: `rkt upgrade`. Todo vive bajo un solo home (`C:\rkt` por defecto).
