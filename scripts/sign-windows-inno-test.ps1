param(
    [Parameter(Mandatory=$true)][string]$EvidenceDir,
    [string]$InnoSetup = 'C:\Program Files (x86)\Inno Setup 6\ISCC.exe'
)
$ErrorActionPreference = 'Stop'
$fixture = Join-Path ([IO.Path]::GetFullPath($EvidenceDir)) ('inno-' + [guid]::NewGuid())
New-Item -ItemType Directory -Force $fixture | Out-Null
. (Join-Path $PSScriptRoot 'sign-windows-command.ps1')
# Use the real installer preprocessor and Setup directives; replace only payload
# and output paths. The probe installer is never executed.
$source = Get-Content -Raw (Join-Path $PSScriptRoot '..\trayapp\installer.iss')
$setup = ($source -split '\[Languages\]')[0]
$setup += "[Files]" + [Environment]::NewLine + 'Source: "payload.txt"; DestDir: "{app}"'
$iss = Join-Path $fixture 'probe.iss'
$setup | Set-Content $iss
'harmless synthetic payload' | Set-Content (Join-Path $fixture 'payload.txt')
$fake = Join-Path $fixture 'fake tool.ps1'
Set-Content $fake @(
    '$args | ConvertTo-Json | Set-Content (Join-Path $PSScriptRoot ''calls.json'')'
    'exit 31'
)
$log = Join-Path $fixture 'calls.json'
foreach ($mode in @('ordinary', 'dry-run')) {
    $output = & $InnoSetup "/O$fixture" "/F$mode" $iss 2>&1
    $code = $LASTEXITCODE
    $output | Set-Content (Join-Path $fixture "$mode.log")
    if ($code -ne 0 -or -not (Test-Path (Join-Path $fixture "$mode.exe"))) { throw "Inno $mode compile failed: $output" }
    if (Test-Path $log) { throw "Unsigned $mode unexpectedly invoked signing." }
    Write-Output "PASS Inno ${mode}: compiled unsigned, no signing call"
}
foreach ($certificate in @('thumbprint', 'pfx')) {
    $options = @{SignTool=$fake;TimestampUrl='http://timestamp.example.invalid/rfc3161'}
    if ($certificate -eq 'thumbprint') { $options.CertificateThumbprint='TEST-THUMBPRINT' }
    else { $options.PfxPath=Join-Path $fixture 'test certificate.pfx' }
    $innoArgs = @(Get-OneProxyInnoSigningArguments @options)
    $innoArgs | Set-Content (Join-Path $fixture "$certificate-command.txt")
    $output = & $InnoSetup @innoArgs "/O$fixture" "/F$certificate" $iss 2>&1
    $code = $LASTEXITCODE
    $output | Set-Content (Join-Path $fixture "$certificate.log")
    if ($code -eq 0 -or -not (Test-Path $log)) { throw "Expected propagated signing failure: $output" }
    $received = @(Get-Content -Raw $log | ConvertFrom-Json)
    $received | ConvertTo-Json | Set-Content (Join-Path $fixture "$certificate-args.json")
    if (($received -join '|') -notmatch 'sign\|/fd\|SHA256\|/tr\|http://timestamp.example.invalid/rfc3161\|/td\|SHA256') { throw 'Inno timestamp arguments corrupted.' }
    if ($certificate -eq 'thumbprint' -and ($received -join '|') -notmatch '/sha1\|TEST-THUMBPRINT') { throw 'Thumbprint missing.' }
    if ($certificate -eq 'pfx' -and $received -notcontains $options.PfxPath) { throw 'PFX path with spaces corrupted.' }
    if ($received[-1] -notmatch '(?i)uninst|\.e32$') { throw "First signing target must be generated uninstaller: $($received[-1])" }
    Write-Output "PASS Inno $certificate command: generated uninstaller, certificate and RFC3161 arguments; failure propagated"
}
Write-Output "Evidence: $fixture"
