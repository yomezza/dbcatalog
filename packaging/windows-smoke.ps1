param(
    [Parameter(Mandatory=$true)][string]$OldInstaller,
    [Parameter(Mandatory=$true)][string]$NewInstaller,
    [Parameter(Mandatory=$true)][string]$ExpectedExe,
    [string]$ReportDirectory = (Join-Path $PSScriptRoot '..\build\windows-smoke')
)
$ErrorActionPreference = 'Stop'
if ($env:OS -ne 'Windows_NT') { throw 'This check requires native Windows.' }
$AppKey = '{77CC22CF-5766-4A80-A049-853E7A9B22E0}_is1'
foreach ($Root in @('HKLM:\Software\Microsoft\Windows\CurrentVersion\Uninstall', 'HKLM:\Software\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall', 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall')) {
    if (Test-Path (Join-Path $Root $AppKey)) { throw 'CatalogDB is already installed. Run this check in a clean Windows VM.' }
}
$OldInstaller = (Resolve-Path -LiteralPath $OldInstaller).Path
$NewInstaller = (Resolve-Path -LiteralPath $NewInstaller).Path
$ExpectedHash = (Get-FileHash -LiteralPath $ExpectedExe -Algorithm SHA256).Hash
$ReportDirectory = [IO.Path]::GetFullPath($ReportDirectory)
New-Item -ItemType Directory -Force $ReportDirectory | Out-Null
$TaskRoot = Join-Path ([IO.Path]::GetTempPath()) ('CatalogDB-smoke-' + [guid]::NewGuid())
$InstallDirectory = Join-Path $TaskRoot 'app'
$ProfileDirectory = Join-Path $TaskRoot 'profile'
$PreviousLocalAppData = $env:LOCALAPPDATA
$Process = $null
$Result = [ordered]@{ installer='not run'; upgrade='not run'; configuration='not run'; window='not run'; credentialManager='run go test -run TestNativeWindowsCredentialManager'; rdp='manual check required'; ssh='manual check required'; dialogs='manual check required' }
function Install-Checked([string]$Installer, [string]$LogName) {
    $Arguments = @('/VERYSILENT','/SUPPRESSMSGBOXES','/NORESTART','/SP-','/TASKS=',('/DIR="' + $InstallDirectory + '"'),('/LOG="' + (Join-Path $ReportDirectory $LogName) + '"'))
    $Setup = Start-Process -FilePath $Installer -ArgumentList $Arguments -Wait -PassThru
    if ($Setup.ExitCode -ne 0) { throw "Installer failed: $($Setup.ExitCode)" }
}
try {
    New-Item -ItemType Directory -Force (Join-Path $ProfileDirectory 'DBCatalog') | Out-Null
    $env:LOCALAPPDATA = $ProfileDirectory
    Install-Checked $OldInstaller 'install-old.log'
    $Result.installer = 'passed'
    $ConfigPath = Join-Path $ProfileDirectory 'DBCatalog\connection.json'
    '{"host":"127.0.0.1","port":5432,"database":"smoke","username":"smoke","schema":"smoke_schema","sslMode":"verify-full","sslRootCert":""}' | Set-Content -LiteralPath $ConfigPath -Encoding utf8
    $ConfigHash = (Get-FileHash -LiteralPath $ConfigPath -Algorithm SHA256).Hash
    Install-Checked $NewInstaller 'upgrade.log'
    $Executable = Join-Path $InstallDirectory 'CatalogDB.exe'
    if ((Get-FileHash -LiteralPath $Executable -Algorithm SHA256).Hash -ne $ExpectedHash) { throw 'Upgrade did not install the expected EXE.' }
    $Result.upgrade = 'passed'
    if ((Get-FileHash -LiteralPath $ConfigPath -Algorithm SHA256).Hash -ne $ConfigHash) { throw 'Upgrade changed saved connection configuration.' }
    $Result.configuration = 'passed'
    $Process = Start-Process -FilePath $Executable -ArgumentList '--setup-connection' -PassThru
    $WindowReady = $false
    for ($Attempt = 0; $Attempt -lt 30; $Attempt++) {
        Start-Sleep -Milliseconds 1000
        $Process.Refresh()
        if ($Process.HasExited) { throw 'Application exited before showing a window.' }
        if ($Process.MainWindowHandle -ne 0) { $WindowReady = $true; break }
    }
    if (!$WindowReady) { throw 'Application did not create a native window in 30 seconds.' }
    $Result.window = 'passed (native window creation; visual rendering and dialogs require interactive checks)'
} finally {
    if ($Process -and !$Process.HasExited) { Stop-Process -Id $Process.Id -Force }
    $Uninstaller = Join-Path $InstallDirectory 'unins000.exe'
    if (Test-Path -LiteralPath $Uninstaller) { Start-Process -FilePath $Uninstaller -ArgumentList '/VERYSILENT','/SUPPRESSMSGBOXES','/NORESTART' -Wait | Out-Null }
    $env:LOCALAPPDATA = $PreviousLocalAppData
    $Result | ConvertTo-Json | Set-Content -LiteralPath (Join-Path $ReportDirectory 'results.json') -Encoding utf8
    if (Test-Path -LiteralPath $TaskRoot) { Remove-Item -LiteralPath $TaskRoot -Recurse -Force }
}
