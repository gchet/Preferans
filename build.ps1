param(
    [ValidateSet('All','Web','Windows','Android','Test')][string]$Target = 'All',
    [string]$AndroidSdk = '',
    [string]$Zig = '',
    [string]$Gradle = '',
    [switch]$SkipWeb,
    [ValidateSet('On','Off')][string]$AI = 'On',
    [switch]$Emulator,
    [switch]$NetworkTest,
    [int]$VersionCode = 0,
    [string]$VersionName = '',
    [string]$WindowsOutput = '',
    [switch]$Public,
    [string]$LocalConfig = ''
)
$ErrorActionPreference = 'Stop'
Set-Location $PSScriptRoot
. (Join-Path $PSScriptRoot 'scripts/Build-Config.ps1')
$localSettings = Read-LocalBuildConfig $PSScriptRoot $LocalConfig $Public.IsPresent
$AndroidSdk = Resolve-BuildTool $AndroidSdk $localSettings 'androidSdk' 'ANDROID_HOME'
if (!$AndroidSdk) { $AndroidSdk = $env:ANDROID_SDK_ROOT }
if (!$AndroidSdk -and $env:LOCALAPPDATA) { $AndroidSdk = Join-Path $env:LOCALAPPDATA 'Android/Sdk' }
$Zig = Resolve-BuildTool $Zig $localSettings 'zig' 'ZIG' 'zig'
$Gradle = Resolve-BuildTool $Gradle $localSettings 'gradle' 'GRADLE' ''
if ($Public -and $SkipWeb) { throw '-Public requires a fresh WebUI build; remove -SkipWeb.' }
Write-BuildNetwork $PSScriptRoot $localSettings
$aiEnabled = $AI -eq 'On'
$aiTags = if ($aiEnabled) { @() } else { @('-tags','noai') }
Write-Host "AI: $AI"
if ($VersionCode -le 0) { $VersionCode = [int][DateTimeOffset]::UtcNow.ToUnixTimeSeconds() }
if (!$VersionName) { $VersionName = "0.1.$VersionCode" }
function Check { if ($LASTEXITCODE -ne 0) { throw "Команда завершилась с кодом $LASTEXITCODE" } }
New-Item -ItemType Directory -Force .build,dist | Out-Null
if (!$SkipWeb -and $Target -in @('All','Web','Windows','Android')) {
    Push-Location web
    try { & npm.cmd ci; Check; & npm.cmd run build; Check } finally { Pop-Location }
}
if ($Target -in @('All','Test')) {
    & go test ./internal/...; Check
    if (!$aiEnabled) { & go test -tags noai ./internal/app -run TestNoAI -count=1; Check }
}
if ($Target -in @('All','Windows')) {
    if (!(Test-Path -LiteralPath $Zig)) { throw 'Укажите -Zig с путём к zig.exe' }
    $cc = Join-Path $PSScriptRoot '.build\cc.cmd'
    $cxx = Join-Path $PSScriptRoot '.build\cxx.cmd'
    Set-Content -LiteralPath $cc -Encoding ascii -Value "@`"$Zig`" cc -target x86_64-windows-gnu %*"
    Set-Content -LiteralPath $cxx -Encoding ascii -Value "@`"$Zig`" c++ -target x86_64-windows-gnu %*"
    $oldCC=$env:CC; $oldCXX=$env:CXX; $oldCGO=$env:CGO_ENABLED; $oldFlags=$env:CGO_CXXFLAGS
    $windowsName = if ($NetworkTest) { 'dist/PreferansNetCheck.exe' } else { 'dist/Preferans.exe' }
    if ($WindowsOutput) { $windowsName = $WindowsOutput }
    # cgo uses Zig's external linker.  Its Windows target defaults to the
    # Console subsystem, so explicitly request the GUI subsystem as well as
    # the Go linker's setting; otherwise Explorer opens a console window.
    $linkFlags = if ($NetworkTest) { '-H windowsgui -X main.appMode=netcheck -extldflags=-Wl,--subsystem,windows' } else { '-H windowsgui -extldflags=-Wl,--subsystem,windows' }
    try { $env:CC=$cc; $env:CXX=$cxx; $env:CGO_ENABLED='1'; $env:CGO_CXXFLAGS='-O2 -g -I'+(Join-Path $PSScriptRoot 'native/include'); & go build @aiTags -buildvcs=false -trimpath -ldflags $linkFlags -o $windowsName ./cmd/preferans; Check }
    finally { $env:CC=$oldCC; $env:CXX=$oldCXX; $env:CGO_ENABLED=$oldCGO; $env:CGO_CXXFLAGS=$oldFlags }
    $reported = (& $windowsName -build-info | Out-String) | ConvertFrom-Json
    if ($LASTEXITCODE -ne 0 -or $reported.aiEnabled -ne $aiEnabled) { throw 'Windows AI build flag mismatch.' }
    [ordered]@{aiEnabled=$aiEnabled; sha256=(Get-FileHash -LiteralPath $windowsName -Algorithm SHA256).Hash.ToLowerInvariant()} |
        ConvertTo-Json | Set-Content -LiteralPath ([IO.Path]::ChangeExtension($windowsName, '.build.json')) -Encoding utf8NoBOM
}
if ($Target -in @('All','Android')) {
    if (!$AndroidSdk -or !(Test-Path -LiteralPath (Join-Path $AndroidSdk 'ndk'))) { throw 'Set -AndroidSdk, ANDROID_HOME or tools.androidSdk in project.local.json; SDK with NDK is required.' }
    $oldSDK=$env:ANDROID_HOME; $oldNDK=$env:ANDROID_NDK_HOME; $oldPath=$env:PATH
    try {
        $env:ANDROID_HOME=$AndroidSdk
        $env:ANDROID_NDK_HOME=(Get-ChildItem (Join-Path $AndroidSdk 'ndk') -Directory | Sort-Object Name -Descending | Select-Object -First 1).FullName
        New-Item -ItemType Directory -Force android/app/libs | Out-Null
        & go build -buildvcs=false -o .build/gobind.exe golang.org/x/mobile/cmd/gobind; Check
        $env:PATH=(Join-Path $PSScriptRoot '.build')+';'+$env:PATH
        # anet v0.0.5 (Pion's Android interface discovery) uses net.zoneCache.
        $mobileTarget='android/arm64'; $androidAbi='arm64-v8a'; $apkName='Preferans.apk'
        if ($Emulator) { $mobileTarget='android/amd64'; $androidAbi='x86_64'; $apkName='Preferans-emulator.apk' }
        if ($NetworkTest) { $apkName= if ($Emulator) { 'PreferansNetCheck-emulator.apk' } else { 'PreferansNetCheck.apk' } }
        & go tool gomobile bind @aiTags -target $mobileTarget -androidapi 26 -ldflags '-checklinkname=0 -s -w' -o android/app/libs/preferans.aar ./mobile; Check
        if (!$Gradle) { $Gradle = Join-Path $PSScriptRoot 'android/gradlew.bat' }
        & $Gradle -p android "-PpreferansAbi=$androidAbi" "-PnetworkTest=$($NetworkTest.IsPresent.ToString().ToLowerInvariant())" "-PpreferansVersionCode=$VersionCode" "-PpreferansVersionName=$VersionName" assembleDebug --console=plain; Check
        Copy-Item -LiteralPath android/app/build/outputs/apk/debug/app-debug.apk -Destination (Join-Path 'dist' $apkName)
        [ordered]@{versionCode=$VersionCode; versionName=$VersionName; apk=$apkName; aiEnabled=$aiEnabled} | ConvertTo-Json | Set-Content -LiteralPath (Join-Path 'dist' 'version.json') -Encoding utf8NoBOM
    } finally { $env:ANDROID_HOME=$oldSDK; $env:ANDROID_NDK_HOME=$oldNDK; $env:PATH=$oldPath }
}
