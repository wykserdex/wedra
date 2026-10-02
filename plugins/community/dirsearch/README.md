# dirsearch

Брутфорс директорий и файлов веб-сервера (maurosoria/dirsearch), обёртка над CLI.
Отчёт `dirsearch -o <file> --output-formats json` разбирается в плоский список path/status/size; рекурсия (`-r`) не включается.

Установка внешнего инструмента: `pip install dirsearch`.
Путь к бинарю переопределяется env `DIRSEARCH_BIN`.