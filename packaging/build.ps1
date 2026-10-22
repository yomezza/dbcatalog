$ErrorActionPreference = 'Stop'
Set-Location (Join-Path $PSScriptRoot '..')
function Invoke-Checked([scriptblock]$Command) {
    & $Command
    if ($LASTEXITCODE -ne 0) { throw "Команда завершилась с кодом $LASTEXITCODE" }
}
Push-Location frontend
try {
    Invoke-Checked { npm ci }
    Invoke-Checked { npm test }
    Invoke-Checked { npm run build }
} finally { Pop-Location }
Invoke-Checked { go test ./... }
Invoke-Checked { go vet ./... }
Invoke-Checked { wails build -clean -platform windows/amd64 }
$Vendor = Join-Path $PSScriptRoot 'vendor'
New-Item -ItemType Directory -Force $Vendor | Out-Null
$Runtime = Join-Path $Vendor 'MicrosoftEdgeWebView2Setup.exe'
if ($env:CATALOGDB_WEBVIEW2_INSTALLER) {
    Copy-Item -LiteralPath $env:CATALOGDB_WEBVIEW2_INSTALLER -Destination $Runtime -Force
} else {
    Invoke-WebRequest -Uri 'https://go.microsoft.com/fwlink/p/?LinkId=2124703' -OutFile $Runtime
}
$Signature = Get-AuthenticodeSignature -LiteralPath $Runtime
if ($Signature.Status -ne 'Valid' -or $Signature.SignerCertificate.Subject -notmatch 'O=Microsoft Corporation') {
    throw 'Установщик WebView2 не имеет действительной подписи Microsoft. Сборка установщика остановлена.'
}
$Compiler = Get-Command ISCC.exe -ErrorAction SilentlyContinue
if ($Compiler) {
    Invoke-Checked { & $Compiler.Source packaging/inno_setup/catalogdb.iss }
} else {
    Write-Host 'EXE собран. Для установщика установите Inno Setup 6 и добавьте ISCC.exe в PATH.'
}
