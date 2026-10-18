# Ручной выпуск новой версии

Эта инструкция описывает полный цикл после ручного изменения проекта: проверки,
локальный коммит и тег, публикацию исходников в публичный репозиторий из очищенного
снимка и публикацию готовых приложений в приватный репозиторий.

Пример ниже продолжает версию `v1.0.3`, поэтому следующая версия указана как
`v1.0.4`. Для будущих выпусков замените номер во всех командах. Не переиспользуйте
опубликованный тег и не перезаписывайте готовый Release.

## Подготовка среды

Откройте PowerShell 7 в корне проекта `<корень-проекта>`. Нужны Git,
GitHub CLI (`gh`), Go версии из `go.mod` (сейчас 1.27), Node.js/npm, Zig, Android
SDK и NDK. Локальные пути инструментов хранятся в игнорируемом Git файле
`project.local.json`; личные адреса сетевых служб и другие локальные значения не
должны попадать в публичный снимок.

Проверьте вход в GitHub и репозитории:

~~~powershell
gh auth status --hostname github.com
gh repo view gchet/Preferans --json nameWithOwner,isPrivate,defaultBranchRef
gh repo view <владелец>/<приватный-репозиторий> --json nameWithOwner,isPrivate,defaultBranchRef
~~~

Ожидается, что `gchet/Preferans` публичный, а `<владелец>/<приватный-репозиторий>` приватный.
У аккаунта должны быть права записи в оба. Если команды не проходят, настройте
доступ через `gh auth login` и не продолжайте публикацию до успешной проверки.

## Проверки перед коммитом

Сначала проверьте изменённую функциональность и выполните тесты. Минимальная
проверка Go и сборки WebUI:

~~~powershell
pwsh ./build.ps1 -Target Test
go test ./config ./internal/... ./mobile ./netcheck
go vet ./config ./internal/... ./cmd/signaling ./mobile ./netcheck
go test -tags noai ./internal/app -run TestNoAI -count=1
Push-Location web
npm.cmd ci
npm.cmd run build
npm.cmd audit --audit-level=high
Pop-Location
~~~

`-Target Test` запускает Go-тесты из `internal`; остальные команды выше расширяют
проверку до всех пакетов, `go vet`, сборки интерфейса и аудита npm-зависимостей.
Если менялась WebUI, выполните также Playwright UI-тесты по инструкции
[`TESTING.md`](TESTING.md), раздел **WebUI**. Там приведена подготовка тестового
сервера и команда `npm.cmd test`.

После проверки убедитесь, что не попали в коммит логи, ключи, локальные настройки,
APK/EXE, `.build` или прочие артефакты:

~~~powershell
git status --short
git diff --check
git diff
~~~

Просмотрите изменения целиком. Добавляйте только файлы, относящиеся к задаче;
не используйте бездумно `git add .`, если в рабочей папке есть самостоятельные
тестовые проекты или иные незакоммиченные файлы.

## Локальный коммит и версия

Сначала выберите следующий свободный номер. Посмотрите существующие локальные
теги и опубликованные релизы, чтобы случайно не повторить тег:

~~~powershell
git tag --list 'v*' --sort=-version:refname
gh release list --repo <владелец>/<приватный-репозиторий> --limit 10
gh release list --repo gchet/Preferans --limit 10
~~~

Для примера создайте коммит и локальный аннотированный тег `v1.0.4`:

~~~powershell
git add internal/game/view.go internal/game/misere_tracker_test.go web/src/main.ts web/tests/ui.spec.ts
git diff --cached --check
git diff --cached --stat
git commit -m "Краткое описание изменений"
git tag -a v1.0.4 -m "Краткое описание изменений"
git status --short
git show --no-patch --decorate HEAD
~~~

Замените пути в `git add` на фактически изменённые файлы и задайте короткое
осмысленное описание. Перед выпуском рабочее дерево по исходникам сборки должно
быть чистым, а `HEAD` — совпадать с тегом:

~~~powershell
git rev-parse HEAD
git rev-parse 'v1.0.4^{commit}'
~~~

Обе команды должны вывести один и тот же hash. Локальный тег используется для
сборки и приватного релиза. Его не следует отправлять напрямую в публичный
репозиторий: локальная история может содержать коммиты до очищенного снимка.

## Публикация исходников в `gchet/Preferans`

Публичный репозиторий имеет собственную очищенную историю. Не выполняйте из
основной рабочей копии `git push` в `gchet/Preferans` и не переносите туда её
локальный коммит или тег. Вместо этого соберите снимок текущих файлов, перенесите
его в отдельный клон публичного репозитория и создайте там новый коммит поверх
публичной истории.

### Создание и проверка снимка

Проверка кандидатов и создание снимка:

~~~powershell
pwsh ./scripts/Prepare-PublicSnapshot.ps1 -CheckOnly
pwsh ./scripts/Prepare-PublicSnapshot.ps1 -Destination .build/public-source-v1.0.4
gitleaks dir --redact .build/public-source-v1.0.4
~~~

Скрипт исключает Git-историю, игнорируемые и известные локальные файлы и проверяет
сетевые значения по умолчанию. Он включает отслеживаемые файлы и новые
неигнорируемые файлы, поэтому проверьте содержимое снимка и результат Gitleaks.
Если среди новых файлов есть посторонний проект, удалите его только из папки
снимка либо повторите подготовку после исключения этого файла из кандидатов.
Исходный проект при этом не трогайте. Не продолжайте, если сканер сообщает
секреты или личные адреса.

### Обновление отдельного публичного клона

Склонируйте `main` в новую временную папку; она не должна существовать:

~~~powershell
git clone --branch main --single-branch https://github.com/gchet/Preferans.git .build/public-publish-v1.0.4
~~~

