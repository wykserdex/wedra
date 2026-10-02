# cloud_enum

Поиск открытых и защищённых облачных ресурсов по ключевым словам: S3/awsapps (AWS), storage/блобы/БД/ВМ (Azure), GCS/Firebase/App Engine/Cloud Functions (GCP) — обёртка над CLI `cloud_enum` (RHISAC/cloud_enum, ныне initstring/cloud_enum).
Ключевые слова приходят из входа плагина (`key`, `name` → два флага `-k`), а не из env: секретов плагин не читает. Флаги: `-k`, `-l/--logfile`, `-f/--format json`, `-m/--mutations`, `-qs/--quickscan`. DigitalOcean текущий апстрим не проверяет.

Установка внешнего инструмента: `pip install cloud_enum` (консольный скрипт `cloud_enum`; работает и `python -m cloud_enum`).
Путь к бинарю переопределяется env `CLOUD_ENUM_BIN`. Если инструмент не нашёл свой список мутаций (при запуске бинаря из PATH ищем `enum_tools/fuzz.txt` рядом с ним) — передайте вход `mutations` с путём к файлу из пакета.