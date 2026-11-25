#Requires -RunAsAdministrator
<#
.SYNOPSIS
  Instala rkt en Windows Server (prod): binario + SQLite + firewall + servicio.
.DESCRIPTION
  Pensado para: irm <url> | iex. Descarga el .exe del último GitHub release,
  abre 80/443 y registra/arranca el servicio. Todo vive bajo un solo home
  (<InstallDir>, por defecto C:\rkt: sin espacios y fuera de carpetas
  protegidas cuya herencia bloquea DELETE incluso a Admins).
  Layout canónico en
  internal/logx; este script lo espeja, no lo define:
    rkt.exe                   lanzador = copia del slot activo (para el PATH)
    VERSION.txt, current.txt  versión activa (previous.txt tras un upgrade)
    versions\<ver>\rkt.exe    slots inmutables: el servicio corre desde aquí,
                              nunca se escribe un exe en uso
    data\rkt.db               SQLite (config + flota)
    data\backups\             copias diarias
    secrets\                  github.token, github-app.pem
    logs\                     rkt.log, access.log, control.token, apps\, deploys\
    apps\<name>\              repos, slots .a/.b y datos de cada app
    certs\                    cache ACME
  La config se edita por dashboard/CLI; no se edita YAML.
.EXAMPLE
  # Actualizar a último release (idempotente, por slots + rollback auto):
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
  [string]$InstallDir = "C:\rkt",
  [string]$LocalExe = "",
  [switch]$SkipFirewall,
  [switch]$Force,
  [switch]$Rollback,
  [string]$GithubToken = "",
  [switch]$NoPublicDashboard,
  [string]$AdminUser = "admin",
  [string]$PublicIP = ""
)
$__rktEAP = $ErrorActionPreference
$ErrorActionPreference = "Stop"
# No fugar Stop a la sesión que hace irm|iex: trap restaura y propaga.
trap { $ErrorActionPreference = $__rktEAP; break }

if ([string]::IsNullOrWhiteSpace($Repo)) { $Repo = "carlos-burelo/rkt" }

# 1. admin obligatorio (bindear 80/443 + servicio)
$isAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()
  ).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $isAdmin) { throw "Ejecuta como Administrador." }

# 1b. home viejo: el defecto anterior era Program Files\rkt (con espacio y
# con herencia que bloquea DELETE incluso a Admins). Si no se pasó
# -InstallDir explícito y el home nuevo está vacío pero el viejo tiene
# instalación (current.txt/VERSION.txt), se conserva el viejo: instalar al
# costado duplicaría el servicio y pelearía 80/443.
$oldDefault = Join-Path $env:SystemDrive "Program Files\rkt"
if (-not $PSBoundParameters.ContainsKey("InstallDir") -and $InstallDir -eq "C:\rkt") {
  $freshHome = -not ((Test-Path (Join-Path $InstallDir "current.txt")) -or (Test-Path (Join-Path $InstallDir "VERSION.txt")))
  $legacyHome = (Test-Path (Join-Path $oldDefault "current.txt")) -or (Test-Path (Join-Path $oldDefault "VERSION.txt"))
  if ($freshHome -and $legacyHome) {
    $InstallDir = $oldDefault
    Write-Warning "Instalación previa en ${oldDefault}: se conserva ese home. Para mudarlo, pasa -InstallDir explícito."
  }
}

$launcher = Join-Path $InstallDir "rkt.exe"
$cfg = Join-Path $InstallDir "rkt.db"
$verFile = Join-Path $InstallDir "VERSION.txt"
$currentFile = Join-Path $InstallDir "current.txt"
$previousFile = Join-Path $InstallDir "previous.txt"
# Árbol del hogar: se crea idempotente (en reinstalación ya existe).
foreach ($sub in @("data", "secrets", "logs", "apps", "certs")) {
  New-Item -ItemType Directory -Force (Join-Path $InstallDir $sub) | Out-Null
}

