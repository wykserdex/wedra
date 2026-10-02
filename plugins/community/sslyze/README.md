# sslyze

Анализ TLS/SSL цели: версии и шифры, renegotiation, heartbleed, certificate
info (SSLyze, nabla-c0t3r) — обёртка над CLI. Машинный отчёт `--json_out`
читается из временного каталога, findings — по записи на каждый сервер и
выполненную проверку.

Установка внешнего инструмента: `pip install sslyze`.
Путь к бинарю переопределяется env `SSLYZE_BIN` (по умолчанию `python3 -m sslyze`).