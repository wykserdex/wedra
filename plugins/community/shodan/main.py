#!/usr/bin/env python3
"""shodan — открытые порты/хостнеймы/CVE по IP (официальный SDK shodan-python).

Вход (stdin JSON): ip (адрес хоста, IPv4 или IPv6), wall_timeout (опц., 120).

Донор — библиотека без пригодного CLI, поэтому паттерн C: плагин запускает тот
же интерпретатор как `python -c <SNIPPET> <ip>` (cwd = временная папка). Сниппет
— константа модуля: он сам импортирует shodan, сам читает SHODAN_API_KEY из
os.environ и печатает в stdout ровно один JSON-объект. Вшитых ключей нет и не
должно быть. SHODAN_BIN переопределяет донора целиком — на этом пути
контрактные тесты идут через mock_shodan.py, без ключа и без сети.

shodan-python 1.31: Shodan(api_key).host(ip) → dict хоста (ip_str, ports[],
hostnames[], org/isp, vulns[] и баннеры в data[] с их vulns). 404 «No
information available for that IP» и прочие ошибки SDK приходят как APIError с
текстом — сниппет классифицирует его в error_class, плагин по классу решает,
ошибка это или «хоста нет» (тогда ok с found:false).

Выход (stdout JSON): {ip, ports[], hostnames[], vulns[], org, found}.
Доменные ошибки: empty_ip, bad_ip (не IP-адрес), shodan_not_installed, timeout
(retryable), no_report, tool_failed (нет ключа/отказ API — retryable по
классу ошибки). Платформенные (exit 2): битый JSON входа, нечитаемый stdout
донара.
"""
import ipaddress
import json
import os
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

SNIPPET = r'''
import json
import os
import sys


def classify(message):
    text = (message or "").lower()
    if "no information available" in text or "404" in text:
        return "not_found"
    if ("invalid api key" in text or "no account found" in text
            or "unauthorized" in text):
        return "auth"
    if "rate limit" in text or "no more queries" in text or "429" in text:
        return "rate_limit"
    if ("unable to connect" in text or "timed out" in text
            or "timeout" in text or "bad gateway" in text):
        return "network"
    return "other"


out = {"ports": [], "hostnames": [], "vulns": [], "org": "", "found": False,
       "error": "", "error_class": "", "error_type": ""}
ip = sys.argv[1] if len(sys.argv) > 1 else ""
try:
    import shodan
except ImportError as exc:
    out["error_class"] = "not_installed"
    out["error"] = str(exc)
else:
    key = os.environ.get("SHODAN_API_KEY", "").strip()
    if not key:
        out["error_class"] = "missing_key"
        out["error"] = "SHODAN_API_KEY is not set"
    else:
        try:
            host = shodan.Shodan(key).host(ip) or {}
        except Exception as exc:
            out["error_class"] = classify(str(exc))
            out["error"] = str(exc)
            out["error_type"] = type(exc).__name__
        else:
            out["found"] = bool(host)
            out["org"] = str(host.get("org") or host.get("isp") or "")
            for port in host.get("ports") or []:
                try:
                    out["ports"].append(int(port))
                except (TypeError, ValueError):
                    out["ports"].append(str(port))
            for name in host.get("hostnames") or []:
                if str(name) not in out["hostnames"]:
                    out["hostnames"].append(str(name))
            raw = host.get("vulns")
            vulns = [str(v) for v in raw] if isinstance(raw, list) else []
            for banner in host.get("data") or []:
                if isinstance(banner, dict):
                    vulns += [str(v) for v in banner.get("vulns") or []]
            out["vulns"] = sorted(set(vulns))
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


def as_list(value):
    return [v for v in value if isinstance(v, (str, int, float))] \
        if isinstance(value, list) else []


def main():
    try:
        data = json.load(sys.stdin)
    except Exception as e:
        return fail("bad_input", f"невалидный JSON на входе: {e}", exit_code=2)
    if not isinstance(data, dict):
        return fail("bad_input", "ожидается JSON-объект на входе", exit_code=2)

    ip = str(data.get("ip") or "").strip()
    if not ip:
        return fail("empty_ip", "ip пуст")
    try:
        parsed = ipaddress.ip_address(ip)
    except ValueError:
        return fail("bad_ip",
                    f"ip='{ip}' не адрес хоста: нужен IPv4 или IPv6 "
                    "(имя хоста резолвьте отдельным инструментом)")
    ip = str(parsed)

    try:
        wall = float(data.get("wall_timeout") or DEFAULT_WALL)
    except (TypeError, ValueError):
        return fail("bad_wall_timeout", "wall_timeout обязан быть числом",
                    exit_code=2)

    cmd = None
    bin_env = os.environ.get("SHODAN_BIN", "").strip()
    if bin_env:
        if "/" in bin_env or "\\" in bin_env:
            cmd = [os.path.abspath(bin_env)]
        else:
            cmd = [shutil.which(bin_env) or os.path.abspath(bin_env)]
        if cmd[0].lower().endswith(".py"):
            cmd = [sys.executable] + cmd
    else:
        cmd = [sys.executable, "-c", SNIPPET]
    cmd += [ip]

    with tempfile.TemporaryDirectory() as td:
        try:
            proc = subprocess.run(cmd, capture_output=True, text=True,
                                  cwd=td, timeout=wall)
        except FileNotFoundError:
            return fail("shodan_not_installed",
                        "донор shodan не запустился: pip install shodan "
                        "(или укажите SHODAN_BIN)")
        except subprocess.TimeoutExpired:
            return fail("timeout",
                        f"запрос к Shodan не уложился в {wall:.0f}s: "
                        "увеличьте wall_timeout", retryable=True)
        if proc.stderr:
            sys.stderr.write(proc.stderr)

        text = (proc.stdout or "").strip()
        if not text:
            tail = (proc.stderr or "").strip().splitlines()
            last = tail[-1] if tail else f"exit {proc.returncode}"
            if proc.returncode != 0:
                return fail("tool_failed", f"донор shodan упал: {last}")
            return fail("no_report", f"донор shodan не напечатал отчёт: {last}")
        try:
            report = json.loads(text)
        except Exception as e:
            return fail("bad_report", f"не прочитан JSON донора shodan: {e}",
                        exit_code=2)

    if not isinstance(report, dict):
        return fail("bad_report", "неожиданный формат отчёта shodan",
                    exit_code=2)

    error_class = str(report.get("error_class") or "")
    error = str(report.get("error") or "")
    if error_class == "not_installed":
        return fail("shodan_not_installed",
                    f"пакет shodan не импортируется ({error}): "
                    "pip install shodan")
    if error_class == "missing_key":
        return fail("tool_failed",
                    "не задан SHODAN_API_KEY — экспортируйте ключ Shodan "
                    "в окружение рана", retryable=False)
    if error_class in ("auth", "other"):
        return fail("tool_failed", f"Shodan API отказал: {error}")
    if error_class in ("rate_limit", "network"):
        return fail("tool_failed", f"Shodan API недоступен: {error}",
                    retryable=True)

    return ok({"ip": ip,
               "ports": as_list(report.get("ports")),
               "hostnames": [str(h) for h in as_list(report.get("hostnames"))],
               "vulns": [str(v) for v in as_list(report.get("vulns"))],
               "org": str(report.get("org") or ""),
               "found": bool(report.get("found"))})


if __name__ == "__main__":
    sys.exit(main())