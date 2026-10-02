# geoiplookup

GeoIP по IP-адресу по локальной базе (GeoIP/GeoLite2/DB-IP), обёртка над CLI
`geoiplookup` из пакета MaxMind `geoip-bin` (`apt install geoip-bin`). Сеть не
нужна: база читается с диска.

Путь к базе задаётся входом `db_path` или env `GEOIP_DB_PATH`; без него — ошибка
`missing_db`. Путь к бинарю переопределяется env `GEOIPLOOKUP_BIN`.
