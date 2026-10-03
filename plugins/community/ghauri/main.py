#!/usr/bin/env python3
"""ghauri — поиск SQL-инъекций на явно заданной цели (обёртка над CLI ghauri).

Вход (stdin JSON): url (http/https), timeout (опц., с/запрос, 30),
wall_timeout (опц., общий лимит рана, 600).

Вызов: <GHAURI_BIN|ghauri> -u <url> --batch --level 1 --timeout N
       (cwd = временная папка; HOME/USERPROFILE дочернего процесса тоже
       временные — ghauri пишет сессию в ~/.ghauri/<netloc>/, наружу от рана
       ничего не остаётся). Все четыре флага — реальные флаги ghauri
       (ghauri/scripts/ghauri.py: -u/--url, --batch, --level, --timeout).

Установка: пакета `ghauri` на PyPI НЕТ (https://pypi.org/pypi/ghauri/json →
404), официальная установка — из репозитория:
`pip install "git+https://github.com/r0oth3x49/ghauri.git"`.

Где что лежит (проверено на ghauri 1.4.3). Отчёта в машинном формате нет, есть
два источника текста, и они НЕ равны:

1. stderr дочернего процесса — весь ColoredLogger (StreamHandler без stream
   → sys.stderr): баннер, "[HH:MM:SS] [LEVEL] сообщение". Именно сюда уходят
   признак инъекции NOTICE "GET parameter 'id' appears to be '<TITLE>'
   injectable" (ghauri/core/tests.py, logger.notice) и итоги уровня CRITICAL
   "all tested parameters do not appear to be injectable." /
   "no parameter(s) found for testing ..." (ghauri/ghauri.py).
   ANSI-последовательности приезжают прямо в тексте сообщения (mc/nc из
   ghauri/common/colors.py), поэтому перед разбором коды вырезаются.

2. ~/.ghauri/<netloc>/log — FileHandler с форматом "%(message)s"
   (ghauri/logger/colored_logger.py), но у него handler.setLevel(SUCCESS), то
   есть в файл попадают ТОЛЬКО записи уровня SUCCESS(70). Ни NOTICE(26), ни
   CRITICAL(50) туда не доходят: на неинъекционном цели файл пустой (0 байт),
   на инъекционной — там только блок успеха от logger.success в
   ghauri/core/tests.py:
       Ghauri identified the following injection point(s) ...
       ---
       Parameter: id (GET)
           Type: boolean-based blind
           Title: AND boolean-based blind - WHERE or HAVING clause
           Payload: id=1 AND 07568=7568
       ---
   По нему тоже разбираем place/parameter/technique — это единственный
  machine-readable след в файле.

Плагин разбирает ОБА источника (файл + перехваченные stdout/stderr) и склеивает
их, дубликаты по (parameter, place, technique) отбрасывает.

Аудит неинвазивный и только по цели, которую явно задал пользователь: плагин НЕ
передаёт флаги перечисления и эксплуатации (--dbs/--tables/--columns/--dump/
--sql-shell и т. п.), поэтому ghauri остаётся детектором уровня 1 по
GET/POST-параметрам — никакой записи файлов на цель и никаких команд ОС.

Выход (stdout JSON): {url, vulnerable, parameter,
injections[{parameter, place, technique}]}. Не нашлось инъекций — это ok с
vulnerable=false, а не ошибка. Доменные ошибки: empty_url, bad_url (не
http(s)-схема либо в URL нет проверяемых параметров), ghauri_not_installed,
timeout (retryable), no_report, tool_failed. Платформенные (exit 2): битый
JSON входа, нечисловые timeout/wall_timeout.
"""
import glob
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile

try:
    sys.stdin.reconfigure(encoding="utf-8")
    sys.stdout.reconfigure(encoding="utf-8")
except Exception:
    pass

DEFAULT_TIMEOUT = 30
DEFAULT_WALL = 600

ANSI_RE = re.compile(r"\x1b\[[0-9;]*[A-Za-z]")
NO_PARAMS_RE = re.compile(r"no parameter\(s\) found for testing", re.I)
INJ_RE = re.compile(
    r"(?:(?P<place>GET|POST|URI|COOKIE|HEADER)\s+)?"
    r"(?:\(custom\)\s+)?parameter\s+'(?P<param>[^']+)'\s+"
    r"appears to be\s+'(?P<technique>[^']+)'\s+injectable")
# признак того, что ghauri вообще отработал (лог-файл создан или в потоке есть
# его фирменные строки старта/конца/разбора)
RUN_HINT_RE = re.compile(r"\[\*\]\s+(?:starting|ending) @|"
                         r"fetched data logged to text files|"
                         r"testing for SQL injection|"
                         r"appears to be '.+'\s+injectable|"
                         r"all tested parameters do not appear", re.I)
# блок успеха в ~/.ghauri/<netloc>/log (единственное, что туда пишется)
LOG_PARAM_RE = re.compile(r"^Parameter:\s*(?P<param>\S+)\s*\((?P<place>[^)]*)\)$")
LOG_TITLE_RE = re.compile(r"^Title:\s*(?P<technique>.+)$")
LOG_TYPE_RE = re.compile(r"^Type:\s*(?P<payload_type>.+)$")


def ok(output):
    print(json.dumps({"status": "ok", "output": output}))
    return 0


