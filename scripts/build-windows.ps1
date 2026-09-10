param(
    [string]$Version = "dev",
    [string]$OutDir = "dist",
    [ValidateSet("amd64")]
    [string]$Arch = "amd64",
    [switch]$SkipTests
)

$ErrorActionPreference = "Stop"

function Resolve-RepoPath([string]$Path) {
    if ([System.IO.Path]::IsPathRooted($Path)) {
        return $Path
    }
    return Join-Path (Resolve-Path "$PSScriptRoot\..").Path $Path
}

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    throw "Go was not found on PATH. Install Go 1.21+ before building AdbPureFlow."
}

$repoRoot = (Resolve-Path "$PSScriptRoot\..").Path
$outPath = Resolve-RepoPath $OutDir
$guiDir = Join-Path $repoRoot "GUI"
$exeName = "AdbPureFlow-$Version-windows-$Arch.exe"
$exePath = Join-Path $outPath $exeName
$checksumPath = Join-Path $outPath "checksums-$Version.txt"
$releaseReadmeSource = Join-Path $repoRoot "docs\README-WINDOWS.txt"
$releaseReadmePath = Join-Path $outPath "README-WINDOWS.txt"

New-Item -ItemType Directory -Force -Path $outPath | Out-Null

Push-Location $guiDir
try {
    if (-not $SkipTests) {
        Write-Host "Running GUI tests..."
        go test ./...
    }

    Write-Host "Building $exeName..."
    $env:GOOS = "windows"
    $env:GOARCH = $Arch
    if (-not $env:CGO_ENABLED) {
        $env:CGO_ENABLED = "1"
    }

    go build -trimpath -ldflags "-s -w -H windowsgui -X main.version=$Version" -o $exePath .
}
finally {
    Pop-Location
    Remove-Item Env:\GOOS -ErrorAction SilentlyContinue
    Remove-Item Env:\GOARCH -ErrorAction SilentlyContinue
}

if (-not (Test-Path $exePath)) {
    throw "Expected executable was not produced: $exePath"
}

if (-not (Test-Path $releaseReadmeSource)) {
    throw "Missing release readme template: $releaseReadmeSource"
}
Copy-Item -Force $releaseReadmeSource $releaseReadmePath

Get-FileHash $exePath -Algorithm SHA256 |
    ForEach-Object { "$($_.Hash.ToLowerInvariant())  $(Split-Path $_.Path -Leaf)" } |
    Set-Content -Encoding ascii $checksumPath

Write-Host "Built: $exePath"
Write-Host "Checksum: $checksumPath"
Write-Host "Release notes: $releaseReadmePath"
