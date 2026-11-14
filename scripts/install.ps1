#Requires -RunAsAdministrator
<#
.SYNOPSIS
  Instala rkt en Windows Server (prod): binario + SQLite + firewall + servicio.
.DESCRIPTION
  Pensado para: irm <url> | iex. Descarga el .exe del último GitHub release,
  abre 80/443 y registra/arranca el servicio. Todo vive bajo un solo home
  (<InstallDir>, por defecto C:\Program Files\rkt-proxy):
    rkt.exe, VERSION.txt      binario y versión
    data\rkt.db               SQLite (config + flota)
    data\backups\             copias diarias
    secrets\                  github.token, github-app.pem
    logs\                     rkt.log, access.log, control.token, apps\, deploys\
    apps\<name>\              repos, slots .a/.b y datos de cada app
    certs\                    cache ACME
  La config se edita por dashboard/CLI; no se edita YAML.
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
  [string]$AdminUser = "admin",
  [string]$PublicIP = ""
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
# Árbol del hogar: se crea idempotente (en reinstalación ya existe).
foreach ($sub in @("data", "secrets", "logs", "apps", "certs")) {
  New-Item -ItemType Directory -Force (Join-Path $InstallDir $sub) | Out-Null
}

# 0b. token deploy en archivo con ACL, nunca en env de máquina (legible
# por usuarios locales desde el registro). El servicio lee la ruta.
# La ACL la pone el pase único de 5b (herencia): el fichero nace cubierta.
if ($GithubToken -ne "") {
  $secretDir = Join-Path $InstallDir "secrets"
  New-Item -ItemType Directory -Force $secretDir | Out-Null
  $tokenFile = Join-Path $secretDir "github.token"
  Set-Content -Path $tokenFile -Value $GithubToken -NoNewline -Encoding ascii
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
    # /releases/latest excluye betas (este proyecto aún no tiene estables
    # recientes: latest = v0.6.6 con otro nombre de asset -> 404). Se toma
    # el publicado más nuevo, betas incluidas; -Version lo fija a mano.
    Write-Host "Resolviendo último release de $Repo (incluye betas)..."
    $rels = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases?per_page=100" `
      -Headers @{ Accept = "application/vnd.github+json" }
    $rel = @($rels | Where-Object { -not $_.draft } | Sort-Object created_at -Descending | Select-Object -First 1)
    if (-not $rel) { throw "Sin releases publicados en $Repo" }
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
  # Sin esto Windows bloquea el ejecutable descargado (Mark of the Web:
  # "Acceso denegado" al arrancarlo, peor con binario nuevo sin reputación).
  Unblock-File $tmp -ErrorAction SilentlyContinue

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
    } else {
      Write-Warning "Sin línea de checksum para $asset (se instala sin verificar)."
    }
  } catch {
    Write-Warning "Sin verificar checksum: $($_.Exception.Message)"
  }
}

Remove-NetFirewallRule -DisplayName "RKT Proxy HTTP/HTTPS" -ErrorAction SilentlyContinue | Out-Null

# Copy-ConReintentos copia con reintentos: un antivirus o un handle
# tardío pueden retener el binario recién descargado en Windows.
function Copy-ConReintentos([string]$origen, [string]$destino, [int]$intentos = 5) {
  for ($i = 0; $i -lt $intentos; $i++) {
    try {
      Copy-Item $origen $destino -Force -ErrorAction Stop
      return $true
    } catch {
      Start-Sleep -Seconds 2
    }
  }
  return $false
}

# 4. parar servicio previo para liberar el .exe + backup para rollback
$svc = Get-Service Rkt -ErrorAction SilentlyContinue
if ($svc -and $svc.Status -eq "Running") {
  Write-Host "Parando servicio previo..."
  Stop-Service Rkt -Force
}
if (Test-Path $exe) {
  if (Copy-ConReintentos $exe $bak) {
    Write-Host "Backup en $bak"
  } elseif (Test-Path $verFile) {
    throw "No pude respaldar $exe (¿rkt.exe en ejecución? revisa con: Get-Process rkt*). Sin backup no sigo en una actualización."
  } else {
    Write-Warning "Sin backup del binario previo (instalación fresca; sigo sin red de seguridad)."
  }
}
if (-not (Copy-ConReintentos $tmp $exe)) {
  throw "No pude instalar $exe (proceso reteniéndolo o ACL). Revisa con Get-Process rkt* y reintenta."
}
Unblock-File $exe -ErrorAction SilentlyContinue # la copia puede heredar la marca de zona
Write-Host "Binario en $exe"

