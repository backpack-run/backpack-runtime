[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [ValidatePattern('^v[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?$')]
    [string]$Version,
    [string]$InstallDirectory = (Join-Path $env:LOCALAPPDATA 'Programs\Backpack\bin')
)

$ErrorActionPreference = 'Stop'
$repository = 'https://github.com/backpack-run/backpack-runtime'
$releaseVersion = $Version.TrimStart('v')
$archive = "backpack_${releaseVersion}_windows_amd64.zip"
$temporaryDirectory = Join-Path ([IO.Path]::GetTempPath()) ("backpack-install-" + [Guid]::NewGuid().ToString('N'))

try {
    New-Item -ItemType Directory -Path $temporaryDirectory | Out-Null
    $archivePath = Join-Path $temporaryDirectory $archive
    $checksumsPath = Join-Path $temporaryDirectory 'checksums.txt'
    Invoke-WebRequest -UseBasicParsing -Uri "$repository/releases/download/$Version/$archive" -OutFile $archivePath
    Invoke-WebRequest -UseBasicParsing -Uri "$repository/releases/download/$Version/checksums.txt" -OutFile $checksumsPath
    $checksumPattern = '^[0-9a-fA-F]{64}\s+\*?' + [regex]::Escape($archive) + '$'
    $line = Get-Content -LiteralPath $checksumsPath | Where-Object { $_ -match $checksumPattern } | Select-Object -First 1
    if (-not $line) { throw "Release checksum for $archive was not published." }
    $expected = ($line -split '\s+')[0].ToLowerInvariant()
    $actual = (Get-FileHash -Algorithm SHA256 -LiteralPath $archivePath).Hash.ToLowerInvariant()
    if ($actual -ne $expected) { throw "SHA-256 mismatch for $archive." }
    $expanded = Join-Path $temporaryDirectory 'expanded'
    Expand-Archive -LiteralPath $archivePath -DestinationPath $expanded
    New-Item -ItemType Directory -Force -Path $InstallDirectory | Out-Null
    Copy-Item -LiteralPath (Join-Path $expanded 'backpack.exe') -Destination (Join-Path $InstallDirectory 'backpack.exe') -Force
    Write-Host "Installed verified Backpack Runtime $Version to $InstallDirectory"
    Write-Host 'Add that directory to your user PATH if it is not already present.'
}
finally {
    if (Test-Path -LiteralPath $temporaryDirectory) { Remove-Item -LiteralPath $temporaryDirectory -Recurse -Force }
}
