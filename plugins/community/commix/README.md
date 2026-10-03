# commix

Поиск OS-командных инъекций по одной явно заданной цели (commixproject/commix),
обёртка над CLI. Только неинвазивный аудит: `--batch`, без краулинга, без
файловых техник (`--skip-technique=f`), без эксплуатации. Машинный отчёт —
`--report-json`.

Установка внешнего инструмента: **ветка `master`** commix —
`git clone https://github.com/commixproject/commix && python3 commix.py -h`.
Зависимостей ставить не надо: в репозитории нет `requirements.txt`, все
сторонние библиотеки лежат в `src/thirdparty`. На PyPI лежит посторонний пакет
`commix` 0.1, это не commixproject.

Почему именно `master`, а не «4.2+»: последний тег — `v4.1` (декабрь 2025), а
`--report-json` добавлен позже, коммитом `0c12b380` от 2026-09-04. Релиза 4.2 не
вышло; ветка `master` сообщает о себе как `v4.2.devN` (`setup.py`:
`version='4.2.dev'`). Путь к бинарю переопределяется env `COMMIX_BIN`.