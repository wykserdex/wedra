# abuseipdb

Репутация IP по базе жалоб AbuseIPDB: `abuseConfidenceScore` (0..100), число жалоб,
страна. Защитная проверка: только запрос репутации, сканирований нет.

## Установка донора

`pip install abuseipdb` **не работает на Python 3**. Единственный релиз на PyPI —
1.3.0 (2018-04-24), и он не ставится: `setup.py` импортирует сам пакет, а
`__init__.py` тянет `_app.py`, где `import unirest` (unirest 1.1.7 — только py2)
и неявный относительный импорт `from parameters import Parameters`. Сборка падает
с `ModuleNotFoundError` ещё до установки.

Рабочий донор — git-master того же репозитория (`vsecades/AbuseIpDb`,
`__version__ = 3.0.0`):

```
pip install requests
pip install --no-build-isolation git+https://github.com/vsecades/AbuseIpDb.git
```

`--no-build-isolation` обязателен: `setup.py` мастера тоже импортирует пакет (а
значит тянет `requests`), а `pyproject.toml` с build-requires у него нет.

Сниппет сам выбирает ветку по наличию класса: у 3.0.0 это
`AbuseIpDb(key).check(ip, days)` (внутри `GET
https://api.abuseipdb.com/api/v2/check`, заголовок `Key`, возвращает готовый
dict), у 1.3.0 — модульные `configure_api_key`/`check_ip` (на py3 недостижимы, и
`check_ip` печатает URL с ключом в stdout — оттого ветка и оставлена только
терпимой).

Ключ — env `ABUSEIPDB_API_KEY` (сниппет читает его сам, в коде ключа нет).
Путь к донору переопределяется env `ABUSEIPDB_BIN` — тесты идут через
`mock_abuseipdb.py`, без сети и без ключа.