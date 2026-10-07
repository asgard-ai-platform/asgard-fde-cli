# One-command install for Windows.
#
#   irm https://raw.githubusercontent.com/asgard-ai-platform/asgard-fde-cli/main/install.ps1 | iex
#
# The Windows half of install.sh, and it makes the same two decisions.
#
# **It verifies what it downloaded.** A script piped into a shell is already a
# trust decision, so what it fetches is checked against the release's own
# checksums before anything is run - matched by HASH rather than by name,
# because the release carries the same bytes twice, with a version in the
# filename and without, and only the versioned name is in checksums.txt.
#
# **It installs somewhere the user owns**, under %LOCALAPPDATA%, rather than
# into Program Files. Two things follow and both are the point: no elevation is
# needed to install, and `asgard-cli update` can replace the binary in place
# afterwards, which it cannot do for a file somebody else owns.

$ErrorActionPreference = 'Stop'

$repo = 'asgard-ai-platform/asgard-fde-cli'
$base = "https://github.com/$repo/releases/latest/download"

$arch = switch ($env:PROCESSOR_ARCHITECTURE) {
    'AMD64' { 'amd64' }
    'ARM64' { 'arm64' }
    default {
        Write-Error "No release is built for $($env:PROCESSOR_ARCHITECTURE). The ones there are: https://github.com/$repo/releases/latest"
    }
}
$asset = "asgard-cli_windows_$arch.zip"

# Where it goes. Overridable so that CI can install into a scratch directory
# and so that somebody who keeps their tools elsewhere can say so.
$dir = if ($env:ASGARD_INSTALL_DIR) { $env:ASGARD_INSTALL_DIR } else {
    Join-Path $env:LOCALAPPDATA 'Programs\asgard-cli'
}

$tmp = Join-Path ([System.IO.Path]::GetTempPath()) ("asgard-cli-" + [System.Guid]::NewGuid())
New-Item -ItemType Directory -Path $tmp -Force | Out-Null

try {
    Write-Host 'Downloading the latest asgard-cli...'
    # TLS 1.2 explicitly: Windows PowerShell 5.1 still defaults to older
    # protocols that GitHub refuses, and the failure reads as a network error.
    [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
    Invoke-WebRequest -Uri "$base/$asset" -OutFile "$tmp\$asset" -UseBasicParsing
    Invoke-WebRequest -Uri "$base/checksums.txt" -OutFile "$tmp\checksums.txt" -UseBasicParsing

    $got = (Get-FileHash -Algorithm SHA256 -Path "$tmp\$asset").Hash.ToLower()
    $want = Get-Content "$tmp\checksums.txt" | ForEach-Object { ($_ -split '\s+')[0].ToLower() }
    if ($want -notcontains $got) {
        Write-Error "Checksum mismatch: $got is not in this release's checksums.txt. Nothing was installed. Report this - it should not happen."
    }
    Write-Host '  checksum ok'

    Expand-Archive -Path "$tmp\$asset" -DestinationPath $tmp -Force
    New-Item -ItemType Directory -Path $dir -Force | Out-Null
    Copy-Item -Path "$tmp\asgard-cli.exe" -Destination "$dir\asgard-cli.exe" -Force

    # **The PATH entry is the user's, not the machine's**, for the same reason
    # the install directory is: neither needs elevation, and a tool installed
    # per-user does not surprise the next account on the machine.
    $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    if ($userPath -notlike "*$dir*") {
        [Environment]::SetEnvironmentVariable('Path', "$userPath;$dir", 'User')
        Write-Host ''
        Write-Host "Added $dir to your PATH. Open a new terminal for it to take effect."
    }
    # This session too, so the run below and anything after it in the same
    # shell find it without opening a new one.
    $env:Path = "$env:Path;$dir"

    Write-Host 'Checking it runs...'
    & "$dir\asgard-cli.exe" version

    # `doctor` exits non-zero when an optional tool is missing, which is
    # information rather than an install failure - the same reason install.sh
    # ends its own call with `|| true`.
    Write-Host ''
    & "$dir\asgard-cli.exe" doctor
    $global:LASTEXITCODE = 0
} finally {
    Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}
