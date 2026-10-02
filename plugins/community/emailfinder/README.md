# emailfinder

Email-адреса домена (soxoj/emailfinder; на PyPI — josue87/emailfinder, тот же
`pip install emailfinder`), обёртка над CLI `emailfinder -d <домен>`. Машинного
формата вывода у пакета не подтверждено, поэтому плагин разбирает stdout
универсальной регуляркой по адресам и берёт те, что на целевом домене.

Установка внешнего инструмента: `pip install emailfinder`.
Ключ Hunter.io: **читает сам инструмент** из env `HUNTER_API_KEY` — плагин его
не проверяет (в `secrets` манифеста он объявлен, в тестах ключ не нужен).
Путь к бинарю переопределяется env `EMAILFINDER_BIN`.