<#
Локальная проверка по изменениям: выполняет только то, что затронуто правками с последнего успешного прогона
каждой стадии (Go-тесты, сборка интерфейса, аудит npm, EXE для тестов, Android, Playwright). Состояние хранится в
.build/checks-state.json (в git не попадает). Подробности: docs/TESTING.md, раздел «Проверка по изменениям».

  pwsh ./scripts/Run-Checks.ps1           # только затронутое изменениями
  pwsh ./scripts/Run-Checks.ps1 -Plan     # показать, что будет выполнено, ничего не запуская
  pwsh ./scripts/Run-Checks.ps1 -All      # всё целиком, как раньше (обязательно перед выпуском версии)
  pwsh ./scripts/Run-Checks.ps1 -Android  # добавить сборку Android, даже если android/ и mobile/ не менялись
#>
param(
  [switch]$All,
  [switch]$Plan,
  [switch]$Android,
  [switch]$NoAudit,
  [string]$Log = '.build/checks-latest.log'
)
$ErrorActionPreference = 'Continue'
Set-Location (Split-Path $PSScriptRoot -Parent)
[Console]::OutputEncoding = [Text.Encoding]::UTF8
New-Item -ItemType Directory -Force .build | Out-Null
$Log = [IO.Path]::GetFullPath($Log, $PWD.Path)
$statePath = '.build/checks-state.json'
$state = @{ stages = @{} }
if (Test-Path $statePath) {
  try { $state = Get-Content $statePath -Raw | ConvertFrom-Json -AsHashtable } catch { $state = @{ stages = @{} } }
  if (-not $state -or -not $state.stages) { $state = @{ stages = @{} } }
}
if (-not $Plan) { "start $(Get-Date)" | Out-File $Log }

# Снимок: путь -> хэш содержимого для всех файлов проекта, кроме игнорируемых git.
$snap = @{}
$sha = [Security.Cryptography.SHA1]::Create()
foreach ($p in (git -c core.quotepath=false ls-files -co --exclude-standard)) {
  if (-not $p -or -not (Test-Path -LiteralPath $p -PathType Leaf)) { continue }
  try { $snap[$p] = [BitConverter]::ToString($sha.ComputeHash([IO.File]::ReadAllBytes((Join-Path $PWD.Path $p)))) } catch { }
}

# Файлы входа стадии, изменившиеся с её последнего успешного прогона; $null, если успешного прогона не было.
function Get-Changed([string]$name, [string]$rx) {
  $old = $state.stages[$name]
  if (-not $old) { return $null }
  $oldFiles = $old.files
  $changed = [Collections.Generic.List[string]]::new()
  foreach ($p in $snap.Keys) {
    if ($p -match $rx -and (-not $oldFiles.ContainsKey($p) -or $oldFiles[$p] -ne $snap[$p])) { $changed.Add($p) }
  }
  foreach ($p in $oldFiles.Keys) { if ($p -match $rx -and -not $snap.ContainsKey($p)) { $changed.Add($p) } }
  return , @($changed | Sort-Object)
}
function Save-Stage([string]$name, [string]$rx, [hashtable]$extra = @{}) {
  $files = @{}
  foreach ($p in $snap.Keys) { if ($p -match $rx) { $files[$p] = $snap[$p] } }
  $entry = @{ files = $files; at = (Get-Date).ToString('s') }
  foreach ($k in $extra.Keys) { $entry[$k] = $extra[$k] }
  $state.stages[$name] = $entry
  $state | ConvertTo-Json -Depth 6 | Set-Content $statePath -Encoding utf8
}
function Run([string]$label, [scriptblock]$cmd) {
  "--- $label" | Tee-Object -FilePath $Log -Append | Out-Host
  & $cmd *>&1 | Tee-Object -FilePath $Log -Append | Out-Host
  $code = $LASTEXITCODE
  "$label exit $code" | Tee-Object -FilePath $Log -Append | Out-Host
  return ($code -eq 0)
}
function Why([string]$name, [string]$rx, [scriptblock]$need = { $false }) {
  $changed = Get-Changed $name $rx
  if ($All) { return '-All' }
  if ($null -eq $changed) { return 'нет данных о прошлом успешном прогоне' }
  if ($changed.Count -gt 0) { return "изменено файлов: $($changed.Count) ($((@($changed) | Select-Object -First 3) -join ', '))" }
  if (& $need) { return 'нужного результата нет или он устарел' }
  return $null
}

$results = [ordered]@{}
$failed = $false
function Stage([string]$name, [string]$rx, [scriptblock]$body, [scriptblock]$need = { $false }, [hashtable]$extra = @{}) {
  $reason = Why $name $rx $need
  if (-not $reason) { Write-Host "[$name] пропуск: входные файлы не менялись"; $results[$name] = 'пропущено'; return }
  Write-Host "[$name] запуск: $reason"
  if ($Plan) { $results[$name] = 'будет выполнено'; return }
  if (& $body) { Save-Stage $name $rx $extra; $results[$name] = 'ok' } else { $results[$name] = 'ОШИБКА'; $script:failed = $true }
}

