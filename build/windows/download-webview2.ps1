[CmdletBinding()]
param(
    [Parameter()]
    [string] $DestinationPath = (Join-Path $PSScriptRoot 'nsis\MicrosoftEdgeWebview2Setup.exe'),

    [Parameter()]
    [ValidateRange(1, 600)]
    [int] $TimeoutSeconds = 120,

    [Parameter()]
    [ValidateRange(1, 104857600)]
    [long] $MaximumDownloadBytes = 20 * 1024 * 1024
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$downloadUri = [Uri] 'https://go.microsoft.com/fwlink/p/?LinkId=2124703'
$destinationFullPath = [System.IO.Path]::GetFullPath($DestinationPath)
$destinationDirectory = [System.IO.Path]::GetDirectoryName($destinationFullPath)
[System.IO.Directory]::CreateDirectory($destinationDirectory) | Out-Null
$temporaryPath = Join-Path $destinationDirectory ('.MicrosoftEdgeWebview2Setup.' + [Guid]::NewGuid().ToString('N') + '.tmp')
$httpClient = $null
$deadline = $null

try {
    Add-Type -AssemblyName System.Net.Http
    $handler = [System.Net.Http.HttpClientHandler]::new()
    $handler.MaxAutomaticRedirections = 5
    $httpClient = [System.Net.Http.HttpClient]::new($handler)
    $httpClient.Timeout = [System.Threading.Timeout]::InfiniteTimeSpan
    $deadline = [System.Threading.CancellationTokenSource]::new()
    $deadline.CancelAfter([TimeSpan]::FromSeconds($TimeoutSeconds))

    $response = $httpClient.GetAsync(
        $downloadUri,
        [System.Net.Http.HttpCompletionOption]::ResponseHeadersRead,
        $deadline.Token
    ).GetAwaiter().GetResult()
    try {
        if ($response.RequestMessage.RequestUri.Scheme -ne 'https') {
            throw 'Microsoft WebView2 download did not finish over HTTPS.'
        }
        if (-not $response.IsSuccessStatusCode) {
            throw "Microsoft WebView2 download returned HTTP status $([int] $response.StatusCode)."
        }
        if ($null -ne $response.Content.Headers.ContentLength -and
            [long] $response.Content.Headers.ContentLength -gt $MaximumDownloadBytes) {
            throw "Microsoft WebView2 bootstrapper exceeds the $MaximumDownloadBytes-byte size limit."
        }

        $inputStream = $response.Content.ReadAsStreamAsync().GetAwaiter().GetResult()
        $outputStream = $null
        try {
            $outputStream = [System.IO.FileStream]::new(
                $temporaryPath,
                [System.IO.FileMode]::CreateNew,
                [System.IO.FileAccess]::Write,
                [System.IO.FileShare]::None
            )
            $buffer = New-Object byte[] (32 * 1024)
            [long] $downloadedBytes = 0
            while ($true) {
                $bytesRead = $inputStream.ReadAsync($buffer, 0, $buffer.Length, $deadline.Token).GetAwaiter().GetResult()
                if ($bytesRead -eq 0) {
                    break
                }
                $downloadedBytes += $bytesRead
                if ($downloadedBytes -gt $MaximumDownloadBytes) {
                    throw "Microsoft WebView2 bootstrapper exceeds the $MaximumDownloadBytes-byte size limit."
                }
                $outputStream.Write($buffer, 0, $bytesRead)
            }
            $outputStream.Flush($true)
        }
        finally {
            if ($null -ne $outputStream) {
                $outputStream.Dispose()
            }
            $inputStream.Dispose()
        }
    }
    finally {
        $response.Dispose()
    }

    if ($downloadedBytes -eq 0) {
        throw 'Microsoft WebView2 download returned an empty file.'
    }

    $signature = Get-AuthenticodeSignature -FilePath $temporaryPath
    if ($signature.Status -ne [System.Management.Automation.SignatureStatus]::Valid) {
        throw "Microsoft WebView2 bootstrapper Authenticode status is '$($signature.Status)', expected Valid."
    }
    if ($null -eq $signature.SignerCertificate -or
        $signature.SignerCertificate.GetNameInfo(
            [System.Security.Cryptography.X509Certificates.X509NameType]::SimpleName,
            $false
        ) -ne 'Microsoft Corporation') {
        throw 'Microsoft WebView2 bootstrapper signer is not Microsoft Corporation.'
    }

    if ([System.IO.File]::Exists($destinationFullPath)) {
        [System.IO.File]::Replace(
            $temporaryPath,
            $destinationFullPath,
            [System.Management.Automation.Language.NullString]::Value
        )
    }
    else {
        [System.IO.File]::Move($temporaryPath, $destinationFullPath)
    }
}
catch [System.OperationCanceledException] {
    throw "Microsoft WebView2 bootstrapper download exceeded the $TimeoutSeconds-second timeout."
}
finally {
    if ($null -ne $deadline) {
        $deadline.Dispose()
    }
    if ($null -ne $httpClient) {
        $httpClient.Dispose()
    }
    if ([System.IO.File]::Exists($temporaryPath)) {
        [System.IO.File]::Delete($temporaryPath)
    }
}
