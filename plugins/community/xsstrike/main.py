#!/usr/bin/env python3
"""xsstrike — поиск XSS по явно заданной цели (обёртка над CLI XSStrike).

Вход (stdin JSON): url (единственная проверяемая цель), wall_timeout (опц., 300).

Вызов: <XSSTRIKE_BIN|xsstrike> -u <url> --skip   (cwd = временная папка).
--skip обязателен: без него XSStrike 3.x после находки задаёт вопрос
«Would you like to continue scanning? [y/N]» и на неинтерактивном запуске
виснет на вводе. Краулинг и --blind не включаются — проверяется только URL из входа.

Файлового отчёта у XSStrike нет (--log-file пишет только свой лог, а
--file-log-level по умолчанию не задан), поэтому отчёт — stdout (паттерн B
спецификации). Разбор консервативный: снимаем ANSI и держимся известных маркеров,
покрывая обе генерации XSStrike:
  «Payload: <vector>»                              — XSStrike 3.x, modes/scan.py
  «Vulnerable webpage: <url>» + «Vector for <p>: <v>» — краул/старый singleTarget
  «Potentially vulnerable objects found»           — DOM XSS (core/dom.py)
Всё нераспознанное игнорируется. Находок нет — status ok с vulnerable=false.
Символ замены U+FFFD в выводе (не декодируется как UTF-8) — bad_report, exit 2.

Выход (stdout JSON): {url, vulnerable, payloads[{kind, url, parameter, vector}],
count}. Доменные ошибки: empty_url, xsstrike_not_installed, timeout (retryable),
no_report ( XSStrike отработал и не напечатал ничего — у него всегда печатается
баннер, так что это не «чистая цель», а сбой вывода), tool_failed.
Платформенные (exit 2): битый JSON входа, bad_wall_timeout, bad_report.
"""
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile

DEFAULT_WALL = 300

ANSI_RE = re.compile(r"\x1b\[[0-9;]*[A-Za-z]")
WEBPAGE_RE = re.compile(r"Vulnerable webpage:\s*(\S+)")
VECTOR_FOR_RE = re.compile(r"Vector for\s+(\S+?)\s*:\s*(.+)$")
PAYLOAD_RE = re.compile(r"\bPayload:\s*(.+)$")
DOM_RE = re.compile(r"Potentially vulnerable objects found")

try:
    sys.stdin.reconfigure(encoding="utf-8")
    sys.stdout.reconfigure(encoding="utf-8")
except Exception:
    pass


def ok(output):
    print(json.dumps({"status": "ok", "output": output}, ensure_ascii=False))
    return 0


def fail(code, message, retryable=False, exit_code=1):
    print(json.dumps({"status": "error",
                      "error": {"code": code, "message": message,
                                "retryable": retryable}},
                     ensure_ascii=False))
    return exit_code


def resolve_bin():
    bin_env = os.environ.get("XSSTRIKE_BIN", "").strip() or "xsstrike"
    if "/" in bin_env or "\\" in bin_env:
        cmd = [os.path.abspath(bin_env)]
    else:
        cmd = [shutil.which(bin_env) or os.path.abspath(bin_env)]
    if len(cmd) == 1 and cmd[0].lower().endswith(".py"):
        cmd = [sys.executable] + cmd
    return cmd


def parse_stdout(raw, url):
    payloads = []
    pending = None

    for line in ANSI_RE.sub("", raw or "").splitlines():
        line = line.strip()
        if not line:
            continue

        match = PAYLOAD_RE.search(line)
        if match:
            payloads.append({"kind": "vector", "url": url,
                             "parameter": "", "vector": match.group(1).strip()})
            pending = None
            continue

        match = WEBPAGE_RE.search(line)
        if match:
            pending = {"kind": "reflected", "url": match.group(1).strip(),
                       "parameter": "", "vector": ""}
            payloads.append(pending)
            continue

        match = VECTOR_FOR_RE.search(line)
        if match:
            if pending is not None and not pending["vector"]:
                pending["parameter"] = match.group(1).strip()
                pending["vector"] = match.group(2).strip()
                pending = None
            else:
                payloads.append({"kind": "reflected", "url": url,
                                 "parameter": match.group(1).strip(),
                                 "vector": match.group(2).strip()})
            continue

        if DOM_RE.search(line):
            payloads.append({"kind": "dom", "url": url,
                             "parameter": "", "vector": ""})
            pending = None

    return payloads


def main():
    try:
        data = json.load(sys.stdin)
    except Exception as e:
        return fail("bad_input", f"невалидный JSON на входе: {e}", exit_code=2)
    if not isinstance(data, dict):
        return fail("bad_input", "ожидается JSON-объект на входе", exit_code=2)

    url = str(data.get("url") or "").strip()
    if not url:
        return fail("empty_url", "url пуст")

    try:
        wall = float(data.get("wall_timeout") or DEFAULT_WALL)
    except (TypeError, ValueError):
        return fail("bad_wall_timeout", "wall_timeout обязан быть числом",
                    exit_code=2)

    with tempfile.TemporaryDirectory() as td:
        cmd = resolve_bin() + ["-u", url, "--skip"]
        try:
            proc = subprocess.run(cmd, capture_output=True, text=True,
                                  encoding="utf-8", errors="replace",
                                  cwd=td, timeout=wall)
        except FileNotFoundError:
            return fail("xsstrike_not_installed",
                        "xsstrike не найден в PATH: pip install xsstrike "
                        "или укажите XSSTRIKE_BIN путём к xsstrike.py из git-клна")
        except subprocess.TimeoutExpired:
            return fail("timeout",
                        f"XSStrike не уложился в {wall:.0f}s: увеличьте "
                        "wall_timeout", retryable=True)
        if proc.stderr:
            sys.stderr.write(proc.stderr)
        if proc.returncode != 0:
            tail = (proc.stdout or proc.stderr or "").strip().splitlines()
            return fail("tool_failed",
                        f"XSStrike упал (exit {proc.returncode}): "
                        f"{tail[-1] if tail else 'пустой вывод'}")
        if "\ufffd" in (proc.stdout or "") or "\ufffd" in (proc.stderr or ""):
            return fail("bad_report",
                        "вывод XSStrike не декодируется как UTF-8 — находки "
                        "не разбираются", exit_code=2)
        if not (proc.stdout or "").strip():
            return fail("no_report",
                        "XSStrike ничего не напечатал (у него всегда печатается "
                        "баннер) — разбирать нечего")

    payloads = parse_stdout(proc.stdout, url)

    return ok({"url": url, "vulnerable": bool(payloads),
               "payloads": payloads, "count": len(payloads)})


if __name__ == "__main__":
    sys.exit(main())
