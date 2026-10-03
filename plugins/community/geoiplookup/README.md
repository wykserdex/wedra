# geoiplookup

GeoIP по IP-адресу по локальной базе, обёртка над CLI `geoiplookup` из пакета
MaxMind `geoip-bin` (`apt install geoip-bin`). Сеть не нужна: база читается с
диска.

Инструмент — легаси-C-утилита (`maxmind/geoip-api-c`, `apps/geoiplookup.c`),
поэтому читает только старые `.dat`-базы (GeoIP.dat, GeoIPCity.dat,
GeoIPRegion.dat, GeoIPOrg.dat, GeoIPASNum.dat, ...); MMDB (`GeoLite2-*.mmdb`,
DB-IP) она не открывает — это `mmdblookup`, другой инструмент.

Свой путь к базе отдаётся флагом `-f` (один файл) или `-d` (каталог: утилита
сама перебирает найденные базы). Флаг `-v` печатает дату/сборку базы вместо
ответа; строки ответа всегда идут с подписью базы — `GeoIP Country Edition:
NL, Netherlands`, — и плагин разбирает хвост после подписи.

Путь к базе задаётся входом `db_path` или env `GEOIP_DB_PATH`; без него — ошибка
`missing_db`. Путь к бинарю переопределяется env `GEOIPLOOKUP_BIN`.