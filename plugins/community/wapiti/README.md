# wapiti

Сканер уязвимостей веб-приложений (wapiti-scanner/wapiti), обёртка над CLI wapiti.
Сканируется ровно URL из входа (`--scope url` — без краулинга сайта и без
расширения на домен); модуль по умолчанию `xss,sql` — неразрушающий аудит,
собственный дефолт wapiti (`common`) не используется, так как включает
разрушающие `exec`/`file`/`upload`.

Установка внешнего инструмента: `pip install wapiti3`. Пакет ставит консольный
скрипт `wapiti` (модуль называется `wapitiCore`, точки входа `python -m wapiti`
у него нет — запускать надо именно `wapiti`). Путь к бинарю переопределяется env
`WAPITI_BIN` (принимается и путь к `wapiti.py` из git-клна).
Отчёт (`-f json -o <файл>`) и sqlite-сессия (`--store-session`) пишутся во временный
каталог запуска, наружу ничего не остаётся.

JSON-отчёт — объект с ключами `classifications`, `vulnerabilities`,
`anomalies`, `additionals`, `infos`; внутри `vulnerabilities` словарь
«категория → список находок», а находка несёт `method`, `path`, `parameter`,
`info`, `module`, `level` (плюс `referer`, `http_request`, `curl_command`,
`wstg`).
