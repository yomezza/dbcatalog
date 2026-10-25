# Android-оболочка CatalogDB

Android-клиент использует **тот же Go application service и React-интерфейс**, что и Windows/Linux. Отдельного HTTP-сервера, REST API или второй реализации правил каталога нет. Основное устройство — планшет. Минимальная версия — Android 8.0 / API 26, target API 35; нужен актуальный Android System WebView.

## Состав

- `MainActivity.java`: WebView, асинхронный вызов Go, жизненный цикл, выбор файлов и внешние клиенты.
- `LocalAssetsClient.java`: только встроенные ресурсы `assets/web` под фиксированным HTTPS-origin `https://appassets.androidplatform.net`.
- `SecureCredentials.java`: реализация Go CredentialStore через Android Keystore AES-GCM и зашифрованные app-private SharedPreferences.
- `ExportProvider.java`: временный доступ внешнему приложению только для одного выбранного файла из app-private `files/downloads`.
- `AndroidManifest.xml`: INTERNET, launcher Activity и закрытый ContentProvider; широких разрешений хранилища нет, резервное копирование app-private данных отключено.

## Контракт с Go и интерфейсом

`gomobile bind` создаёт AAR из пакета `github.com/yomezza/test/mobile`. Java использует `mobile.Mobile.newClient(configPath, store, sink)` и `mobile.Client`. Путь конфигурации находится в `Context.getFilesDir()`; desktop `OSStore` и поиск системного каталога настроек не используются.

JS отправляет в `window.CatalogDBAndroid.postMessage()` JSON:

```json
{"id":"request-1","method":"GetStatus","args":[]}
```

Метод вызывается на worker, результат передаётся на UI thread:

```js
window.__catalogdbAndroidReceive({id: "request-1", result: {}});
window.__catalogdbAndroidReceive({id: "request-2", error: "Сообщение пользователю"});
```

События LISTEN/NOTIFY проходят через Go EventSink и `window.__catalogdbAndroidEvent({name: 'catalogdb:change', data: {schema, type}})`. Обычные методы ограничены явным allowlist в `mobile/client.go`. Настоящие пути файлов выдаются отдельными Go-методами только native host; JavaScript не получает произвольного чтения файлов.

Синхронных SQL-вызовов с UI thread нет. При завершении Activity закрывается Go client. Смена ориентации сохраняет Activity и WebView. Возврат приложения запускает UI resync; работа постоянной службы в фоне не обещается.

## Нативные действия

- Загрузка XLS/XLSX и вложений — стандартный WebView file chooser, который выдаёт доступ только выбранному документу.
- «Сохранить как…» и диагностический JSON — `ACTION_CREATE_DOCUMENT`. Отмена возвращает `false`.
- Выбор корневого сертификата — `ACTION_OPEN_DOCUMENT`, до 2 МиБ. X.509 PEM/DER преобразуется в PEM в app-private папке; отмена возвращает пустую строку.
- «Открыть» использует ACTION_VIEW и отдельный read-grant ContentProvider. SQL/PS1 автоматически не открываются.
- Android-подпись «Поделиться» соответствует прежнему вызову `ShowDownload` и использует ACTION_SEND с тем же ограниченным read-grant. Папка приложения наружу не открывается.
- RDP/SSH — внешние клиенты, поддерживающие ссылки `rdp:` / `ssh:`; перед запуском проверяется клиент и TCP-порт 3389/22. Сообщения сертификата или аутентификации после запуска принадлежат внешнему клиенту.

WebView не открывает внешние страницы и не получает доступ к `file:` URL. Скрипты разрешены только из встроенных ресурсов; фреймы и WebView-сетевые подключения блокирует CSP. Соединение с PostgreSQL выполняется Go-кодом и сохраняет выбранные пользователем настройки TLS.

## Упаковка APK

Оболочка не зависит от Gradle/AndroidX. Сборка использует Android SDK platform/build-tools, JDK, AAR `classes.jar` и `jni/<ABI>/libgojni.so`, затем `aapt2`, `javac`, `d8`, `zipalign`, `apksigner`. `packaging/build-android.sh` сам упаковывает готовый `frontend/dist` в `assets/web`. Инструкция по сборке и подписи — в [docs/android.md](../docs/android.md).

ABI APK должны совпадать с архитектурами AAR. Для современных Android с 16 КиБ страницами дополнительно проверьте ELF alignment `libgojni.so`; при необходимости задайте native linker `-Wl,-z,max-page-size=16384`. APK zip alignment не заменяет правильное ELF alignment.

APK должен быть подписан для установки. **Закрытый ключ и пароль не включаются в Git.** Для обновления поверх установленного APK нужен тот же signing certificate и увеличенный `versionCode`. Генерация нового ключа в другой среде не позволит обновить прежнюю установку без удаления. Порядок безопасного хранения signing material задаёт владелец проекта; наличие текущего локального ключа не гарантируется на другой машине.

## Проверки

Сборка Java/AAR и проверка подписи подтверждают пригодность пакета, но не заменяют запуск на эмуляторе/планшете. Отдельно проверяются WebView bridge, адаптация portrait/landscape, TLS и VPN, Keystore после перезапуска, системные файловые диалоги, передача внешним приложениям и RDP/SSH.

У Android `localhost` — сам планшет. Для центральной PostgreSQL нужен доступный адрес сервера и при необходимости VPN. Прикладные роли и существующие ограничения общей PostgreSQL-учётной записи сохраняются; эта оболочка не внедряет обсуждавшийся переход на персональные PG-логины.
