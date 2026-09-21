# LokroNet Windows-Installer - gehostet unter https://net.lokro.dev/install.ps1
# Quelle der Wahrheit in diesem Repo: deploy/install.ps1 (docs/install.ps1 ist die Kopie, sync per `make pages`)
#
# Weiterleitbarer Einzeiler (PowerShell, kein Admin noetig):
#   irm https://net.lokro.dev/install.ps1 | iex
# Mit Version:
#   & ([scriptblock]::Create((irm https://net.lokro.dev/install.ps1))) -Version 0.1.4
param(
  [string]$Version = "0.1.4",
  [string]$Base = "",
  [string]$InstallDir = "$env:LOCALAPPDATA\LokroNet"
)

$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"

if ([string]::IsNullOrWhiteSpace($Base)) {
  $Base = "https://github.com/Lokrogaming/lokronet/releases/download/v$Version"
}

$arch = $env:PROCESSOR_ARCHITECTURE
if ($arch -eq "AMD64") { $goarch = "amd64" }
elseif ($arch -eq "ARM64") { $goarch = "arm64" }
else { throw "Architektur $arch wird nicht unterstuetzt (nur AMD64/ARM64)" }

$zipName = "lokronet_${Version}_windows_${goarch}.zip"
$tmp = Join-Path $env:TEMP ("lokronet-" + [guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $tmp | Out-Null
try {
  Write-Output "[lokronet] lade $Base/$zipName ..."
  Invoke-WebRequest -Uri "$Base/$zipName" -OutFile (Join-Path $tmp $zipName)
  Write-Output "[lokronet] lade $Base/checksums.txt ..."
  Invoke-WebRequest -Uri "$Base/checksums.txt" -OutFile (Join-Path $tmp "checksums.txt")

  Write-Output "[lokronet] verifiziere sha256 ..."
  $line = (Select-String -LiteralPath (Join-Path $tmp "checksums.txt") -Pattern ([regex]::Escape($zipName)) | Select-Object -First 1).Line
  if (-not $line) { throw "$zipName fehlt in checksums.txt - Abbruch" }
  $expected = ($line -split '\s+')[0].ToLower()
  $actual = (Get-FileHash -Algorithm SHA256 -LiteralPath (Join-Path $tmp $zipName)).Hash.ToLower()
  if ($expected -ne $actual) { throw "Checksumme stimmt NICHT - Abbruch (evtl. manipulierter Download)" }

  New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
  Expand-Archive -LiteralPath (Join-Path $tmp $zipName) -DestinationPath $InstallDir -Force
  # Zip enthaelt lokronet_windows_<arch>.exe -> auf lokronet.exe normalisieren
  $dl = Get-ChildItem -LiteralPath $InstallDir -Filter "lokronet*.exe" | Select-Object -First 1
  if (-not $dl) { throw "kein lokronet.exe im Zip gefunden - Abbruch" }
  $exe = Join-Path $InstallDir "lokronet.exe"
  if ($dl.FullName -ne $exe) { Move-Item -LiteralPath $dl.FullName -Destination $exe -Force }
  Write-Output "[lokronet] installiert: $exe"

  # Windows-Firewall: eingehendes Mesh-UDP erlauben (best effort, braucht Admin)
  try {
    Get-NetFirewallRule -DisplayName "LokroNet Mesh (UDP)" -ErrorAction Stop | Out-Null
  } catch {
    try {
      New-NetFirewallRule -DisplayName "LokroNet Mesh (UDP)" -Direction Inbound -Protocol UDP -Action Allow -Program $exe -ErrorAction Stop | Out-Null
      Write-Output "[lokronet] Firewall-Regel angelegt"
    } catch {
      Write-Output "[lokronet] Hinweis: Firewall-Regel ging nicht (Admin?) - ggf. UDP manuell freigeben"
    }
  }

  # User-PATH (kein Admin noetig) + aktuelle Session
  $regPath = "HKCU:\Environment"
  $cur = (Get-ItemProperty -Path $regPath -Name Path -ErrorAction SilentlyContinue).Path
  if ($cur -notlike "*$InstallDir*") {
    Set-ItemProperty -Path $regPath -Name Path -Value "$cur;$InstallDir"
    Write-Output "[lokronet] PATH ergaenzt (neues Terminal oeffnen zum Uebernehmen)"
  }
  if ($env:Path -notlike "*$InstallDir*") { $env:Path += ";$InstallDir" }

  & $exe version
  Write-Output ""
  & $exe ips
}
finally {
  Remove-Item -LiteralPath $tmp -Recurse -Force -ErrorAction SilentlyContinue
}

Write-Output ""
Write-Output "[lokronet] fertig. Weiter mit (neues Terminal, falls PATH neu):"
Write-Output "  lokronet setup --rendezvous http://DEIN-SERVER:8787 --endpoint `"[DEINE-IP]:51820`""
