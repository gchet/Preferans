# Сборка и локальная конфигурация

Сборка выполняется в Windows с PowerShell 7. Нужны Go 1.26+, Node.js 22.12+,
Zig, JDK 17, Android SDK 36 и NDK. Gradle 8.13 закреплён Wrapper в android/.
Первый запуск Wrapper может загрузить дистрибутив; контрольная сумма проверяется.
Код и WebUI проверялись с Go 1.27 и Node.js 24.

## Обычные команды

~~~powershell
./build.ps1 -Target All
./build.ps1 -Target Windows
./build.ps1 -Target Android
./build.ps1 -Target Test
./build.ps1 -Target Windows -AI Off -WindowsOutput dist/Preferans-NoAI.exe
~~~

Результаты: dist/Preferans.exe, dist/Preferans.apk и dist/version.json.
Имена, параметры -AI, -SkipWeb, -Emulator и -NetworkTest сохранены.
APK по-прежнему подписывается существующим debug-ключом. Не удаляйте его:
другой ключ не позволит обновить уже установленное приложение поверх старого.

## Настройки этого компьютера

Скопируйте project.local.example.json в project.local.json и задайте нужные поля.
Не копируйте пример серверов буквально: замените адресами своей инфраструктуры
или удалите блок network для локальной игры. project.local.json исключён из Git.

- tools.androidSdk, tools.zig, tools.gradle — пути инструментов. Пустой Gradle использует Wrapper.
- network.roomServiceURL — полный URL службы комнат, например https://signaling.example.org/v2/rooms.
- network.turnServer и turnUser — адрес и имя TURN. Пароль вводится в приложении.
- network.legacySignalingURL — необязательный адрес прежнего /v1/singleton.
- network.updateAddressHint — подсказка адреса компьютера для обновления APK и переноса настроек ИИ.
- deployment.server — SSH-адрес для Update-Signaling.ps1 -Deploy.

Пароли и API-ключи в конфигурацию сборки не помещаются. Значения network встраиваются
в EXE/APK и поэтому не являются секретными. Ключи моделей вводятся или переносятся
в приложении; они защищены DPAPI/Android Keystore.

Приоритет инструментов: аргумент командной строки → локальная конфигурация →
переменная окружения → PATH/стандартный путь SDK. Поддерживаются ANDROID_HOME,
ANDROID_SDK_ROOT, ZIG и GRADLE. Другой файл выбирается через -LocalConfig.
Существующее сохранение сетевого адреса в приложении имеет приоритет над значением
сборки; PREFERANS_ROOMS_URL служит переопределением при запуске Go-приложения.

## Публичная сборка

~~~powershell
./build.ps1 -Target All -Public -AI Off
~~~

-Public использует локальные пути инструментов, но исключает личные адреса network
и deployment. Генерируемый config/local/build.json исключён из Git и читается
одновременно Go и Vite. Следующая обычная сборка заново подставит локальные адреса.
Не используйте -SkipWeb при переключении между публичной и локальной конфигурацией:
фронтенд должен быть пересобран вместе с ядром.

В чистой публичной сборке сервер владельца проекта не задан. Выберите локальную
игру с ботами или укажите собственный сервер комнат и TURN в приложении.

Адрес сервера меняется только вне партии и после выхода из сетевой комнаты,
чтобы идентификаторы одной инфраструктуры не отправлялись другой.

## Ресурсы

Параметры интерфейса и ботов: config/interface.json. Числа с Ms — миллисекунды,
с Px — пиксели; пояснения находятся в _comments. После изменения нужна пересборка.
Языки: locales/ru.json, uk.json, en.json; см. [LOCALIZATION.md](LOCALIZATION.md).

Все 96 действующих карт уже находятся в web/public/cards и не загружаются при игре.
Для повторного импорта нужны Python 3 и Inkscape:

~~~powershell
python scripts/import-card-decks.py --inkscape "C:/Program Files/Inkscape/bin/inkscape.exe"
python scripts/import-card-decks.py --output .build/card-preview --inkscape "C:/Program Files/Inkscape/bin/inkscape.exe"
~~~

Атласный оригинал загружается в .build/card-sources с проверкой SHA-256.
English и Woodcut закреплены по ревизиям Git. Импорт заменяет рисунки в указанном
каталоге; сначала сохраните ручные изменения. --source позволяет выбрать свой SVG-лист.

Текущая Windows-иконка: native/windows/icon.svg, icon.png и bullet.ico.
После изменения ICO пересоберите ресурс:

~~~powershell
./scripts/Rebuild-WindowsIcon.ps1
~~~

Это обновляет cmd/preferans/bullet_windows_amd64.syso. Android-иконки расположены
в android/app/src/main/res/mipmap-*; заменяйте их отдельно после изменения рисунка.
Обычная сборка не перерисовывает иконку автоматически.

## Проверки

См. [TESTING.md](TESTING.md), [RELEASES.md](RELEASES.md) и [SERVER.md](SERVER.md).
Ни конфигурацию устройств, ни .info, .build, dist, ключи подписи и журналы
не включайте в архив исходников.