def fail(code, message, retryable=False, exit_code=1):
    print(json.dumps({"status": "error",
                      "error": {"code": code, "message": message,
                                "retryable": retryable}}))
    return exit_code


def resolve(bin_env):
    if "/" in bin_env or "\\" in bin_env:
        return os.path.abspath(bin_env)
    return shutil.which(bin_env) or os.path.abspath(bin_env)


def injections_from_stream(text):
    """Маркеры уровня NOTICE из вывода ghauri (stderr/stdout)."""
    found = []
    for line in text.splitlines():
        for m in INJ_RE.finditer(ANSI_RE.sub("", line)):
            found.append({"parameter": m.group("param").strip(),
                          "place": (m.group("place") or "").strip(),
                          "technique": m.group("technique").strip()})
    return found


def injections_from_log(text):
    """Блок 'Parameter:/Type:/Title:' из ~/.ghauri/<netloc>/log."""
    found = []
    cur = None
    for raw in text.splitlines():
        line = raw.strip()
        m = LOG_PARAM_RE.match(line)
        if m:
            if cur is not None:
                found.append(cur)
            cur = {"parameter": m.group("param").strip(),
                   "place": m.group("place").strip(),
                   "technique": "", "payload_type": ""}
            continue
        if cur is None:
            continue
        m = LOG_TYPE_RE.match(line)
        if m:
            cur["payload_type"] = m.group("payload_type").strip()
            continue
        m = LOG_TITLE_RE.match(line)
        if m:
            cur["technique"] = m.group("technique").strip()
    if cur is not None:
        found.append(cur)
    return found


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
    if not (url.startswith("http://") or url.startswith("https://")):
        return fail("bad_url", "url должен начинаться с http:// или https://")

    try:
        timeout = float(data.get("timeout") or DEFAULT_TIMEOUT)
    except (TypeError, ValueError):
        return fail("bad_timeout", "timeout обязан быть числом", exit_code=2)
    try:
        wall = float(data.get("wall_timeout") or DEFAULT_WALL)
    except (TypeError, ValueError):
        return fail("bad_wall_timeout", "wall_timeout обязан быть числом",
                    exit_code=2)

    cmd = [resolve(os.environ.get("GHAURI_BIN", "ghauri").strip() or "ghauri"),
           "-u", url, "--batch", "--level", "1",
           "--timeout", str(int(timeout))]
    if cmd[0].lower().endswith(".py"):
        cmd = [sys.executable] + cmd

    with tempfile.TemporaryDirectory() as td:
        env = dict(os.environ)
        env["HOME"] = td
        env["USERPROFILE"] = td
        try:
            proc = subprocess.run(cmd, capture_output=True, text=True,
                                  cwd=td, env=env, timeout=wall)
        except FileNotFoundError:
            return fail("ghauri_not_installed",
                        "ghauri не найден в PATH: пакет ghauri на PyPI "
                        "не существует, ставьте из репозитория "
                        "pip install "
                        "\"git+https://github.com/r0oth3x49/ghauri.git\" "
                        "(или укажите GHAURI_BIN)")
        except subprocess.TimeoutExpired:
            return fail("timeout",
                        f"ghauri не уложился в {wall:.0f}s: увеличьте "
                        "wall_timeout", retryable=True)
        if proc.stderr:
            sys.stderr.write(proc.stderr)
        if proc.returncode != 0:
            tail = (proc.stderr or proc.stdout or "").strip().splitlines()
            last = tail[-1] if tail else f"exit {proc.returncode}"
            return fail("tool_failed", f"ghauri упал: {last}")

        logs = sorted(glob.glob(os.path.join(td, ".ghauri", "*", "log")))
        log_text = ""
        if logs:
            try:
                with open(logs[-1], encoding="utf-8", errors="replace") as f:
                    log_text = f.read()
            except Exception as e:
                return fail("bad_report", f"не прочитан лог ghauri: {e}",
                            exit_code=2)
        # весь ColoredLogger ghauri идёт в stderr, поэтому разбираем поток
        # процесса вместе с файловым отчётом (см. докстринг)
        stream_text = (proc.stdout or "") + "\n" + (proc.stderr or "")
        clean_stream = ANSI_RE.sub("", stream_text)
        if not logs and not RUN_HINT_RE.search(clean_stream):
            tail = clean_stream.strip().splitlines()
            last = tail[-1] if tail else "ни лог, ни вывод"
            return fail("no_report", f"ghauri не дал отчёт: {last}")
        if not (log_text + stream_text).strip():
            return fail("no_report", "ghauri не дал отчёт: лог и вывод пусты")

    text = log_text + "\n" + stream_text
    clean = ANSI_RE.sub("", text)

    if NO_PARAMS_RE.search(clean):
        return fail("bad_url",
                    "в URL нет проверяемых параметров — добавьте query "
                    "(например ?id=1) или POST-данные")

    injections = []
    seen = set()

    def add(item):
        key = (item["parameter"], item["place"], item["technique"])
        if key in seen:
            return
        seen.add(key)
        injections.append(item)

    for item in injections_from_stream(stream_text):
        add(item)
    for item in injections_from_log(log_text):
        add({"parameter": item["parameter"], "place": item["place"],
             "technique": item["technique"] or item["payload_type"]})

    return ok({"url": url, "vulnerable": bool(injections),
               "parameter": injections[0]["parameter"] if injections else "",
               "injections": injections})


if __name__ == "__main__":
    sys.exit(main())