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
на домен. Лог `sqlmap.log` и `--output-dir` уходят во временный каталог запуска.

У sqlmap нет признанного машинного формата вывода, поэтому отчёт разбирается
консервативно по stdout (маркеры `testing for SQL injection on ... parameter`,
`Parameter:`/`Payload:`/`Type:`, `does not seem to be injectable`). Находок нет —
это `status: ok` с `vulnerable: false`, а не ошибка.
