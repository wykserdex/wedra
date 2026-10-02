# python_whois

WHOIS по домену с разбором ответа на поля (richardpenman/whois, PyPI
`python-whois`), обёртка над библиотекой.

Установка внешнего инструмента: `pip install python-whois`. CLI у пакета нет,
поэтому прод-путь — `python -c <сниппет>` из main.py. Путь к запускаемому файлу
переопределяется env `PYTHON_WHOIS_BIN` (в тестах — `mock_python_whois.py`).
