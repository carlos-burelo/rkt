# Gate estricto pre-prod: lo máximo exigible antes de garantizar prod.
# Uso: powershell -NoProfile -ExecutionPolicy Bypass -File scripts/strict.ps1
# Siempre completo, sin incremental (el incremental vive en gate.ps1).
# Fases (fail-fast con tiempos): gofmt total, diff --check, vet, build,
# web íntegra, tests -count=1 -cover con umbrales máximos, race en TODOS
# los paquetes, Chromium + e2e browser, y gates de prod (versión, vulns).
# Requiere: Go, pnpm, gcc para -race (choco install mingw), red para
# Chromium/govulncheck (si no hay red, esas fases fallan en claro).
# Umbrales: la tabla de blindados es >= que ci.yml; el resto no baja de
# $floorRest. Si un umbral se vuelve imposible, se baja con motivo en el
# comentario (como logx/proxy=95) y test de bordes nuevo, nunca en silencio.
[CmdletBinding()]
param()
$ErrorActionPreference = "Stop"
trap { $ErrorActionPreference = "Continue"; break }
$root = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
Set-Location $root
$sw = [Diagnostics.Stopwatch]::StartNew()
function Step([string]$name, [scriptblock]$b) {
  $t = [Diagnostics.Stopwatch]::StartNew()
  Write-Host "`n== $name ==" -ForegroundColor Cyan
  & $b
  if ($LASTEXITCODE -ne 0) { throw "falló: $name" }
  $t.Stop()
  Write-Host ("ok {0:N1}s" -f $t.Elapsed.TotalSeconds) -ForegroundColor Green
}

