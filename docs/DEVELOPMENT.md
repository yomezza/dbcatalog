# Разработка, проверки и выпуск CatalogDB

Рабочая инструкция к [контексту проекта](../PROJECT_CONTEXT.md). Состояние обновлено для 0.13.0, 25 октября 2026 года. Команды предполагают текущую папку корня репозитория, кроме явно указанных переходов.

## Инструменты

| Инструмент | Требование / проверенная версия |
| --- | --- |
| Go | В `go.mod` — 1.26.0; проверено 1.27.3 |
| Wails CLI | Закреплённая версия 2.17.0 |
| Node.js | Проверено семейство 24 |
| PostgreSQL | Проверки схемы и интеграции — PostgreSQL 17 |
| Windows runtime | Microsoft Edge WebView2 |
| Linux desktop | GTK 3, WebKitGTK 4.1, графическая сессия и D-Bus |

Установка зависимостей:

```sh
go mod download
go install github.com/wailsapp/wails/v2/cmd/wails@v2.17.0
npm --prefix frontend ci
npm --prefix frontend run build
```

Добавьте каталог установленного Wails в PATH. Для обычного Linux `go test ./...` GTK/WebKit не требуются; нативная desktop-сборка требует их development-пакеты. На Windows frontend должен существовать для встраивания ресурсов.

Для разработки интерфейса вместе с настоящим desktop-приложением:

```sh
wails dev
```

Браузерный frontend сам по себе не заменяет Wails bridge. Тесты Playwright подставляют bridge отдельно.

## Тестовая PostgreSQL

В [compose.yaml](../compose.yaml) описана **одноразовая среда разработки**, не конфигурация production. Порт на локальной машине — `55432`.

```sh
docker compose up -d postgres
```

Дождитесь готовности сервиса. Linux/macOS shell:

```sh
export CATALOGDB_TEST_DATABASE_URL='postgres://catalogdb:catalogdb-test-only@127.0.0.1:55432/catalogdb_test?sslmode=disable'
go test -race ./...
go vet ./...
```

PowerShell:

```powershell
$env:CATALOGDB_TEST_DATABASE_URL = 'postgres://catalogdb:catalogdb-test-only@127.0.0.1:55432/catalogdb_test?sslmode=disable'
go test -race ./...
go vet ./...
```

`catalogdb-test-only` — опубликованный тестовый пароль из compose, его нельзя использовать в рабочей среде. На Windows race detector может требовать подходящий C toolchain.

**Интеграционные тесты могут пропускаться без `CATALOGDB_TEST_DATABASE_URL`.** Успешный вывод `go test` без реальной тестовой PostgreSQL не подтверждает миграции или работу SQL API. Не направляйте эти тесты на рабочую БД: они создают случайные схемы, проверяют запись/удаление/откат и очищают собственные схемы.

Для тестов SQL API нужны достаточные права именно в одноразовой тестовой среде. Выдача таких прав обычным production-пользователям — отдельная проблема модели доступа, см. [KNOWN_ISSUES.md](KNOWN_ISSUES.md).

## Frontend

```sh
npm --prefix frontend test
npm --prefix frontend run build
```

Браузерные проверки из папки `frontend`:

```sh
cd frontend
npm exec playwright install chromium
npm run test:e2e
```

Если Chromium уже установлен, можно использовать его без скачивания:

```sh
CATALOGDB_CHROMIUM=/usr/bin/chromium npm run test:e2e
```

Playwright-конфигурация сама собирает frontend и поднимает preview на `127.0.0.1:4173`. Не запускайте параллельно другой сервер на этом порту или очистку `node_modules`/`dist` во время браузерных проверок.

После браузерных проверок вернитесь в корень репозитория (`cd ..`), прежде чем выполнять команды Wails из следующих разделов.

Последний выпуск: 14 Vitest-сценариев, 50 браузерных сценариев проверены (включая 6 Android). Go race с PostgreSQL 17 и vet пройдены. Это результаты **0.13.0**, а не обещание результата после новых изменений. [Отчёт](validation-0.13.0.md).

Проверяйте подходящую изменению область. Для правки только Markdown достаточно проверить ссылки, факты и diff; пересборка приложения и повтор всего тестового набора не нужны.

## Сборка Windows

После подготовки frontend:

```sh
wails build -platform windows/amd64 -s -skipbindings
```

