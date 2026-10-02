# ipwhois

WHOIS/RDAP по IP-адресу v4/v6 (Philip Hane, pip-пакет `ipwhois`), обёртка над CLI.

Установка внешнего инструмента: `pip install ipwhois` — он ставит скрипт
`ipwhois_cli` в Scripts вашего Python-окружения. Путь к бинарю переопределяется
env `IPWHOIS_BIN` (в тестах туда подставляется `mock_ipwhois.py`).
