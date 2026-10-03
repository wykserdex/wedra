# exifread

EXIF-метаданные изображений (lexiflex/exifread, на PyPI — ExifRead), обёртка
над библиотекой: у донора CLI печатает человекочитаемые строки, а не JSON,
поэтому плагин запускает `python -c <сниппет>` поверх `exifread.process_file`.

Установка внешнего инструмента: `pip install exifread`.
Путь к интерпретатору/скрипту переопределяется env `EXIFREAD_BIN`
(в тестах — `mock_exifread.py`). Сеть не нужна, файл берётся с диска.
