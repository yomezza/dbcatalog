# Выпуски CatalogDB

Версии 0.1.0–0.13.1: https://github.com/yomezza/dbcatalog/releases.

Исторические выпуски помечены как предварительные; актуальная версия публикуется обычным релизом с меткой Latest. Архивы взяты из соответствующих исторических коммитов без пересборки; файлы SHA-256 опубликованы рядом с ними. Linux появился в 0.10.0, Android — в 0.13.0.

| Версия | Изменения | Платформы |
|---|---|---|
| [0.13.1](https://github.com/yomezza/dbcatalog/releases/tag/v0.13.1) | APK с профилем v1/v2 и стандартными именами сертификатов; проверка совместимости установки | Windows x64, Linux x64, Android ARMv7/ARM64/x86_64 |
| [0.13.0](https://github.com/yomezza/dbcatalog/releases/tag/v0.13.0) | Android-планшеты, общий Go backend, Keystore и системные документы | Windows x64, Linux x64, Android ARMv7/ARM64/x86_64 |
| [0.12.0](https://github.com/yomezza/dbcatalog/releases/tag/v0.12.0) | Сворачиваемая рабочая область и единые иконки навигации | Windows x64, Linux x64 |
| [0.11.0](https://github.com/yomezza/dbcatalog/releases/tag/v0.11.0) | Диагностика приложения и безопасный отчёт JSON | Windows x64, Linux x64 |
| [0.10.0](https://github.com/yomezza/dbcatalog/releases/tag/v0.10.0) | Windows и Linux | Windows x64, Linux x64 |
| [0.9.0](https://github.com/yomezza/dbcatalog/releases/tag/v0.9.0) | PostgreSQL backend и сохранение входа | Windows x64 |
| [0.8.1](https://github.com/yomezza/dbcatalog/releases/tag/v0.8.1) | Управление паролями пользователей | Windows x64 |
| [0.8.0](https://github.com/yomezza/dbcatalog/releases/tag/v0.8.0) | Документы и большие каталоги | Windows x64 |
| [0.7.1](https://github.com/yomezza/dbcatalog/releases/tag/v0.7.1) | Оптимизация структуры приложения | Windows x64 |
| [0.7.0](https://github.com/yomezza/dbcatalog/releases/tag/v0.7.0) | Массовые операции и восстановление | Windows x64 |
| [0.6.0](https://github.com/yomezza/dbcatalog/releases/tag/v0.6.0) | Версии записей и исправление импорта | Windows x64 |
| [0.5.0](https://github.com/yomezza/dbcatalog/releases/tag/v0.5.0) | Oracle непосредственно под сервером | Windows x64 |
| [0.4.0](https://github.com/yomezza/dbcatalog/releases/tag/v0.4.0) | Загрузки, уведомления и подключения | Windows x64 |
| [0.3.0](https://github.com/yomezza/dbcatalog/releases/tag/v0.3.0) | Поиск и управление каталогом | Windows x64 |
| [0.2.0](https://github.com/yomezza/dbcatalog/releases/tag/v0.2.0) | Справочники, избранное и Excel | Windows x64 |
| [0.1.0](https://github.com/yomezza/dbcatalog/releases/tag/v0.1.0) | Основа каталога на Go | Windows x64 |

## Публикация следующих версий

Каждая новая версия приложения обязательно сопровождается GitHub Release; постоянное правило записано в [AGENTS.md](../AGENTS.md).

1. Зафиксируйте сборки в downloads вместе с SHA-256.
2. Добавьте выпуск в packaging/releases/manifest.json с полным SHA коммита, содержащего эти файлы, описанием и списком вложений.
3. Выполните `python3 packaging/publish-releases.py`: проверяются контрольные суммы и точное соответствие файлов историческому коммиту.
4. Отправьте изменения манифеста в catalogdb-go или master. Workflow Publish CatalogDB releases опубликует выпуски через CATALOGDB_RELEASE_TOKEN (fallback: GITHUB_TOKEN) с правом contents:write.

Повторный запуск не перезаписывает файлы с отличающимся содержимым и не перемещает теги. Новый выпуск создаётся черновиком и публикуется после загрузки всех вложений. Для 0.9.0 и 0.10.0 дополнительно опубликован SQL-переход 009 → 016. Самостоятельный запуск с --publish требует авторизованного GitHub CLI.

## Если GitHub отклоняет создание Release

В этой среде GitHub вернул `403 Resource not accessible by integration` на
`POST repos/yomezza/dbcatalog/releases` для встроенного GITHUB_TOKEN, несмотря на
`contents: write` в workflow. Публикация выполнена через CATALOGDB_RELEASE_TOKEN после настройки прав.
Для будущих запусков требуются учётные данные с правом создания выпусков.

Создайте fine-grained personal access token только для репозитория yomezza/dbcatalog
с разрешением **Contents: Read and write**. Добавьте его как repository secret
**CATALOGDB_RELEASE_TOKEN** в Settings → Secrets and variables → Actions.
Не вставляйте токен в исходники, описание выпуска или чат.

Затем повторно запустите последнее задание **Publish CatalogDB releases** через
**Re-run all jobs**. Workflow использует секрет вместо встроенного токена;
архивы уже находятся в репозитории, пересборка не требуется.
