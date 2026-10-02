# virustotal

Репутация файла, домена или IP через официальный клиент VirusTotal `vt-py`
(import `vt`, REST API v3: `vt.Client(key).get_object("/files/<hash>")`,
`/domains/<домен>`, `/ip_addresses/<ip>`).

Донор — библиотека, поэтому плагин идёт по паттерну C: запускает
`python -c <SNIPPET> <indicator> <kind>`, сниппет сам импортирует `vt`, читает
`VT_API_KEY` из env и печатает JSON. Индикатора в базе нет — это `ok` с
`found: false`.

Установка внешнего инструмента: `pip install vt-py`.
Ключ: env `VT_API_KEY`. Донор переопределяется env `VIRUSTOTAL_BIN`.

Про имя пакета: `pip install virustotal` — это **не** официальный SDK, а
сторонний клиент Gawen Arab 2012 года под API 2.0 (внутри `httplib`/`urlparse`,
на Python 3 не импортируется), и API v2 у VirusTotal давно выведен из
эксплуатации. Официальный клиент — `vt-py`.