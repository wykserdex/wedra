#!/usr/bin/env python3
"""censys — сервисы и гео хоста по IP/домену (официальный SDK censys-python).

Вход (stdin JSON): target (IPv4/IPv6 или домен), wall_timeout (опц., 120).
Домен нормализуется: конечная точка отбрасывается, регистрlower (DNS не
чувствителен к регистру), IP приводится к каноническому виду ipaddress.

Донор — библиотека без пригодного для этой задачи CLI, поэтому паттерн C:
плагин запускает тот же интерпретатор как `python -c <SNIPPET> <target>`
(cwd = временная папка). Сниппет — константа модуля: сам импортирует censys,
сам берёт CENSYS_API_ID/CENSYS_API_SECRET из os.environ (censys.common.config
умеет и env, и ~/.censys/censys-config.yaml) и печатает в stdout ровно один
JSON-объект. Вшитых ключей нет и не должно быть. CENSYS_BIN переопределяет
донора целиком — на этом пути тесты идут через mock_censys.py, без ключа и без
сети.

censys-python 2.3: CensysHosts() без аргументов (креды из env), .view(ip) →
хост {ip, services[], location{}, autonomous_system{}, ...}; для домена —
.search("dns.names: <домен>", per_page=1), и search() в 2.x отдаёт объект
запроса, поэтому сниппет зовёт его через callable() — так одинаково
отрабатывают и объект-запрос, и обычный список. Ошибки приходят
исключениями censys.common.exceptions (CensysHostNotFoundException,
CensysRateLimitExceededException, CensysMissingApiKeyException,
CensysInvalidAPIKeyException, CensysSearchAPITimeoutException, ...) — сниппет
по имени класса даёт error_class, плагин решает по нему.

Выход (stdout JSON): {target, services[{port, service_name,
transport_protocol, observed_at}], location{country, country_code, continent,
postal_code, timezone, coordinates{latitude, longitude}, registered_country,
registered_country_code}, found}. location чистится рекурсивно (None → ""),
чтобы в JSON не попало ничего лишнего. Хоста в индексе нет — это ok с
found:false. Доменные ошибки: empty_target, bad_target (ни IP, ни домен),
censys_not_installed, timeout (retryable), no_report, tool_failed (retryable по
классу ошибки). Платформенные (exit 2): битый JSON входа, нечитаемый stdout.
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

DOMAIN_RE = re.compile(
    r"^(?:[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?\.)+"
    r"[A-Za-z](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?$")

SNIPPET = r'''
import json
import os
import sys


def classify(exc):
    name = type(exc).__name__
    text = str(exc).lower()
    if name in ("CensysHostNotFoundException", "CensysNotFoundException",
                "CensysCertificateNotFoundException"):
        return "not_found"
    if name in ("CensysRateLimitExceededException",
                "CensysTooManyRequestsException"):
        return "rate_limit"
    if name == "CensysMissingApiKeyException":
        return "missing_key"
    if name in ("CensysInvalidAPIKeyException", "CensysUnauthorizedException"):
        return "auth"
    if name in ("CensysSearchAPITimeoutException",
                "CensysInternalServerException",
                "CensysAppDownForMaintenanceException"):
        return "network"
    if "not found" in text or "404" in text:
        return "not_found"
    if "rate limit" in text or "429" in text:
        return "rate_limit"
    return "other"


out = {"services": [], "location": {}, "found": False, "error": "",
       "error_class": "", "error_type": ""}
target = sys.argv[1] if len(sys.argv) > 1 else ""
try:
    import ipaddress
    from censys.search import CensysHosts
except ImportError as exc:
    out["error_class"] = "not_installed"
    out["error"] = str(exc)
else:
    try:
        censys = CensysHosts()
        try:
            ip = str(ipaddress.ip_address(target))
        except ValueError:
            ip = ""
        if ip:
            host = censys.view(ip) or {}
        else:
            hits = censys.search("dns.names: " + target, per_page=1)
            if callable(hits):
                hits = hits()
            host = (hits[0] if hits else {}) or {}
    except Exception as exc:
        out["error_class"] = classify(exc)
        out["error"] = str(exc)
        out["error_type"] = type(exc).__name__
    else:
        out["found"] = bool(host)
        for service in host.get("services") or []:
            if not isinstance(service, dict):
                continue
            try:
                port = int(service.get("port"))
            except (TypeError, ValueError):
                port = str(service.get("port") or "")
            out["services"].append({
                "port": port,
                "service_name": str(service.get("service_name") or ""),
                "transport_protocol":
                    str(service.get("transport_protocol") or ""),
                "observed_at": str(service.get("observed_at") or ""),
            })
        location = host.get("location")
        out["location"] = location if isinstance(location, dict) else {}
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


def clean(value):
    if isinstance(value, dict):
        return {str(k): clean(v) for k, v in value.items()}
    if isinstance(value, list):
        return [clean(v) for v in value]
    if isinstance(value, bool) or value is None:
        return "" if value is None else value
    if isinstance(value, (int, float, str)):
        return value
    return str(value)


def as_services(value):
    if not isinstance(value, list):
        return []
    out = []
    for item in value:
        if not isinstance(item, dict):
            continue
        out.append({
            "port": item.get("port") if isinstance(item.get("port"), int)
            else str(item.get("port") or ""),
            "service_name": str(item.get("service_name") or ""),
            "transport_protocol": str(item.get("transport_protocol") or ""),
            "observed_at": str(item.get("observed_at") or ""),
        })
    return out


def main():
    try:
        data = json.load(sys.stdin)
    except Exception as e:
        return fail("bad_input", f"невалидный JSON на входе: {e}", exit_code=2)
    if not isinstance(data, dict):
        return fail("bad_input", "ожидается JSON-объект на входе", exit_code=2)

    target = str(data.get("target") or "").strip()
    if not target:
        return fail("empty_target", "target пуст")
    try:
        target = str(ipaddress.ip_address(target))
    except ValueError:
        name = target[:-1] if target.endswith(".") else target
        if not DOMAIN_RE.match(name):
            return fail("bad_target",
                        f"target='{target}' не IP-адрес и не домен")
        target = name.lower()

    try:
        wall = float(data.get("wall_timeout") or DEFAULT_WALL)
    except (TypeError, ValueError):
        return fail("bad_wall_timeout", "wall_timeout обязан быть числом",
                    exit_code=2)

    bin_env = os.environ.get("CENSYS_BIN", "").strip()
    if bin_env:
        if "/" in bin_env or "\\" in bin_env:
            cmd = [os.path.abspath(bin_env)]
        else:
            cmd = [shutil.which(bin_env) or os.path.abspath(bin_env)]
        if cmd[0].lower().endswith(".py"):
            cmd = [sys.executable] + cmd
    else:
        cmd = [sys.executable, "-c", SNIPPET]
    cmd += [target]

    with tempfile.TemporaryDirectory() as td:
        try:
            proc = subprocess.run(cmd, capture_output=True, text=True,
                                  cwd=td, timeout=wall)
        except FileNotFoundError:
            return fail("censys_not_installed",
                        "донор censys не запустился: pip install censys "
                        "(или укажите CENSYS_BIN)")
        except subprocess.TimeoutExpired:
            return fail("timeout",
                        f"запрос к Censys не уложился в {wall:.0f}s: "
                        "увеличьте wall_timeout", retryable=True)
        if proc.stderr:
            sys.stderr.write(proc.stderr)

        text = (proc.stdout or "").strip()
        if not text:
            tail = (proc.stderr or "").strip().splitlines()
            last = tail[-1] if tail else f"exit {proc.returncode}"
            if proc.returncode != 0:
                return fail("tool_failed", f"донор censys упал: {last}")
            return fail("no_report", f"донор censys не напечатал отчёт: {last}")
        try:
            report = json.loads(text)
        except Exception as e:
            return fail("bad_report", f"не прочитан JSON донора censys: {e}",
                        exit_code=2)

    if not isinstance(report, dict):
        return fail("bad_report", "неожиданный формат отчёта censys",
                    exit_code=2)

    error_class = str(report.get("error_class") or "")
    error = str(report.get("error") or "")
    if error_class == "not_installed":
        return fail("censys_not_installed",
                    f"пакет censys не импортируется ({error}): "
                    "pip install censys")
    if error_class == "missing_key":
        return fail("tool_failed",
                    "не заданы CENSYS_API_ID/CENSYS_API_SECRET — "
                    "экспортируйте их в окружение рана", retryable=False)
    if error_class in ("auth", "other"):
        return fail("tool_failed", f"Censys API отказал: {error}")
    if error_class in ("rate_limit", "network"):
        return fail("tool_failed", f"Censys API недоступен: {error}",
                    retryable=True)

    return ok({"target": target,
               "services": as_services(report.get("services")),
               "location": clean(report.get("location")),
               "found": bool(report.get("found"))})


if __name__ == "__main__":
    sys.exit(main())