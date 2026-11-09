[CmdletBinding()]
param(
    [Parameter(Position = 0)]
    [string]$Version = "latest",
    [switch]$Uninstall,
    [switch]$Purge
)

$ErrorActionPreference = "Stop"

if ($env:OS -ne "Windows_NT") {
    throw "This installer supports Windows only."
}

if ([string]::IsNullOrWhiteSpace($env:LOCALAPPDATA)) {
    throw "LOCALAPPDATA is not set."
}

$installDirectory = Join-Path $env:LOCALAPPDATA "Programs\Akey\bin"
$destinationPath = Join-Path $installDirectory "akey.exe"

function Remove-AkeyData {
    $binaryPath = $null
    if (Test-Path -LiteralPath $destinationPath -PathType Leaf) {
        $binaryPath = $destinationPath
    } else {
        $installed = Get-Command akey -ErrorAction SilentlyContinue
        if ($null -ne $installed) {
            $binaryPath = $installed.Source
        }
    }

    if ($binaryPath) {
        $profileNames = @()
        try {
            $raw = (& $binaryPath profile list --output json 2>$null) -join "`n"
            if ($LASTEXITCODE -eq 0 -and -not [string]::IsNullOrWhiteSpace($raw)) {
                $profileNames = @($raw | ConvertFrom-Json | ForEach-Object { $_.name })
            }
        } catch {
            $profileNames = @()
        }
        foreach ($profileName in $profileNames) {
            if ($profileName -notmatch '^[a-z0-9][a-z0-9_-]{0,31}$') {
                continue
            }
            try {
                & $binaryPath profile delete $profileName --yes 2>$null | Out-Null
                if ($LASTEXITCODE -ne 0) {
                    Write-Warning "Could not delete profile $profileName; remove it manually before purging."
                }
            } catch {
                Write-Warning "Could not delete profile $profileName; remove it manually before purging."
            }
        }
    } else {
        Write-Warning "akey binary not found; API keys in the OS credential store were not removed."
    }

    $dataDirectory = Join-Path $env:USERPROFILE ".akey"
    if (Test-Path -LiteralPath $dataDirectory) {
        Remove-Item -LiteralPath $dataDirectory -Recurse -Force
        Write-Host "Removed profile data at $dataDirectory"
    }
}

if ($Uninstall) {
    if ($Purge) {
        Remove-AkeyData
    }

    if (Test-Path -LiteralPath $destinationPath -PathType Leaf) {
        Remove-Item -LiteralPath $destinationPath -Force
        Write-Host "Removed $destinationPath"
    } else {
        Write-Host "akey is not installed at $destinationPath"
    }

    if ((Test-Path -LiteralPath $installDirectory) -and -not (Get-ChildItem -LiteralPath $installDirectory -Force)) {
        Remove-Item -LiteralPath $installDirectory -Force
    }

    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    if ($null -ne $userPath) {
        $normalizedInstallDirectory = $installDirectory.TrimEnd('\')
        $keptEntries = @($userPath -split ';' | Where-Object {
            $_ -and ($_.Trim().TrimEnd('\') -ine $normalizedInstallDirectory)
        })
        $updatedPath = $keptEntries -join ';'
        if ($updatedPath -ne $userPath) {
            [Environment]::SetEnvironmentVariable("Path", $updatedPath, "User")
            Write-Host "Removed $installDirectory from the current user PATH."
        }
    }

    if (-not $Purge) {
        Write-Host "Profile data and API keys were kept. Re-run with -Purge to remove them."
    }

    exit 0
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

$tempDirectory = Join-Path ([System.IO.Path]::GetTempPath()) ("akey-install-" + [Guid]::NewGuid().ToString("N"))
$stagePath = $null
$backupPath = $null
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

    New-Item -ItemType Directory -Path $installDirectory -Force | Out-Null
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
