#!/usr/bin/env python3
"""csv_loader — загрузчик CSV для проверки file_ref, массивов, exit-кодов.

Контракт:
  stdin: {path: file_ref, delimiter?: string, has_header?: bool}
  stdout ok: {row_count, headers, rows}
  ошибки:
    bad_input (exit 2) — path не строка, невалидный JSON
    empty_path / file_not_found (exit 1) — доменные ошибки
"""
import csv
import json
import os
import sys


def fail(code, message, exit_code=1, retryable=False):
    print(json.dumps({"status": "error", "error": {"code": code, "message": message, "retryable": retryable}}, ensure_ascii=False))
    return exit_code


def fail_platform(code, message):
    # exit 2 — платформенная ошибка, но с конвертом для триажа (v9: platform:<code>)
    print(json.dumps({"status": "error", "error": {"code": code, "message": message, "retryable": False}}, ensure_ascii=False))
    return 2


def main():
    try:
        data = json.load(sys.stdin)
    except Exception as e:
        return fail_platform("bad_input", f"невалидный JSON: {e}")

    path_raw = data.get("path")
    # guard для проверки exit 2 vs 1 — если path не строка, это bad_input
    if path_raw is None:
        return fail("empty_path", "поле path пустое")
    if not isinstance(path_raw, str):
        return fail_platform("bad_input", f"path должен быть строкой, пришло {type(path_raw).__name__}")

    path = path_raw.strip()
    if not path:
        return fail("empty_path", "поле path пустое")

    delimiter = data.get("delimiter", ",")
    if not isinstance(delimiter, str):
        return fail_platform("bad_input", f"delimiter должен быть строкой, пришло {type(delimiter).__name__}")
    if len(delimiter) != 1:
        return fail("bad_delimiter", f"delimiter должен быть одним символом, пришло {delimiter!r}")

    has_header = data.get("has_header", True)
    if not isinstance(has_header, bool):
        return fail_platform("bad_input", f"has_header должен быть boolean, пришло {type(has_header).__name__}")

    # cwd плагина = его директория (PROTOCOL §1) — относительные file_ref отсюда
    if not os.path.isabs(path):
        # проверяем от cwd плагина
        abs_from_plugin = os.path.join(os.getcwd(), path)
        if not os.path.isfile(abs_from_plugin):
            # подсказка как в v5: файл не найден с cwd в тексте
            return fail("file_not_found", f"файл не найден: {path} (cwd={os.getcwd()})")
        path = abs_from_plugin
    else:
        if not os.path.isfile(path):
            return fail("file_not_found", f"файл не найден: {path} (cwd={os.getcwd()})")

    try:
        with open(path, newline='', encoding='utf-8') as f:
            reader = csv.reader(f, delimiter=delimiter)
            rows = list(reader)
            if not rows:
                return fail("empty_file", f"файл пустой: {path}")
            if has_header:
                headers = rows[0]
                data_rows = rows[1:]
            else:
                headers = []
                data_rows = rows
            # rows как массив массивов — для простоты, проверка array-типа
            out_rows = [dict(zip(headers, r)) if has_header and len(r) == len(headers) else r for r in data_rows]
            print(json.dumps({"status": "ok", "output": {"row_count": len(data_rows), "headers": headers, "rows": out_rows}}, ensure_ascii=False))
            return 0
    except Exception as e:
        return fail("read_error", f"ошибка чтения {path}: {e}")


if __name__ == "__main__":
    sys.exit(main())
