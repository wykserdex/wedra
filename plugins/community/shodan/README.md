# shodan

Открытые порты, хостнеймы и известные CVE по IP через официальный SDK
`shodan` (shodan-python 1.31, `Shodan(api_key).host(ip)`).

Донор — библиотека без пригодного CLI, поэтому плагин идёт по паттерну C:
запускает `python -c <SNIPPET> <ip>`, сниппет сам импортирует `shodan`, читает
`SHODAN_API_KEY` из env и печатает JSON. Хоста в базе Shodan (404) — это `ok` с
`found: false`, а не ошибка.

Установка внешнего инструмента: `pip install shodan`.
Ключ: env `SHODAN_API_KEY`. Донор переопределяется env `SHODAN_BIN`.