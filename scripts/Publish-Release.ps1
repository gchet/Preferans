#requires -Version 7.0
[CmdletBinding()]
param(
    [Parameter(Mandatory)][ValidatePattern('^v[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?$')][string]$Tag,
    [ValidatePattern('^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$')][string]$Repository = 'GC-Preferans/Preferans',
    [string]$Notes = '',
    [string]$NotesFile = '',
    [switch]$Draft,
    [switch]$CheckOnly,
    [switch]$SourceRepository,
    [ValidateSet('On','Off')][string]$AI = 'Off',
    [ValidateRange(0,2147483647)][int]$VersionCode = 0,
    [string]$AndroidSdk = '',
    [string]$Zig = '',
    [string]$Gradle = ''
)
$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $false
Set-StrictMode -Version Latest
$projectRoot = Split-Path $PSScriptRoot -Parent

function Invoke-Gh([string[]]$Arguments) {
    $output = & gh @Arguments
    if ($LASTEXITCODE -ne 0) { throw "GitHub CLI завершился с кодом $LASTEXITCODE" }
    return ($output -join "`n")
}
function Read-DistributionRepository {
    $info = (Invoke-Gh @('api', "repos/$Repository")) | ConvertFrom-Json
    if (!$info.permissions.push) { throw 'Нет права публикации в выбранном репозитории.' }
    if ($SourceRepository) {
        if ($info.private) { throw 'The public source release requires a public repository.' }
        $null = Invoke-Gh @('api', "repos/$Repository/git/ref/tags/$Tag")
        $remoteCommit = (Invoke-Gh @('api', "repos/$Repository/commits/$Tag", '--jq', '.sha')).Trim()
        if ($remoteCommit -ne $sourceCommit) { throw 'Push the matching source commit and tag before publishing.' }
        return $sourceCommit
    }
    if (!$info.private) { throw 'Публикация разрешена только в приватный репозиторий.' }
    # Empty repositories have no commit. Nonempty repositories must contain
    # only README.md; never target the local source repository's commit/tag.
    if ($info.size -eq 0) {
        $branches = @((Invoke-Gh @('api', "repos/$Repository/branches")) | ConvertFrom-Json)
        if ($branches.Count -eq 0) { return '' }
    }
    $branch = [uri]::EscapeDataString($info.default_branch)
    $head = (Invoke-Gh @('api', "repos/$Repository/branches/$branch")) | ConvertFrom-Json
    $sha = $head.commit.sha
    $tree = (Invoke-Gh @('api', "repos/$Repository/git/trees/${sha}?recursive=1")) | ConvertFrom-Json
    if ($tree.truncated) { throw 'Невозможно проверить полное содержимое репозитория.' }
    $unexpected = @($tree.tree | Where-Object { $_.path -ne 'README.md' -or $_.type -ne 'blob' })
    if ($unexpected.Count) { throw 'Репозиторий распространения должен содержать только README.md.' }
    return $sha
}

