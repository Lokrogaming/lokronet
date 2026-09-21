# Dev-Registrierung: legt das Repo-Verzeichnis an den ANFANG des User-PATH,
// damit `lokronet` überall den Dev-Build aufruft (statt .\lokronet).
// Idempotent, kein Admin nötig. Neues Terminal danach öffnen.
// Aufruf: powershell -ExecutionPolicy Bypass -File tools\dev-path.ps1
$ErrorActionPreference = "Stop"
$devDir = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$regPath = "HKCU:\Environment"
$parts = @(((Get-ItemProperty -Path $regPath -Name Path -ErrorAction SilentlyContinue).Path -split ";") | Where-Object { $_ -ne "" })
if ($parts -notcontains $devDir) {
  Set-ItemProperty -Path $regPath -Name Path -Value (($devDir + ";" + ($parts -join ";")).Trim(";"))
  Write-Output "registriert: $devDir (neues Terminal öffnen)"
} else {
  Write-Output "schon registriert: $devDir"
}
