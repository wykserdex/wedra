# instaloader

Метаданные публичного профиля Instagram (instaloader/instaloader) без скачивания
медиа и **без логина**: плагин запускает публичный API instaloader
(`Profile.from_username`, анонимная сессия) и печатает из него JSON. Логин,
пароль и cookie-файл не передаются и не читаются.

У CLI instaloader нет флага `--json` (JSON в stdout) и нет `--no-login` —
проверено по `instaloader/__main__.py` 4.15.3 и запуском самого CLI
(`instaloader --json x` и `instaloader --no-login x` → exit 2,
«unrecognized arguments»); есть только `--no-metadata-json`, и он отключает
запись JSON-файлов постов. Поэтому используется паттерн C: `python -c <сниппет>`;
загрузка картинок/видео и запись метаданных в файлы в сниппете выключены.

Сниппет читает у `Profile` только свойства, которые реально нужны выходу:
`username`, `full_name`, `followers`, `followees`, `mediacount` (`posts`),
`is_private`. Свойства `biography` и `profile_pic_url` в выход не идут и в
сниппете намеренно отсутствуют: первое падает `TypeError` на биографии `null`,
второе читает ключ `profile_pic_url_hd`, который `Profile.from_username` не
добавляет, и падает `KeyError` — то есть любое из них уронило бы прогон целиком.

Установка внешнего инструмента: `pip install instaloader`.
Вся команда переопределяется env `INSTALOADER_BIN` (единственный путь для
тестов).
