<#
.SYNOPSIS
    Registers the pmmcpd per-user logon task (no elevation required).

.DESCRIPTION
    Expects `pmmcp install-service` to have been run first — it writes
    pmmcpd-start.bat (and its own copy of the task XML) under
    %LOCALAPPDATA%\pmmcp\. This script registers the checked-in template
    against that start script and optionally starts the daemon immediately.

.PARAMETER Start
    Also start the task (and therefore the daemon) right away.

.EXAMPLE
    ./Register-PmmcpdTask.ps1 -Start
#>
[CmdletBinding()]
param(
    [switch]$Start
)

$ErrorActionPreference = 'Stop'

$taskName = 'pmmcpd'
$startBat = Join-Path $env:LOCALAPPDATA 'pmmcp\pmmcpd-start.bat'
$xmlPath  = Join-Path $PSScriptRoot 'pmmcpd-logon-task.xml'

if (-not (Test-Path $startBat)) {
    Write-Error "Missing $startBat — run 'pmmcp install-service' first (it writes the start script this task points at)."
}

if (Get-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue) {
    Write-Host "Task '$taskName' already exists; replacing it."
    Unregister-ScheduledTask -TaskName $taskName -Confirm:$false
}

# Register from XML so the checked-in template stays the single source of truth.
schtasks.exe /Create /TN $taskName /XML $xmlPath | Out-Null
if ($LASTEXITCODE -ne 0) {
    Write-Error "schtasks /Create failed with exit code $LASTEXITCODE"
}
Write-Host "Registered logon task '$taskName' (runs $startBat at logon, LeastPrivilege)."

if ($Start) {
    Start-ScheduledTask -TaskName $taskName
    Write-Host "Started. Verify with: pmmcp doctor"
} else {
    Write-Host "Start now with:  Start-ScheduledTask -TaskName $taskName"
    Write-Host "Or wait for the next logon. Verify with: pmmcp doctor"
}
