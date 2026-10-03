# xsstrike

Поиск XSS по явно заданной цели (s0md3v/XSStrike), обёртка над CLI XSStrike.
Краулинг и `--blind` не включаются: проверяется ровно URL из входа. `--skip`
обязателен — без него XSStrike после находки задаёт вопрос и на неинтерактивном
запуске зависает на вводе.

Установка внешнего инструмента: `pip install xsstrike` — пакет даёт консольный
скрипт `xsstrike` (модуль `xsstrike.xsstrikesback`), но **не** даёт `__main__`,
поэтому `python -m xsstrike` у донора не работает. Либо git-клон
`git clone https://github.com/s0md3v/XSStrike` и тогда
`XSSTRIKE_BIN=<клон>/xsstrike.py`. Путь к бинарю переопределяется env
`XSSTRIKE_BIN` (без него берётся `xsstrike` из PATH).

Файлового отчёта у XSStrike нет, поэтому разбор идёт по stdout: маркеры
`Payload: <vector>` (`modes/scan.py`), `Vulnerable webpage:` + `Vector for <param>:`
(`modes/crawl.py`, только при крауле, который обёртка не включает) и
`Potentially vulnerable objects found` (DOM XSS, `modes/scan.py`). Находок нет —
это `status: ok` с `vulnerable: false`, а не ошибка.