$goRx = '\.go$|^go\.(mod|sum)$|^(locales|config)/.*\.json$|^native/'
Stage 'go' $goRx {
  $a = Run 'go test' { go test ./config ./internal/... ./mobile }
  $b = Run 'go vet' { go vet ./config ./internal/... ./cmd/signaling ./mobile }
  $c = Run 'go test noai' { go test -tags noai ./internal/app -run TestNoAI -count=1 }
  return ($a -and $b -and $c)
}

$webRx = '^web/[^/]+$|^web/(src|public)/|^(locales|config)/|^scripts/check-'
Stage 'web' $webRx {
  Push-Location web
  try {
    if (-not (Test-Path node_modules)) { if (-not (Run 'npm ci' { npm.cmd ci })) { return $false } }
    return (Run 'npm run build' { npm.cmd run build })
  } finally { Pop-Location }
} { -not (Test-Path web/dist/index.html) }

if (-not $NoAudit) {
  Stage 'audit' '^web/package(-lock)?\.json$' {
    Push-Location web
    try { return (Run 'npm audit' { npm.cmd audit --audit-level=high }) } finally { Pop-Location }
  } {
    $at = $state.stages['audit'].at
    return (-not $at) -or (((Get-Date) - [datetime]$at).TotalDays -gt 7)
  }
}

$sigRx = '^(cmd/signaling/|internal/rooms/)(?!.*_test\.go$).*\.go$|^go\.(mod|sum)$'
Stage 'signaling' $sigRx {
  return (Run 'build signaling-test.exe' { go build -buildvcs=false -o .build/signaling-test.exe ./cmd/signaling })
} { -not (Test-Path .build/signaling-test.exe) }

$exeRx = '^(?!.*_test\.go$)(.*\.go$|go\.(mod|sum)$|(locales|config)/.*\.json$|native/|web/[^/]+$|web/(src|public)/)'
Stage 'exe' $exeRx {
  $mb = 'J:\Programs\winlibs\mingw64\bin'
  $env:PATH = "$mb;$env:PATH"; $env:CC = "$mb\gcc.exe"; $env:CXX = "$mb\g++.exe"; $env:CGO_ENABLED = '1'
  $env:CGO_CXXFLAGS = '-O2 -g -I' + (Join-Path $PWD.Path 'native/include')
  if (-not (Test-Path .build/ui-test-config.json)) { node scripts/prepare-ui-tests.mjs | Out-Host }
  return (Run 'build preferans-test.exe' { go build -buildvcs=false -trimpath -ldflags '-s -w -H windowsgui -extldflags=-static' -o .build/preferans-test.exe ./cmd/preferans })
} { -not (Test-Path .build/preferans-test.exe) }

