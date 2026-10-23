# CatalogDB для Linux

Linux и Windows используют один интерфейс, Wails App и backend. Платформенные
операции находятся в desktop_windows.go и desktop_linux.go; проверка протокола
и доступности сервера — в desktop_connection.go. Основная точка входа общая:
main_desktop.go. Wails включает её через тег desktop; обычный go test ./...
не требует GTK/WebKit. PostgreSQL и схема каталога одинаковы на обеих ОС.

## Запуск

Поставка: Linux x86_64, GTK 3 и WebKitGTK 4.1. Облачный архив требует glibc 2.41 или новее (Debian 13). Распакуйте архив и выполните
`./CatalogDB`. Приложение запускают от обычного пользователя внутри графической
сессии с D-Bus. Это динамическая сборка: совместимость со всеми дистрибутивами
не предполагается. Облачная сборка подготовлена на Debian 13; CI — Ubuntu 24.04.
Для старых дистрибутивов собирайте из исходников в целевой системе.

Debian 13 / Ubuntu 24.04 (от администратора):

```sh
apt-get install libgtk-3-0t64 libwebkit2gtk-4.1-0 xdg-utils openssh-client
```

Для запоминания пароля PostgreSQL и токенов входа нужен Secret Service в
пользовательской D-Bus-сессии: например, установленный и разблокированный GNOME
Keyring (`gnome-keyring`). KDE Wallet подходит при включённом совместимом
Secret Service. При отсутствии или блокировке хранилища сохранение выдаст ошибку;
можно подключиться без сохранения и выбрать 0 часов для сессии. Секреты не
переносятся в JSON и не передаются внешним клиентам.

RDP: установите FreeRDP (`xfreerdp3` или `xfreerdp`) либо Remmina. Сначала выбирается
FreeRDP, затем Remmina. SSH: нужен `ssh` и один из терминалов: x-terminal-emulator,
gnome-terminal, konsole, xfce4-terminal, xterm. Наличие клиента и доступность порта
проверяются до запуска; дальнейшие ошибки подключения показывает сам клиент.
Аргументы передаются без shell. Используются стандартные порты 3389 и 22.

Открытие вложений и загрузок использует xdg-open и ассоциации рабочего стола.
«Показать в папке» открывает содержащую папку. Скрипты SQL/PowerShell автоматически
не исполняются. Системные диалоги открытия/сохранения предоставляет Wails/GTK.

Несекретные настройки: `$XDG_CONFIG_HOME/DBCatalog/connection.json`, либо
`~/.config/DBCatalog/connection.json`. Файлы загрузок и их история хранятся в
подкаталогах рядом с настройками. Схема PostgreSQL и миграции общие с Windows.

Для ярлыка установите бинарник под именем CatalogDB в пользовательский PATH,
скопируйте catalogdb.desktop в ~/.local/share/applications, а catalogdb.png —
в ~/.local/share/icons. Либо запускайте бинарник непосредственно из архива.

## Сборка

Нужны Go согласно go.mod, Node.js 24, npm и Wails CLI 2.17.0.
Зависимости сборки Debian/Ubuntu:

```sh
apt-get install build-essential pkg-config libgtk-3-dev libwebkit2gtk-4.1-dev
```

```sh
go install github.com/wailsapp/wails/v2/cmd/wails@v2.17.0
bash packaging/build-linux.sh
```

Результат: build/bin/CatalogDB и архив build/bin/CatalogDB-linux-x64-VERSION.tar.gz.
Проверки backend и frontend входят в сценарий. При ручной сборке нужен
`wails build -platform linux/amd64 -tags webkit2_41`. Тег выбирает WebKitGTK 4.1
вместо устаревшей 4.0. Wails автоматически добавляет desktop/production.

Windows EXE можно собрать из Linux после сборки frontend:

```sh
wails build -platform windows/amd64 -s -skipbindings
```

Windows-установщик и Credential Manager проверяются на Windows. Облачная проверка
старта Linux под Xvfb не заменяет интерактивную проверку Wayland/X11, ассоциаций
файлов, внешних клиентов и системных диалогов на пользовательских рабочих местах.
