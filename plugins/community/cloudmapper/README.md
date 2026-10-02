# cloudmapper

Карта собственной облачной инфраструктуры AWS (nodes/edges из `web/data.json`),
обёртка над CLI CloudMapper: шаги `collect` + `prepare` во временном каталоге.

Установка внешнего инструмента: в репозитории CloudMapper нет `setup.py`, pip-пакета
`cloudmapper` нет — ставьте из git:
`git clone https://github.com/duo-labs/cloudmapper && pip install -r requirements.txt`.
Путь к бинарю/скрипту переопределяется env `CLOUDMAPPER_BIN`.

Ключи AWS нужны самому инструменту (он читает `AWS_ACCESS_KEY_ID`,
`AWS_SECRET_ACCESS_KEY`, `AWS_SESSION_TOKEN`, `AWS_REGION`/`AWS_DEFAULT_REGION`
через boto3). Обёртка их не читает и не требует: без ключей донор вернёт ошибку
сам. Аудит только собственных аккаунтов.