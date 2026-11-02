# rkt-proxy

Reverse proxy en Go para VPS Windows Server. Asigna dominios/subdominios a puertos locales. TLS automático vía Let's Encrypt.

## Layout

```text
cmd/rkt-proxy/main.go       servicio Windows + servidores HTTP/HTTPS
internal/config/            carga/validación YAML + match Host
internal/proxy/             ReverseProxy (headers, 404/502)
configs/config.example.yaml ejemplo canónico
scripts/install-service.ps1 alta servicio (admin)
bin/                        salida build (gitignored)
```

## Cómo funciona

- Escucha `:80` (HTTP) y `:443` (HTTPS).
- `/.well-known/acme-challenge/` se sirve para Let's Encrypt.
- Resto de HTTP redirige a HTTPS (configurable).
- HTTPS usa certs auto (autocert, cache en disco).
- Routing por header `Host`: exacto primero, luego `*.dominio`.
- Sin ruta → `404`. Backend caído → `502`.
- Headers: `X-Forwarded-For` (auto), `X-Forwarded-Proto`, `X-Forwarded-Host`, `X-Real-IP`.
- WebSocket funciona (ReverseProxy estándar).

## Requisitos VPS

- Windows Server con Go instalado (solo para compilar; el `.exe` no necesita Go).
- Puertos 80/443 abiertos en firewall + grupo seguridad cloud.
- DNS A de cada dominio/subdominio → IP pública del VPS.
- Correr con privilegio para bindear 80/443 (admin o servicio).

## Compilar

```powershell
go mod tidy
go build -o bin/rkt-proxy.exe ./cmd/rkt-proxy
```

## Config

```powershell
Copy-Item configs/config.example.yaml config.yaml
# editar config.yaml: email + routes
.\bin\rkt-proxy.exe -config config.yaml run
```

Reglas: `hosts` canónico (sin dual `host`/`hosts`). Exacto gana a wildcard. `*.dominio` no cubre base `dominio`.

## Servicio Windows

```powershell
# consola admin:
.\bin\rkt-proxy.exe -config C:\ruta\config.yaml install
sc.exe start RktProxy
sc.exe query RktProxy
sc.exe stop RktProxy
.\bin\rkt-proxy.exe uninstall
```

O usa `scripts/install-service.ps1`. El servicio auto-arranca con el servidor.

## Tests

```powershell
go vet ./...
go test ./...
```
