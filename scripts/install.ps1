#Requires -RunAsAdministrator
<#
.SYNOPSIS
  Instala rkt en Windows Server (prod): binario + SQLite + firewall + servicio.
.DESCRIPTION
  Pensado para: irm <url> | iex. Descarga el .exe del último GitHub release,
  abre 80/443 y registra/arranca el servicio. La config se siembra sola en
  SQLite (`<dir>/logs/rkt.db`) y se edita por dashboard/CLI.
.EXAMPLE
  # Actualizar a último release (idempotente, con backup + rollback auto):
  irm https://raw.githubusercontent.com/carlos-burelo/rkt/main/scripts/install.ps1 | iex
.EXAMPLE
  # Pin de versión, rollback o token deploy:
  $s = irm https://raw.githubusercontent.com/carlos-burelo/rkt/main/scripts/install.ps1
  & ([scriptblock]::Create($s)) -Version v0.1.0
  & ([scriptblock]::Create($s)) -Rollback
  & ([scriptblock]::Create($s)) -GithubToken "ghp_..."
#>
[CmdletBinding()]
param(
  [string]$Repo = $env:RKT_REPO,
  [string]$Version = "latest",
  [string]$InstallDir = "C:\Program Files\rkt-proxy",
  [string]$LocalExe = "",
  [switch]$SkipFirewall,
  [switch]$Force,
  [switch]$Rollback,
  [string]$GithubToken = "",
  [switch]$NoPublicDashboard,
  [string]$AdminUser = "admin"
)
$ErrorActionPreference = "Stop"

if ([string]::IsNullOrWhiteSpace($Repo)) { $Repo = "carlos-burelo/rkt" }

# 1. admin obligatorio (bindear 80/443 + servicio)
$isAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()
  ).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $isAdmin) { throw "Ejecuta como Administrador." }

$exe = Join-Path $InstallDir "rkt.exe"
$cfg = Join-Path $InstallDir "rkt.db"
$bak = "$exe.bak"
$verFile = Join-Path $InstallDir "VERSION.txt"
New-Item -ItemType Directory -Force $InstallDir | Out-Null

# 0b. token deploy en archivo con ACL, nunca en env de máquina (legible
# por usuarios locales desde el registro). El servicio lee la ruta.
if ($GithubToken -ne "") {
  $secretDir = Join-Path $InstallDir "secrets"
  New-Item -ItemType Directory -Force $secretDir | Out-Null
  $tokenFile = Join-Path $secretDir "github.token"
  Set-Content -Path $tokenFile -Value $GithubToken -NoNewline -Encoding ascii
  icacls $secretDir /inheritance:r /grant:r "SYSTEM:(F)" "Administradores:(F)" | Out-Null
  [Environment]::SetEnvironmentVariable("RKT_GITHUB_TOKEN_FILE", $tokenFile, "Machine")
  Write-Host "GITHUB_TOKEN guardado en $tokenFile (ACL SYSTEM+Admins)."
}

# 0. rollback a versión anterior
if ($Rollback) {
  if (-not (Test-Path $bak)) { throw "Sin backup en $bak" }
  Stop-Service Rkt -Force -ErrorAction SilentlyContinue
  Copy-Item $bak $exe -Force
  sc.exe start Rkt
  Write-Host "Rollback OK."
  return
}

# 2. binario: local (dev) o release GitHub (prod)
if ($LocalExe -ne "") {
  if (-not (Test-Path $LocalExe)) { throw "No existe $LocalExe" }
  Write-Host "Usando binario local $LocalExe"
  $tmp = $LocalExe
} else {
  if ($Version -eq "latest") {
    Write-Host "Resolviendo último release de $Repo..."
    $rel = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest" `
      -Headers @{ Accept = "application/vnd.github+json" }
    $Version = $rel.tag_name
  }
  Write-Host "Descargando $Repo $Version..."
  $asset = "rkt-windows-amd64.exe"

  # 2b. idempotencia: si ya corre esa versión, nada que hacer (salvo -Force)
  $current = ""
  if (Test-Path $verFile) { $current = (Get-Content $verFile -Raw).Trim() }
  if ($current -eq $Version -and -not $Force -and $LocalExe -eq "") {
    Write-Host "Ya instalado $Version en $InstallDir. Nada que hacer."
    return
  }
  $tmp = Join-Path $env:TEMP "rkt-install.exe"
  Invoke-WebRequest -Uri "https://github.com/$Repo/releases/download/$Version/$asset" `
    -OutFile $tmp -UseBasicParsing

  # 3. checksum si el release lo publica
  try {
    $sums = (Invoke-WebRequest -Uri "https://github.com/$Repo/releases/download/$Version/checksums.txt" `
      -UseBasicParsing).Content
    $line = ($sums -split "`n") | Where-Object { $_ -match [regex]::Escape($asset) } | Select-Object -First 1
    if ($line) {
      $expected = ($line -split "\s+")[0].Trim().ToUpper()
      $actual = (Get-FileHash -Path $tmp -Algorithm SHA256).Hash.ToUpper()
      if ($expected -ne $actual) { throw "Checksum mismatch en $asset" }
      Write-Host "Checksum OK."
    }
  } catch {
    Write-Warning "Sin verificar checksum: $($_.Exception.Message)"
  }
}

