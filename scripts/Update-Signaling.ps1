param(
    [switch]$SkipWeb,
    [switch]$Deploy,
    [ValidatePattern('^[a-zA-Z0-9_.@-]*$')][string]$Server = '',
    [ValidatePattern('^/[a-zA-Z0-9/_-]+$')][string]$RemoteBinary = '/usr/local/bin/preferans-signaling',
    [ValidatePattern('^[a-zA-Z0-9_-]+$')][string]$Service = 'preferans-signaling'
)
$ErrorActionPreference = 'Stop'
$project = Split-Path $PSScriptRoot -Parent
. (Join-Path $PSScriptRoot 'Build-Config.ps1')
$localSettings = Read-LocalBuildConfig $project
if (!$Server -and $localSettings.deployment) { $Server = [string]$localSettings.deployment.server }
if ($Deploy -and (!$Server -or $Server -notmatch '^[a-zA-Z0-9_.@-]+$')) { throw 'Specify -Server user@host or deployment.server in project.local.json.' }
Write-BuildNetwork $project $localSettings
$output = Join-Path $project 'dist/preferans-signaling'
New-Item -ItemType Directory -Force (Join-Path $project 'dist') | Out-Null
$oldOS = $env:GOOS; $oldArch = $env:GOARCH; $oldCGO = $env:CGO_ENABLED
Push-Location $project
try {
    if (!$SkipWeb) {
        Push-Location (Join-Path $project 'web')
        try { & npm.cmd run build:admin; if ($LASTEXITCODE -ne 0) { throw 'Admin web build failed.' } }
        finally { Pop-Location }
    }
    $env:GOOS = 'linux'; $env:GOARCH = 'amd64'; $env:CGO_ENABLED = '0'
    & go build -buildvcs=false -trimpath -ldflags '-s -w' -o $output ./cmd/signaling
    if ($LASTEXITCODE -ne 0) { throw 'Signaling build failed.' }
} finally {
    $env:GOOS = $oldOS; $env:GOARCH = $oldArch; $env:CGO_ENABLED = $oldCGO
    Pop-Location
}
Write-Host "Built: $output"
if (!$Deploy) { return }
$remoteTemp = '/tmp/preferans-signaling-' + [Guid]::NewGuid().ToString('N')
& scp $output "${Server}:$remoteTemp"
if ($LASTEXITCODE -ne 0) { throw 'Upload failed; the running server was not changed.' }
# All substituted remote values are constrained by validation or generated here.
$remoteScript = @'
set -e
bin='__BIN__'
tmp='__TMP__'
svc='__SERVICE__'
if sudo test -f "$bin"; then sudo cp -p "$bin" "$bin.previous"; fi
sudo install -o root -g root -m 755 "$tmp" "$bin.new"
sudo mv -f "$bin.new" "$bin"
rm -f "$tmp"
if sudo systemctl restart "$svc" && sudo systemctl is-active --quiet "$svc"; then
  sudo systemctl status "$svc" --no-pager
else
  echo 'Restart failed. Restoring previous binary.' >&2
  if sudo test -f "$bin.previous"; then
    sudo cp -p "$bin.previous" "$bin.rollback"
    sudo mv -f "$bin.rollback" "$bin"
    sudo systemctl restart "$svc"
  fi
  exit 1
fi
'@
$remoteScript = $remoteScript.Replace('__BIN__',$RemoteBinary).Replace('__TMP__',$remoteTemp).Replace('__SERVICE__',$Service).Replace("`r`n","`n")
& ssh -t $Server $remoteScript
if ($LASTEXITCODE -ne 0) { throw 'Deployment failed. Check the remote service status.' }
Write-Host 'Signaling server updated. Configuration and room storage were preserved.'
