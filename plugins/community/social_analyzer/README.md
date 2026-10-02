# social_analyzer

Поиск username по 400+ соцсетям (qeeqbox/social-analyzer, он же
okandemirci/social-analyzer), обёртка над CLI. Инструмент умеет обходить простую
антибот-защиту; плагин просит `--output json` и разбирает машинный JSON из
stdout, при неудаче — текстовые строки `[*] Found: …`.

Установка внешнего инструмента: `pip install social-analyzer`.
Запуск по умолчанию: `python3 -m social-analyzer` (имя модуля с дефисом — так
его документирует автор; консольный скрипт `social-analyzer` даёт тот же CLI).
Путь к бинарю переопределяется env `SOCIAL_ANALYZER_BIN`.