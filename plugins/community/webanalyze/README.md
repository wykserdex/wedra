# webanalyze

Отпечаток технологий сайта (порт Wappalyzer, rverton/webanalyze), обёртка над CLI.

Это Go-программа, а не pip-пакет (`pip install webanalyze` на PyPI нет):
`go install github.com/rverton/webanalyze/cmd/webanalyze@latest`. Путь к бинарю
переопределяется env `WEBANALYZE_BIN` (в тестах — `mock_webanalyze.py`).
Файл определений технологий задаётся входом `apps_file`, иначе инструмент
ищет `technologies.json` сам.
