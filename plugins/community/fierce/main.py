#!/usr/bin/env python3
"""fierce — DNS-брутфорс домена по словарю (обёртка над CLI fierce3).

Вход (stdin JSON): domain, words[] (опц.), dns_servers[] (опц.), tcp (опц.,
bool), delay (опц., с), wall_timeout (опц., 300).

Вызов: <FIERCE_BIN|fierce3> --domain <domain> [--subdomain-file <tmp>/words.txt]
       [--dns-servers <ip> ...] [--tcp] [--delay N]   (cwd = временная папка).

У донора машинного вывода нет: весь отчёт — текст в stdout, разбор
консервативный. Строка находки у fierce печатается как
`print("Found: {} ({})".format(url, ip))`, где url — dns.name.Name и печатается
с точкой на конце: ловим строго `^Found: <host> (<ip>)$` (хвост отбрасываем).
Отчёт считается разобранным, если в выводе есть служебные маркеры `NS:` или
`SOA:` — их fierce печатает всегда; если их нет и нет ни одной находки, то это
не текст fierce → bad_report. Ноль строк `Found:` при маркерах — нормальный
результат (subdomains: [], total: 0), не ошибка. Если словарь не передан,
флаг --subdomain-file не добавляется и fierce берёт свой встроенный
default.txt.

Выход (stdout JSON): {domain, subdomains[{host, ip}], total}.

Доменные ошибки: empty_domain, fierce_not_installed, timeout (retryable),
tool_failed. Платформенные (exit 2): битый JSON входа, words/dns_servers не
массивы, tcp не bool, delay/wall_timeout не числа, вывод не от fierce.
"""
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile

DEFAULT_WALL = 300

FOUND_RE = re.compile(r"^Found:\s+(\S+)\s+\(([^)]*)\)\s*$")
MARKERS = ("NS:", "SOA:")


def ok(output):
    print(json.dumps({"status": "ok", "output": output}))
    return 0


def fail(code, message, retryable=False, exit_code=1):
    print(json.dumps({"status": "error",
                      "error": {"code": code, "message": message,
                                "retryable": retryable}}))
    return exit_code


def parse_output(text):
    found = []
    seen = set()
    markers = False
    for line in text.splitlines():
        line = line.strip()
        if not line:
            continue
        if line.startswith(MARKERS):
            markers = True
        match = FOUND_RE.match(line)
        if not match:
            continue
        host = match.group(1).strip().rstrip(".")
        ip = match.group(2).strip()
        if not host or host in seen:
            continue
        seen.add(host)
        found.append({"host": host, "ip": ip})
    return found, markers


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

    domain = str(data.get("domain") or "").strip().lower().rstrip(".")
    if not domain:
        return fail("empty_domain", "domain пуст")

    words = data.get("words")
    if words is not None:
        if not isinstance(words, list):
            return fail("bad_words", "words обязан быть массивом", exit_code=2)
        words = [str(w).strip().lower().strip(".")
                 for w in words if str(w).strip()]

    dns_servers = data.get("dns_servers")
    if dns_servers is not None:
        if not isinstance(dns_servers, list):
            return fail("bad_dns_servers",
                        "dns_servers обязан быть массивом", exit_code=2)
        dns_servers = [str(s).strip() for s in dns_servers if str(s).strip()]

    tcp = data.get("tcp")
    if tcp is None:
        tcp = False
    if not isinstance(tcp, bool):
        return fail("bad_tcp", "tcp обязан быть bool", exit_code=2)

    delay = data.get("delay")
    if delay is not None:
        try:
            delay = float(delay)
        except (TypeError, ValueError):
            return fail("bad_delay", "delay обязан быть числом", exit_code=2)
        if delay < 0:
            return fail("bad_delay", "delay не может быть отрицательным",
                        exit_code=2)

    try:
        wall = float(data.get("wall_timeout") or DEFAULT_WALL)
    except (TypeError, ValueError):
        return fail("bad_wall_timeout", "wall_timeout обязан быть числом",
                    exit_code=2)

    bin_env = os.environ.get("FIERCE_BIN", "fierce3").strip()
    if "/" in bin_env or "\\" in bin_env:
        cmd = [os.path.abspath(bin_env)]
    else:
        cmd = [shutil.which(bin_env) or os.path.abspath(bin_env)]
    if len(cmd) == 1 and cmd[0].lower().endswith(".py"):
        cmd = [sys.executable] + cmd

    with tempfile.TemporaryDirectory() as td:
        cmd += ["--domain", domain]
        if words is not None:
            words_path = os.path.join(td, "words.txt")
            with open(words_path, "w", encoding="utf-8") as f:
                f.write("\n".join(words) + ("\n" if words else ""))
            cmd += ["--subdomain-file", words_path]
        if dns_servers:
            cmd += ["--dns-servers"] + dns_servers
        if tcp:
            cmd += ["--tcp"]
        if delay is not None:
            cmd += ["--delay", str(delay)]

        try:
            proc = subprocess.run(cmd, capture_output=True, text=True,
                                  cwd=td, timeout=wall)
        except FileNotFoundError:
            return fail("fierce_not_installed",
                        "fierce3 не найден в PATH: pip install fierce3 "
                        "(или укажите FIERCE_BIN)")
        except subprocess.TimeoutExpired:
            return fail("timeout",
                        f"fierce не уложился в {wall:.0f}s: уменьшите words "
                        "или увеличьте wall_timeout", retryable=True)
        if proc.stderr:
            sys.stderr.write(proc.stderr)

        found, markers = parse_output(proc.stdout or "")

    if proc.returncode != 0 and not found:
        tail = (proc.stderr or proc.stdout or "").strip().splitlines()
        last = tail[-1] if tail else ""
        return fail("tool_failed",
                    f"fierce упал с кодом {proc.returncode}: {last}")
    if not markers:
        return fail("bad_report",
                    "вывод fierce не разобран: нет ни строк 'Found:', "
                    "ни маркеров NS:/SOA:", exit_code=2)

    return ok({"domain": domain, "subdomains": found, "total": len(found)})


if __name__ == "__main__":
    sys.exit(main())