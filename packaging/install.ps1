# Install Ctrl+Shift+3 CLI (cs3) on Windows and start it.
# Usage (PowerShell):
#   irm https://ctrlshift3.com/CLI/install.ps1 | iex

$ErrorActionPreference = 'Stop'

$baseUrl = if ($env:CS3_BASE_URL) { $env:CS3_BASE_URL.TrimEnd('/') } else { 'https://ctrlshift3.com' }
$zipName = 'cs3-windows-amd64.zip'
$url = "$baseUrl/CLI/$zipName"
$sumsUrl = "$baseUrl/CLI/SHA256SUMS"
$destDir = Join-Path $env:LOCALAPPDATA 'cs3'
$dest = Join-Path $destDir 'cs3.exe'

$tmpdir = Join-Path ([System.IO.Path]::GetTempPath()) ("cs3-install-" + [guid]::NewGuid().ToString('n'))
New-Item -ItemType Directory -Path $tmpdir | Out-Null
try {
    $zipPath = Join-Path $tmpdir $zipName
    $sumsPath = Join-Path $tmpdir 'SHA256SUMS'
    $expected = $null

    # Fetch checksums first, then download zip with ?h=<hash> to bypass stale CDN caches.
    try {
        Invoke-WebRequest -Uri $sumsUrl -OutFile $sumsPath -UseBasicParsing
        Get-Content -LiteralPath $sumsPath | ForEach-Object {
            $parts = $_ -split '\s+', 2
            if ($parts.Count -ge 2 -and $parts[1].Trim() -eq $zipName) {
                $expected = $parts[0].Trim().ToLowerInvariant()
            }
        }
        if (-not $expected) {
            throw "Checksum list has no entry for $zipName."
        }
        $url = "$url`?h=$expected"
    } catch {
        if ($_.Exception.Message -like 'Checksum list*') { throw }
        Write-Warning "Could not download SHA256SUMS; continuing without verify."
    }

    Write-Host "Downloading $url ..."
    Invoke-WebRequest -Uri $url -OutFile $zipPath -UseBasicParsing

    if ($expected) {
        $hash = (Get-FileHash -LiteralPath $zipPath -Algorithm SHA256).Hash.ToLowerInvariant()
        if ($hash -ne $expected) {
            throw "Checksum mismatch for $zipName. Expected $expected, got $hash. If you just redeployed, purge the CDN cache for /CLI/ and retry."
        }
        Write-Host "Checksum OK."
    }

    Expand-Archive -Path $zipPath -DestinationPath $tmpdir -Force
    $src = Join-Path $tmpdir 'cs3.exe'
    if (-not (Test-Path -LiteralPath $src)) {
        throw 'Zip did not contain cs3.exe.'
    }

    New-Item -ItemType Directory -Path $destDir -Force | Out-Null
    Copy-Item -LiteralPath $src -Destination $dest -Force
    Write-Host "Installed to $dest"

    $pathParts = $env:Path -split ';' | Where-Object { $_ -ne '' }
    if ($pathParts -notcontains $destDir) {
        Write-Host ""
        Write-Host "Note: $destDir is not on your PATH."
        Write-Host "Add it for this session:  `$env:Path = `"$destDir;`$env:Path`""
        Write-Host "Or permanently: Settings → System → About → Advanced system settings → Environment Variables → Path"
    }

    Write-Host "Starting cs3 ..."
    Write-Host ""
    & $dest
    exit $LASTEXITCODE
}
finally {
    Remove-Item -LiteralPath $tmpdir -Recurse -Force -ErrorAction SilentlyContinue
}
