# volatility3

Анализ дампа памяти (VolatilityFoundation/volatility3): обёртка над CLI `vol`.
Запускает плагин volatility3 (`banners.Banners` по умолчанию, например `windows.pslist`)
на локальном дампе и разбирает JSON-рендерер (`-r json`).

Установка внешнего инструмента: `pip install volatility3` (даёт CLI-скрипт `vol`).
Путь к бинарю переопределяется env `VOLATILITY3_BIN`.
Сеть нужна только volatility3 для ISF-символов; сам плагин сеть не ходит.