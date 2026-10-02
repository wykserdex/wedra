# scoutsuite

Аудит безопасности и конфигурации облачного аккаунта ScoutSuite (nccgroup):
обёртка над CLI `scout`, находки читаются из
`<report-dir>/scoutsuite-results/scoutsuite_results_<name>.js`.

Установка внешнего инструмента: `pip install scoutsuite`.
Путь к бинарю переопределяется env `SCOUTSUITE_BIN`.

Учётные данные нужны самому инструменту (`AWS_ACCESS_KEY_ID`,
`AWS_SECRET_ACCESS_KEY`, `AWS_SESSION_TOKEN`, `AWS_DEFAULT_REGION`) — обёртка их
не читает и не требует. Вход `account` — имя AWS-профиля (`scout aws
--profile`), применим только к `provider: aws`. Аудит только собственных
аккаунтов.