# 5. la configuración vive en SQLite: el primer arranque siembra defaults.
#    Edítala desde el dashboard (loopback) o con `rkt settings/routes/apps`.

# 5b. ACL del hogar entero (datos, secretos, logs, apps, certs):
# solo SYSTEM + Administradores, con herencia. Un solo pase en la raíz
# cubre lo presente y lo futuro (antes eran dos pases por carpeta y los
# ficheros nuevos heredaban ACLs distintas).
# (OI)(CI) es imprescindible: sin herencia, rkt.db y -wal nacen sin acceso
# para el admin y `rkt settings` falla con "readonly database".
$adminSid = "*S-1-5-32-544"
icacls $InstallDir /inheritance:r /grant:r "*S-1-5-18:(OI)(CI)F" "${adminSid}:(OI)(CI)F" /T /C | Out-Null

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

function Test-PublicIPv4([string]$ip) {
  if ([string]::IsNullOrWhiteSpace($ip)) { return $false }
  return -not ($ip -match '^(10\.|127\.|169\.254\.|192\.168\.|100\.(6[4-9]|[7-9][0-9]|1[01][0-9]|12[0-7])\.|172\.(1[6-9]|2[0-9]|3[0-1])\.)')
}

$publicIp = $PublicIP
if ([string]::IsNullOrWhiteSpace($publicIp)) {
  try {
    $route = Get-NetRoute -DestinationPrefix "0.0.0.0/0" -ErrorAction SilentlyContinue |
      Sort-Object RouteMetric | Select-Object -First 1
    if ($route) {
      $candidate = Get-NetIPAddress -AddressFamily IPv4 -InterfaceIndex $route.ifIndex -ErrorAction SilentlyContinue |
        Where-Object { $_.AddressState -eq "Preferred" } | Select-Object -First 1 -ExpandProperty IPAddress
      if (Test-PublicIPv4 $candidate) { $publicIp = $candidate }
    }
  } catch {}
}
if ([string]::IsNullOrWhiteSpace($publicIp)) {
  try { $publicIp = (Invoke-RestMethod -Uri "https://api.ipify.org" -TimeoutSec 6).Trim() } catch {}
}
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
      # Verificar persistencia: si el ACL de la DB está mal, el CLI
      # imprime "readonly database" pero no lanza excepción.
      $after = ((& $exe -config $cfg settings list) -join "`n")
      if ($after -match "(?m)^dashboard_listen=0\.0\.0\.0:8090\s*$") {
        $dashboardExposed = $true
        $dashChanged = $true
        $settings["dashboard_listen"] = "0.0.0.0:8090"
      } else {
        Write-Warning "No pude persistir dashboard_listen. Revisa ACL de $InstallDir\data."
      }
    } catch { Write-Warning "No pude configurar dashboard_listen: $_" }
  } else {
    Write-Warning "Puerto 8090 ocupado por otro proceso; dashboard queda solo en loopback."
  }
}

# Si el setting ya existía pero el listener no está en wildcard, reiniciar.
if ($dashboardExposed) {
  $wild = Get-NetTCPConnection -LocalPort $dashPort -State Listen -ErrorAction SilentlyContinue |
    Where-Object { $_.LocalAddress -eq "0.0.0.0" -or $_.LocalAddress -eq "::" }
  if (-not $wild) { $dashChanged = $true }
}

if ($dashChanged) {
  Restart-Service Rkt -Force
  Start-Sleep 2
}

if ($dashboardExposed) {
  $wild = Get-NetTCPConnection -LocalPort $dashPort -State Listen -ErrorAction SilentlyContinue |
    Where-Object { $_.LocalAddress -eq "0.0.0.0" -or $_.LocalAddress -eq "::" }
  if (-not $wild) {
    $dashboardExposed = $false
    Write-Warning "El dashboard no quedó escuchando en 0.0.0.0:$dashPort. Revisa logs."
  }
}

if (-not $SkipFirewall -and $dashboardExposed) {
  Remove-NetFirewallRule -DisplayName "RKT Dashboard" -ErrorAction SilentlyContinue | Out-Null
  New-NetFirewallRule -DisplayName "RKT Dashboard" -Direction Inbound `
    -LocalPort $dashPort -Protocol TCP -Action Allow -Profile Any | Out-Null
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