# 0b. token deploy en archivo bajo secrets/, nunca en env de máquina (legible
# por usuarios locales desde el registro). El servicio lee la ruta.
if ($GithubToken -ne "") {
  $secretDir = Join-Path $InstallDir "secrets"
  New-Item -ItemType Directory -Force $secretDir | Out-Null
  $tokenFile = Join-Path $secretDir "github.token"
  Set-Content -Path $tokenFile -Value $GithubToken -NoNewline -Encoding ascii
  [Environment]::SetEnvironmentVariable("RKT_GITHUB_TOKEN_FILE", $tokenFile, "Machine")
  Write-Host "GITHUB_TOKEN guardado en $tokenFile."
}

# 0. rollback al slot previo (previous.txt): flip de binpath + restart.
# Sin reintentos de copia: el slot viejo sigue intacto.
if ($Rollback) {
  $prev = ""
  if (Test-Path $previousFile) { $prev = (Get-Content $previousFile -Raw).Trim() }
  if ([string]::IsNullOrWhiteSpace($prev)) { throw "Sin previous.txt, nada a lo que volver" }
  $slotExe = Join-Path $InstallDir "versions\$prev\rkt.exe"
  if (-not (Test-Path $slotExe)) { throw "Falta slot $prev" }
  Stop-Service Rkt -Force -ErrorAction SilentlyContinue
  & sc.exe config Rkt binPath= "`"$slotExe`" -config `"$cfg`""
  if (Copy-ConReintentos $slotExe $launcher 5) { Unblock-File $launcher -ErrorAction SilentlyContinue }
  sc.exe start Rkt
  Start-Sleep 3
  $prev | Out-File $verFile -Encoding ascii -NoNewline
  $prev | Out-File $currentFile -Encoding ascii -NoNewline
  Write-Host "Rollback a $prev OK."
  $ErrorActionPreference = $__rktEAP
  return
}

