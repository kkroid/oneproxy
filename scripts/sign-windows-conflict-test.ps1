param([Parameter(Mandatory=$true)][string]$EvidenceDir)
$ErrorActionPreference = 'Stop'
$EvidenceDir = [IO.Path]::GetFullPath($EvidenceDir)
New-Item -ItemType Directory -Force $EvidenceDir | Out-Null
$fixture = Join-Path $EvidenceDir ('conflict-' + [guid]::NewGuid())
New-Item -ItemType Directory $fixture | Out-Null
$source = Join-Path $fixture 'sing-box.exe'
$alias = Join-Path $fixture 'alias.exe'
Set-Content $source 'synthetic third-party'
Copy-Item $source $alias
Set-Content (Join-Path $fixture 'oneproxy-tray.exe') 'synthetic tray'
Set-Content (Join-Path $fixture 'oneproxy.dll') 'synthetic dll'
$setup = Join-Path $fixture 'setup.exe'
Set-Content $setup 'synthetic setup'
$pe = Join-Path $fixture 'unsigned.dll'
Add-Type -TypeDefinition 'public class UnsignedSigningFixture { public static int Value() { return 1; } }' -OutputAssembly $pe
Copy-Item $pe $setup -Force
Copy-Item $pe (Join-Path $fixture 'oneproxy-tray.exe') -Force
Copy-Item $pe (Join-Path $fixture 'oneproxy.dll') -Force
$calls = Join-Path $fixture 'calls.txt'
$fake = Join-Path $fixture 'fake-signtool.ps1'
Set-Content $fake @(
    'Add-Content (Join-Path $PSScriptRoot ''calls.txt'') ($args -join ''|'')'
    'exit 31'
)
$script = Join-Path $PSScriptRoot 'sign-windows.ps1'
foreach ($entry in @('Installer', 'Uninstaller')) {
    foreach ($kind in @('direct', 'alias', 'missing-reference')) {
        $target = if ($kind -eq 'alias') { $alias } else { $source }
        $reference = if ($kind -eq 'missing-reference') { Join-Path $fixture 'absent.exe' } else { $source }
        $params = @('-Sign', '-CertificateThumbprint', 'fake', '-SignTool', $fake,
            '-BuildDir', $fixture, '-SingBox', $reference)
        if ($entry -eq 'Uninstaller') { $params += @('-Installer', $setup) }
        $params += @("-$entry", $target)
        $output = & pwsh -NoProfile -File $script @params 2>&1
        $code = $LASTEXITCODE
        $output | Set-Content (Join-Path $fixture "$entry-$kind.log")
        if ($code -eq 0 -or "$output" -notmatch 'Refusing to sign third-party') { throw "Expected conflict rejection: $entry/$kind" }
        if (Test-Path $calls) { throw "signtool called before rejection: $entry/$kind" }
        Write-Output "PASS $entry/${kind}: rejected, zero signtool calls"
    }
}
$output = & pwsh -NoProfile -File $script -Sign -CertificateThumbprint fake -SignTool $fake -BuildDir $fixture -Installer $setup -SingBox $source 2>&1
$code = $LASTEXITCODE
$output | Set-Content (Join-Path $fixture 'signing-failure.log')
if ($code -eq 0 -or "$output" -notmatch 'signtool failed' -or -not (Test-Path $calls)) { throw 'Signing failure control failed.' }
Write-Output 'PASS signing failure control: fake invoked, failure propagated'
$paths = @($setup, (Join-Path $fixture 'oneproxy-tray.exe'), (Join-Path $fixture 'oneproxy.dll'))
$hashes = @(Get-FileHash $paths | ForEach-Object Hash)
$manifest = Join-Path $fixture 'hashes.txt'
$output = & pwsh -NoProfile -File $script -VerifyOnly -BuildDir $fixture -Installer $setup -SingBox $source -Manifest $manifest 2>&1
$output | Set-Content (Join-Path $fixture 'inspection.log')
if ($LASTEXITCODE -ne 0 -or "$output" -notmatch 'NotSigned' -or "$output" -notmatch 'Missing') { throw 'Inspection must show NotSigned and missing timestamp.' }
$output | Set-Content (Join-Path $fixture 'inspection.log')
foreach ($kind in @('unsigned', 'missing')) {
    $inputSetup = if ($kind -eq 'missing') { Join-Path $fixture 'absent.exe' } else { $setup }
    $output = & pwsh -NoProfile -File $script -StrictVerify -BuildDir $fixture -Installer $inputSetup -SingBox $source -Manifest $manifest 2>&1
    if ($LASTEXITCODE -eq 0 -or "$output" -notmatch 'verification failed') { throw "Strict verification accepted $kind files." }
    $output | Set-Content (Join-Path $fixture "strict-$kind.log")
    Write-Output "PASS strict verification rejects $kind inputs"
}
$after = @(Get-FileHash $paths | ForEach-Object Hash)
if (($hashes -join ',') -ne ($after -join ',')) { throw 'Verification changed release bytes.' }
if (@(Get-Content $calls).Count -ne 1) { throw 'Verification invoked the signing tool.' }
Write-Output "PASS verification preserved release hashes; evidence: $fixture"
