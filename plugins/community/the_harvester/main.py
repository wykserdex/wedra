#!/usr/bin/env python3
"""the_harvester — email/имена/субдомены по домену (обёртка над CLI theHarvester).

Вход (stdin JSON): domain, sources[] (опц., поисковики), limit (опц., 500),
wall_timeout (опц., 600).

Вызов: <THEHARVESTER_BIN|theHarvester> -d <domain> [-b s1,s2] [-l N]
       -f <tmpfile> -q  (cwd = временная папка; theHarvester пишет
       <tmpfile>.json и <tmpfile>.xml в CWD).

theHarvester 4.9.x кладёт в JSON (пороговые ключи, появляются при наличии
данных): emails, hosts, people, vhosts, asns, interesting_urls, shodan,
trello_urls, takeover_results + cmd. Мы нормализуем стабильный набор
массивов в выход (отсутствует → []).

Выход (stdout JSON): {domain, emails[], hosts[], people[], vhosts[], asns[],
interesting_urls[]}.

Установка: pip install git+https://github.com/laramies/theHarvester.git@4.9.2
(на PyPI «theharvester» — чужой squatted-пакет 0.0.1, не он).

Доменные ошибки: empty_domain, the_harvester_not_installed, timeout
(retryable), no_report. Платформенные (exit 2): битый JSON входа, битый
репорт.
"""
import glob
import json
import os
import shutil
import subprocess
import sys
import tempfile

DEFAULT_LIMIT = 500
DEFAULT_WALL = 600

# стабильный набор массивов в выходе; порядок зафиксирован для тестов
ARRAY_FIELDS = ["emails", "hosts", "people", "vhosts", "asns",
                "interesting_urls"]


def ok(output):
    print(json.dumps({"status": "ok", "output": output}))
    return 0


def fail(code, message, retryable=False, exit_code=1):
    print(json.dumps({"status": "error",
                      "error": {"code": code, "message": message,
                                "retryable": retryable}}))
    return exit_code


def main():
    try:
        data = json.load(sys.stdin)
    except Exception as e:
        return fail("bad_input", f"невалидный JSON на входе: {e}", exit_code=2)
    if not isinstance(data, dict):
        return fail("bad_input", "ожидается JSON-объект на входе", exit_code=2)

    domain = str(data.get("domain") or "").strip().lower()
    if not domain:
        return fail("empty_domain", "domain пуст")

    sources = data.get("sources")
    if sources is not None and not isinstance(sources, list):
        return fail("bad_sources", "sources обязан быть массивом", exit_code=2)
    if isinstance(sources, list):
        sources = [str(s) for s in sources]

    try:
        limit = int(float(data.get("limit") or DEFAULT_LIMIT))
    except (TypeError, ValueError):
        return fail("bad_limit", "limit обязан быть числом", exit_code=2)
    try:
        wall = float(data.get("wall_timeout") or DEFAULT_WALL)
    except (TypeError, ValueError):
        return fail("bad_wall_timeout", "wall_timeout обязан быть числом",
                    exit_code=2)

    bin_env = os.environ.get("THEHARVESTER_BIN", "theHarvester").strip()
    if "/" not in bin_env:
        # имя из PATH; если PATH пуст — относительное от cwd (моки в тестах)
        bin_env = shutil.which(bin_env) or os.path.abspath(bin_env)
    else:
        bin_env = os.path.abspath(bin_env)

    with tempfile.TemporaryDirectory() as td:
        tmpfile = os.path.join(td, "th_report")
        cmd = [bin_env, "-d", domain, "-l", str(limit),
               "-f", tmpfile, "-q"]
        if sources:
            cmd += ["-b", ",".join(sources)]

        try:
            proc = subprocess.run(cmd, capture_output=True, text=True,
                                  cwd=td, timeout=wall)
        except FileNotFoundError:
            return fail("the_harvester_not_installed",
                        "theHarvester не найден: pip install "
                        "git+https://github.com/laramies/theHarvester.git"
                        "@4.9.2 (или укажите THEHARVESTER_BIN)")
        except subprocess.TimeoutExpired:
            return fail("timeout",
                        f"theHarvester не уложился в {wall:.0f}s: "
                        "уменьшите sources/limit или увеличьте wall_timeout",
                        retryable=True)
        if proc.stderr:
            sys.stderr.write(proc.stderr)
        if proc.stdout:
            sys.stderr.write(proc.stdout)

        matches = sorted(glob.glob(tmpfile + ".*"))
        json_path = tmpfile + ".json"
        if not os.path.exists(json_path):
            tail = ((proc.stderr or proc.stdout) or "").strip().splitlines()
            last = tail[-1] if tail else f"exit {proc.returncode}"
            return fail("no_report",
                        f"theHarvester не дал JSON-репорт: {last}")
        try:
            with open(json_path, encoding="utf-8") as f:
                report = json.load(f)
        except Exception as e:
            return fail("bad_report", f"не прочитан JSON theHarvester: {e}",
                        exit_code=2)

    if not isinstance(report, dict):
        return fail("bad_report", "неожиданный формат репорта theHarvester",
                    exit_code=2)

    out = {"domain": domain}
    for field in ARRAY_FIELDS:
        value = report.get(field)
        out[field] = value if isinstance(value, list) else []

    return ok(out)


if __name__ == "__main__":
    sys.exit(main())
