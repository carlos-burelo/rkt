#Requires -RunAsAdministrator
# Instala rkt-proxy como servicio Windows.
param(
  [string]$Dir = (Split-Path -Parent $MyInvocation.MyCommand.Path | Split-Path -Parent | Join-Path -ChildPath "" | Split-Path -Parent),
  [string]$Config = "config.yaml"
)
$ErrorActionPreference = "Stop"
$exe = Join-Path $Dir "bin\rkt-proxy.exe"
if (-not (Test-Path $exe)) { throw "falta $exe: compila primero con go build -o bin/rkt-proxy.exe ./cmd/rkt-proxy" }
& $exe -config (Join-Path $Dir $Config) install
sc.exe start RktProxy
sc.exe query RktProxy
