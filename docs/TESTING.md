# Проверки

Это инструкция для текущего кода, а не заявление о прохождении всех проверок
на любой версии. Исторические отчёты не заменяют новый прогон.

~~~powershell
./build.ps1 -Target Test
go vet ./config ./internal/... ./cmd/signaling ./mobile
go test -tags noai ./internal/app -run TestNoAI -count=1
cd web
npm ci
npm run build
npm audit --omit=dev --audit-level=high
~~~

Go-тесты проверяют торговлю, снос, расчёт, договорённости, распасы, мизер,
ботов, скрытые руки, сохранения, паузу, переподключение, голосовой обмен,
каталог комнат и админку. Обычные тесты не требуют личного VPS или ключей моделей.

## Проверка по изменениям

Локально полный набор (Go-тесты, `go vet`, сборка интерфейса, аудит npm, EXE и Playwright, около 12 минут) нужен не после
каждой правки. Скрипт `scripts/Run-Checks.ps1` запускает только то, что затронуто изменениями с последнего успешного прогона
каждой стадии. Состояние хранится в `.build/checks-state.json` (хэши файлов входа каждой стадии; в git не попадает).

~~~powershell
pwsh ./scripts/Run-Checks.ps1            # только затронутое изменениями
pwsh ./scripts/Run-Checks.ps1 -Plan      # показать, что будет выполнено, ничего не запуская
pwsh ./scripts/Run-Checks.ps1 -All       # всё целиком; обязательно перед выпуском версии
pwsh ./scripts/Run-Checks.ps1 -Android   # добавить компиляцию Android принудительно
~~~

Стадии и их входные файлы:

| Стадия | Запускается при изменении |
| --- | --- |
| `go` (тесты, vet, noai) | `*.go`, `go.mod`, `go.sum`, `locales/*.json`, `config/*.json`, `native/` |
| `web` (проверка локалей и конфига, tsc, vite) | `web/` (кроме тестов), `locales/`, `config/` |
| `audit` | `web/package*.json` или прошло больше 7 дней |
| `signaling`, `exe` (EXE для тестов) | Go-код без `_test.go`, `locales/`, `config/`, `web/`, `native/` |
| `android` (gomobile и компиляция Kotlin, APK в `dist/` не собирается) | `android/`, `mobile/`, `locales/` |
| `playwright` | см. ниже |

Playwright выбирает тесты по изменённым файлам:

- `web/tests/<имя>.spec.ts` запускает этот файл;
- `voice.ts`, `voice-relay.ts` — `voice.spec.ts` и `voice-relay.spec.ts`;
- `rooms.ts`, `rooms-table.ts`, `rooms.css` — `voice.spec.ts` и тесты `ui.spec.ts` с «rooms» в названии;
- админка (`admin*.ts`, `internal/adminweb/`) — `admin-web.spec.ts`;
- документация, Kotlin, `mobile/`, сборочные скрипты, `VERSION` и Go-тесты — Playwright не запускается;
- Go-код в `internal/` и `cmd/signaling/` — сокращённый набор: голос, история, расчёт, правила и тесты `ui.spec.ts` про
  комнаты, выход, обновление, подключение и настройки;
- всё остальное, в том числе `main.ts`, CSS, локали, конфиги и помощники тестов, — полный прогон.

Сокращённые прогоны принимают риск: остальные тесты ждут следующего полного. Дата последнего полного прогона
печатается в итоге. Перед выпуском версии выполняйте `-All`. Тест `public-config` пропускается, если нет
`.build/public-build/Preferans.exe` (сборка с `-Public`).

## WebUI

Playwright использует изолированные каталоги .build и headless Go-приложение.
Для тестов задаются адреса-примеры, сетевые ответы подменяются сценариями.

~~~powershell
go build -o .build/signaling-test.exe ./cmd/signaling
node scripts/prepare-ui-tests.mjs
./build.ps1 -Target Windows -LocalConfig .build/ui-test-config.json -WindowsOutput .build/preferans-test.exe
./build.ps1 -Target Windows -LocalConfig .build/ui-test-config.json -AI Off -SkipWeb -WindowsOutput dist/Preferans-NoAI.exe
cd web
npm test
~~~

По умолчанию локально используется установленный Chrome. CHROME_PATH позволяет
выбрать другой Chromium. В CI браузер устанавливается через Playwright.
Тест public-config.spec.ts использует отдельную публичную сборку:
на Windows скопируйте EXE, собранный с -Public, в .build/public-build/Preferans.exe.
При этом WebUI для остальных тестов должен оставаться собран с тестовой конфигурацией.
В CI эти версии готовятся автоматически.

После проверки выполните обычную сборку, чтобы вернуть локальные адреса.

## Проверки устройств

~~~powershell
node scripts/windows-smoke.mjs
./build.ps1 -Target Android -Emulator
adb -s emulator-5554 install -r dist/Preferans-emulator.apk
adb -s emulator-5554 shell am start -n com.preferans.game.v2/com.preferans.game.MainActivity
node scripts/android-smoke.mjs
~~~

ADB и ANDROID_SERIAL задают инструмент и устройство для Android smoke-теста.
После эмулятора обычная Android-сборка возвращает ARM64.
Настоящие микрофоны, выход из сна, NAT разных операторов и восстановление
после смены сети нужно проверять на физических устройствах отдельно.
Синтетические аудиотесты не подтверждают качество реального голосового чата.

## Безопасность и CI

GitHub Actions запускает Go-тесты, проверку WebUI, Playwright, npm audit,
govulncheck и Gitleaks. Секреты GitHub и личные серверы этим заданиям не нужны.
Проверка секретов не гарантирует обнаружение всех форматов; перед первой публикацией
проверяется отдельно подготовленный снимок и новая история. Не отправляйте старую
локальную историю в новый публичный репозиторий без отдельного согласования.
