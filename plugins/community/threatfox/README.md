# threatfox

Поиск IOC (ip, ip:port, домен, URL, хэш) в базе abuse.ch ThreatFox: тип угрозы,
семейство малвари, `confidence_level`. Защитная проверка: только запрос к базе,
сканирований нет.

Внешнего пакета нет: запрос делает дочерний процесс со сниппетом на stdlib-urllib
(POST `search_ioc` на `threatfox-api.abuse.ch`). abuse.ch требует Auth-Key для
Community API — задайте env `THREATFOX_AUTH_KEY`, если API вернёт 401.
Путь к донору переопределяется env `THREATFOX_BIN` — тесты идут через
`mock_threatfox.py`, без сети и без ключа.