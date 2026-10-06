#requires -Version 7.0
[CmdletBinding()]
param(
    [Parameter(Mandatory)][ValidatePattern('^v[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?$')][string]$Tag,
    [Parameter(Mandatory)][ValidatePattern('^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$')][string]$Repository,
    [string]$Notes = '',
    [string]$NotesFile = '',
    [switch]$Draft,
    [switch]$CheckOnly,
    [ValidateSet('On','Off')][string]$AI = 'Off',
    [int]$VersionCode = 0,
    [string]$AndroidSdk = '',
    [string]$Zig = '',
    [string]$Gradle = ''
)
# The source tag must already be pushed. Existing private publishing is unchanged.
& (Join-Path $PSScriptRoot 'Publish-Release.ps1') @PSBoundParameters -SourceRepository
