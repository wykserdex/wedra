#!/usr/bin/env python3
"""detect_secrets — поиск секретов в коде (обёртка над detect-secrets).

Вход (stdin JSON): path (каталог или файл), wall_timeout (опц., 300).

Вызов: <DETECT_SECRETS_BIN|python3 -m detect_secrets> scan <abspath> --all-files
       (cwd = временная папка).

detect-secrets печатает baseline-JSON в stdout:
{version, plugins_used[], filters_used[], results{<file>:[{type, filename,
line_number, hashed_secret, is_verified}]}}. Значения секретов и их хэши в
output НЕ попадают — только тип, имя файла и номер строки. Пустой results —
нормальный результат («секретов не нашли»).

Доменные ошибки: empty_path, missing_path, detect_secrets_not_installed,
timeout (retryable), no_report, tool_failed. Платформенные (exit 2): битый
JSON входа, не-объект/не-число во входе, битый baseline-репорт.
"""
import json
import os
import shutil
import subprocess
import sys
import tempfile

DEFAULT_WALL = 300


def ok(output):
    print(json.dumps({"status": "ok", "output": output}, ensure_ascii=False))
    return 0


def fail(code, message, retryable=False, exit_code=1):
    print(json.dumps({"status": "error",
                      "error": {"code": code, "message": message,
                                "retryable": retryable}},
                     ensure_ascii=False))
    return exit_code


def extract_json(text):
    """Достаёт первый JSON-объект из stdout (он может идти после прогресса).

    Возвращает (объект|None, похоже_на_json): второе нужно, чтобы отличить
    «репорта нет вовсе» (no_report) от «репорт есть, но битый» (bad_report).
    """
    try:
        obj = json.loads(text)
        if isinstance(obj, dict):
            return obj, True
    except ValueError:
        pass
    start = text.find("{")
    while start != -1:
        try:
            obj, _ = json.JSONDecoder().raw_decode(text, start)
            if isinstance(obj, dict):
                return obj, True
        except ValueError:
            pass
        start = text.find("{", start + 1)
    return None, "{" in text


def main():
    try:
        sys.stdin.reconfigure(encoding="utf-8", errors="replace")
        sys.stdout.reconfigure(encoding="utf-8")
        sys.stderr.reconfigure(encoding="utf-8", errors="replace")
    except Exception:
        pass

    try:
        data = json.load(sys.stdin)
    except Exception as e:
        return fail("bad_input", f"невалидный JSON на входе: {e}", exit_code=2)
    if not isinstance(data, dict):
        return fail("bad_input", "ожидается JSON-объект на входе", exit_code=2)

    path = str(data.get("path") or "").strip()
    if not path:
        return fail("empty_path", "path пуст")

    try:
        wall = float(data.get("wall_timeout") or DEFAULT_WALL)
    except (TypeError, ValueError):
        return fail("bad_wall_timeout", "wall_timeout обязан быть числом",
                    exit_code=2)

    if not os.path.exists(path):
        return fail("missing_path", f"путь не найден: {path}")
    target = os.path.abspath(path)

    bin_env = os.environ.get("DETECT_SECRETS_BIN", "").strip()
    if bin_env:
        # subprocess поедет с cwd во временной папке: имя из PATH ищем
        # which'ем, путь — приводим к абсолютному
        if "/" not in bin_env and "\\" not in bin_env:
            cmd = [shutil.which(bin_env) or os.path.abspath(bin_env)]
        else:
            cmd = [os.path.abspath(bin_env)]
    else:
        cmd = [sys.executable, "-m", "detect_secrets"]
    if len(cmd) == 1 and cmd[0].lower().endswith(".py"):
        # v0.29: .py-мок запускаем через интерпретатор — прямой exec
        # непереносим (shebang+CRLF на Linux, ассоциации на Windows).
        cmd = [sys.executable] + cmd
    cmd += ["scan", target, "--all-files"]

    with tempfile.TemporaryDirectory() as td:
        try:
            proc = subprocess.run(cmd, capture_output=True, text=True,
                                  cwd=td, timeout=wall)
        except FileNotFoundError:
            return fail("detect_secrets_not_installed",
                        "detect-secrets не найден: pip install detect-secrets "
                        "(или укажите DETECT_SECRETS_BIN)")
        except subprocess.TimeoutExpired:
            return fail("timeout",
                        f"detect-secrets не уложился в {wall:.0f}s: уменьшите "
                        "объём сканируемого пути или увеличьте wall_timeout",
                        retryable=True)
        if proc.stderr:
            sys.stderr.write(proc.stderr)

        stdout = proc.stdout or ""
        if proc.returncode != 0 and not stdout.strip():
            tail = (proc.stderr or "").strip().splitlines()
            last = tail[-1] if tail else f"exit {proc.returncode}"
            return fail("tool_failed",
                        f"detect-secrets упал: {last}", retryable=False)

        baseline, looks_like_json = extract_json(stdout)
        if baseline is None:
            if looks_like_json:
                return fail("bad_report",
                            "baseline detect-secrets не разбирается как JSON",
                            exit_code=2)
            return fail("no_report",
                        "detect-secrets не дал baseline-JSON в stdout "
                        "(нет репорта или он пуст)")

    results = baseline.get("results")
    if results is None:
        return fail("bad_report",
                    "в baseline detect-secrets нет поля results", exit_code=2)
    if not isinstance(results, dict):
        return fail("bad_report", "results в baseline — не объект",
                    exit_code=2)

    secrets = []
    for filename, items in results.items():
        if not isinstance(items, list):
            continue
        for item in items:
            if not isinstance(item, dict):
                continue
            line = item.get("line_number")
            try:
                line = int(line)
            except (TypeError, ValueError):
                line = 0
            secrets.append({
                "type": str(item.get("type") or ""),
                "filename": str(item.get("filename") or filename),
                "line_number": line,
            })
    secrets.sort(key=lambda s: (s["filename"], s["line_number"], s["type"]))

    return ok({"path": target, "secrets": secrets, "count": len(secrets)})


if __name__ == "__main__":
    sys.exit(main())