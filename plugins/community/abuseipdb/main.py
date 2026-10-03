#!/usr/bin/env python3
"""abuseipdb — репутация IP по базе жалоб AbuseIPDB (обёртка над SDK, паттерн C).

Вход (stdin JSON): ip, days (опц., окно жалоб, 1..365, default 30),
wall_timeout (опц., общий лимит рана, 300).

Сеть делает только ДОЧЕРНИЙ процесс: тот же интерпретатор с -c SNIPPET (или
ABUSEIPDB_BIN, если он задан — единственный путь для тестов). Реальный ключ в
код не вшит: сниппет читает os.environ['ABUSEIPDB_API_KEY'].

ДОНОР. `pip install abuseipdb` на py3 НЕ работает: единственный релиз на PyPI —
1.3.0 (2018-04-24), и он не ставится вообще. Его setup.py импортирует сам пакет,
а __init__.py тянет _app.py, где `import unirest` (unirest 1.1.7 — py2-only) и
`from parameters import Parameters` (неявный относительный импорт, в py3
запрещён) — сборка падает ещё до установки. Рабочий донор — git-master того же
репозитория (vsecades/AbuseIpDb, __version__ = 3.0.0), ставится так:
    pip install requests
    pip install --no-build-isolation git+https://github.com/vsecades/AbuseIpDb.git
(--no-build-isolation обязателен: setup.py мастера тоже импортирует пакет, а
pyproject.toml с build-requires у него нет.)

SNIPPET работает с обоими API по-разному и выбирает ветку по hasattr:
  * 3.0.0 (master) — класс AbuseIpDb, метод check(ip_address, max_age_in_days);
    внутри GET https://api.abuseipdb.com/api/v2/check?ipAddress=&maxAgeInDays=,
    заголовок Key, вернёт ГОТОВЫЙ dict из response.json()['data'] — печати в
    stdout нет, stdout остаётся чистым JSON;
  * 1.3.0 (PyPI, недостижим на py3) — модульные configure_api_key/check_ip.
    Ветка оставлена терпимо: вернули бы unirest raw_body (байты/строка), а
    check_ip вдобавок печатает URL с ключом в stdout. Так что на 1.3.0 этот
    сниппет всё равно непригоден — ставьте master.
Ответ APIv2 /check: {data: {ipAddress, isPublic, isWhitelisted,
abuseConfidenceScore, countryCode, countryName, totalReports, ...}}; читаем
abuseConfidenceScore, totalReports, countryCode — имена сверены с
docs.abuseipdb.com. payload разбирается терпимо и как dict, и как обёртка
{"data": ...}, и как JSON-строка.

Выход (stdout JSON): {ip, abuse_score, total_reports, country, found}.
found=true — по IP есть жалобы (totalReports > 0); «чистый» IP — это ok с
found:false и нулём, а не ошибка. Доменные ошибки: empty_ip, bad_ip, bad_days,
abuseipdb_not_installed, timeout (retryable), no_report, tool_failed.
Платформенные (exit 2): битый JSON входа, нечитаемый/не-JSON ответ донора.
"""
import ipaddress
import json
import os
import shutil
import subprocess
import sys
import tempfile

DEFAULT_DAYS = 30
MAX_DAYS = 365          # docs.abuseipdb.com: maxAgeInDays min 1, max 365
DEFAULT_WALL = 300

SNIPPET = (
    "import json, os, sys\n"
    "ip = sys.argv[1]\n"
    "days = sys.argv[2] if len(sys.argv) > 2 else '30'\n"
    "import abuseipdb\n"
    "key = os.environ['ABUSEIPDB_API_KEY']\n"
    "if hasattr(abuseipdb, 'AbuseIpDb'):\n"
    "    payload = abuseipdb.AbuseIpDb(key).check(ip, days)\n"
    "else:\n"
    "    abuseipdb.configure_api_key(key)\n"
    "    payload = abuseipdb.check_ip(ip=ip, days=days)\n"
    "if isinstance(payload, (bytes, bytearray)):\n"
    "    payload = payload.decode('utf-8', 'replace')\n"
    "print(json.dumps({'payload': payload}, default=str))\n"
)


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


def to_number(value):
    if isinstance(value, bool) or value is None:
        return 0
    try:
        num = float(value)
    except (TypeError, ValueError):
        try:
            num = float(str(value).strip())
        except (TypeError, ValueError):
            return 0
    return int(num) if num.is_integer() else num


def pick(payload, *names):
    for name in names:
        val = payload.get(name)
        if val is not None and val != "":
            return val
    return None