Remove-NetFirewallRule -DisplayName "RKT Proxy HTTP/HTTPS" -ErrorAction SilentlyContinue | Out-Null

# 4. parar servicio previo para liberar el .exe + backup para rollback
$svc = Get-Service Rkt -ErrorAction SilentlyContinue
if ($svc -and $svc.Status -eq "Running") {
  Write-Host "Parando servicio previo..."
  Stop-Service Rkt -Force
}
if (Test-Path $exe) {
  Copy-Item $exe $bak -Force
  Write-Host "Backup en $bak"
}
Copy-Item $tmp $exe -Force
Write-Host "Binario en $exe"

# 5. la configuración vive en SQLite: el primer arranque siembra defaults.
#    Edítala desde el dashboard (loopback) o con `rkt settings/routes/apps`.
New-Item -ItemType Directory -Force (Join-Path $InstallDir "logs") | Out-Null

# 5b. ACL de datos (DB + logs): solo SYSTEM + Administradores
icacls (Join-Path $InstallDir "logs") /inheritance:r /grant:r "SYSTEM:(F)" "Administradores:(F)" | Out-Null

# 6. firewall 80/443
if (-not $SkipFirewall) {
  if (-not (Get-NetFirewallRule -DisplayName "RKT HTTP/HTTPS" -ErrorAction SilentlyContinue)) {
    New-NetFirewallRule -DisplayName "RKT HTTP/HTTPS" -Direction Inbound `
      -LocalPort 80, 443 -Protocol TCP -Action Allow | Out-Null
    Write-Host "Firewall: 80/443 abiertos."
  }
}

# 6c. PATH máquina para usar rkt-proxy desde cualquier terminal
$machinePath = [Environment]::GetEnvironmentVariable("Path", "Machine")
$clean = $machinePath -split ";" | ForEach-Object { $_.Trim().TrimEnd("\") }
if ($clean -notcontains $InstallDir.TrimEnd("\")) {
  [Environment]::SetEnvironmentVariable("Path", "$machinePath;$InstallDir", "Machine")
  Add-Type -Namespace Win32 -Name Native -MemberDefinition '[DllImport("user32.dll", SetLastError=true, CharSet=CharSet.Auto)] public static extern IntPtr SendMessageTimeout(IntPtr hWnd, uint Msg, UIntPtr wParam, string lParam, uint fuFlags, uint uTimeout, out UIntPtr lpdwResult);' | Out-Null
  $res = [UIntPtr]::Zero
  [Win32.Native]::SendMessageTimeout([IntPtr]0xffff, 0x1a, [UIntPtr]::Zero, "Environment", 0x2, 5000, [ref]$res) | Out-Null
  Write-Host "PATH: $InstallDir agregado (reabre la terminal para usarlo)."
}

# 6d. puertos libres: IIS suele ocupar 80 en Windows Server
foreach ($p in 80, 443) {
  $conn = Get-NetTCPConnection -LocalPort $p -State Listen -ErrorAction SilentlyContinue | Select-Object -First 1
  if ($conn) {
    $proc = (Get-Process -Id $conn.OwningProcess -ErrorAction SilentlyContinue).Name
    throw "Puerto $p ocupado por PID $($conn.OwningProcess) ($proc). Si es IIS/W3SVC: sc.exe stop W3SVC; Set-Service W3SVC -StartupType Disabled. Luego re-corre el instalador."
  }
}

# 7. servicio: instala solo si falta (misma ruta, sin churn), luego arranca
if (-not (Get-Service Rkt -ErrorAction SilentlyContinue)) {
  & $exe -config $cfg install
}
sc.exe start Rkt
Start-Sleep 3

# 8. health check: servicio arriba o rollback automático
$st = (Get-Service Rkt).Status
if ($st -ne "Running") {
  Write-Warning "Servicio no arrancó tras update. Rollback..."
  if (Test-Path $bak) {
    Copy-Item $bak $exe -Force
    sc.exe start Rkt
  }
  throw "Update fallido, rollback aplicado. Revisa config y logs."
}
$Version | Out-File $verFile -Encoding ascii -NoNewline
sc.exe query Rkt

# 9. autoconfiguración: IP pública, dashboard externo, firewall y admin.
# El instalador no deja pasos manuales para entrar desde fuera.
function Rkt-CLI { param([string[]]$CliArgs) & $exe -config $cfg @CliArgs }

function Wait-RktHealth {
  for ($i = 0; $i -lt 40; $i++) {
    try {
      $r = Invoke-WebRequest -Uri "http://127.0.0.1:8090/api/v1/auth/status" -UseBasicParsing -TimeoutSec 3
      if ($r.StatusCode -eq 200) { return $true }
    } catch {}
    Start-Sleep 2
  }
  return $false
}

$settings = @{}
try {
  (& $exe -config $cfg settings list) | ForEach-Object {
    $kv = $_ -split "=", 2
    if ($kv.Count -eq 2) { $settings[$kv[0]] = $kv[1] }
  }
} catch {}

$publicIp = ""
try { $publicIp = (Invoke-RestMethod -Uri "https://api.ipify.org" -TimeoutSec 6).Trim() } catch {}
if ($publicIp -and [string]::IsNullOrWhiteSpace($settings["public_ip"])) {
  try { Rkt-CLI @("settings", "set", "public_ip=$publicIp") | Out-Null } catch {}
  $settings["public_ip"] = $publicIp
}

$adminCreated = $false
$adminPass = ""
$usersOut = ""
try { $usersOut = ((& $exe -config $cfg users list) -join "`n") } catch {}
if ($usersOut -match "sin usuarios") {
  $chars = (48..57) + (65..90) + (97..122)
  $adminPass = -join ($chars | Get-Random -Count 24 | ForEach-Object { [char]$_ })
  $adminPass | & $exe -config $cfg users add $AdminUser --role admin --password-stdin | Out-Null
  if ($LASTEXITCODE -eq 0) { $adminCreated = $true }
}

$dashboardExposed = $false
$dashChanged = $false
$dashPort = 8090
if ($NoPublicDashboard) {
  try {
    Rkt-CLI @("settings", "set", "dashboard_listen=off") | Out-Null
    Rkt-CLI @("settings", "set", "dashboard_host=off") | Out-Null
    $dashChanged = $true
  } catch {}
}
$existingListen = [string]$settings["dashboard_listen"]
if ($existingListen -match ":(\d+)$") { $dashPort = [int]$Matches[1] }
if (-not $NoPublicDashboard -and -not [string]::IsNullOrWhiteSpace($existingListen) -and $existingListen -ne "off") {
  $dashboardExposed = $true
}
if (-not $NoPublicDashboard -and [string]::IsNullOrWhiteSpace($existingListen)) {
  $busy = Get-NetTCPConnection -LocalPort 8090 -State Listen -ErrorAction SilentlyContinue | Select-Object -First 1
  $busyByRkt = $false
  if ($busy) {
    try { $busyByRkt = ((Get-Process -Id $busy.OwningProcess -ErrorAction Stop).ProcessName -eq "rkt") } catch {}
  }
  if (-not $busy -or $busyByRkt) {
    try {
      Rkt-CLI @("settings", "set", "dashboard_listen=0.0.0.0:8090") | Out-Null
      $dashboardExposed = $true
      $dashChanged = $true
    } catch { Write-Warning "No pude configurar dashboard_listen: $_" }
  } else {
    Write-Warning "Puerto 8090 ocupado por otro proceso; dashboard queda solo en loopback."
  }
}

if (-not $SkipFirewall -and $dashboardExposed) {
  if (-not (Get-NetFirewallRule -DisplayName "RKT Dashboard" -ErrorAction SilentlyContinue)) {
    New-NetFirewallRule -DisplayName "RKT Dashboard" -Direction Inbound `
      -LocalPort $dashPort -Protocol TCP -Action Allow | Out-Null
  }
}

if ($dashChanged) {
  Restart-Service Rkt -Force
  Start-Sleep 2
}
$ready = Wait-RktHealth
$dashLocal = "http://127.0.0.1:8090/"
$dashUrl = $dashLocal
if ($dashboardExposed -and $publicIp) { $dashUrl = "http://${publicIp}:${dashPort}/" }
if ($dashboardExposed) { Write-Host "Dashboard 8090 es HTTP. Para TLS: define base_domain y listen_https=:443." }

Write-Host ""
if ($ready) {
  Write-Host "Dashboard: $dashUrl"
  if ($dashboardExposed) { Write-Host "También local: $dashLocal" }
  if ($adminCreated) {
    Write-Host "Admin: $AdminUser"
    Write-Host "Password: $adminPass"
    Write-Host "Cambia la password tras entrar: rkt users passwd $AdminUser"
  } else {
    Write-Host "Usa el admin existente (o crea uno: rkt users add $AdminUser --role admin)."
  }
  try { Start-Process $dashUrl } catch { Write-Warning "No pude abrir el browser: abre $dashUrl a mano." }
} else {
  Write-Warning "Servicio arriba pero healthz no responde. Revisa logs y luego abre $dashLocal"
}
Write-Host "DNS de apps: crea A <app>.<tu-dominio> -> $publicIp y define base_domain en Ajustes."
