<#
.SYNOPSIS
    Removes the pmmcpd logon task. Does not touch daemon state or binaries.

.DESCRIPTION
    Counterpart to Register-PmmcpdTask.ps1. Stops the task if running, then
    unregisters it. Files under %LOCALAPPDATA%\pmmcp\ are owned by
    `pmmcp install-service` / `pmmcp uninstall-service`; the state directory
    (database, logs, secrets) is never touched by either.
#>
[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'
$taskName = 'pmmcpd'

$task = Get-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue
if (-not $task) {
    Write-Host "Task '$taskName' is not registered; nothing to do."
    return
}

if ($task.State -eq 'Running') {
    Stop-ScheduledTask -TaskName $taskName
    Write-Host "Stopped running task '$taskName' (daemon shuts down gracefully)."
}

Unregister-ScheduledTask -TaskName $taskName -Confirm:$false
Write-Host "Unregistered '$taskName'."
Write-Host "To also remove the generated files: pmmcp uninstall-service"
