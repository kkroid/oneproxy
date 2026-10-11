[CmdletBinding()]
param(
    [Parameter(Mandatory=$true)][string]$FilePath,
    [Parameter(Mandatory=$true)][string]$SignTool,
    [string]$CertificateThumbprint,
    [string]$PfxPath,
    [string]$TimestampUrl = 'http://timestamp.digicert.com'
)
$ErrorActionPreference='Stop'
if ([bool]$CertificateThumbprint -eq [bool]$PfxPath) { throw 'Select exactly one certificate source.' }
if (-not $TimestampUrl) { throw 'Timestamp URL is required.' }
if ([IO.Path]::GetFileName($FilePath) -ieq 'sing-box.exe') { throw 'Refusing third-party sing-box target.' }
$a=@('sign','/fd','SHA256','/tr',$TimestampUrl,'/td','SHA256')
if($CertificateThumbprint){$a+=@('/sha1',$CertificateThumbprint)}
if($PfxPath){$a+=@('/f',$PfxPath)}
$a+=$FilePath
& $SignTool @a
if($LASTEXITCODE -ne 0){throw "signtool failed for $FilePath (exit $LASTEXITCODE)"}
$signature = Get-AuthenticodeSignature -LiteralPath $FilePath
if ($signature.Status -ne 'Valid' -or -not $signature.TimeStamperCertificate) { throw 'Inno signing output lacks a valid timestamped signature.' }
& $SignTool verify /pa /all /tw $FilePath
if ($LASTEXITCODE -ne 0) { throw 'Inno signing output failed trust verification.' }
