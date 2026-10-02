# nosqlmap

Проверка веб-приложения на MongoDB/NoSQL-инъекции по одной явно заданной цели
(c0rsh/nosqlmap), обёртка над CLI. Только неинвазивный web-app вектор
(`--attack 2`, GET-параметры): без сканирования анонимного доступа, без атак на
NoSQL-порт, без извлечения данных.

Установка внешнего инструмента: `git clone https://github.com/c0rsh/nosqlmap`
(на PyPI пакета `nosqlmap` нет). **Донор 0.7 написан на Python 2** (`print`-инструкции, `raw_input`, `urllib2`), поэтому для реального запуска нужен Python 2: укажите в
`NOSQLMAP_BIN` launcher-скрипт с shebang на python2 (обёртка запускает `.py`
через интерпретатор плагина — Python 3, код nosqlmap под ним не собирается).
Требования донора: `requests`, `httplib2`, `ipcalc`, `pymongo`, `CouchDB`, `pbkdf2`.