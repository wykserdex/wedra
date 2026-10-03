#!/usr/bin/env python3
"""oletools — анализ макросов офисного документа (обёртка над olevba).

Вход (stdin JSON): file (путь к документу), wall_timeout (опц., 300).

Вызов: <OLETOOLS_BIN|olevba> -j -a <abspath>  (cwd = временная папка).
`-j/--json` — машинный режим оlevba (oletools 0.60+), он не дефолтный; `-a` гасит
печать исходников макросов (display_code=False), поэтому в вывод плагина код
не попадает.

Формат stdout в -j: ОДИН JSON-МАССИВ `[...]`. Его открывает
`log_helper.enable_logging()` (печатает `[` в stdout), каждый объект печатает
`print_json()` — первый с отступом, остальные с `,` + отступ перед ним, что
ровно разделитель элементов массива, — а закрывает `log_helper.end_logging()`
(печатает `]`). Внутри: сначала MetaInformation (script_name/version/
python_version/url/type), затем по одному объекту на файл:
{container, file, json_conversion_successful, analysis, code_deobfuscated,
do_deobfuscate, show_pcode, type, macros}, где
analysis = [{type, keyword, description}] (null, если макросов нет),
macros = [{vba_filename, subfilename, ole_stream, code}], а code с `-a` = null.
MetaInformation пропускаем.

Доменные ошибки: empty_file, missing_file, oletools_not_installed, timeout
(retryable), no_report, tool_failed. Платформенные (exit 2): битый JSON входа,
не-объект/не-число во входе, нечитаемый JSON-репорт.
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


def read_json_objects(text):
    """Разбирает stdout olevba в режиме -j в список объектов.

    Настоящий olevba печатает ОДИН JSON-массив (см. docstring), поэтому сначала
    пробуем целиком json.loads(). Если не вышло (мусор до/после, вариант без
    скобок) — читаем объекты потоком через raw_decode, пропуская разделители
    `,` и скобки массива.
    """
    text = text or ""
    try:
        parsed = json.loads(text)
    except ValueError:
        pass
    else:
        return parsed if isinstance(parsed, list) else [parsed]

    decoder = json.JSONDecoder()
    objects = []
    pos = 0
    length = len(text)
    while pos < length:
        while pos < length and (text[pos].isspace() or text[pos] in ",[]"):
            pos += 1
        if pos >= length:
            break
        obj, pos = decoder.raw_decode(text, pos)
        objects.append(obj)
    return objects


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

    path = str(data.get("file") or "").strip()
    if not path:
        return fail("empty_file", "file пуст")

    try:
        wall = float(data.get("wall_timeout") or DEFAULT_WALL)
    except (TypeError, ValueError):
        return fail("bad_wall_timeout", "wall_timeout обязан быть числом",
                    exit_code=2)

    if not os.path.exists(path):
        return fail("missing_file", f"файл не найден: {path}")
    if os.path.isdir(path):
        return fail("missing_file", f"ожидался файл, а не каталог: {path}")
    target = os.path.abspath(path)

    bin_env = os.environ.get("OLETOOLS_BIN", "olevba").strip()
    # subprocess поедет с cwd во временной папке: имя из PATH ищем
    # which'ем, путь — приводим к абсолютному
    if "/" not in bin_env and "\\" not in bin_env:
        bin_env = shutil.which(bin_env) or os.path.abspath(bin_env)
    else:
        bin_env = os.path.abspath(bin_env)

    cmd = [bin_env, "-j", "-a", target]
    if bin_env.lower().endswith(".py"):
        # v0.29: .py-мок запускаем через интерпретатор — прямой exec
        # непереносим (shebang+CRLF на Linux, ассоциации на Windows).
        cmd = [sys.executable] + cmd

    with tempfile.TemporaryDirectory() as td:
        try:
            proc = subprocess.run(cmd, capture_output=True, text=True,
                                  encoding="utf-8", errors="replace",
                                  cwd=td, timeout=wall)
        except FileNotFoundError:
            return fail("oletools_not_installed",
                        "olevba не найден: pip install oletools "
                        "(или укажите OLETOOLS_BIN)")
        except subprocess.TimeoutExpired:
            return fail("timeout",
                        f"olevba не уложился в {wall:.0f}s: увеличьте "
                        "wall_timeout", retryable=True)
        if proc.stderr:
            sys.stderr.write(proc.stderr)

        if proc.returncode != 0:
            tail = (proc.stderr or proc.stdout or "").strip().splitlines()
            return fail("tool_failed",
                        f"olevba упал (exit {proc.returncode}): "
                        f"{tail[-1] if tail else 'пустой вывод'}")

        stdout = proc.stdout or ""
        try:
            objects = read_json_objects(stdout)
        except ValueError as e:
            return fail("bad_report",
                        f"JSON-репорт olevba не разбирается: {e}", exit_code=2)

    report = None
    for obj in objects:
        if not isinstance(obj, dict):
            continue
        if obj.get("type") == "MetaInformation" or "script_name" in obj:
            continue
        report = obj
        break

    if report is None:
        return fail("no_report",
                    "olevba не дал JSON-репорт по файлу (нет репорта или он "
                    "пуст)")
    if not report.get("json_conversion_successful"):
        return fail("bad_report",
                    "olevba не смог разобрать документ "
                    "(json_conversion_successful != true)", exit_code=2)
    raw_macros = report.get("macros")
    if not isinstance(raw_macros, list):
        return fail("bad_report", "в репорте olevba нет списка macros",
                    exit_code=2)

    macros = []
    for macro in raw_macros:
        if not isinstance(macro, dict):
            continue
        macros.append({
            "vba_filename": str(macro.get("vba_filename") or ""),
            "subfilename": str(macro.get("subfilename") or ""),
            "ole_stream": str(macro.get("ole_stream") or ""),
        })

    findings = []
    analysis = report.get("analysis")
    if isinstance(analysis, list):
        for item in analysis:
            if not isinstance(item, dict):
                continue
            findings.append({
                "type": str(item.get("type") or ""),
                "keyword": str(item.get("keyword") or ""),
                "description": str(item.get("description") or ""),
            })

    return ok({"file": target, "macros": macros, "findings": findings,
               "count": len(findings)})


if __name__ == "__main__":
    sys.exit(main())