def build_cmd(ip, days):
    bin_env = os.environ.get("ABUSEIPDB_BIN", "").strip()
    if bin_env:
        # имя из PATH ищем which'ем, путь приводим к абсолютному
        if "/" in bin_env or "\\" in bin_env:
            cmd = [os.path.abspath(bin_env)]
        else:
            cmd = [shutil.which(bin_env) or os.path.abspath(bin_env)]
        if len(cmd) == 1 and cmd[0].lower().endswith(".py"):
            # .py-мок запускаем через интерпретатор — прямой exec непереносим
            cmd = [sys.executable] + cmd
    else:
        cmd = [sys.executable, "-c", SNIPPET]
    return cmd + [ip, str(days)]


def unwrap(payload):
    if isinstance(payload, str):
        try:
            payload = json.loads(payload)
        except Exception as e:
            raise ValueError(f"ответ SDK — не JSON: {e}")
    if isinstance(payload, list):
        payload = payload[0] if payload else {}
    if isinstance(payload, dict) and isinstance(payload.get("data"), (dict, list)):
        payload = payload["data"]
    if not isinstance(payload, dict):
        raise ValueError("ответ SDK — не объект")
    return payload


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
        addr = ipaddress.ip_address(ip)
    except ValueError:
        return fail("bad_ip", f"не IP-адрес: {ip!r}")

    days_raw = data.get("days")
    if days_raw is None or days_raw == "":
        days = DEFAULT_DAYS
    else:
        try:
            days = int(float(days_raw))
        except (TypeError, ValueError):
            return fail("bad_days", "days обязан быть числом")
        if days < 1 or days > MAX_DAYS:
            return fail("bad_days", f"days вне 1..{MAX_DAYS}: {days}")

    try:
        wall = float(data.get("wall_timeout") or DEFAULT_WALL)
    except (TypeError, ValueError):
        return fail("bad_wall_timeout", "wall_timeout обязан быть числом",
                    exit_code=2)

    cmd = build_cmd(str(addr), days)
    with tempfile.TemporaryDirectory() as td:
        try:
            proc = subprocess.run(cmd, capture_output=True, text=True,
                                  cwd=td, timeout=wall)
        except FileNotFoundError:
            return fail("abuseipdb_not_installed",
                        "донор не найден: поставьте его (см. README) "
                        "(или укажите ABUSEIPDB_BIN)")
        except subprocess.TimeoutExpired:
            return fail("timeout",
                        f"abuseipdb не уложился в {wall:.0f}s: увеличьте "
                        "wall_timeout", retryable=True)
        if proc.stderr:
            sys.stderr.write(proc.stderr)
        if proc.returncode != 0:
            tail = (proc.stderr or "").strip().splitlines()
            last = tail[-1] if tail else f"exit {proc.returncode}"
            if "No module named" in last or "ModuleNotFoundError" in last:
                return fail("abuseipdb_not_installed",
                            "пакет abuseipdb не импортируется на py3 — "
                            "PyPI-релиз 1.3.0 нерабочий, ставьте master "
                            "(см. README) либо укажите ABUSEIPDB_BIN")
            if "ABUSEIPDB_API_KEY" in last and not os.environ.get(
                    "ABUSEIPDB_API_KEY", "").strip():
                return fail("tool_failed",
                            f"не задан ABUSEIPDB_API_KEY (ключ читает сниппет "
                            f"донора): {last}")
            return fail("tool_failed", f"abuseipdb упал: {last}")

        raw = (proc.stdout or "").strip()

    if not raw:
        return fail("no_report", "abuseipdb не вернул JSON в stdout")
    try:
        envelope = json.loads(raw)
        if not isinstance(envelope, dict):
            raise ValueError("ожидался объект")
        payload = unwrap(envelope.get("payload", envelope))
    except Exception as e:
        return fail("bad_report", f"не разобран ответ abuseipdb: {e}",
                    exit_code=2)
    if not payload:
        return fail("no_report", "abuseipdb вернул пустой отчёт")

    reports = to_number(pick(payload, "totalReports", "total_reports",
                             "numReports"))
    score = to_number(pick(payload, "abuseConfidenceScore",
                           "abuse_confidence_score", "abuseScore"))
    country = pick(payload, "countryCode", "country_code", "country",
                   "countryName", "country_name") or ""

    return ok({"ip": str(addr), "abuse_score": score, "total_reports": reports,
               "country": str(country), "found": reports > 0})


if __name__ == "__main__":
    sys.exit(main())