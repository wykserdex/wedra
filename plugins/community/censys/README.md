# censys

Сервисы и гео хоста по IP или домену через официальный SDK `censys`
(censys-python 2.x: `CensysHosts().view(ip)`, для домена —
`search("dns.names: <домен>")`).

Донор — библиотека, поэтому плагин идёт по паттерну C: запускает
`python -c <SNIPPET> <target>`, сниппет сам импортирует `censys`, читает
`CENSYS_API_ID`/`CENSYS_API_SECRET` из env и печатает JSON. Хоста в индексе
нет — это `ok` с `found: false`.

Установка внешнего инструмента: `pip install censys`.
Ключи: env `CENSYS_API_ID`, `CENSYS_API_SECRET`. Донор переопределяется env
`CENSYS_BIN`.

Оговорка: Censys объявил Search API v1/v2 на вывод из эксплуатации (новый SDK
— `censys-sdk-python`, Censys Platform). Плагин работает с тем API, который
есть в пакете `censys`.