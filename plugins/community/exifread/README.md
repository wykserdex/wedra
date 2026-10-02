# exifread

EXIF-метаданные изображений (lexiflex/exifread, на PyPI — ExifRead), обёртка
над библиотекой: CLI у донора нет, плагин запускает `python -c <сниппет>`.

Установка внешнего инструмента: `pip install exifread`.
Путь к интерпретатору/скрипту переопределяется env `EXIFREAD_BIN`
(в тестах — `mock_exifread.py`). Сеть не нужна, файл берётся с диска.
