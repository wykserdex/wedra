# prowler

Аудит соответствия и безопасности облака Prowler (prowler-cloud), обёртка над
CLI `prowler`: JSON-OCSF-отчёт `<output-dir>/<name>.ocsf.json` приводится к
`findings[{check, severity, title, status}]`.

Установка внешнего инструмента: `pip install prowler`.
Путь к бинарю переопределяется env `PROWLER_BIN`.

Ключи нужны самому инструменту (`AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`,
`AWS_SESSION_TOKEN`, `AWS_DEFAULT_REGION`/`AWS_REGION`) — обёртка их не читает и
не требует. Аудит только собственных аккаунтов.

ВНИМАНИЕ к версиям: нативных `-f json`/`-o <файл>` в парсере prowler нет — в
wheel 5.44.0 (`prowler/lib/cli/parser.py`) таких флагов нет вовсе, а
`-M/--output-modes` принимает `{csv,json-asff,json-ocsf,html,sarif}`. Обёртка
поэтому использует актуальный `-M json-ocsf -o <dir> -F <name>`. Флаг
`-z/--ignore-exit-code-3` обязателен: без него находки дают exit 3.