Push-Location $projectRoot
try {
    foreach ($tool in @('gh','git','pwsh')) { $null = Get-Command $tool -ErrorAction Stop }
    $null = Invoke-Gh @('auth','status','--hostname','github.com')
    $sourceCommit = (& git rev-parse --verify "refs/tags/$Tag^{commit}")
    if ($LASTEXITCODE -ne 0) { throw "Локальный тег $Tag не найден. Сначала отметьте нужную версию в локальном Git." }
    $headCommit = (& git rev-parse HEAD)
    if ($LASTEXITCODE -ne 0 -or $sourceCommit -ne $headCommit) { throw 'HEAD должен совпадать с локальным тегом выпуска.' }
    # Documentation and this publishing script do not enter the binaries.
    $buildInputs = @('android','cmd','config','locales','internal','mobile','native','web','build.ps1','scripts/Build-Config.ps1','go.mod','go.sum')
    $dirty = @(& git status --porcelain --untracked-files=all -- @buildInputs)
    if ($LASTEXITCODE -ne 0 -or $dirty.Count) { throw 'Есть незакоммиченные изменения в исходниках сборки. Создайте коммит и новый тег.' }
    if ($SourceRepository -and @(& git status --porcelain --untracked-files=all).Count) { throw 'Commit all public source changes before publishing.' }
    if ($Notes -and $NotesFile) { throw 'Укажите только один параметр: Notes или NotesFile.' }
    if ($NotesFile) { $Notes = Get-Content -LiteralPath $NotesFile -Raw -Encoding utf8 }
    if (!$Notes.Trim()) { $Notes = "Версия $Tag для Windows и Android." }
    $remoteHead = Read-DistributionRepository
    $releases = @((Invoke-Gh @('api', '--paginate', "repos/$Repository/releases?per_page=100", '--jq', '.[].tag_name')) -split "`n")
    if ($releases -contains $Tag) { throw "Release $Tag уже существует. Перезапись запрещена; используйте новый тег." }
    Write-Host "AI: $AI (по умолчанию публикация без ИИ)."
    Write-Host "Checked: $Repository; source repository: $SourceRepository; tag $Tag, commit $sourceCommit."
    if ($CheckOnly) {
        Write-Host 'Проверка завершена. Сборка, изменение репозитория и публикация не выполнялись.'
        return
    }

    $previousCode = 0
    $versionPath = Join-Path $projectRoot 'dist/version.json'
    if (Test-Path -LiteralPath $versionPath) {
        $previousCode = [long](Get-Content -LiteralPath $versionPath -Raw | ConvertFrom-Json).versionCode
    }
    if (!$VersionCode) { $VersionCode = [int][Math]::Max([DateTimeOffset]::UtcNow.ToUnixTimeSeconds(), $previousCode + 1) }
    if ($VersionCode -le $previousCode) { throw 'VersionCode должен быть больше кода предыдущей локальной сборки.' }
    $versionName = $Tag.Substring(1)
    $buildArgs = @('-NoProfile','-File',(Join-Path $projectRoot 'build.ps1'),'-Target','All',
        '-AI',$AI,'-VersionCode',"$VersionCode",'-VersionName',$versionName,'-AndroidSdk',$AndroidSdk,'-Zig',$Zig)
    if ($Gradle) { $buildArgs += @('-Gradle',$Gradle) }
    if ($SourceRepository) { $buildArgs += '-Public' }
    & pwsh @buildArgs
    if ($LASTEXITCODE -ne 0) { throw 'Сборка не завершена. Release не создан.' }
    $afterCommit = & git rev-parse HEAD
    $dirty = @(& git status --porcelain --untracked-files=all -- @buildInputs)
    if ($LASTEXITCODE -ne 0 -or $afterCommit -ne $sourceCommit -or $dirty.Count) {
        throw 'Исходники изменились во время сборки. Публикация остановлена.'
    }
    $manifest = Get-Content -LiteralPath $versionPath -Raw | ConvertFrom-Json
    if ($manifest.versionName -ne $versionName -or $manifest.versionCode -ne $VersionCode -or $manifest.apk -ne 'Preferans.apk') {
        throw 'Манифест Android не соответствует выпускаемой версии.'
    }

    $expectedAI = $AI -eq 'On'
    if ($manifest.aiEnabled -ne $expectedAI) { throw 'APK AI build flag mismatch.' }
    $windowsMetadata = Get-Content -LiteralPath (Join-Path $projectRoot 'dist/Preferans.build.json') -Raw | ConvertFrom-Json
    $windowsHash = (Get-FileHash -LiteralPath (Join-Path $projectRoot 'dist/Preferans.exe') -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($windowsMetadata.aiEnabled -ne $expectedAI -or $windowsMetadata.sha256 -ne $windowsHash) { throw 'Windows AI build flag or binary hash mismatch.' }

    $packageDir = Join-Path $projectRoot ".build/releases/$Tag"
    $null = New-Item -ItemType Directory -Force -Path $packageDir
    $files = @('Preferans.exe','Preferans.apk','version.json')
    foreach ($name in $files) {
        $source = Join-Path $projectRoot "dist/$name"
        if (!(Test-Path -LiteralPath $source -PathType Leaf) -or (Get-Item -LiteralPath $source).Length -eq 0) { throw "Нет готового файла $name" }
        Copy-Item -LiteralPath $source -Destination (Join-Path $packageDir $name)
    }
    foreach ($name in @('LICENSE','THIRD_PARTY_NOTICES.md')) {
        Copy-Item -LiteralPath (Join-Path $projectRoot $name) -Destination (Join-Path $packageDir $name)
        $files += $name
    }
    Compress-Archive -Path (Join-Path $projectRoot 'docs/licenses') -DestinationPath (Join-Path $packageDir 'third-party-licenses.zip') -Force
    $files += 'third-party-licenses.zip'
    $checksums = foreach ($name in $files) {
        $hash = (Get-FileHash -LiteralPath (Join-Path $packageDir $name) -Algorithm SHA256).Hash.ToLowerInvariant()
        "$hash  $name"
    }
    $checksums | Set-Content -LiteralPath (Join-Path $packageDir 'SHA256SUMS.txt') -Encoding utf8NoBOM
    $files += 'SHA256SUMS.txt'
    $notesPath = Join-Path $packageDir 'release-notes.md'
    "$Notes`n`nWindows: Preferans.exe (требуется WebView2 Runtime).`nAndroid: Preferans.apk (Android 8+, ARM64; прямая установка, debug-подпись).`n`nВерсия Android: $versionName ($VersionCode). ИИ: $AI.`nSHA-256 файлов: SHA256SUMS.txt." |
        Set-Content -LiteralPath $notesPath -Encoding utf8NoBOM

    # Recheck immediately before the first write to GitHub.
    $remoteHead = Read-DistributionRepository
    if (!$remoteHead) {
        $readme = @'
# Преферанс — готовые версии

Этот приватный репозиторий содержит только готовые приложения и инструкции.
Исходники проекта здесь не публикуются.

Откройте **Releases**, выберите версию и скачайте нужное вложение:

- **Preferans.exe** — Windows x64. Требуется Microsoft Edge WebView2 Runtime.
- **Preferans.apk** — Android 8 и новее, ARM64. Разрешите установку из выбранного браузера или файлового менеджера.
- **version.json** — манифест для существующего обновления Android по Wi-Fi.
- **SHA256SUMS.txt** — контрольные суммы файлов.

Для доступа примите приглашение владельца и войдите в свой аккаунт GitHub.
Архивы Source code содержат только эту инструкцию — скачивать их для установки не нужно.
APK подписан текущим debug-ключом разработчика; устанавливайте обновление поверх предыдущей версии.
Пароль подключения к игре передаёт организатор отдельно.
'@
        $payloadPath = Join-Path $packageDir 'repository-readme.json'
        @{message='Add installation instructions';content=[Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($readme))} |
            ConvertTo-Json | Set-Content -LiteralPath $payloadPath -Encoding utf8NoBOM
        $null = Invoke-Gh @('api', '--method','PUT',"repos/$Repository/contents/README.md",'--input',$payloadPath)
        $remoteHead = Read-DistributionRepository
    }
    $releaseArgs = @('release','create',$Tag,'--repo',$Repository,'--target',$remoteHead,
        '--title',"Преферанс $Tag",'--notes-file',$notesPath,'--draft')
    if ($SourceRepository) { $releaseArgs += '--verify-tag' }
    if ($Tag -match '-') { $releaseArgs += '--prerelease' }
    $created = $false
    try {
        $null = Invoke-Gh $releaseArgs
        $created = $true
        foreach ($name in $files) {
            $null = Invoke-Gh @('release','upload',$Tag,(Join-Path $packageDir $name),'--repo',$Repository)
        }
        $release = (Invoke-Gh @('release','view',$Tag,'--repo',$Repository,'--json','assets,isDraft')) | ConvertFrom-Json
        if (!$release.isDraft -or @($release.assets).Count -ne $files.Count) { throw 'Неожиданный состав вложений или состояние Release.' }
        foreach ($name in $files) {
            $asset = @($release.assets | Where-Object name -eq $name)
            if ($asset.Count -ne 1 -or $asset[0].size -ne (Get-Item -LiteralPath (Join-Path $packageDir $name)).Length) { throw "Вложение $name не прошло проверку." }
        }
        if (!$Draft) { $null = Invoke-Gh @('release','edit',$Tag,'--repo',$Repository,'--draft=false') }
        Write-Host "Готово: https://github.com/$Repository/releases/tag/$Tag"
        if ($Draft) { Write-Host 'Release оставлен черновиком; публикация не выполнена.' }
    } catch {
        if ($created) { Write-Warning "Проверьте состояние созданного Release. Файлы для восстановления: $packageDir. Ранее существовавшие версии не изменялись." }
        throw
    }
} finally {
    Pop-Location
}
