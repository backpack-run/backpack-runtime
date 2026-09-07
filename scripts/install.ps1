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
$stagedBinary = $null
$backupBinary = $null

if ($env:OS -ne 'Windows_NT' -or -not [Environment]::Is64BitOperatingSystem) {
    throw 'This installer supports Windows x64 only.'
}

function Save-HttpsFile {
    param([Parameter(Mandatory = $true)][Uri]$Uri, [Parameter(Mandatory = $true)][string]$Destination)
    Add-Type -AssemblyName System.Net.Http
    $handler = [Net.Http.HttpClientHandler]::new()
    $handler.AllowAutoRedirect = $false
    $client = [Net.Http.HttpClient]::new($handler)
    $client.DefaultRequestHeaders.UserAgent.ParseAdd('Backpack-Runtime-Installer')
    try {
        $current = $Uri
        for ($redirects = 0; $redirects -le 8; $redirects++) {
            if ($current.Scheme -ne 'https') { throw "Refusing non-HTTPS download: $current" }
            $response = $client.GetAsync($current, [Net.Http.HttpCompletionOption]::ResponseHeadersRead).GetAwaiter().GetResult()
            try {
                if ([int]$response.StatusCode -ge 300 -and [int]$response.StatusCode -lt 400) {
                    if ($null -eq $response.Headers.Location) { throw "Redirect from $current omitted Location." }
                    $current = [Uri]::new($current, $response.Headers.Location)
                    continue
                }
                $response.EnsureSuccessStatusCode() | Out-Null
                $source = $response.Content.ReadAsStreamAsync().GetAwaiter().GetResult()
                try {
                    $destinationStream = [IO.File]::Open($Destination, [IO.FileMode]::CreateNew, [IO.FileAccess]::Write, [IO.FileShare]::None)
                    try { $source.CopyTo($destinationStream) } finally { $destinationStream.Dispose() }
                } finally { $source.Dispose() }
                return
            } finally { $response.Dispose() }
        }
        throw "Too many redirects while downloading $Uri"
    } finally {
        $client.Dispose()
        $handler.Dispose()
    }
}

try {
    New-Item -ItemType Directory -Path $temporaryDirectory | Out-Null
    $archivePath = Join-Path $temporaryDirectory $archive
    $checksumsPath = Join-Path $temporaryDirectory 'checksums.txt'
    Save-HttpsFile -Uri "$repository/releases/download/$Version/$archive" -Destination $archivePath
    Save-HttpsFile -Uri "$repository/releases/download/$Version/checksums.txt" -Destination $checksumsPath
    $checksumPattern = '^[0-9a-fA-F]{64}\s+\*?' + [regex]::Escape($archive) + '$'
    $line = Get-Content -LiteralPath $checksumsPath | Where-Object { $_ -match $checksumPattern } | Select-Object -First 1
    if (-not $line) { throw "Release checksum for $archive was not published." }
    $expected = ($line -split '\s+')[0].ToLowerInvariant()
    $actual = (Get-FileHash -Algorithm SHA256 -LiteralPath $archivePath).Hash.ToLowerInvariant()
    if ($actual -ne $expected) { throw "SHA-256 mismatch for $archive." }
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $zip = [IO.Compression.ZipFile]::OpenRead($archivePath)
    $candidate = Join-Path $temporaryDirectory 'backpack.exe'
    try {
        $allowed = @('LICENSE', 'README.md', 'THIRD_PARTY_NOTICES.md', 'backpack.exe')
        $seen = [Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal)
        foreach ($entry in $zip.Entries) {
            $name = $entry.FullName.Replace('\', '/')
            if ($name -notin $allowed -or -not $seen.Add($name)) { throw "Unsafe or unexpected archive entry: $name" }
            $unixType = (($entry.ExternalAttributes -shr 16) -band 0xF000)
            if ($unixType -eq 0xA000 -or $entry.FullName.EndsWith('/')) { throw "Links and directories are not allowed in the release archive: $name" }
        }
        if ($seen.Count -ne $allowed.Count) { throw 'Release archive is incomplete.' }
        $binaryEntry = $zip.GetEntry('backpack.exe')
        $source = $binaryEntry.Open()
        try {
            $destinationStream = [IO.File]::Open($candidate, [IO.FileMode]::CreateNew, [IO.FileAccess]::Write, [IO.FileShare]::None)
            try { $source.CopyTo($destinationStream) } finally { $destinationStream.Dispose() }
        } finally { $source.Dispose() }
    } finally { $zip.Dispose() }
    $reportedVersion = & $candidate version
    if ($LASTEXITCODE -ne 0 -or "$reportedVersion" -notmatch [regex]::Escape($releaseVersion)) {
        throw "Downloaded binary did not report expected version $releaseVersion."
    }
    New-Item -ItemType Directory -Force -Path $InstallDirectory | Out-Null
    $destination = Join-Path $InstallDirectory 'backpack.exe'
    $stagedBinary = Join-Path $InstallDirectory ('.backpack-install-' + [Guid]::NewGuid().ToString('N') + '.exe')
    Copy-Item -LiteralPath $candidate -Destination $stagedBinary
    if (Test-Path -LiteralPath $destination) {
        $backupBinary = Join-Path $InstallDirectory ('.backpack-backup-' + [Guid]::NewGuid().ToString('N') + '.exe')
        [IO.File]::Replace($stagedBinary, $destination, $backupBinary, $true)
        Remove-Item -LiteralPath $backupBinary -Force
        $backupBinary = $null
    } else {
        Move-Item -LiteralPath $stagedBinary -Destination $destination
    }
    $stagedBinary = $null
    Write-Host "Installed verified Backpack Runtime $Version to $InstallDirectory"
    Write-Host 'Add that directory to your user PATH if it is not already present.'
}
finally {
    if ($stagedBinary -and (Test-Path -LiteralPath $stagedBinary)) { Remove-Item -LiteralPath $stagedBinary -Force }
    if ($backupBinary -and (Test-Path -LiteralPath $backupBinary)) { Remove-Item -LiteralPath $backupBinary -Force }
    if (Test-Path -LiteralPath $temporaryDirectory) { Remove-Item -LiteralPath $temporaryDirectory -Recurse -Force }
}
