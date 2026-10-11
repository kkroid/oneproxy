# Shared by build.ps1 and the real Inno command-line regression probe.
function Get-OneProxyInnoSigningArguments {
    param([string]$SignTool, [string]$CertificateThumbprint, [string]$PfxPath,
          [string]$TimestampUrl = 'http://timestamp.digicert.com')
    if ([bool]$CertificateThumbprint -eq [bool]$PfxPath) { throw 'Select exactly one certificate source.' }
    if (-not $TimestampUrl) { throw 'Timestamp URL is required.' }
    $values = @((Get-Command pwsh -ErrorAction Stop).Source,
        (Join-Path $PSScriptRoot 'sign-windows-inno.ps1'), $SignTool, $TimestampUrl)
    # These values become Inno command text. Reject metacharacters rather than
    # interpreting user-provided paths or certificate selectors as commands.
    foreach ($value in ($values + @($CertificateThumbprint, $PfxPath))) {
        if ($value -match '["$\r\n&|<>^%]') { throw 'Unsupported character in signing command value.' }
    }
    $command = '$q{0}$q -NoProfile -File $q{1}$q -FilePath $f -SignTool $q{2}$q -TimestampUrl $q{3}$q' -f $values
    if ($CertificateThumbprint) { $command += ' -CertificateThumbprint $q{0}$q' -f $CertificateThumbprint }
    if ($PfxPath) { $command += ' -PfxPath $q{0}$q' -f $PfxPath }
    '/DSignToolCommand'
    "/Soneproxy-sign=$command"
}
