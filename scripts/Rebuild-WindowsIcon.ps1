#requires -Version 7.0
# Пересобирает ресурс значка Windows (cmd/preferans/bullet_windows_amd64.syso) из native/windows/bullet.ico.
# Сами картинки рисует scripts/render-icons.mjs из native/windows/icon.svg.
# Компилятор ресурсов: windres из MinGW-w64 (-Mingw или tools.mingw), иначе zig rc.
param([string]$Zig = '', [string]$Mingw = '')
$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot -Parent
. (Join-Path $PSScriptRoot 'Build-Config.ps1')
$local = Read-LocalBuildConfig $root
$Mingw = Resolve-BuildTool $Mingw $local 'mingw' 'MINGW' ''
$destination = Join-Path $root 'cmd/preferans/bullet_windows_amd64.syso'
Push-Location (Join-Path $root 'native/windows')
try {
    if ($Mingw) {
        $windres = @($Mingw, (Join-Path $Mingw 'bin'), (Join-Path $Mingw 'mingw64\bin')) |
            ForEach-Object { Join-Path $_ 'windres.exe' } | Where-Object { Test-Path -LiteralPath $_ } | Select-Object -First 1
        if (!$windres) { throw "В '$Mingw' не найден windres.exe" }
        & $windres -O coff -F pe-x86-64 -i bullet.rc -o $destination
    } else {
        $Zig = Resolve-BuildTool $Zig $local 'zig' 'ZIG' 'zig'
        if (!$Zig) { throw 'Укажите -Mingw (или tools.mingw) либо -Zig, либо поставьте Zig в PATH.' }
        & $Zig rc /:output-format coff /:target x64 /fo $destination bullet.rc
    }
    if ($LASTEXITCODE -ne 0) { throw 'Windows resource compilation failed.' }
} finally { Pop-Location }