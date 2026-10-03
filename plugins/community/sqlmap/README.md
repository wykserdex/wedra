# sqlmap

Аудит SQL-инъекций по явно заданной цели (sqlmapproject/sqlmap), обёртка над CLI sqlmap.

**Sqlmap не публикуется на PyPI.** Установка из git:

```
git clone https://github.com/sqlmapproject/sqlmap
```

Дальше два равнозначных способа запуска: `SQLMAP_BIN=<клон>/sqlmap.py` (обёртка
подставит интерпретатор) либо положить клон в `PYTHONPATH` и оставить дефолт
`[python3 -m sqlmap]`. Путь к бинарю переопределяется env `SQLMAP_BIN`.

Обёртка запускает только безопасный аудит: `--batch --flush-session --level=1
--risk=1`, техники ограничены неразрушающими `B/E/U/T` (stacked-queries `S` и
inline `Q` отклоняются как `bad_technique`), а разрушающие `--os-shell`,
`--file-write`, `--file-read`, `--sql-shell`, `--os-pwn`, `--dump` не применяются
никогда. Сканируется ровно URL из входа: ни `--crawl`, ни `--forms`, ни расширения
на домен. `--output-dir` уходит во временный каталог запуска, и туда же sqlmap
кладёт свой лог (`<output-dir>/<host>/log`, а не `sqlmap.log` в CWD).

Машинный отчёт (`--report-json`) появился только в dev-ветке sqlmap
(1.10.9.32#dev, с 2026-07-19); в стабильном релизе 1.10 его нет, поэтому отчёт
разбирается консервативно по stdout — иначе обёртка ломалась бы на стабильной
версии. Разбираемые маркеры: `testing for SQL injection on ... parameter`,
`does not seem to be injectable`, `parameter '...' is vulnerable`,
`sqlmap identified|resumed the following injection point(s)` и блок
`Parameter: <name> (<place>)`, где на каждую технику идут `Type:`, `Title:`,
`Payload:` именно в таком порядке. Находок нет — это `status: ok` с
`vulnerable: false`, а не ошибка.
