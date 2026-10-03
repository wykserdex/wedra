# sslyze

Анализ TLS/SSL цели: версии и шифры, renegotiation, heartbleed, certificate
info (SSLyze, nabla-c0t3r) — обёртка над CLI. Машинный отчёт `--json_out`
читается из временного каталога, findings — по записи на каждый сервер и
выполненную проверку.

Установка внешнего инструмента: `pip install sslyze`.
Путь к бинарю переопределяется env `SSLYZE_BIN` (по умолчанию `python3 -m sslyze`).

`commands` — булевы флаги из раздела "Scan commands" в `sslyze --help`:
`certinfo`, `compression`, `early_data`, `elliptic_curves`, `ems`, `fallback`,
`heartbleed`, `http_headers`, `openssl_ccs`, `reneg`, `resum`, `robot`,
`sslv2`, `sslv3`, `tlsv1`, `tlsv1_1`, `tlsv1_2`, `tlsv1_3`. Имена флагов не
совпадают с ключами `scan_result` в JSON-отчёте: `--reneg` → `session_renegotiation`,
`--resum` → `session_resumption`, `--certinfo` → `certificate_info`.