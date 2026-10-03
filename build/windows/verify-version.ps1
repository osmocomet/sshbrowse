param(
    [Parameter(Mandatory = $true)]
    [string]$Path,
    [Parameter(Mandatory = $true)]
    [string]$ExpectedVersion
)

$ErrorActionPreference = "Stop"

if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) {
    throw "Windows release executable was not produced: $Path"
}

$versionInfo = (Get-Item -LiteralPath $Path).VersionInfo
if ($versionInfo.FileVersion -ne $ExpectedVersion -or $versionInfo.ProductVersion -ne $ExpectedVersion) {
    throw "Windows release executable version metadata is incorrect. Expected FileVersion and ProductVersion $ExpectedVersion; got '$($versionInfo.FileVersion)' and '$($versionInfo.ProductVersion)'."
}
