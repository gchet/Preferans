param(
    [string]$Destination = '',
    [switch]$CheckOnly
)
$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot -Parent
if (!$Destination) { $Destination = Join-Path $root '.build/public-source' }
$destinationPath = [IO.Path]::GetFullPath($Destination)
if ($destinationPath -eq $root -or (Test-Path -LiteralPath $destinationPath)) {
    throw 'Choose a new, empty destination; existing files are never overwritten.'
}
Push-Location $root
try {
    $paths = @(& git -c core.quotepath=false ls-files --cached --others --exclude-standard)
    if ($LASTEXITCODE -ne 0) { throw 'Cannot list source files.' }
    $files = @($paths | Sort-Object -Unique | Where-Object { Test-Path -LiteralPath $_ -PathType Leaf })
    foreach ($path in $files) {
        $normalized = $path.Replace('\','/')
        if ($normalized -match '(^|/)(\.git|\.info|\.build|node_modules|signaling-data)(/|$)' -or
            $normalized -match '(^|/)(project\.local\.json|local\.properties|adbkey|id_rsa|id_ed25519)$' -or
            $normalized -match '\.(log|keystore|jks|p12|pfx|pem|key)$' -or
            ($normalized -like 'config/local/*' -and $normalized -ne 'config/local/.gitkeep')) {
            throw "Private/generated file included: $path"
        }
        $source = [IO.Path]::GetFullPath((Join-Path $root $path))
        if (!$source.StartsWith($root + [IO.Path]::DirectorySeparatorChar)) { throw 'Source path escapes project.' }
        if ((Get-Item -LiteralPath $source).LinkType) { throw "Review symlink before export: $path" }
    }
    $publicDefaults = Get-Content config/interface.json -Raw | ConvertFrom-Json
    $network = Get-Content config/network.json -Raw | ConvertFrom-Json
    if ($publicDefaults.roomServiceURL -or $network.turnServer -or $network.legacySignalingURL) {
        throw 'Public source defaults must not contain personal server endpoints.'
    }
    Write-Host "Snapshot candidates: $($files.Count) files. Git history and ignored files are excluded."
    if ($CheckOnly) { return }
    $null = New-Item -ItemType Directory -Path $destinationPath
    foreach ($path in $files) {
        $target = Join-Path $destinationPath $path
        $null = New-Item -ItemType Directory -Force -Path (Split-Path $target -Parent)
        Copy-Item -LiteralPath (Join-Path $root $path) -Destination $target
    }
    Write-Host "Prepared: $destinationPath"
    Write-Host 'Review and scan this directory before creating new Git history. This command does not initialize or publish Git.'
} finally { Pop-Location }
