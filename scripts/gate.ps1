# Gate local rápido con los mismos comandos que CI (fail-fast con tiempos).
# Uso diario: .\scripts\gate.ps1                 (fmt + build + vet + tests afectados)
# Pre-push:    .\scripts\gate.ps1 -Full           (suite completa -count=1 como CI)
#              .\scripts\gate.ps1 -Full -E2E      (+ browser, necesita chromium)
# Compara el worktree contra HEAD; -Ref añade el rango <ref>...HEAD (p. ej. main).
[CmdletBinding()]
param(
  [switch]$Full,
  [switch]$E2E,
  [string]$Ref = ""
)
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
function ChangedFiles {
  $files = @()
  $files += git diff --name-only HEAD
  $files += git ls-files --others --exclude-standard
  if ($Ref -ne "") { $files += git diff --name-only "$Ref...HEAD" }
  $files | Where-Object { $_ -ne "" } | Sort-Object -Unique
}
function MapPackages([string[]]$files) {
  $pkgs = @{}
  $full = $false
  $web = $false
  foreach ($f in $files) {
    $p = $f -replace "/", "\"
    if ($p -match "^(internal|cmd)[\\/]") {
      $segs = $p -split "[\\/]"
      $pkgs["rkt/" + $segs[0] + "/" + $segs[1]] = $true
    } elseif ($p -match "^tests[\\/]go[\\/]([^\\/]+)") {
      $pkgs["rkt/internal/" + $Matches[1]] = $true
    } elseif ($p -match "^tests[\\/]e2e") {
      $pkgs["e2e"] = $true
    } elseif ($p -match "^(go\.mod|go\.sum|tests[\\/]run\.go)$") {
      $full = $true
    } elseif ($p -match "^scripts[\\/].*\.ps1$") {
      $pkgs["rkt/internal/cli"] = $true
    } elseif ($p -match "^(README|docs|PRODUCT|DESIGN)") {
      $pkgs["rkt/internal/cli"] = $true
    } elseif ($p -match "^web[\\/]") {
      $web = $true
    } elseif ($p -match "^\.github[\\/]") {
      Write-Host "(solo CI config, sin tests locales)"
    }
  }
  return @{ pkgs = @($pkgs.Keys); full = $full; web = $web }
}
function ReverseDeps([string[]]$roots) {
  $graph = @{}
  foreach ($line in (go list -f '{{.ImportPath}} {{join .Imports " "}}' ./...)) {
    $parts = $line -split " ", 2
    $graph[$parts[0]] = @($parts[1] -split " " | Where-Object { $_ -ne "" })
  }
  $seen = @{}
  $queue = @($roots)
  while ($queue.Count -gt 0) {
    $p = $queue[0]; $queue = @($queue | Select-Object -Skip 1)
    if ($seen.ContainsKey($p)) { continue }
    $seen[$p] = $true
    foreach ($kv in $graph.GetEnumerator()) {
      if ($kv.Value -contains $p -and -not $seen.ContainsKey($kv.Key)) { $queue += $kv.Key }
    }
  }
  return @($seen.Keys)
}

$files = ChangedFiles
if ($files.Count -eq 0 -and -not $Full) { Write-Host "sin cambios vs HEAD: solo fmt+build+vet"; }
$m = MapPackages $files
if ($m.full -and -not $Full) { Write-Host "cambio global (go.mod/harness): subo a suite completa"; $Full = $true }

Step "gofmt" { $out = gofmt -l internal, tests, cmd, scripts 2>$null; if ($out) { throw "gofmt pendiente:`n$out" } }
Step "web dist al día" {
  # Primero: go:embed exige dist con ficheros reales; go build falla en
  # checkout limpio si el dashboard no se compiló antes.
  $src = Get-ChildItem web/src -Recurse -File -ErrorAction SilentlyContinue | Sort-Object LastWriteTime -Descending | Select-Object -First 1
  $dist = Get-Item web/dist/index.html -ErrorAction SilentlyContinue
  if (-not $dist -or $src.LastWriteTime -gt $dist.LastWriteTime) {
    Write-Host "dist viejo o ausente: reconstruyo (8s aprox)"
    pnpm --dir web build
    if ($LASTEXITCODE -ne 0) { throw "pnpm build" }
  } else { Write-Host "dist al día" }
}
Step "build" {
  go build ./...
  if ($LASTEXITCODE -ne 0) { throw "build" }
}
if ($m.web -or $Full) {
  Step "web tsc+test+i18n+fixtures" {
    pnpm --dir web exec tsc --noEmit; if ($LASTEXITCODE -ne 0) { throw "tsc" }
    pnpm --dir web test; if ($LASTEXITCODE -ne 0) { throw "vitest" }
    node scripts/i18n-check.mjs; if ($LASTEXITCODE -ne 0) { throw "i18n" }
    node scripts/fixtures-check.mjs; if ($LASTEXITCODE -ne 0) { throw "fixtures" }
  }
}
if ($Full) {
  Step "vet full" { go run ./tests/run.go vet ./...; if ($LASTEXITCODE -ne 0) { return }; go vet -tags=e2e ./tests/e2e }
  Step "tests full -count=1" { go run ./tests/run.go test ./... -count=1 }
  if ($E2E) {
    Step "e2e browser" { go test -tags=e2e ./tests/e2e -count=1 }
  }
} else {
  $targets = @($m.pkgs | Where-Object { $_ -ne "e2e" })
  if ($targets.Count -gt 0) {
    $all = ReverseDeps $targets
    Write-Host ("paquetes: " + ($all -join ", "))
    Step "vet afectados" { go run ./tests/run.go vet $all }
    Step "test afectados (cache)" { go run ./tests/run.go test $all }
  } else {
    Step "vet full (sin mapa)" { go run ./tests/run.go vet ./... }
  }
  if ($m.pkgs -contains "e2e") { Write-Host "hay cambios en e2e: corre con -E2E para browser" -ForegroundColor Yellow }
}
$sw.Stop()
Write-Host ("`nGATE VERDE en {0:N0}s" -f $sw.Elapsed.TotalSeconds) -ForegroundColor Green
