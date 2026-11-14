#Requires -RunAsAdministrator
<#
.SYNOPSIS
  Borra TODOS los rastros de rkt en la máquina (servicio, datos, firewall, env, PATH).
.DESCRIPTION
  Complemento de install.ps1: lo deja todo como antes de la primera instalación
  para que el install posterior arranque limpio. Los datos de usuario que viven
  fuera del home (repos y datos de apps) NO se borran: se mueven a *.old dentro
  del home para revisarlos y borrarlos a mano. Uso:
    & ([scriptblock]::Create((irm https://raw.githubusercontent.com/carlos-burelo/rkt/main/scripts/nuke.ps1)))
  O con home personalizado:
    & ([scriptblock]::Create((irm https://raw.githubusercontent.com/carlos-burelo/rkt/main/scripts/nuke.ps1))) -InstallDir C:\rkt
#>
[CmdletBinding()]
param(
  [string]$InstallDir = "C:\Program Files\rkt-proxy"
)
$ErrorActionPreference = "Stop"

# 1. admin obligatorio (servicio + ACLs + env de máquina)
$isAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()
  ).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $isAdmin) { throw "Ejecuta como Administrador." }

# Test-RktNukeDir dice si InstallDir es borrable: nunca la raíz de una
# unidad ni carpetas del sistema. Aborta antes de tocar nada.
function Test-RktNukeDir([string]$Dir) {
  if ([string]::IsNullOrWhiteSpace($Dir)) { return $false }
  try { $full = [System.IO.Path]::GetFullPath($Dir) } catch { return $false }
  $root = [System.IO.Path]::GetPathRoot($full)
  if ($full.TrimEnd('\', '/') -ieq $root.TrimEnd('\', '/')) { return $false }
  foreach ($b in @($env:SystemRoot, "$env:SystemDrive\Windows",
      "$env:SystemDrive\Program Files", "$env:SystemDrive\Program Files (x86)",
      "$env:SystemDrive\Users")) {
    if ($b -and $full.TrimEnd('\', '/') -ieq $b.TrimEnd('\', '/')) { return $false }
  }
  return $true
}

$exe = Join-Path $InstallDir "rkt.exe"
$cfg = Join-Path $InstallDir "rkt.db"

if (-not (Test-RktNukeDir $InstallDir)) { throw "InstallDir no borrable (raíz o sistema): $InstallDir" }
$full = [System.IO.Path]::GetFullPath($InstallDir)
Write-Host "Borrando todo rastro de rkt en $full ..."
Stop-Service Rkt -Force -ErrorAction SilentlyContinue
# Espera al servicio: si Remove-Item corre antes de que muera, sus
# handles (logs, db) tumban el borrado con "Access denied".
for ($i = 0; $i -lt 30; $i++) {
  $svc = Get-Service Rkt -ErrorAction SilentlyContinue
  if (-not $svc -or $svc.Status -eq "Stopped") { break }
  Start-Sleep 1
}
if (Test-Path $exe) { try { & $exe -config $cfg uninstall } catch {} }
try { sc.exe delete Rkt | Out-Null } catch {}
# Restos del árbol (nieto huérfano fuera del job): solo binarios bajo el home.
try {
  Get-Process -ErrorAction SilentlyContinue | ForEach-Object {
    try {
      if ($_.Path -and $_.Path.StartsWith($full + "\", [StringComparison]::OrdinalIgnoreCase)) {
        $_ | Stop-Process -Force -ErrorAction SilentlyContinue
      }
    } catch {}
  } | Out-Null
} catch {}
foreach ($rule in @("RKT HTTP/HTTPS", "RKT Dashboard", "RKT Proxy HTTP/HTTPS")) {
  Remove-NetFirewallRule -DisplayName $rule -ErrorAction SilentlyContinue | Out-Null
}
[Environment]::SetEnvironmentVariable("RKT_GITHUB_TOKEN_FILE", $null, "Machine")
$mp = [Environment]::GetEnvironmentVariable("Path", "Machine")
if ($mp) {
  $kept = ($mp -split ";" | ForEach-Object { $_.Trim() } |
    Where-Object { $_ -ne "" -and $_.TrimEnd("\") -ine $full.TrimEnd("\") })
  [Environment]::SetEnvironmentVariable("Path", ($kept -join ";"), "Machine")
}
# ACLs previas pueden dejar a Admins sin DELETE: tomar posesión y heredar
# de Program Files antes de borrar. Sin esto, ni siquiera elevado limpia.
# (icacls y no takeown: el /D de takeown pide el "sí" localizado.)
try {
  & icacls $full /setowner "*S-1-5-32-544" /T /C 2>$null | Out-Null
  & icacls $full /reset /T /C 2>$null | Out-Null
} catch {}
# Borrado con reintentos: los handles del árbol recién matado tardan en
# soltarse en Windows; sin esto, "Access denied" aborta a mitad y deja
# residuos a medias.
$borrado = $false
for ($i = 0; $i -lt 30; $i++) {
  if (-not (Test-Path $full)) { $borrado = $true; break }
  Remove-Item $full -Recurse -Force -ErrorAction SilentlyContinue
  if (-not (Test-Path $full)) { $borrado = $true; break }
  Start-Sleep -Seconds 2
}
if (-not $borrado) { throw "No pude borrar $full (un proceso retiene archivos). Reinicia el servidor y reintenta." }
New-Item -ItemType Directory -Force $full | Out-Null
# Restos de layouts viejos (pre-hogar único): se mueven, no se borran.
# apps.old/certs.old quedan a un Remove-Item de desaparecer cuando
# confirmes que no hay nada que rescatar.
foreach ($legacy in @(
  @{old = "C:\apps"; nuevo = "apps.old"},
  @{old = "C:\ProgramData\rkt-proxy"; nuevo = "certs.old"}
)) {
  $src = $legacy.old
  $dst = Join-Path $full $legacy.nuevo
  if ($src -ne $full -and (Test-Path $src) -and -not (Test-Path $dst)) {
    try {
      Move-Item $src $dst -Force
      Write-Host "Legado movido: $src -> $dst (revísalo y bórralo cuando quieras)"
    } catch {
      Write-Warning "No pude mover ${src}: ${_}"
    }
  }
}
Write-Host "Limpieza total OK. El install posterior parte de cero."
