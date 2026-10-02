#!/usr/bin/env python3
"""virustotal — репутация файла/домена/IP через официальный SDK VirusTotal.

Вход (stdin JSON): indicator (MD5/SHA1/SHA256, домен или IP), wall_timeout
(опц., 120).

Донор — библиотека, поэтому паттерн C: плагин запускает тот же интерпретатор
как `python -c <SNIPPET> <indicator> <kind>` (cwd = временная папка). Сниппет —
константа модуля: сам импортирует донор, сам читает VT_API_KEY из
os.environ и печатает в stdout ровно один JSON-объект. Вшитых ключей нет и не
должно быть. VIRUSTOTAL_BIN переопределяет донора целиком — на этом пути тесты
идут через mock_virustotal.py, без ключа и без сети.

Донор — vt-py (import vt, REST API v3, pip install vt-py): vt.Client(key) и
get_object() по путям /files/<hash>, /domains/<домен>, /ip_addresses/<ip>.
Счётчики берём из last_analysis_stats (malicious и сумма harmless+malicious+
suspicious+undetected+timeout), репутацию — из reputation. Ошибки vt-py — это
vt.APIError с кодом ("NotFoundError", "AuthenticationError", "ServerError", …),
сниппет по коду даёт error_class, плагин решает по нему.

Про имя пакета: одноимённый `virustotal` на PyPI — сторонний клиент Gawen Arab
от 2012 года под API 2.0 (внутри httplib/urlparse, то есть на Python 3 не
импортируется), а не официальный SDK; официальный клиент называется vt-py.

Выход (stdout JSON): {indicator, malicious, total, reputation, found}.
Индикатора в базе нет — это ok с found:false и нулями. Доменные ошибки:
empty_indicator, bad_indicator (ни хэш, ни домен, ни IP),
virustotal_not_installed, timeout (retryable), no_report, tool_failed
(retryable по классу ошибки). Платформенные (exit 2): битый JSON входа,
нечитаемый stdout донора.
"""
import ipaddress
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

DEFAULT_WALL = 120

HASH_RE = re.compile(r"^(?:[0-9a-fA-F]{32}|[0-9a-fA-F]{40}|[0-9a-fA-F]{64})$")
DOMAIN_RE = re.compile(
    r"^(?:[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?\.)+"
    r"[A-Za-z](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?$")

SNIPPET = r'''
import json
import os
import sys


def classify(exc):
    name = type(exc).__name__
    code = str(getattr(exc, "code", "") or "")
    text = str(exc).lower()
    if code == "NotFoundError" or "404" in code or "not found" in text:
        return "not_found"
    if code in ("AuthenticationError", "UnauthorizedError") or "401" in code:
        return "auth"
    if code in ("QuotaExceededError", "TooManyRequestsError") or "429" in code:
        return "rate_limit"
    if code == "ServerError" or "500" in code or "503" in code:
        return "network"
    if "timeout" in name.lower() or "connection" in name.lower():
        return "network"
    return "other"


def as_int(value):
    try:
        return int(value)
    except (TypeError, ValueError):
        return 0


out = {"malicious": 0, "total": 0, "reputation": 0, "found": False,
       "error": "", "error_class": "", "error_type": ""}
indicator = sys.argv[1] if len(sys.argv) > 1 else ""
kind = sys.argv[2] if len(sys.argv) > 2 else ""
paths = {"file": "/files/", "domain": "/domains/", "ip": "/ip_addresses/"}
if kind not in paths:
    kind = "file" if len(indicator) in (32, 40, 64) else "domain"
key = os.environ.get("VT_API_KEY", "").strip()
if not key:
    out["error_class"] = "missing_key"
    out["error"] = "VT_API_KEY is not set"
else:
    try:
        import vt
    except ImportError as exc:
        out["error_class"] = "not_installed"
        out["error"] = str(exc)
    else:
        try:
            with vt.Client(key) as client:
                report = client.get_object(paths[kind] + indicator)
        except Exception as exc:
            out["error_class"] = classify(exc)
            out["error"] = str(exc)
            out["error_type"] = type(exc).__name__
        else:
            stats = report.get("last_analysis_stats")
            out["malicious"] = as_int(stats.get("malicious", 0)) if stats else 0
            out["total"] = sum(as_int(stats.get(name, 0)) for name in
                               ("harmless", "malicious", "suspicious",
                                "undetected", "timeout")) if stats else 0
            out["reputation"] = as_int(report.get("reputation", 0))
            out["found"] = True
print(json.dumps(out))
'''