Следующий блок заменяет файлы только внутри временного клона, сохраняя его `.git`.
Проверка пути не даст удалить содержимое вне `.build/public-publish-v1.0.4`.
Поменяйте номер в обоих путях, если выпускаете другую версию.

~~~powershell
$snapshot = (Resolve-Path .build/public-source-v1.0.4).Path
$destination = (Resolve-Path .build/public-publish-v1.0.4).Path
$buildRoot = [IO.Path]::GetFullPath((Join-Path $PWD '.build'))
$gitDir = [IO.Path]::GetFullPath((Join-Path $destination '.git'))
if (!$destination.StartsWith($buildRoot + [IO.Path]::DirectorySeparatorChar) -or !(Test-Path $gitDir)) {
    throw 'Путь временного публичного клона не прошёл проверку.'
}
foreach ($item in Get-ChildItem -LiteralPath $destination -Force | Where-Object { $_.FullName -ne $gitDir }) {
    $path = [IO.Path]::GetFullPath($item.FullName)
    if (!$path.StartsWith($destination + [IO.Path]::DirectorySeparatorChar)) { throw 'Неожиданный путь.' }
    Remove-Item -LiteralPath $path -Recurse -Force
}
foreach ($item in Get-ChildItem -LiteralPath $snapshot -Force) {
    Copy-Item -LiteralPath $item.FullName -Destination $destination -Recurse -Force
}
~~~

Проверьте, что в публичном клоне изменилось только ожидаемое, затем зафиксируйте
и отправьте очищенный снимок:

~~~powershell
git -C .build/public-publish-v1.0.4 status --short
git -C .build/public-publish-v1.0.4 add -A
git -C .build/public-publish-v1.0.4 diff --cached --check
git -C .build/public-publish-v1.0.4 diff --cached --stat
git -C .build/public-publish-v1.0.4 commit -m "Краткое описание изменений"
git -C .build/public-publish-v1.0.4 tag -a v1.0.4 -m "Краткое описание изменений"
git -C .build/public-publish-v1.0.4 push origin main refs/tags/v1.0.4
~~~

Проверьте итог на GitHub: откройте `gchet/Preferans`, убедитесь, что коммит и тег
есть, а Actions для отправленного коммита завершились успешно. Если `git push`
отклонён из-за новых коммитов на GitHub, не используйте `--force`: получите
обновлённую ветку в отдельный клон, перенесите снимок повторно и создайте коммит
поверх её актуального `main`.

Тег `v1.0.4` в публичном репозитории будет указывать на очищенный коммит, а
локальный тег с тем же названием — на локальный коммит сборки. Это ожидаемо:
репозитории имеют разную историю, но файлы версии соответствуют друг другу.

## Сборка и приватный Release

Обычная сборка, которую можно запускать отдельно:

~~~powershell
pwsh ./build.ps1 -Target All
~~~

Она создаёт WebUI, Windows EXE, Android APK и выполняет Go-тесты. По умолчанию
локальная сборка включает ИИ. Для проверки варианта без ИИ укажите `-AI Off`.

Для выпуска используйте издательский скрипт: он сам пересоберёт оба приложения,
выставит Android `versionName` по тегу и автоматически увеличит `versionCode`.
Сначала выполните проверку готовности — она ничего не собирает и не публикует:

~~~powershell
pwsh ./scripts/Publish-Release.ps1 -Repository <владелец>/<приватный-репозиторий> -Tag v1.0.4 -CheckOnly
~~~

Если проверка прошла, создайте приватный Release с заметками:

~~~powershell
pwsh ./scripts/Publish-Release.ps1 -Repository <владелец>/<приватный-репозиторий> -Tag v1.0.4 -Notes "Краткое описание изменений"
~~~

По умолчанию издательский скрипт собирает с `-AI Off`. В релиз загружаются
`Preferans.exe`, `Preferans.apk`, `version.json`, лицензии и `SHA256SUMS.txt`.
Скрипт проверяет флаг ИИ, совпадение версии и размеры файлов; существующий Release
он не перезаписывает. Если загрузка прервётся, проверьте черновик и готовые
файлы в `.build/releases/v1.0.4` перед повторными действиями.

Не передавайте этому приватному выпуску `-SourceRepository`: этот режим
предназначен для другого сценария и проверяет совпадение публичного тега с
локальным коммитом. Для проекта с очищенной историей публичный снимок публикуется
описанным выше отдельным клоном, а бинарный выпуск — в приватный репозиторий.

## Проверка результата

Проверьте приватный Release и набор вложений:

~~~powershell
gh release view v1.0.4 --repo <владелец>/<приватный-репозиторий>
Get-Content dist/version.json
Get-Content .build/releases/v1.0.4/SHA256SUMS.txt
~~~

В `version.json` должны быть `versionName: "1.0.4"`, новый возрастающий
`versionCode`, `apk: "Preferans.apk"` и `aiEnabled: false`. Скачанные файлы можно
проверить командами `Get-FileHash -Algorithm SHA256` и сравнить с
`SHA256SUMS.txt`.

В GitHub откройте вкладку **Actions** публичного `gchet/Preferans`, найдите запуски
для последнего коммита и дождитесь `success`. При отправке ветки вместе с тегом
GitHub может создать отдельные запуски для событий ветки и тега; проверяйте оба.
В приватном репозитории проверьте, что `v1.0.4` опубликован и содержит EXE и APK.

Секреты, локальный адрес обновлений и серверные настройки не публикуйте. APK
использует текущую debug-подпись; храните ключ подписи отдельно от Git, иначе
пользователи не смогут установить APK поверх предыдущей версии.
