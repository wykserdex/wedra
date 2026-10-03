# ghauri

Детектор SQL-инъекций (r0oth3x49/ghauri, Python-наследник sqlmap), обёртка над
CLI `ghauri -u <url> --batch --level 1 --timeout N` (все четыре флага — реальные
флаги ghauri).

Плагин работает **только** по явно заданной пользователем цели и только как
неинвазивный детектор: флаги перечисления и эксплуатации (`--dbs`, `--tables`,
`--dump`, `--sql-shell`) не передаются никогда, поэтому файлы на цель не
пишутся и команды ОС не выполняются. HOME гоняется во временный каталог, так
что наружу от рана ничего не остаётся.

Отчётных источников два, и плагин разбирает оба:

1. **вывод процесса (stderr)** — весь ColoredLogger ghauri: признак инъекции
   `GET parameter 'id' appears to be '<TITLE>' injectable` (уровень NOTICE) и
   итоги уровня CRITICAL (`all tested parameters do not appear to be
   injectable.`, `no parameter(s) found for testing ...`). ANSI-коды приезжают
   внутри самого сообщения и вырезаются перед разбором;
2. **`~/.ghauri/<host>/log`** — FileHandler у него имеет
   `setLevel(SUCCESS)`, поэтому в файл попадают только записи уровня SUCCESS:
   блок `Parameter: id (GET)` / `Type:` / `Title:` / `Payload:`. На
   неинъекционном цели файл пустой (0 байт) — ошибкой это не считается,
   результат `vulnerable: false`.

Проверено на ghauri 1.4.3.

Установка внешнего инструмента: пакета `ghauri` на PyPI нет (404 на
`https://pypi.org/pypi/ghauri/json`), официальный способ —
`pip install "git+https://github.com/r0oth3x49/ghauri.git"`.
Путь к бинарю переопределяется env `GHAURI_BIN`.