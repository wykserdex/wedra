#!/usr/bin/env python3
"""webanalyze — отпечаток технологий сайта (обёртка над CLI webanalyze).

Вход (stdin JSON): url, apps_file (опц., путь к technologies.json),
crawl (опц., ссылок Follow, 0), wall_timeout (опц., общий лимит, 300).

Вызов: <WEBANALYZE_BIN|webanalyze> -host <host> -output json -silent
       [-apps <file>] [-crawl N] [-search=false]     (cwd = временная папка).

rverton/webanalyze — Go-порт Wappalyzer, pip-пакета webanalyze на PyPI нет.
Флаги сверены с README и cmd/webanalyze/main.go проекта: -host, -apps,
-crawl, -output (stdout|csv|json), -search (по умолчанию true), -silent.
С `-output json` инструмент печатает в stdout по одному json-объекту на хост:
{"hostname": ..., "matches": [{app_name, version, ...}, ...]}. Отдельного
файла-отчёта нет, поэтому разбираем stdout (паттерн B).

search отключаем флагом: иначе инструмент дополнительно обходит все поддомены
базового домена — это отдельные запросы, о которых оператор не просил.

Выход (stdout JSON): {url, technologies[{name, version}], count}. Ничего не
определено — ok с пустым массивом и count 0. Доменные ошибки: empty_url,
bad_url, webanalyze_not_installed, timeout (retryable), no_report,
tool_failed. Платформенные (exit 2): битый JSON входа, нечитаемый JSON
инструмента, нечисловой crawl/wall_timeout.
"""
import json
import os
import shutil
import subprocess
import sys
import tempfile

DEFAULT_WALL = 300
DEFAULT_BIN = "webanalyze"
DEFAULT_CRAWL = 0


try:
    sys.stdin.reconfigure(encoding="utf-8", errors="replace")
    sys.stdout.reconfigure(encoding="utf-8")
    sys.stderr.reconfigure(encoding="utf-8", errors="replace")
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
    bin_env = os.environ.get("WEBANALYZE_BIN", DEFAULT_BIN).strip() or DEFAULT_BIN
    if "/" in bin_env or "\\" in bin_env:
        return [os.path.abspath(bin_env)]
    return [shutil.which(bin_env) or os.path.abspath(bin_env)]


def split_url(raw):
    rest = raw
    scheme = ""
    if "://" in raw:
        scheme, _, rest = raw.partition("://")
        scheme = scheme.strip().lower()
        if scheme not in ("http", "https"):
            return "", "", "схема %r не поддерживается, ждём http или https" % scheme
    host = rest.split("/", 1)[0].split("?", 1)[0].split("#", 1)[0]
    host = host.strip().rstrip(".")
    if not host:
        return "", "", "не удалось выделить хост"
    if "@" in host:
        host = host.rsplit("@", 1)[-1]
    return (scheme + "://" + host if scheme else host), host, ""


def build_cmd(host, apps_file, crawl):
    cmd = resolve_bin()
    if cmd[0].lower().endswith(".py"):
        # v0.29: .py-мок запускаем через интерпретатор — прямой exec
        # непереносим (shebang+CRLF на Linux, ассоциации на Windows).
        cmd = [sys.executable] + cmd
    cmd += ["-host", host, "-output", "json", "-silent"]
    if apps_file:
        cmd += ["-apps", apps_file]
    cmd += ["-crawl", str(int(crawl))]
    cmd.append("-search=false")
    return cmd


def collect_technologies(stdout):
    for raw in stdout.splitlines():
        line = raw.strip()
        if not line or not line.startswith("{"):
            continue
        try:
            payload = json.loads(line)
        except Exception:
            continue
        if isinstance(payload, dict):
            return payload
    return None


def pick_technologies(payload):
    technologies = []
    seen = set()
    for match in payload.get("matches") or []:
        if not isinstance(match, dict):
            continue
        name = str(match.get("app_name") or "").strip()
        if not name or name in seen:
            continue
        seen.add(name)
        technologies.append({"name": name,
                             "version": str(match.get("version") or "")})
    return technologies


def main():
    try:
        data = json.load(sys.stdin)
    except Exception as e:
        return fail("bad_input", f"невалидный JSON на входе: {e}", exit_code=2)
    if not isinstance(data, dict):
        return fail("bad_input", "ожидается JSON-объект на входе", exit_code=2)

    raw_url = str(data.get("url") or "").strip()
    if not raw_url:
        return fail("empty_url", "url пуст")
    url, host, problem = split_url(raw_url)
    if problem:
        return fail("bad_url", f"url {raw_url!r}: {problem}")

    apps_file = str(data.get("apps_file") or "").strip()

    try:
        crawl = float(data.get("crawl") or DEFAULT_CRAWL)
    except (TypeError, ValueError):
        return fail("bad_crawl", "crawl обязан быть числом", exit_code=2)
    if crawl < 0:
        return fail("bad_crawl", "crawl не может быть отрицательным")
    try:
        wall = float(data.get("wall_timeout") or DEFAULT_WALL)
    except (TypeError, ValueError):
        return fail("bad_wall_timeout", "wall_timeout обязан быть числом",
                    exit_code=2)

    cmd = build_cmd(host, apps_file, crawl)

    with tempfile.TemporaryDirectory() as td:
        try:
            proc = subprocess.run(cmd, capture_output=True, text=True,
                                  cwd=td, timeout=wall)
        except FileNotFoundError:
            return fail("webanalyze_not_installed",
                        "webanalyze не найден: go install "
                        "github.com/rverton/webanalyze/cmd/webanalyze@latest "
                        "(или укажите WEBANALYZE_BIN)")
        except subprocess.TimeoutExpired:
            return fail("timeout",
                        f"webanalyze не уложился в {wall:.0f}s: уменьшите crawl "
                        "или увеличьте wall_timeout", retryable=True)
        if proc.stderr:
            sys.stderr.write(proc.stderr)

        stdout = (proc.stdout or "").strip()
        tail = [ln for ln in (proc.stderr or "").strip().splitlines()
                if ln.strip()]
        last = tail[-1] if tail else f"exit {proc.returncode}"

        if proc.returncode != 0:
            return fail("tool_failed", f"webanalyze упал: {last}")
        if not stdout:
            return fail("no_report", "webanalyze ничего не напечатал")

    payload = collect_technologies(stdout)
    if payload is None:
        return fail("bad_report",
                    f"в выводе webanalyze нет JSON-объекта: {stdout[:200]}",
                    exit_code=2)

    technologies = pick_technologies(payload)
    return ok({"url": url, "technologies": technologies,
               "count": len(technologies)})


if __name__ == "__main__":
    sys.exit(main())
