#!/usr/bin/env python3
"""dnstwist — перестановки домена и NS-записи (обёртка над CLI dnstwist).

Вход (stdin JSON): domain, threads (опц.), registered_only (опц., bool),
wall_timeout (опц., общий лимит, 300).

Вызов: <DNSTWIST_BIN|python3 -m dnstwist> <domain> -f json [-t N]
       [--registered]  (cwd = временная папка).

dnstwist печатает JSON-массив в stdout (`--format json`; флаг -f, -o FILE мы
не используем) — по элементу на перестановку: {domain, fuzzer, dns_a,
dns_ns, ...}. Прогресс при --format json в stdout не попадает (p_cli печатает
только для cli и только в tty), так что stdout — чистый JSON.

У самого dnstwist нет отдельного режима «истории NS» (нет флага history),
поэтому ns_history здесь — NS-записи, собранные по всем перестановкам:
[{domain, nameservers[]}]. permutations — [{domain, fuzzer, resolved}],
где resolved означает наличие A-записи. total — сколько перестановок вернул
инструмент. Пустой stdout (нулевая перестановка) — ok с пустыми массивами.

Выход (stdout JSON): {domain, permutations[], ns_history[], total}.
Доменные ошибки: empty_domain, dnstwist_not_installed, timeout (retryable),
tool_failed. Платформенные (exit 2): битый JSON входа, нечисловые
threads/wall_timeout, не-JSON вывод инструмента.
"""
import json
import os
import shutil
import subprocess
import sys
import tempfile

DEFAULT_WALL = 300


def ok(output):
    print(json.dumps({"status": "ok", "output": output}))
    return 0


def fail(code, message, retryable=False, exit_code=1):
    print(json.dumps({"status": "error",
                      "error": {"code": code, "message": message,
                                "retryable": retryable}}))
    return exit_code


def is_transient(stderr):
    tail = (stderr or "").lower()
    return any(w in tail for w in ("timeout", "timed out", "connection",
                                   "temporary", "reset by peer", "servfail",
                                   "resolve"))


def parse_report(stdout):
    """JSON-массив dnstwist -> ([перестановки], [ns-записи])."""
    raw = json.loads(stdout)
    if not isinstance(raw, list):
        raise ValueError("ожидался массив перестановок, а не объект")
    permutations = []
    ns_history = []
    for item in raw:
        if not isinstance(item, dict):
            continue
        domain = str(item.get("domain") or "").strip()
        if not domain:
            continue
        addresses = item.get("dns_a") or []
        if not isinstance(addresses, list):
            addresses = [addresses]
        permutations.append({"domain": domain,
                             "fuzzer": str(item.get("fuzzer") or ""),
                             "resolved": bool(addresses)})
        nameservers = item.get("dns_ns") or []
        if not isinstance(nameservers, list):
            nameservers = [nameservers]
        nameservers = [str(ns).strip() for ns in nameservers if str(ns).strip()]
        if nameservers:
            ns_history.append({"domain": domain, "nameservers": nameservers})
    return permutations, ns_history


def main():
    try:
        sys.stdin.reconfigure(encoding="utf-8")
        sys.stdout.reconfigure(encoding="utf-8")
    except Exception:
        pass

    try:
        data = json.load(sys.stdin)
    except Exception as e:
        return fail("bad_input", f"невалидный JSON на входе: {e}", exit_code=2)
    if not isinstance(data, dict):
        return fail("bad_input", "ожидается JSON-объект на входе", exit_code=2)

    domain = str(data.get("domain") or "").strip()
    if not domain:
        return fail("empty_domain", "domain пуст")

    threads = data.get("threads")
    if threads is not None:
        try:
            threads = int(threads)
        except (TypeError, ValueError):
            return fail("bad_threads", "threads обязан быть целым числом",
                        exit_code=2)
        if threads < 1:
            return fail("bad_threads", "threads обязан быть >= 1", exit_code=2)

    registered = data.get("registered_only")
    if registered is not None and not isinstance(registered, bool):
        return fail("bad_registered_only",
                    "registered_only обязан быть true/false", exit_code=2)

    try:
        wall = float(data.get("wall_timeout") or DEFAULT_WALL)
    except (TypeError, ValueError):
        return fail("bad_wall_timeout", "wall_timeout обязан быть числом",
                    exit_code=2)

    bin_env = os.environ.get("DNSTWIST_BIN", "").strip()
    if bin_env:
        # subprocess поедет с cwd во временной папке: имя из PATH ищем
        # which'ем, путь — приводим к абсолютному
        if "/" not in bin_env and "\\" not in bin_env:
            bin_env = shutil.which(bin_env) or os.path.abspath(bin_env)
        else:
            bin_env = os.path.abspath(bin_env)
        cmd = [bin_env]
    else:
        cmd = [sys.executable, "-m", "dnstwist"]
    if len(cmd) == 1 and cmd[0].lower().endswith(".py"):
        # v0.29: .py-мок запускаем через интерпретатор (прямой exec
        # непереносим: shebang+CRLF на Linux, ассоциации на Windows).
        cmd = [sys.executable] + cmd
    cmd += [domain, "-f", "json"]
    if threads is not None:
        cmd += ["-t", str(threads)]
    if registered:
        cmd.append("--registered")

    with tempfile.TemporaryDirectory() as td:
        try:
            proc = subprocess.run(cmd, capture_output=True, text=True,
                                  cwd=td, timeout=wall)
        except FileNotFoundError:
            return fail("dnstwist_not_installed",
                        "dnstwist не найден: pip install dnstwist "
                        "(или укажите DNSTWIST_BIN)")
        except subprocess.TimeoutExpired:
            return fail("timeout",
                        f"dnstwist не уложился в {wall:.0f}s: увеличьте "
                        "wall_timeout", retryable=True)
        if proc.stderr:
            sys.stderr.write(proc.stderr)
        if proc.returncode != 0:
            tail = (proc.stderr or "").strip().splitlines()
            last = tail[-1] if tail else f"exit {proc.returncode}"
            return fail("tool_failed", f"dnstwist упал: {last}",
                        retryable=is_transient(proc.stderr))
        stdout = proc.stdout or ""

    # dnstwist печатает JSON только если перестановки нашлись — пустой stdout
    # это «ничего не сгенерировано», а не сбой.
    if not stdout.strip():
        return ok({"domain": domain, "permutations": [], "ns_history": [],
                   "total": 0})

    try:
        permutations, ns_history = parse_report(stdout)
    except ValueError as e:
        return fail("bad_report", f"не разобран JSON dnstwist: {e}",
                    exit_code=2)

    return ok({"domain": domain, "permutations": permutations,
               "ns_history": ns_history, "total": len(permutations)})


if __name__ == "__main__":
    sys.exit(main())