def ok(output):
    print(json.dumps({"status": "ok", "output": output}))
    return 0


def fail(code, message, retryable=False, exit_code=1):
    print(json.dumps({"status": "error",
                      "error": {"code": code, "message": message,
                                "retryable": retryable}}))
    return exit_code


def as_int(value):
    try:
        return int(value)
    except (TypeError, ValueError):
        return 0


def main():
    try:
        data = json.load(sys.stdin)
    except Exception as e:
        return fail("bad_input", f"невалидный JSON на входе: {e}", exit_code=2)
    if not isinstance(data, dict):
        return fail("bad_input", "ожидается JSON-объект на входе", exit_code=2)

    indicator = str(data.get("indicator") or "").strip()
    if not indicator:
        return fail("empty_indicator", "indicator пуст")
    if HASH_RE.match(indicator):
        kind = "file"
        indicator = indicator.lower()
    else:
        try:
            indicator = str(ipaddress.ip_address(indicator))
            kind = "ip"
        except ValueError:
            name = indicator[:-1] if indicator.endswith(".") else indicator
            if not DOMAIN_RE.match(name):
                return fail("bad_indicator",
                            f"indicator='{indicator}' не хэш (MD5/SHA1/SHA256), "
                            "не домен и не IP — донор так не ищет")
            indicator = name.lower()
            kind = "domain"

    try:
        wall = float(data.get("wall_timeout") or DEFAULT_WALL)
    except (TypeError, ValueError):
        return fail("bad_wall_timeout", "wall_timeout обязан быть числом",
                    exit_code=2)

    bin_env = os.environ.get("VIRUSTOTAL_BIN", "").strip()
    if bin_env:
        if "/" in bin_env or "\\" in bin_env:
            cmd = [os.path.abspath(bin_env)]
        else:
            cmd = [shutil.which(bin_env) or os.path.abspath(bin_env)]
        if cmd[0].lower().endswith(".py"):
            cmd = [sys.executable] + cmd
    else:
        cmd = [sys.executable, "-c", SNIPPET]
    cmd += [indicator, kind]

    with tempfile.TemporaryDirectory() as td:
        try:
            proc = subprocess.run(cmd, capture_output=True, text=True,
                                  cwd=td, timeout=wall)
        except FileNotFoundError:
            return fail("virustotal_not_installed",
                        "донор virustotal не запустился: pip install vt-py "
                        "(или укажите VIRUSTOTAL_BIN)")
        except subprocess.TimeoutExpired:
            return fail("timeout",
                        f"запрос к VirusTotal не уложился в {wall:.0f}s: "
                        "увеличьте wall_timeout", retryable=True)
        if proc.stderr:
            sys.stderr.write(proc.stderr)

        text = (proc.stdout or "").strip()
        if not text:
            tail = (proc.stderr or "").strip().splitlines()
            last = tail[-1] if tail else f"exit {proc.returncode}"
            if proc.returncode != 0:
                return fail("tool_failed", f"донор virustotal упал: {last}")
            return fail("no_report",
                        f"донор virustotal не напечатал отчёт: {last}")
        try:
            report = json.loads(text)
        except Exception as e:
            return fail("bad_report",
                        f"не прочитан JSON донора virustotal: {e}", exit_code=2)

    if not isinstance(report, dict):
        return fail("bad_report", "неожиданный формат отчёта virustotal",
                    exit_code=2)

    error_class = str(report.get("error_class") or "")
    error = str(report.get("error") or "")
    if error_class == "not_installed":
        return fail("virustotal_not_installed",
                    f"официальный SDK vt-py не импортируется ({error}): "
                    "pip install vt-py")
    if error_class == "missing_key":
        return fail("tool_failed",
                    "не задан VT_API_KEY — экспортируйте ключ VirusTotal "
                    "в окружение рана", retryable=False)
    if error_class in ("auth", "other"):
        return fail("tool_failed", f"VirusTotal API отказал: {error}")
    if error_class in ("rate_limit", "network"):
        return fail("tool_failed", f"VirusTotal API недоступен: {error}",
                    retryable=True)

    return ok({"indicator": indicator,
               "malicious": as_int(report.get("malicious")),
               "total": as_int(report.get("total")),
               "reputation": as_int(report.get("reputation")),
               "found": bool(report.get("found"))})


if __name__ == "__main__":
    sys.exit(main())