# Android: компиляция Kotlin с привязкой gomobile; dist/ не затрагивается (APK не собирается).
$androidBody = {
  $sdk = 'J:/Android/Sdk'
  $env:ANDROID_HOME = $sdk
  $env:ANDROID_NDK_HOME = (Get-ChildItem (Join-Path $sdk 'ndk') -Directory | Sort-Object Name -Descending | Select-Object -First 1).FullName
  New-Item -ItemType Directory -Force android/app/libs | Out-Null
  $version = (Get-Content VERSION -Raw).Trim()
  if (-not (Run 'build gobind' { go build -buildvcs=false -o .build/gobind.exe golang.org/x/mobile/cmd/gobind })) { return $false }
  $env:PATH = (Join-Path $PWD.Path '.build') + ';' + $env:PATH
  if (-not (Run 'gomobile bind' { go tool gomobile bind -tags noai -target android/arm64 -androidapi 26 -ldflags '-checklinkname=0 -s -w' -o android/app/libs/preferans.aar ./mobile })) { return $false }
  return (Run 'gradle compileDebugKotlin' { android/gradlew.bat -p android '-PpreferansAbi=arm64-v8a' '-PpreferansVersionCode=1' "-PpreferansVersionName=$version" :app:compileDebugKotlin --console=plain })
}
Stage 'android' '^android/|^mobile/|^locales/' $androidBody { [bool]$Android }
# Playwright: какие тесты затронуты изменениями. Неизвестные пути (CSS, main.ts, локали, конфиги, помощники тестов)
# всегда означают полный прогон; Go-код без изменений интерфейса даёт сокращённый набор.
$pwRx = '.'
$pwChanged = Get-Changed 'playwright' $pwRx
$full = $false; $backend = $false
$specs = [Collections.Generic.SortedSet[string]]::new(); $grep = [Collections.Generic.List[string]]::new()
if ($All -or $null -eq $pwChanged) { $full = $true; $pwWhy = $(if ($All) { '-All' } else { 'нет данных о прошлом успешном прогоне' }) }
else {
  $pwWhy = "изменено файлов: $($pwChanged.Count)"
  foreach ($p in $pwChanged) {
    switch -Regex ($p) {
      '^web/tests/([\w.-]+\.spec\.ts)$' { [void]$specs.Add("tests/$($matches[1])"); break }
      '^web/src/(voice|voice-relay)\.(ts|css)$' { [void]$specs.Add('tests/voice.spec.ts'); [void]$specs.Add('tests/voice-relay.spec.ts'); break }
      '^web/src/(rooms|rooms-table)\.ts$|^web/src/rooms\.css$' { [void]$specs.Add('tests/voice.spec.ts'); $grep.Add('rooms'); break }
      '^web/src/(admin|admin-page)\.(ts|css)$|^internal/adminweb/' { [void]$specs.Add('tests/admin-web.spec.ts'); break }
      '_test\.go$|^docs/|\.md$|^android/|^\.github/|^build\.ps1$|^VERSION$|^mobile/|^scripts/(?!prepare-ui-tests)|^\.gitignore$|^\.gitattributes$' { break }
      '^internal/.*\.go$|^cmd/signaling/' { $backend = $true; break }
      default { $full = $true }
    }
  }
}
if ($backend -and -not $full) {
  foreach ($s in 'voice', 'voice-relay', 'history', 'settlement', 'rules-options') { [void]$specs.Add("tests/$s.spec.ts") }
  $grep.Add('rooms|exit|update|connection|settings')
}
$pwSelect = $(if ($full) { 'все тесты' } elseif ($specs.Count -eq 0 -and $grep.Count -eq 0) { $null } else { "файлы: $($specs -join ', ')$(if ($grep.Count -gt 0 -and -not $specs.Contains('tests/ui.spec.ts')) { "; ui.spec.ts по шаблону: $(($grep | Select-Object -Unique) -join '|')" })" })
$exeFailed = $results['web'] -eq 'ОШИБКА' -or $results['exe'] -eq 'ОШИБКА' -or $results['signaling'] -eq 'ОШИБКА'
if (-not $pwSelect) { Write-Host '[playwright] пропуск: изменения не затрагивают UI-тесты'; $results['playwright'] = 'пропущено'; if (-not $Plan -and $null -ne $pwChanged) { Save-Stage 'playwright' $pwRx @{ fullAt = $state.stages['playwright'].fullAt } } }
else {
  Write-Host "[playwright] запуск ($pwWhy): $pwSelect"
  if ($Plan) { $results['playwright'] = 'будет выполнено' }
  elseif ($exeFailed) { $results['playwright'] = 'пропущено: сборка не удалась'; $failed = $true }
  else {
    $pwArgs = @('--reporter=line', '--retries=1') # один повтор: редкие гонки в тестах не должны блокировать прогон; такие тесты Playwright помечает flaky
    if (-not (Test-Path .build/public-build/Preferans.exe)) {
      $pwArgs += '--grep-invert', 'public build starts without personal servers'
      Write-Host 'Тест public-config исключён: нет .build/public-build/Preferans.exe (сборка с -Public).'
    }
    Push-Location web
    try {
      $ok = $true
      if ($full) { $ok = Run 'playwright (все)' { npx playwright test @pwArgs } }
      else {
        $files = @($specs)
        if ($files.Count -gt 0) { $ok = (Run "playwright ($($files -join ' '))" { npx playwright test @pwArgs @files }) -and $ok }
        if (-not $specs.Contains('tests/ui.spec.ts') -and $grep.Count -gt 0) {
          $pattern = ($grep | Select-Object -Unique) -join '|'
          $ok = (Run "playwright (ui.spec.ts: $pattern)" { npx playwright test @pwArgs tests/ui.spec.ts --grep $pattern --pass-with-no-tests }) -and $ok
        }
      }
    } finally { Pop-Location }
    if ($ok) {
      $fullAt = $(if ($full) { (Get-Date).ToString('s') } else { $state.stages['playwright'].fullAt })
      Save-Stage 'playwright' $pwRx @{ fullAt = $fullAt }; $results['playwright'] = $(if ($full) { 'ok (полный)' } else { 'ok (частичный)' })
    } else { $results['playwright'] = 'ОШИБКА'; $failed = $true }
  }
}
$fullAt = $state.stages['playwright'].fullAt
if ($fullAt) { Write-Host "Последний полный прогон Playwright: $fullAt" } else { Write-Host 'Полного прогона Playwright ещё не было.' }
Write-Host ''
Write-Host 'Итог:'
foreach ($k in $results.Keys) { Write-Host ("  {0,-12} {1}" -f $k, $results[$k]) }
if (-not $Plan) { "DONE $(Get-Date) failed=$failed" | Out-File $Log -Append }
if ($failed) { exit 1 }