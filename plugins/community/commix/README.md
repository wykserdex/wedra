# commix

Поиск OS-командных инъекций по одной явно заданной цели (commixproject/commix),
обёртка над CLI. Только неинвазивный аудит: `--batch`, без краулинга, без
файловых техник (`--skip-technique=f`), без эксплуатации. Машинный отчёт —
`--report-json`.

Установка внешнего инструмента: commix 4.2+ из исходников —
`git clone https://github.com/commixproject/commix && pip install -r requirements.txt`
(на PyPI лежит посторонний пакет `commix` 0.1, это не commixproject).
Путь к бинарю переопределяется env `COMMIX_BIN`.