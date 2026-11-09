[CmdletBinding()]
param(
    [Parameter(Position = 0)]
    [string]$Version = "latest"
)

$ErrorActionPreference = "Stop"

if ($env:OS -ne "Windows_NT") {
    throw "This installer supports Windows only."
}

if ($Version -ne "latest") {
    if ($Version -notmatch "^v") {
        $Version = "v$Version"
    }
    if ($Version -notmatch '^v[0-9A-Za-z][0-9A-Za-z._-]*$') {
        throw "Invalid version. Use latest, v1.0.0, or 1.0.0."
    }
}

$processorArchitecture = $env:PROCESSOR_ARCHITEW6432
if ([string]::IsNullOrWhiteSpace($processorArchitecture)) {
    $processorArchitecture = $env:PROCESSOR_ARCHITECTURE
}

switch ($processorArchitecture.ToUpperInvariant()) {
    "AMD64" { $assetArchitecture = "x86_64" }
    "ARM64" { $assetArchitecture = "arm64" }
    default { throw "Unsupported Windows architecture: $processorArchitecture" }
}

$assetName = "akey_Windows_$assetArchitecture.zip"
$releaseBase = "https://github.com/Lensi-zhang/Akey-CLI/releases"
if ($Version -eq "latest") {
    $downloadBase = "$releaseBase/latest/download"
} else {
    $downloadBase = "$releaseBase/download/$Version"
}

if ([string]::IsNullOrWhiteSpace($env:LOCALAPPDATA)) {
    throw "LOCALAPPDATA is not set."
}

$tempDirectory = Join-Path ([System.IO.Path]::GetTempPath()) ("akey-install-" + [Guid]::NewGuid().ToString("N"))
$stagePath = $null
$backupPath = $null
$destinationPath = $null
New-Item -ItemType Directory -Path $tempDirectory | Out-Null

try {
    [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

    $archivePath = Join-Path $tempDirectory $assetName
    $checksumPath = Join-Path $tempDirectory "checksums.txt"
    Invoke-WebRequest -Uri "$downloadBase/$assetName" -OutFile $archivePath -UseBasicParsing
    Invoke-WebRequest -Uri "$downloadBase/checksums.txt" -OutFile $checksumPath -UseBasicParsing

    $escapedAssetName = [regex]::Escape($assetName)
    $checksumLine = Get-Content -LiteralPath $checksumPath | Where-Object {
        $_ -match "^[0-9a-fA-F]{64}\s+\*?$escapedAssetName$"
    } | Select-Object -First 1
    if (-not $checksumLine) {
        throw "No SHA256 checksum found for $assetName."
    }

    $expectedHash = $checksumLine.Substring(0, 64)
    $actualHash = (Get-FileHash -LiteralPath $archivePath -Algorithm SHA256).Hash
    if (-not [string]::Equals($expectedHash, $actualHash, [StringComparison]::OrdinalIgnoreCase)) {
        throw "SHA256 verification failed for $assetName."
    }

    $extractDirectory = Join-Path $tempDirectory "extract"
    New-Item -ItemType Directory -Path $extractDirectory | Out-Null
    Expand-Archive -LiteralPath $archivePath -DestinationPath $extractDirectory -Force
    $extractedBinary = Join-Path $extractDirectory "akey.exe"
    if (-not (Test-Path -LiteralPath $extractedBinary -PathType Leaf)) {
        throw "The archive does not contain akey.exe."
    }

    $installDirectory = Join-Path $env:LOCALAPPDATA "Programs\Akey\bin"
    New-Item -ItemType Directory -Path $installDirectory -Force | Out-Null
    $destinationPath = Join-Path $installDirectory "akey.exe"
    $stagePath = Join-Path $installDirectory (".akey-" + [Guid]::NewGuid().ToString("N") + ".exe")
    Copy-Item -LiteralPath $extractedBinary -Destination $stagePath

    if (Test-Path -LiteralPath $destinationPath) {
        if (-not (Test-Path -LiteralPath $destinationPath -PathType Leaf)) {
            throw "Installation destination is not a file: $destinationPath"
        }
        $backupPath = "$destinationPath.$([Guid]::NewGuid().ToString('N')).backup"
        [System.IO.File]::Replace($stagePath, $destinationPath, $backupPath, $true)
    } else {
        [System.IO.File]::Move($stagePath, $destinationPath)
    }
    $stagePath = $null

    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    $normalizedInstallDirectory = $installDirectory.TrimEnd('\')
    $pathPresent = $false
    if ($null -ne $userPath) {
        foreach ($entry in ($userPath -split ';')) {
            if ($entry.Trim().TrimEnd('\') -ieq $normalizedInstallDirectory) {
                $pathPresent = $true
                break
            }
        }
    }

    if (-not $pathPresent) {
        if ([string]::IsNullOrWhiteSpace($userPath)) {
            $updatedPath = $installDirectory
        } else {
            $updatedPath = $userPath.TrimEnd(';') + ';' + $installDirectory
        }
        [Environment]::SetEnvironmentVariable("Path", $updatedPath, "User")
        Write-Host "Added $installDirectory to the current user PATH. Restart your terminal to use akey."
    }

    Write-Host "Installed akey to $destinationPath"
} finally {
    if ($stagePath -and (Test-Path -LiteralPath $stagePath)) {
        Remove-Item -LiteralPath $stagePath -Force -ErrorAction SilentlyContinue
    }
    if ($backupPath -and $destinationPath -and (Test-Path -LiteralPath $destinationPath) -and (Test-Path -LiteralPath $backupPath)) {
        Remove-Item -LiteralPath $backupPath -Force -ErrorAction SilentlyContinue
    }
    if (Test-Path -LiteralPath $tempDirectory) {
        Remove-Item -LiteralPath $tempDirectory -Recurse -Force -ErrorAction SilentlyContinue
    }
}
