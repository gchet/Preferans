function Read-LocalBuildConfig([string]$Root, [string]$Path = '', [bool]$Public = $false) {
    if (!$Path) { $Path = Join-Path $Root 'project.local.json' }
    if (!(Test-Path -LiteralPath $Path)) { return @{} }
    $settings = Get-Content -LiteralPath $Path -Raw | ConvertFrom-Json -AsHashtable
    # Local tool paths do not enter the binary. Public builds exclude site data.
    if ($Public) { $settings.network = @{}; $settings.deployment = @{} }
    return $settings
}

function Resolve-BuildTool([string]$Explicit, [hashtable]$Local, [string]$Key, [string]$Variable, [string]$Command = '') {
    if ($Explicit) { return $Explicit }
    if ($Local.tools -and $Local.tools[$Key]) { return [string]$Local.tools[$Key] }
    $value = [Environment]::GetEnvironmentVariable($Variable)
    if ($value) { return $value }
    if ($Command) {
        $found = Get-Command $Command -ErrorAction SilentlyContinue
        if ($found) { return $found.Source }
    }
    return ''
}

function Write-BuildNetwork([string]$Root, [hashtable]$Local) {
    $network = @{}
    $allowed = @('roomServiceURL','turnServer','turnUser','legacySignalingURL','updateAddressHint')
    if ($Local.network) {
        foreach ($key in $Local.network.Keys) {
            if ($key -notin $allowed) { throw "Unsupported network setting: $key. Do not store credentials in build configuration." }
            $value = [string]$Local.network[$key]
            if ($value -match '[|\r\n]' -or $value -match '://[^/]+@') { throw "Credentials are not allowed in network setting: $key" }
            if ($value) { $network[$key] = $value }
        }
    }
    $directory = Join-Path $Root 'config/local'
    $null = New-Item -ItemType Directory -Force -Path $directory
    ConvertTo-Json -InputObject $network | Set-Content -LiteralPath (Join-Path $directory 'build.json') -Encoding utf8NoBOM
}
