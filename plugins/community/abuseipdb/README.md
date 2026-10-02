# abuseipdb

Репутация IP по базе жалоб AbuseIPDB: `abuseConfidenceScore` (0..100), число жалоб,
страна. Защитная проверка: только запрос репутации, сканирований нет.

Установка донора: `pip install abuseipdb`. Ключ — env `ABUSEIPDB_API_KEY`
(сниппет читает его сам, в коде ключа нет).
Путь к донору переопределяется env `ABUSEIPDB_BIN` — тесты идут через
`mock_abuseipdb.py`, без сети и без ключа.