# 2. binario: local (dev) o release GitHub (prod)
if ($LocalExe -ne "") {
  if (-not (Test-Path $LocalExe)) { throw "No existe $LocalExe" }
  Write-Host "Usando binario local $LocalExe"
  try { $Version = (& $LocalExe version).Trim() } catch { $Version = "" }
  if ([string]::IsNullOrWhiteSpace($Version)) { $Version = "dev-local" }
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
  if (Test-Path $currentFile) { $current = (Get-Content $currentFile -Raw).Trim() }
  elseif (Test-Path $verFile) { $current = (Get-Content $verFile -Raw).Trim() }
  if ($current -eq $Version -and -not $Force -and $LocalExe -eq "") {
    Write-Host "Ya instalado $Version en $InstallDir. Nada que hacer."
    $ErrorActionPreference = $__rktEAP
    return
  }
  $tmp = Join-Path $env:TEMP "rkt-install.exe"
  Invoke-WebRequest -Uri "https://github.com/$Repo/releases/download/$Version/$asset" `
    -OutFile $tmp -UseBasicParsing
  # Sin esto Windows bloquea el ejecutable descargado (Mark of the Web:
  # "Acceso denegado" al arrancarlo, peor con binario nuevo sin reputación).
  Unblock-File $tmp -ErrorAction SilentlyContinue

  # 3. checksum si el release lo publica. Se baja a fichero y se lee como
  # texto: el .Content crudo de Invoke-WebRequest puede venir en bytes
  # (octet-stream) y entonces el match nunca acierta (verificación muda).
  try {
    $sumFile = Join-Path $env:TEMP "rkt-checksums.txt"
    Invoke-WebRequest -Uri "https://github.com/$Repo/releases/download/$Version/checksums.txt" `
      -OutFile $sumFile -UseBasicParsing
    $sums = (Get-Content $sumFile -Raw).ToString()
    $line = ($sums -split "`r?`n") | Where-Object { $_ -match [regex]::Escape($asset) } | Select-Object -First 1
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

# 2c. slot versionado: el servicio corre desde versions\<ver>\rkt.exe
# (inmutable; nunca se escribe un exe en uso). $exe apunta al slot.
$slotDir = Join-Path $InstallDir "versions\$Version"
$exe = Join-Path $slotDir "rkt.exe"
New-Item -ItemType Directory -Force $slotDir | Out-Null

Remove-NetFirewallRule -DisplayName "RKT Proxy HTTP/HTTPS" -ErrorAction SilentlyContinue | Out-Null

# Copy-ConReintentos copia con reintentos: un antivirus o un handle
# tardío pueden retener el binario recién descargado en Windows.
# Guarda el último error y lo avisa: sin esto el throw posterior
# no dice si fue lock, ACL o Defender.
function Copy-ConReintentos([string]$origen, [string]$destino, [int]$intentos = 5) {
  $ultimo = $null
  for ($i = 0; $i -lt $intentos; $i++) {
    try {
      Copy-Item $origen $destino -Force -ErrorAction Stop
      return $true
    } catch {
      $ultimo = $_
      Start-Sleep -Seconds 2
    }
  }
  if ($ultimo) {
    Write-Warning ("Copy-ConReintentos falló ${origen} -> ${destino}: $($ultimo.Exception.Message)")
  }
  return $false
}

# 4. parar servicio previo. Sin backup del exe en uso: el slot nuevo se
# publica al costado y el flip es solo binpath (sección 7).
$svc = Get-Service Rkt -ErrorAction SilentlyContinue
if ($svc -and $svc.Status -ne "Stopped") {
  Write-Host "Parando servicio previo..."
  Stop-Service Rkt -Force -ErrorAction SilentlyContinue
  # Espera al servicio: sin esto el start posterior corre con el
  # proceso viejo aún en Stopping y el flip parece no surtir efecto.
  for ($i = 0; $i -lt 30; $i++) {
    $s = Get-Service Rkt -ErrorAction SilentlyContinue
    if (-not $s -or $s.Status -eq "Stopped") { break }
    Start-Sleep 1
  }
  # Resto bajo el home (handle tardío): solo binarios del home.
  try {
    Get-Process -ErrorAction SilentlyContinue | ForEach-Object {
      try {
        if ($_.Path -and $_.Path.StartsWith($InstallDir.TrimEnd("\") + "\", [StringComparison]::OrdinalIgnoreCase)) {
          $_ | Stop-Process -Force -ErrorAction SilentlyContinue
        }
      } catch {}
    } | Out-Null
  } catch {}
}
# 4b. publicar slot (fichero fresco, nunca bloqueado) + verificar versión.
if (-not (Copy-ConReintentos $tmp $exe 5)) {
  throw "No pude publicar $exe. Revisa con Get-Process rkt* y reintenta."
}
Unblock-File $exe -ErrorAction SilentlyContinue
$got = ""
try { $got = (& $exe version).Trim() } catch { $got = "" }
if ($LocalExe -eq "" -and $got -ne $Version) {
  throw "El slot no reporta $Version (dio '$got')."
}
Write-Host "Binario en $exe"

# 5. la configuración vive en SQLite: el primer arranque siembra defaults.
#    Edítala desde el dashboard (loopback) o con `rkt settings/routes/apps`.
#    Todo el proyecto usa un solo directorio ($InstallDir) para todo,
#    con permisos estándar de sistema de archivos sin permisos especiales
#    que bloqueen su modificación/eliminación o cuentas de usuario.

# 6. firewall 80/443
if (-not $SkipFirewall) {
  if (-not (Get-NetFirewallRule -DisplayName "RKT HTTP/HTTPS" -ErrorAction SilentlyContinue)) {
    New-NetFirewallRule -DisplayName "RKT HTTP/HTTPS" -Direction Inbound `
      -LocalPort 80, 443 -Protocol TCP -Action Allow | Out-Null
    Write-Host "Firewall: 80/443 abiertos."
  }
}

# 6c. PATH máquina para usar rkt desde cualquier terminal
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

# 7. servicio: se registra desde el slot (binpath = slot, nunca el lanzador).
# En update, flip de binpath al slot nuevo (el viejo queda para rollback).
$fresh = -not (Get-Service Rkt -ErrorAction SilentlyContinue)
$oldVer = ""
if (Test-Path $currentFile) { $oldVer = (Get-Content $currentFile -Raw).Trim() }
if ($fresh) {
  & $exe -config $cfg install
} else {
  if ($oldVer -ne "" -and $oldVer -ne $Version) {
    $oldVer | Out-File $previousFile -Encoding ascii -NoNewline
    Write-Host "Flip $($oldVer) -> $($Version)"
  }
  & sc.exe config Rkt binPath= "`"$exe`" -config `"$cfg`""
}
sc.exe start Rkt
Start-Sleep 3

# 8. health gate: servicio arriba y respondiendo, o flip atrás automático.
$st = (Get-Service Rkt).Status
$ready = $false
if ($st -eq "Running") { $ready = Wait-RktHealth }
if (-not $ready) {
  Write-Warning "Servicio no sano tras update. Rollback..."
  try {
    $det = Get-MpThreatDetection -ErrorAction SilentlyContinue |
      Where-Object { ($_.Resources -join " ") -match [regex]::Escape($InstallDir) } |
      Select-Object -First 1
    if ($det) {
      Write-Warning "Defender marca algo bajo $InstallDir (heurístico habitual en exe sin firmar). Revisa con Get-MpThreatDetection; exclusión: Add-MpPreference -ExclusionPath '$InstallDir'."
    }
  } catch {}
  $prev = ""
  if (Test-Path $previousFile) { $prev = (Get-Content $previousFile -Raw).Trim() }
  $prevExe = Join-Path $InstallDir "versions\$prev\rkt.exe"
  if ($prev -ne "" -and (Test-Path $prevExe)) {
    & sc.exe config Rkt binPath= "`"$prevExe`" -config `"$cfg`""
    sc.exe start Rkt
    Start-Sleep 3
    if ((Get-Service Rkt).Status -eq "Running" -and (Wait-RktHealth)) {
      $prev | Out-File $verFile -Encoding ascii -NoNewline
      $prev | Out-File $currentFile -Encoding ascii -NoNewline
      if (Copy-ConReintentos $prevExe $launcher 5) { Unblock-File $launcher -ErrorAction SilentlyContinue }
      throw "Update fallido, rollback a $prev aplicado. Revisa config y logs."
    }
  }
  throw "Update fallido y rollback imposible. Revisa config y logs."
}
$Version | Out-File $verFile -Encoding ascii -NoNewline
$Version | Out-File $currentFile -Encoding ascii -NoNewline
# Lanzador (PATH): copia del slot activo. No corre como servicio, se pisa libre.
if (Copy-ConReintentos $exe $launcher 5) { Unblock-File $launcher -ErrorAction SilentlyContinue }
else { Write-Warning "No pude refrescar el lanzador $launcher (¿CLI en uso? se actualiza en la próxima corrida)." }
# Recovery: reinicia solo ante crash (3 intentos), sin loop infinito.
& sc.exe failure Rkt reset= 86400 actions= restart/60000/restart/60000// 2>$null
if ($LASTEXITCODE -ne 0) { Write-Warning "No pude programar recovery del servicio." }
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

# 10. doctor: lo que falta se avisa aquí, no en el wizard. Solo fallos y
# avisos (git bloquea detección y deploys); el parte ok no hace ruido.
# El --json del doctor mezcla texto humano: se toma la línea del array.
try {
  $docLines = & $exe -config $cfg doctor --json 2>$null
  $docJson = @($docLines | Where-Object { $_ -match '^\s*\[' }) | Select-Object -First 1
  if ($docJson) {
    $bad = @($docJson | ConvertFrom-Json | Where-Object { $_.state -ne "ok" })
    foreach ($b in $bad) {
      $mark = if ($b.state -eq "fail") { "FALTA" } else { "AVISO" }
      $hint = if ($b.hint) { " $($b.hint)" } else { "" }
      Write-Warning "doctor [$mark] $($b.name): $($b.detail).$hint"
    }
  } elseif ($null -eq (Get-Command git -ErrorAction SilentlyContinue)) {
    Write-Warning "doctor [FALTA] git: no instalado. instala git (https://git-scm.com): sin git no hay deploys"
  }
} catch {
  if ($null -eq (Get-Command git -ErrorAction SilentlyContinue)) {
    Write-Warning "doctor [FALTA] git: no instalado. instala git (https://git-scm.com): sin git no hay deploys"
  }
}
Write-Host "DNS de apps: crea A <app>.<tu-dominio> -> $publicIp y define base_domain en Ajustes."
$ErrorActionPreference = $__rktEAP
