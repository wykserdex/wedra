# instaloader

Метаданные публичного профиля Instagram (instaloader/instaloader) без скачивания
медиа и **без логина**: плагин запускает публичный API instaloader
(`Profile.from_username`, анонимная сессия) и печатает из него JSON. Логин,
пароль и cookie-файл не передаются и не читаются.

У CLI instaloader нет флага `--json` (JSON в stdout) и нет `--no-login` —
проверено по `docs/cli-options.rst` и `instaloader/__main__.py` 4.15.3; есть
только `--no-metadata-json`, и он отключает запись JSON-файлов постов. Поэтому
используется паттерн C: `python -c <сниппет>`; загрузка картинок/видео и запись
метаданных в файлы в сниппете выключены.

Установка внешнего инструмента: `pip install instaloader`.
Вся команда переопределяется env `INSTALOADER_BIN` (единственный путь для
тестов).
