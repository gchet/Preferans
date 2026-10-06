#requires -Version 7.0
param([string]$Zig = '')
$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot -Parent
. (Join-Path $PSScriptRoot 'Build-Config.ps1')
$local = Read-LocalBuildConfig $root
$Zig = Resolve-BuildTool $Zig $local 'zig' 'ZIG' 'zig'
if (!$Zig) { throw 'Set -Zig or install Zig on PATH.' }
$destination = Join-Path $root 'cmd/preferans/bullet_windows_amd64.syso'
Push-Location (Join-Path $root 'native/windows')
try {
    & $Zig rc /:output-format coff /:target x64 /fo $destination bullet.rc
    if ($LASTEXITCODE -ne 0) { throw 'Windows resource compilation failed.' }
} finally { Pop-Location }
