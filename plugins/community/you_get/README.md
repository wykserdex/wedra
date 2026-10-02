# you_get

Метаданные ролика/медиа с разных видеосервисов (soimort/you-get), обёртка над
CLI в режиме «только метаданные»: `you-get --json <url>`. Флаг сам включает
dry-run и заканчивает работу в `json_output.output()` до загрузки файла, так
что медиа не скачивается (флага `--skip-download` в CLI you-get нет).

Из отчёта (`you_get/json_output.py`: url, title, site, streams) плагин отдаёт
title, extractor (`site`) и число потоков; ключей `uploader`/`duration` в схеме
you-get нет — они нормализуются в `""` и `0`.

Установка внешнего инструмента: `pip install you-get`.
Путь к бинарю переопределяется env `YOU_GET_BIN`.
