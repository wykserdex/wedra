# sherlock

Поиск username по 400+ соцсетям (sherlock-project/sherlock), обёртка над CLI.
Плагин гоняет `sherlock <username> --print-all --no-color` и разбирает текстовый
репорт из stdout (`[+] Сайт: url` / `[-] Сайт: …`).

Установка внешнего инструмента: `pip install sherlock-project`.
Путь к бинарю переопределяется env `SHERLOCK_BIN` (используется и в тестах).