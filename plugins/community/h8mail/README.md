# h8mail

Email в утечках (Prosper0800/h8mail; поддерживается khast3x/h8mail, тот же
pip-пакет), обёртка над CLI `h8mail -t <цель> -c <конфиг> -o <csv> -j <json>`.
Плагин **всегда** собирает конфиг с ключами из env во временный каталог и
передаёт его флагом `-c`; отчёт читает из CSV (`Target,Type,Data`), при
отсутствии CSV — из `-j`.

Установка внешнего инструмента: `pip install h8mail`.
Формат конфига — **ini в секции `[h8mail]`**, ровно как его читает сам h8mail
(configparser; тот же формат у `h8mail --gen-config`). JSON-конфиг h8mail не
понимает, поэтому расширение `.ini`, а не `.json`.
Ключи (все опциональны, подставляются пустыми): `HIBP_API_KEY`,
`HUNTERIO_API_KEY`, `SNUSBASE_TOKEN`, `WELEAKINFO_PRIV_KEY` (или `XTLX_API_KEY`),
`WELEAKINFO_PUB_KEY`, `LEAK_LOOKUP_PUB_KEY`, `LEAK_LOOKUP_PRIV_KEY`,
`EMAILREP_API_KEY`, `DEHASHED_EMAIL`, `DEHASHED_KEY`, `INTELX_KEY`,
`INTELX_MAXFILE`, `BREACHDIRECTORY_USER`, `BREACHDIRECTORY_PASS`.
Путь к бинарю переопределяется env `H8MAIL_BIN`.