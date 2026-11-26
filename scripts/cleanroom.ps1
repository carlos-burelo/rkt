# Build desde checkout LIMPIO de HEAD: lo comiteado compila y arranca.
# Sustituye al job `build` de CI. Detecta lo que el árbol de trabajo
# esconde: archivos sin `git add`, lockfiles desfasados, go:embed sin dist.
#
# Uso: .\scripts\cleanroom.ps1
#
# Trabaja en un worktree temporal; no toca tu árbol ni tu node_modules.
$ErrorActionPreference = "Stop"
$sw = [Diagnostics.Stopwatch]::StartNew()
$root = (git rev-parse --show-toplevel).Trim()
$tmp = Join-Path ([IO.Path]::GetTempPath()) ("rkt-cleanroom-" + [guid]::NewGuid().ToString("N").Substring(0, 8))

if (git -C $root status --porcelain --untracked-files=no) {
  Write-Warning "hay cambios sin comitear: el cleanroom prueba HEAD, no tu árbol"
}

function Step([string]$name, [scriptblock]$b) {
  Write-Host "== cleanroom: $name ==" -ForegroundColor Cyan
  & $b
  if ($LASTEXITCODE -ne 0) { throw "cleanroom falló: $name" }
}

git -C $root worktree add --detach $tmp HEAD | Out-Null
try {
  Push-Location $tmp
  Step "pnpm install" { pnpm install --frozen-lockfile }
  Step "web build" { pnpm --dir web build }
  Step "go build" { go build -trimpath -o bin/rkt.exe ./cmd/rkt }
  Step "arranca (version)" {
    $v = (& .\bin\rkt.exe version).Trim()
    if ([string]::IsNullOrWhiteSpace($v)) { throw "rkt.exe sin version" }
    Write-Host "rkt.exe version: $v"
    $global:LASTEXITCODE = 0
  }
}
finally {
  Pop-Location
  git -C $root worktree remove --force $tmp 2>$null
  if (Test-Path $tmp) { Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue }
}
$sw.Stop()
Write-Host ("`nCLEANROOM VERDE en {0:N0}s" -f $sw.Elapsed.TotalSeconds) -ForegroundColor Green
