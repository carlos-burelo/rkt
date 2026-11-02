#Requires -RunAsAdministrator
<#
.SYNOPSIS
  Instala rkt-proxy en Windows Server (prod): binario + config + firewall + servicio.
.DESCRIPTION
  Pensado para: irm <url> | iex. Descarga el .exe del último GitHub release,
  deja config.yaml si no existe, abre 80/443 y registra/arranca el servicio.
.EXAMPLE
  # Actualizar a último release (idempotente, con backup + rollback auto):
  irm https://raw.githubusercontent.com/carlos-burelo/rkt-proxy/main/scripts/install.ps1 | iex
.EXAMPLE
  # Pin de versión o rollback:
  $s = irm https://raw.githubusercontent.com/carlos-burelo/rkt-proxy/main/scripts/install.ps1
  & ([scriptblock]::Create($s)) -Version v0.1.0
  & ([scriptblock]::Create($s)) -Rollback
#>
[CmdletBinding()]
param(
  [string]$Repo = $env:RKT_REPO,
  [string]$Version = "latest",
  [string]$InstallDir = "C:\Program Files\rkt-proxy",
  [string]$LocalExe = "",
  [switch]$SkipFirewall,
  [switch]$Force,
  [switch]$Rollback
)
$ErrorActionPreference = "Stop"

if ([string]::IsNullOrWhiteSpace($Repo)) { $Repo = "carlos-burelo/rkt-proxy" }

# 1. admin obligatorio (bindear 80/443 + servicio)
$isAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()
  ).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $isAdmin) { throw "Ejecuta como Administrador." }

$exe = Join-Path $InstallDir "rkt-proxy.exe"
$cfg = Join-Path $InstallDir "config.yaml"
$bak = "$exe.bak"
$verFile = Join-Path $InstallDir "VERSION.txt"
New-Item -ItemType Directory -Force $InstallDir | Out-Null

# 0. rollback a versión anterior
if ($Rollback) {
  if (-not (Test-Path $bak)) { throw "Sin backup en $bak" }
  Stop-Service RktProxy -Force -ErrorAction SilentlyContinue
  Copy-Item $bak $exe -Force
  sc.exe start RktProxy
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
  $asset = "rkt-proxy-windows-amd64.exe"

  # 2b. idempotencia: si ya corre esa versión, nada que hacer (salvo -Force)
  $current = ""
  if (Test-Path $verFile) { $current = (Get-Content $verFile -Raw).Trim() }
  if ($current -eq $Version -and -not $Force -and $LocalExe -eq "") {
    Write-Host "Ya instalado $Version en $InstallDir. Nada que hacer."
    return
  }
  $tmp = Join-Path $env:TEMP "rkt-proxy-install.exe"
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

# 4. parar servicio previo para liberar el .exe + backup para rollback
$svc = Get-Service RktProxy -ErrorAction SilentlyContinue
if ($svc -and $svc.Status -eq "Running") {
  Write-Host "Parando servicio previo..."
  Stop-Service RktProxy -Force
}
if (Test-Path $exe) {
  Copy-Item $exe $bak -Force
  Write-Host "Backup en $bak"
}
Copy-Item $tmp $exe -Force
Write-Host "Binario en $exe"

# 5. config por defecto si no existe (editar email + routes tras instalar)
if (-not (Test-Path $cfg)) {
  @'
listen_http: ":80"
listen_https: ":443"
acme_email: "admin@TU-DOMINIO.com"
acme_staging: false
acme_cache_dir: "C:\\ProgramData\\rkt-proxy\\certs"
redirect_http_to_https: true
routes:
  - hosts: ["app.TU-DOMINIO.com"]
    target: "http://127.0.0.1:3000"
'@ | Out-File $cfg -Encoding utf8
  Write-Host "Config creada en $cfg — EDÍTALA (email + routes) y reinicia el servicio."
}

# 6. firewall 80/443
if (-not $SkipFirewall) {
  if (-not (Get-NetFirewallRule -DisplayName "RKT Proxy HTTP/HTTPS" -ErrorAction SilentlyContinue)) {
    New-NetFirewallRule -DisplayName "RKT Proxy HTTP/HTTPS" -Direction Inbound `
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
if (-not (Get-Service RktProxy -ErrorAction SilentlyContinue)) {
  & $exe -config $cfg install
}
sc.exe start RktProxy
Start-Sleep 3

# 8. health check: servicio arriba o rollback automático
$st = (Get-Service RktProxy).Status
if ($st -ne "Running") {
  Write-Warning "Servicio no arrancó tras update. Rollback..."
  if (Test-Path $bak) {
    Copy-Item $bak $exe -Force
    sc.exe start RktProxy
  }
  throw "Update fallido, rollback aplicado. Revisa config y logs."
}
$Version | Out-File $verFile -Encoding ascii -NoNewline
sc.exe query RktProxy

Write-Host ""
Write-Host "OK. Siguiente: edita $cfg y luego: sc.exe stop RktProxy; sc.exe start RktProxy"
Write-Host "Requiere: DNS A de tus dominios -> IP del VPS, puertos 80/443 públicos."