`-s` пропускает сборку frontend, поэтому `npm run build` перед этим обязателен. EXE появляется в `build/bin/CatalogDB.exe`. Кросс-компиляция из Linux поддерживается.

На Windows [packaging/build.ps1](../packaging/build.ps1) дополнительно запускает проверки, получает официальный WebView2 bootstrapper с проверкой подписи Microsoft и собирает Inno Setup при доступном `ISCC.exe`. Установщик для закрытой сети задаётся через `CATALOGDB_WEBVIEW2_INSTALLER` и также проверяется.

Кросс-сборка не подтверждает GUI, установщик, Credential Manager, нативные диалоги и интерактивную авторизацию RDP/SSH. Для этого есть [план Windows-проверки](windows-release-check.md), `packaging/windows-smoke.ps1` и Windows job CI. CI 0.13.0 проверил silent-установку/обновление и появление нативного окна; интерактивное отображение и диалоги требуют отдельной проверки.

## Сборка Linux

Нужны development-пакеты GTK 3 и WebKitGTK 4.1; например, в Debian/Ubuntu — `libgtk-3-dev`, `libwebkit2gtk-4.1-dev`, `pkg-config` и средства компиляции.

Полный сценарий:

```sh
bash packaging/build-linux.sh
```

Он устанавливает npm-зависимости, проверяет frontend/Go, собирает приложение и создаёт TAR.GZ с SHA-256 в `build/bin`. Если проверки и frontend уже подготовлены, нативную сборку можно выполнить отдельно:

```sh
wails build -platform linux/amd64 -tags webkit2_41 -s -skipbindings
```

Результат — `build/bin/CatalogDB`. Сборки, опубликованные из прежней облачной среды Debian 13, требуют **glibc 2.41+**, GTK 3 и WebKitGTK 4.1. Успех CI на Ubuntu 24.04 не делает архив, собранный в Debian 13, совместимым со старой glibc. Для старого дистрибутива собирайте в соответствующей целевой среде.

Нативный запуск проверяется в графической сессии. Для автоматического smoke под Linux используются `dbus-run-session`, Xvfb и [packaging/linux-smoke.sh](../packaging/linux-smoke.sh). Загрузка снимка экрана из Chromium с mock bridge не равна проверке настоящего Wails/WebKitGTK.

## Сборка Android

Подготовка SDK/NDK/JDK, подпись, установка и нативный smoke — в [android.md](android.md). `mobile/client.go` использует общий `internal/application`, Java-оболочка заменяет платформенные операции Wails. `packaging/build-android.sh` создаёт APK и SHA-256. Закрытый ключ не входит в исходники; для обновления установки нужен тот же сертификат. CI APK подписан одноразовым тестовым ключом и не является публикуемым APK.

## Среда предыдущего ассистента — не обязательное окружение

В прежнем workspace использовалась папка `/workspace/test`, несмотря на новое имя GitHub-репозитория. Go и Wails были в `/workspace/.tools`, кеши — в `/workspace/.cache`. Эти каталоги, контейнер `catalogdb-linux-build`, локальный seed и графическая сессия могут отсутствовать у следующего ассистента.

[scripts/cloud-setup.sh](../scripts/cloud-setup.sh) содержит настройки конкретной облачной среды. Прочитайте его перед запуском: не переносите фиксированные `/workspace`-пути в приложение и не считайте этот скрипт универсальным установщиком.

При сборках в root-контейнере следите за владельцем сгенерированных `frontend/wailsjs` и `build/bin`: файлы с правами только root мешают следующей сборке обычным пользователем. Не добавляйте сгенерированный bridge, локальные настройки и кеши в Git.

Git push и публичные GitHub-страницы работали; доступ к API и локальная авторизация `gh` ранее ограничивались средой. В новой среде доступ нужно проверить заново. Наличие прежнего repository secret не даёт ассистенту права читать или выводить его значение.

## Выпуск новой версии

Постоянные требования — в [AGENTS.md](../AGENTS.md), история — в [releases.md](releases.md).

### Версионные поля

Обновляйте согласованно:

- `internal/buildinfo/info.go`;
- `android/AndroidManifest.xml` (`versionName` и растущий `versionCode`);
- `frontend/package.json` и корневую package metadata в `frontend/package-lock.json`;
- `wails.json` (`info.productVersion`);
- `packaging/inno_setup/catalogdb.iss`;
- имена текущего установщика в `.github/workflows/checks.yml`;
- актуальные ссылки README, отчёт проверки, историю и манифест выпуска.

