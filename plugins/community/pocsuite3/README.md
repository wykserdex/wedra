# pocsuite3

Проверка явно заданного URL на уязвимости из набора PoC Suite3 (poc-suite/pocsuite3), обёртка над CLI `pocsuite`.
Неинтерактивный прогон: `-u <url> --verify`; машинный вывод — `-o <file>` (JSON Lines, плагин file_record). Флага `--format` у pocsuite3 нет.
Флаг `--batch` не передаём: в 2.1.0 он требует значения (`--batch BATCH`), голый `--batch` роняет argparse, и `conf.batch` всё равно нигде не читается.

Установка внешнего инструмента: `pip install pocsuite3`.
Точка входа — консольный скрипт `pocsuite` (второй — `poc-console`, интерактивный). Скрипта `poc` у пакета нет и `python -m pocsuite3` не работает.
Путь к бинарю переопределяется env `POCSUITE3_BIN`.