Step "gofmt total" { $out = gofmt -l . 2>$null; if ($out) { throw "gofmt pendiente:`n$out" } }
Step "diff --check" { git diff --check; if ($LASTEXITCODE -ne 0) { throw "diff --check" } }
Step "web íntegra" {
  # Primero: go:embed exige dist con ficheros reales; vet/build/test
  # fallan en checkout limpio si el dashboard no se compiló antes.
  pnpm --dir web install --frozen-lockfile; if ($LASTEXITCODE -ne 0) { throw "pnpm install" }
  pnpm --dir web build; if ($LASTEXITCODE -ne 0) { throw "pnpm build" }
  pnpm --dir web exec tsc --noEmit; if ($LASTEXITCODE -ne 0) { throw "tsc" }
  pnpm --dir web test; if ($LASTEXITCODE -ne 0) { throw "vitest" }
  node scripts/i18n-check.mjs; if ($LASTEXITCODE -ne 0) { throw "i18n" }
  node scripts/fixtures-check.mjs; if ($LASTEXITCODE -ne 0) { throw "fixtures" }
}
Step "vet total" { go run ./tests/run.go vet ./...; if ($LASTEXITCODE -ne 0) { throw "vet" } }
Step "build -trimpath" {
  go build -trimpath -o bin/rkt.exe ./cmd/rkt
  if ($LASTEXITCODE -ne 0) { throw "build" }
}
Step "tests -count=1 -cover" {
  go run ./tests/run.go test ./... -count=1 -cover -timeout 20m > strict.log 2>&1
  if ($LASTEXITCODE -ne 0) { Get-Content strict.log; throw "go test" }
}
Step "cobertura máxima" {
  # Umbrales máximos sostenibles, medidos en baseline local
  # (strict-baseline.log): cada paquete no puede bajar de su suelo.
  # Blindados de CI se respetan (store=90, hosts=98, logx/proxy=95 por
  # ramas de error difíciles sin inyectar fallos del SO) y se suben donde
  # el baseline da margen (store=91, proxy=98).
  # TODO(cli-cover): cli=3 es casi sin tests conductuales del CLI (solo
  # sintaxis y estáticos); subir el suelo cuando aterricen tests de
  # comandos reales. Sin tests: solo internal/build (una var inyectada
  # por CI con -X, nada testeable). cmd/rkt y tests/ (guards) fuera de
  # tabla: entrypoint y guard de layout, sin lógica que cubrir.
  $min = @{
    accesslog = 100; acme = 93; alerts = 100; api = 56; app = 36
    appusage = 58; appview = 100; authz = 70; buildpack = 75
    cfgfile = 79; cli = 3; config = 83; cronjobs = 44
    dashboard = 100; deploy = 93; devca = 65; discover = 69
    dnscheck = 97; doctor = 62; domains = 97; dotenv = 100
    github = 53; health = 55; hook = 86; hosts = 98; hub = 100
    logx = 95; mcpserver = 46; netutil = 60; pg = 43
    platform = 50; procstat = 88; proxy = 98; ratelimit = 100
    server = 100; servermetrics = 80; settings = 83
    staticweb = 100; store = 91; supervisor = 83; waf = 85
    weblog = 100
  }
  # servermetrics=80 (no 81 medido en local): muestrea el sistema real y su
  # cobertura varía entre máquinas; el suelo deja 1 punto de aire.
  $allowNoTests = @("rkt/internal/build")
  $text = Get-Content strict.log -Raw
  $fail = @()
  $seen = @{}
  foreach ($m in [regex]::Matches($text, "(?m)^ok\s+(rkt/internal/\S+)\s+[\d.]+s\s+coverage: ([0-9.]+)%")) {
    $pkg = $m.Groups[1].Value
    $short = $pkg -replace "^rkt/internal/", ""
    $seen[$short] = $true
    if (-not $min.ContainsKey($short)) { $fail += "$short sin suelo en strict.ps1"; continue }
    if ([double]$m.Groups[2].Value -lt $min[$short]) { $fail += "${short}: $($m.Groups[2].Value)% (mín $($min[$short])%)" }
  }
  foreach ($short in $min.Keys) {
    if (-not $seen.ContainsKey($short)) { $fail += "$short sin línea de cobertura" }
  }
  foreach ($m in [regex]::Matches($text, "(?m)^\?\s+(rkt/internal/\S+)\s+\[no test files\]")) {
    if ($allowNoTests -notcontains $m.Groups[1].Value) { $fail += "$($m.Groups[1].Value) sin tests" }
  }
  if ($fail.Count -gt 0) {
    Write-Error "cobertura insuficiente: $($fail -join ', ')"
    exit 1
  }
}
Step "race total" {
  if (-not (Get-Command gcc -ErrorAction SilentlyContinue)) {
    throw "race exige gcc: choco install mingw (CI lo instala en el job strict)"
  }
  go run ./tests/run.go test -race ./... -count=1 -p 1 -timeout 30m
  if ($LASTEXITCODE -ne 0) { throw "race" }
}
Step "chromium" {
  go run github.com/mxschmitt/playwright-go/cmd/playwright@v0.6401.0 install chromium
  if ($LASTEXITCODE -ne 0) { throw "playwright install" }
}
Step "e2e browser" {
  go test -tags=e2e ./tests/e2e -count=1 -timeout 20m
  if ($LASTEXITCODE -ne 0) { throw "e2e" }
}
Step "gates prod" {
  $ver = (& .\bin\rkt.exe version).Trim()
  if ([string]::IsNullOrWhiteSpace($ver)) { throw "rkt.exe sin version" }
  Write-Host "rkt.exe version: $ver"
  if (Get-Command govulncheck -ErrorAction SilentlyContinue) {
    govulncheck ./...
    if ($LASTEXITCODE -ne 0) { throw "govulncheck" }
  } else {
    Write-Warning "govulncheck ausente: go install golang.org/x/vuln/cmd/govulncheck@latest"
  }
}
$sw.Stop()
Write-Host ("`nSTRICT VERDE en {0:N0}s" -f $sw.Elapsed.TotalSeconds) -ForegroundColor Green
