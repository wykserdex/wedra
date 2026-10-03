# nosqlmap

Проверка веб-приложения на MongoDB/NoSQL-инъекции по одной явно заданной цели
(codingo/NoSQLMap), обёртка над CLI. Только неинвазивный web-app вектор
(`--attack 2`, GET-параметры): без сканирования анонимного доступа, без атак на
NoSQL-порт, без извлечения данных.

Установка внешнего инструмента: `git clone https://github.com/codingo/NoSQLMap`
(на PyPI пакета `nosqlmap` нет; репозиторий `c0rsh/nosqlmap`, который раньше
указывался здесь, больше не существует — 404, канонический форк сейчас
`codingo/NoSQLMap`). **Донор 0.7 написан на Python 2** (`print`-инструкции, `raw_input`, `urllib2`), поэтому для реального запуска нужен Python 2: укажите в
`NOSQLMAP_BIN` launcher-скрипт с shebang на python2 (обёртка запускает `.py`
через интерпретатор плагина — Python 3, код nosqlmap под ним не собирается).
Требования донора: `requests`, `httplib2`, `ipcalc`, `pymongo`, `CouchDB`, `pbkdf2`.

Обязательные флаги CLI, которые обёртка всегда передаёт: `--params` (номера
query-параметров с единицы — донор зовёт `int(p)-1`), `--injectSize`,
`--injectFormat`, `--doTimeAttack y` и `--savePath`. `--params` и
`--doTimeAttack` обязательны на уровне донора: без них `nsmweb.buildUri()` и
`nsmweb.getApps()` падают на `None.split()`/`.lower()` раньше `save_to()`, так
что файла отчёта не появляется.