# Lo que corre el hook pre-push: toda la verificación vive en local, CI no
# la repite. Normal: gate -Full + cleanroom (HEAD compila desde cero).
# -Release (push de un tag v*): strict completo (race total, e2e, gates prod).
#
# Omitir a conciencia: RKT_SKIP_PREPUSH=1 git push   (o git push --no-verify)
param([switch]$Release)
$ErrorActionPreference = "Stop"
$sw = [Diagnostics.Stopwatch]::StartNew()
$here = Split-Path -Parent $MyInvocation.MyCommand.Path

# gcc recién instalado con winget aún no está en el PATH de procesos viejos
$env:Path = $env:Path + ";" + [Environment]::GetEnvironmentVariable("Path", "User")

if ($Release) {
  Write-Host "pre-push (release): strict.ps1" -ForegroundColor Cyan
  & powershell -NoProfile -ExecutionPolicy Bypass -File (Join-Path $here "strict.ps1")
  if ($LASTEXITCODE -ne 0) { Write-Error "strict falló: no se publica el tag"; exit 1 }
}
else {
  Write-Host "pre-push: gate -Full + cleanroom" -ForegroundColor Cyan
  & powershell -NoProfile -ExecutionPolicy Bypass -File (Join-Path $here "gate.ps1") -Full
  if ($LASTEXITCODE -ne 0) { Write-Error "gate -Full falló: push bloqueado"; exit 1 }
  & powershell -NoProfile -ExecutionPolicy Bypass -File (Join-Path $here "cleanroom.ps1")
  if ($LASTEXITCODE -ne 0) { Write-Error "cleanroom falló: push bloqueado"; exit 1 }
}
Write-Host ("`nPRE-PUSH VERDE en {0:N0}s" -f $sw.Elapsed.TotalSeconds) -ForegroundColor Green
