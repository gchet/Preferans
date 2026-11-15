#requires -Version 7.0
<#
.SYNOPSIS
Скачивает опубликованный Release с GitHub в каталог dist и проверяет файлы.

.DESCRIPTION
Берёт Preferans.exe, Preferans.apk и version.json из Release, сверяет их с SHA256SUMS.txt,
проверяет, что versionName в version.json совпадает с тегом, и кладёт в dist
(по умолчанию — каталог dist проекта). Рядом обновляется Preferans.build.json
(флаг ИИ и SHA-256 exe), поэтому dist остаётся согласованным с тем, что выпущено.
Нужен для выпусков, которые собирает GitHub Actions (публичный gchet/Preferans):
локальный dist после них отличается от опубликованных файлов.
Черновики не скачиваются, пока не указан -AllowDraft.
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory)][ValidatePattern('^v[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?$')][string]$Tag,
    [ValidatePattern('^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$')][string]$Repository = 'gchet/Preferans',
    [string]$Destination = '',
    [switch]$AllowDraft
)
$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $false
Set-StrictMode -Version Latest
$projectRoot = Split-Path $PSScriptRoot -Parent
if (!$Destination) { $Destination = Join-Path $projectRoot 'dist' }

function Invoke-Gh([string[]]$Arguments) {
    $output = & gh @Arguments 2>&1
    if ($LASTEXITCODE -ne 0) { throw "GitHub CLI завершился с кодом ${LASTEXITCODE}: $($output -join ' ')" }
    return ($output -join "`n")
}

$null = Get-Command gh -ErrorAction Stop
$null = Invoke-Gh @('auth', 'status', '--hostname', 'github.com')
$release = (Invoke-Gh @('release', 'view', $Tag, '--repo', $Repository, '--json', 'isDraft,assets,tagName')) | ConvertFrom-Json
if ($release.isDraft -and !$AllowDraft) { throw "Release $Tag ещё черновик. Опубликуйте его или добавьте -AllowDraft." }
$binaries = @('Preferans.exe', 'Preferans.apk', 'version.json')
$names = @($release.assets | ForEach-Object { $_.name })
# Остальные файлы выпуска обновляются в dist, если они есть: иначе рядом остались бы суммы и лицензии прежней версии.
$extras = @('SHA256SUMS.txt', 'LICENSE', 'THIRD_PARTY_NOTICES.md', 'third-party-licenses.zip') | Where-Object { $names -contains $_ }
foreach ($name in $binaries + 'SHA256SUMS.txt') {
    if ($names -notcontains $name) { throw "В Release $Tag нет файла $name." }
}

$work = Join-Path $projectRoot ".build/download-$Tag"
if (Test-Path -LiteralPath $work) { Remove-Item -LiteralPath $work -Recurse -Force }
$null = New-Item -ItemType Directory -Force -Path $work
try {
    $downloadArgs = @('release', 'download', $Tag, '--repo', $Repository, '--dir', $work, '--clobber')
    foreach ($name in $binaries + $extras) { $downloadArgs += @('--pattern', $name) }
    $null = Invoke-Gh $downloadArgs

    $expected = @{}
    foreach ($line in (Get-Content -LiteralPath (Join-Path $work 'SHA256SUMS.txt') -Encoding utf8)) {
        if ($line -match '^([0-9a-fA-F]{64})\s+\*?(.+?)\s*$') { $expected[$Matches[2]] = $Matches[1].ToLowerInvariant() }
    }
    $hashes = @{}
    foreach ($name in $binaries) {
        $path = Join-Path $work $name
        if (!(Test-Path -LiteralPath $path -PathType Leaf) -or (Get-Item -LiteralPath $path).Length -eq 0) { throw "Файл $name не скачан." }
        $hashes[$name] = (Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant()
        if (!$expected.ContainsKey($name)) { throw "Для $name нет суммы в SHA256SUMS.txt." }
        if ($hashes[$name] -ne $expected[$name]) { throw "Контрольная сумма $name не совпадает с SHA256SUMS.txt." }
    }
    foreach ($name in $extras) {
        if ($name -eq 'SHA256SUMS.txt' -or !$expected.ContainsKey($name)) { continue }
        if ((Get-FileHash -LiteralPath (Join-Path $work $name) -Algorithm SHA256).Hash.ToLowerInvariant() -ne $expected[$name]) { throw "Контрольная сумма $name не совпадает с SHA256SUMS.txt." }
    }
    $manifest = Get-Content -LiteralPath (Join-Path $work 'version.json') -Raw -Encoding utf8 | ConvertFrom-Json
    if ($manifest.versionName -ne $Tag.Substring(1)) { throw "versionName ($($manifest.versionName)) не соответствует тегу $Tag." }
    if ($manifest.apk -ne 'Preferans.apk') { throw 'version.json указывает не на Preferans.apk.' }

    $null = New-Item -ItemType Directory -Force -Path $Destination
    foreach ($name in $binaries + $extras) {
        $target = Join-Path $Destination $name
        try {
            Copy-Item -LiteralPath (Join-Path $work $name) -Destination "$target.new" -Force
            Move-Item -LiteralPath "$target.new" -Destination $target -Force
        } catch {
            Remove-Item -LiteralPath "$target.new" -Force -ErrorAction SilentlyContinue
            throw "Не удалось обновить $target (файл занят запущенной программой?): $($_.Exception.Message)"
        }
    }
    [ordered]@{ aiEnabled = [bool]$manifest.aiEnabled; sha256 = $hashes['Preferans.exe'] } | ConvertTo-Json |
        Set-Content -LiteralPath (Join-Path $Destination 'Preferans.build.json') -Encoding utf8NoBOM
    Write-Host "Скачано: $Repository $Tag -> $Destination (версия $($manifest.versionName), код $($manifest.versionCode), ИИ: $($manifest.aiEnabled))."
    foreach ($name in $binaries) { Write-Host ("  {0}  {1}" -f $hashes[$name], $name) }
} finally {
    Remove-Item -LiteralPath $work -Recurse -Force -ErrorAction SilentlyContinue
}