`SettingsPanel.tsx` уже берёт версию из package.json; новую константу там создавать не надо.

### Порядок

1. Завершите код и проверки. Windows, Linux и Android должны собираться из одного состояния исходников.
2. Соберите архивы и SHA-256. Проверьте содержимое: без реквизитов подключения, токенов и рабочих данных.
3. Зафиксируйте архивы в `downloads/` отдельным коммитом.
4. Добавьте запись в `packaging/releases/manifest.json`. `commit` — **полный SHA коммита с архивами**, а не будущего коммита манифеста.
5. Создайте `vVERSION` на коммите архивов; опубликованные теги и файлы не перемещайте и не заменяйте.
6. Выполните проверку без публикации:

   ```sh
   python3 packaging/publish-releases.py
   ```

7. Отправьте ветку и тег. Workflow `Publish CatalogDB releases` реагирует на изменения манифеста/публикатора в `catalogdb-go` или `master` и может запускаться вручную.
8. Дождитесь успешного workflow, проверьте публичную страницу Release и скачивание всех вложений; сравните контрольные суммы. Только после этого сообщайте о завершении выпуска.

Для проверки исторических архивов нужен Git-history с указанными в манифесте коммитами. В shallow checkout сначала восстановите необходимую историю.

`--publish` у Python-скрипта выполняет запись в GitHub. Workflow использует `CATALOGDB_RELEASE_TOKEN` с fallback `github.token`; выдача прав и наличие секрета зависят от репозитория. Не запрашивайте и не сохраняйте значение токена в контекстных файлах или чате.

### Latest / None: настройка нового выпуска

Для новых выпусков пользователь предпочитает Release label **Latest**, иначе **None**. Publisher поддерживает явное поле `make_latest` отдельно для каждой записи манифеста. Допустимы строки `"true"` и `"false"`, как в GitHub API; булевы значения и другие строки отклоняются.

Пример новой записи:

```json
{
  "version": "0.13.0",
  "commit": "<полный SHA коммита с архивами>",
  "title": "CatalogDB 0.13.0",
  "body": "Описание изменений и проверок",
  "prerelease": false,
  "make_latest": "true",
  "assets": [
    "downloads/<имя-файла>",
    "downloads/<имя-файла>.sha256"
  ]
}
```

Перед добавлением замените placeholders реальным 40-символьным SHA и именами всех файлов. Для метки None используйте `prerelease: false` и `make_latest: "false"`.

GitHub не назначает Latest черновику или prerelease. Publisher всегда создаёт черновик с `make_latest: "false"`, загружает и проверяет вложения, затем применяет выбранную настройку при публикации. Сочетание `prerelease: true` и `make_latest: "true"` отклоняется до записи в GitHub.

У опубликованных исторических записей без `make_latest` publisher не передаёт в PATCH ни `make_latest`, ни `prerelease`, сохраняя фактические labels, в том числе изменённые владельцем вручную. Не добавляйте новую политику ко всей истории. Контракт проверяется командой `python3 -m unittest discover -s packaging -p test_publish_releases.py` без обращения к GitHub.

## Wiki

Wiki — отдельный Git-репозиторий, отсутствующий в обычном checkout основного проекта:

```sh
git clone https://github.com/yomezza/dbcatalog.wiki.git
```

Рабочая ветка Wiki — `master`. На момент передачи опубликованы 17 страниц, `_Sidebar.md`, `_Footer.md`, 11 настоящих Linux-скриншотов и два учебных XLSX. Последнее обновление — `5a45cf8`.

При изменении пользовательского сценария обновите соответствующие страницы; push основного репозитория Wiki не меняет. Внутренние ссылки в Wiki используют имена страниц без `.md`. Иллюстрации и учебные файлы лежат в `assets/`, публичные ссылки имеют вид:

```text
https://raw.githubusercontent.com/wiki/yomezza/dbcatalog/assets/screenshots/11-compact-sidebar.png
```

Снимки и примеры должны содержать вымышленные данные. Исходные приватные Excel пользователя не коммитятся. Учебные XLSX на 21 и 28 колонок проверялись настоящим импортом во временные схемы; это не импорт рабочих данных.
