# joomscan

Неинвазивный аудит Joomla-сайта (OWASP/joomscan), обёртка над CLI: версия, известные уязвимости и ошибки конфигурации, перечисление компонентов.
Флаги донора: `-u/--url`, `--enumerate-components` (`-ec`), `--timeout`, `--no-report` (`-nr`). Флагов `--force` и `--enumerate` у joomscan НЕТ — не выдумываем.
Отчёт: `reports/<host>/<host>_report_<дата>_at_<время>.txt`, маркеры строк — `[+] <проверка>` и `[++] <детали>`. Каталог `reports/<host>` создаёт сам donor, но его `mkdir` не рекурсивный — плагин заранее создаёт `reports/` во временной папке, иначе отчёта не бывает.

Установка внешнего инструмента: `apt install joomscan` (Kali/Debian) или `git clone https://github.com/OWASP/joomscan` и `perl joomscan.pl`.
Донор — Perl-скрипт, пакета на PyPI нет (`pip install joomscan` не существует), точки входа `python -m joomscan` тоже нет: запускается сам бинарь `joomscan`/`joomscan.pl`.
Путь к нему переопределяется env `JOOMSCAN_BIN`.