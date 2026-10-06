# Проверки

Это инструкция для текущего кода, а не заявление о прохождении всех проверок
на любой версии. Исторические отчёты не заменяют новый прогон.

~~~powershell
./build.ps1 -Target Test
go vet ./config ./internal/... ./cmd/signaling ./mobile ./netcheck
go test -tags noai ./internal/app -run TestNoAI -count=1
cd web
npm ci
npm run build
npm audit --audit-level=high
~~~

Go-тесты проверяют торговлю, снос, расчёт, договорённости, распасы, мизер,
ботов, скрытые руки, сохранения, паузу, переподключение, голосовой обмен,
каталог комнат и админку. Обычные тесты не требуют личного VPS или ключей моделей.

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
adb -s emulator-5554 shell am start -n com.preferans.game/.MainActivity
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
