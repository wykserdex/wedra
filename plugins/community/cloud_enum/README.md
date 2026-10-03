# cloud_enum

Поиск открытых и защищённых облачных ресурсов по ключевым словам: S3/awsapps (AWS), storage/блобы/БД/ВМ (Azure), GCS/Firebase/App Engine/Cloud Functions (GCP) — обёртка над CLI `cloud_enum` (RHISAC/cloud_enum, ныне initstring/cloud_enum).
Ключевые слова приходят из входа плагина (`key`, `name` → два флага `-k`), а не из env: секретов плагин не читает. Флаги: `-k`, `-l/--logfile`, `-f/--format json`, `-qs/--quickscan`. DigitalOcean текущий апстрим не проверяет.

Установка внешнего инструмента: `pip install git+https://github.com/initstring/cloud_enum` (консольный скрипт `cloud_enum`; работает и `python -m cloud_enum`).
На PyPI имя `cloud-enum` помечено quarantined и файлов не содержит, `pip install cloud_enum` не ставит ничего.
Путь к бинарю переопределяется env `CLOUD_ENUM_BIN`.

Список мутаций: у донора `parse_arguments()` проверяет читаемость ДВУХ файлов — `-m/--mutations` и `-b/--brute`, оба по умолчанию `<каталог запуска>/enum_tools/fuzz.txt`, то есть у консольного скрипта это `venv/Scripts`, а не `site-packages`. Поэтому:
- вход `mutations` плагин отдаёт и в `-m`, и в `-b` (список мутаций годится и как brute-лист — ровно этим же файлом заполнены оба дефолта донора);
- задавайте `mutations` всегда: wheel из pip не везёт `fuzz.txt` вообще (в `[tool.setuptools]` нет package-data), так что без явного пути прогон уйдёт с кодом 0 и без лога — плагин вернёт `no_report` с подсказкой. Путь берите из клона репозитория: `enum_tools/fuzz.txt`.