#!/usr/bin/env python3
"""ipwhois — WHOIS/RDAP по IP-адресу (обёртка над CLI ipwhois).

Вход (stdin JSON): ip, protocol (опц., "rdap"|"whois", по умолчанию rdap),
timeout (опц., с/сокет, 5), wall_timeout (опц., общий лимит, 300).

Вызов: <IPWHOIS_BIN|ipwhois_cli> --addr <ip> --json [--whois] --timeout N
       (cwd = временная папка).

ipwhois 1.3.x (pip install ipwhois) объявляет console_script ipwhois_cli
(точка входа ipwhois.scripts.ipwhois_cli:main). Модульного запуска
`python -m ipwhois` у пакета нет, артефакта-отчёта на диске тоже нет: с
--json инструмент печатает в stdout ровно один json.dumps(...) результата
lookup — для RDAP это {query, network, objects[, nir]}, для legacy whois —
{query, nets, referral[, nir]}. Поэтому разбираем stdout.

Выход (stdout JSON): {ip, record{...}, found}. found — в ответе есть сеть
(network для RDAP, nets для legacy), то есть адрес не «defined».
Доменные ошибки: empty_ip, bad_ip, ipwhois_not_installed, timeout (retryable),
no_report, tool_failed. Платформенные (exit 2): битый JSON входа, нечитаемый
ответ инструмента.
"""
import ipaddress
import json
import os
import shutil
import subprocess
import sys
import tempfile

DEFAULT_TIMEOUT = 5
DEFAULT_WALL = 300
DEFAULT_BIN = "ipwhois_cli"
PROTOCOLS = ("rdap", "whois")


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
    bin_env = os.environ.get("IPWHOIS_BIN", DEFAULT_BIN).strip() or DEFAULT_BIN
    # subprocess поедет с cwd во временной папке: имя ищем which'ом,
    # путь — приводим к абсолютному (моки в тестах лежат рядом с main.py).
    if "/" in bin_env or "\\" in bin_env:
        return [os.path.abspath(bin_env)]
    return [shutil.which(bin_env) or os.path.abspath(bin_env)]


def build_cmd(ip, protocol, timeout):
    cmd = resolve_bin()
    if cmd[0].lower().endswith(".py"):
        # v0.29: .py-мок запускаем через интерпретатор — прямой exec
        # непереносим (shebang+CRLF на Linux, ассоциации на Windows).
        cmd = [sys.executable] + cmd
    cmd += ["--addr", ip, "--json"]
    if protocol == "whois":
        cmd.append("--whois")
    cmd += ["--timeout", str(int(timeout))]
    return cmd


def has_network(record):
    if not isinstance(record, dict):
        return False
    for key in ("network", "nets"):
        if record.get(key):
            return True
    return False


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
        ipaddress.ip_address(ip)
    except ValueError:
        return fail("bad_ip", f"ip {ip!r} не IPv4/IPv6 адрес")

    protocol = str(data.get("protocol") or "rdap").strip().lower()
    if protocol not in PROTOCOLS:
        return fail("bad_protocol",
                    f"protocol {protocol!r}: ожидается rdap или whois")

    try:
        timeout = float(data.get("timeout") or DEFAULT_TIMEOUT)
    except (TypeError, ValueError):
        return fail("bad_timeout", "timeout обязан быть числом", exit_code=2)
    try:
        wall = float(data.get("wall_timeout") or DEFAULT_WALL)
    except (TypeError, ValueError):
        return fail("bad_wall_timeout", "wall_timeout обязан быть числом",
                    exit_code=2)

    cmd = build_cmd(ip, protocol, timeout)

    with tempfile.TemporaryDirectory() as td:
        try:
            proc = subprocess.run(cmd, capture_output=True, text=True,
                                  cwd=td, timeout=wall)
        except FileNotFoundError:
            return fail("ipwhois_not_installed",
                        "ipwhois_cli не найден: pip install ipwhois "
                        "(или укажите IPWHOIS_BIN)")
        except subprocess.TimeoutExpired:
            return fail("timeout",
                        f"ipwhois не уложился в {wall:.0f}s: увеличьте "
                        "wall_timeout", retryable=True)
        if proc.stderr:
            sys.stderr.write(proc.stderr)

        stdout = (proc.stdout or "").strip()
        stderr_tail = (proc.stderr or "").strip().splitlines()
        last = stderr_tail[-1] if stderr_tail else f"exit {proc.returncode}"

        if proc.returncode != 0:
            return fail("tool_failed", f"ipwhois_cli упал: {last}")
        if not stdout:
            return fail("no_report", "ipwhois_cli ничего не напечатал")

        try:
            record = json.loads(stdout)
        except Exception as e:
            return fail("bad_report",
                        f"не разобран JSON ipwhois: {e}", exit_code=2)

    if not isinstance(record, dict):
        return fail("bad_report", "ipwhois_cli вернул не объект",
                    exit_code=2)

    return ok({"ip": ip, "record": record, "found": has_network(record)})


if __name__ == "__main__":
    sys.exit(main())
