#!/usr/bin/env python3
"""prowler — аудит соответствия и безопасности облака (обёртка над CLI prowler).

Вход (stdin JSON): provider (опц., по умолчанию aws), severity (опц.,
critical|high|medium|low|informational), wall_timeout (опц., 1800).

Вызов: <PROWLER_BIN|prowler|python3 -m prowler> <provider> -M json-ocsf
          -o <tmp>/report -F wedra -b -z [--severity <severity>]
       (cwd = временная папка).

Флаги подтверждены по prowler/lib/cli/parser.py и docs.prowler.com
(user-guide/cli/tutorials/reporting): -M/--output-modes/--output-formats
{nargs=+, choices csv|json-asff|json-ocsf|html}, -o/--output-directory,
-F/--output-filename (имя БЕЗ расширения), --severity {critical,high,medium,low,
informational}, -b/--no-banner, -z/--ignore-exit-code-3. Нативный `-f json`
и `-o <файл>` удалены в v4 (см. issue #3742) — поэтому json-ocsf + -o/-F.
-z обязателен: иначе находки дают exit 3, и мы сочли бы это падением.

Отчёт. prowler/config/config.py: json_ocsf_file_suffix = ".ocsf.json", имя
файла = <output_directory>/<output_filename>.ocsf.json. Содержимое — JSON-массив
OCSF Detection Finding (v1.1.0), маппинг native-JSON → OCSF задокументирован в
docs.prowler.com: CheckID → metadata.event_code, CheckTitle → finding_info.title,
Severity → severity, Status → status_code.

Выход (stdout JSON): {provider, findings[{check,severity,title,status}],
count}. count = число находок. Скан без находок — ok с пустым массивом.

Учётные данные. main.py ключей НЕ читает и не требует: prowler сам берёт их из
окружения. В тестах инструмент подменён моком.

Доменные ошибки: bad_provider, bad_severity, prowler_not_installed, timeout
(retryable), no_report, tool_failed. Платформенные (exit 2): битый JSON входа,
нечисловой wall_timeout, нечитаемый отчёт.
"""
import glob
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile

DEFAULT_PROVIDER = "aws"
DEFAULT_WALL = 1800
REPORT_DIR = "report"
REPORT_BASENAME = "wedra"
REPORT_SUFFIX = ".ocsf.json"

SEVERITIES = ("critical", "high", "medium", "low", "informational")
# Провайдеры prowler, документированные в prowler/lib/cli/parser.py и
# docs.prowler.com: aws, azure, gcp, kubernetes, github, cloudflare, iac,
# alibaba, oci. Не выдумываем остальные.
PROVIDERS = ("aws", "azure", "gcp", "kubernetes", "github", "cloudflare",
             "iac", "alibaba", "oci")

NOT_MODULE_RE = re.compile(r"No module named", re.IGNORECASE)


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
    bin_env = os.environ.get("PROWLER_BIN", "").strip()
    if bin_env:
        if "/" in bin_env or "\\" in bin_env:
            return [os.path.abspath(bin_env)]
        return [shutil.which(bin_env) or os.path.abspath(bin_env)]
    found = shutil.which("prowler")
    if found:
        return [found]
    return [sys.executable, "-m", "prowler"]


def text(value):
    if isinstance(value, str):
        return value
    return ""


def dig(node, *keys):
    for key in keys:
        if not isinstance(node, dict):
            return None
        node = node.get(key)
    return node


def normalize(item):
    metadata = item.get("metadata") if isinstance(item.get("metadata"), dict) else {}
    finding_info = (item.get("finding_info")
                    if isinstance(item.get("finding_info"), dict) else {})
    check = text(metadata.get("event_code"))
    if not check:
        uid = text(finding_info.get("uid"))
        parts = uid.split("-")
        check = parts[2] if len(parts) > 2 else uid
    title = text(finding_info.get("title")) or text(item.get("message"))
    status = text(item.get("status_code"))
    if not status:
        status = text(dig(item, "unmapped", "status")) or text(item.get("status"))
    return {
        "check": check,
        "severity": text(item.get("severity")),
        "title": title,
        "status": status,
    }


def main():
    try:
        sys.stdin.reconfigure(encoding="utf-8", errors="replace")
        sys.stdout.reconfigure(encoding="utf-8")
        sys.stderr.reconfigure(encoding="utf-8", errors="replace")
    except Exception:
        pass

    try:
        data = json.load(sys.stdin)
    except Exception as e:
        return fail("bad_input", f"невалидный JSON на входе: {e}", exit_code=2)
    if not isinstance(data, dict):
        return fail("bad_input", "ожидается JSON-объект на входе", exit_code=2)

    provider = str(data.get("provider") or DEFAULT_PROVIDER).strip().lower()
    if not provider:
        provider = DEFAULT_PROVIDER
    if provider not in PROVIDERS:
        return fail("bad_provider",
                    "provider должен быть одним из: " + ", ".join(PROVIDERS))

    severity = str(data.get("severity") or "").strip().lower()
    if severity and severity not in SEVERITIES:
        return fail("bad_severity",
                    "severity должна быть одной из: " + ", ".join(SEVERITIES))

    try:
        wall = float(data.get("wall_timeout") or DEFAULT_WALL)
    except (TypeError, ValueError):
        return fail("bad_wall_timeout", "wall_timeout обязан быть числом",
                    exit_code=2)

    cmd = resolve_bin()
    if len(cmd) == 1 and cmd[0].lower().endswith(".py"):
        cmd = [sys.executable] + cmd
    cmd += [provider, "-M", "json-ocsf", "-o", REPORT_DIR,
            "-F", REPORT_BASENAME, "-b", "-z"]
    if severity:
        cmd += ["--severity", severity]

    with tempfile.TemporaryDirectory() as td:
        try:
            proc = subprocess.run(cmd, capture_output=True, text=True,
                                  cwd=td, timeout=wall)
        except FileNotFoundError:
            return fail("prowler_not_installed",
                        "prowler не найден: pip install prowler "
                        "(или укажите PROWLER_BIN)")
        except subprocess.TimeoutExpired:
            return fail("timeout",
                        f"prowler не уложился в {wall:.0f}s: сузьте набор "
                        "проверок или увеличьте wall_timeout", retryable=True)
        if proc.stderr:
            sys.stderr.write(proc.stderr)

        blob = (proc.stdout or "") + "\n" + (proc.stderr or "")
        if proc.returncode != 0:
            if NOT_MODULE_RE.search(blob):
                return fail("prowler_not_installed",
                            "prowler не установлен: pip install prowler "
                            "(или укажите PROWLER_BIN)")
            tail = blob.strip().splitlines()
            last = tail[-1] if tail else f"exit {proc.returncode}"
            return fail("tool_failed",
                        f"prowler упал (exit {proc.returncode}): {last}")

        report_glob = os.path.join(REPORT_DIR, REPORT_BASENAME + ".*" +
                                   REPORT_SUFFIX)
        matches = sorted(glob.glob(os.path.join(td, report_glob)))
        if not matches:
            matches = sorted(glob.glob(os.path.join(td, REPORT_DIR,
                                                    "*" + REPORT_SUFFIX)))
        if not matches:
            tail = blob.strip().splitlines()
            last = tail[-1] if tail else "пустой вывод"
            return fail("no_report",
                        f"prowler не дал {REPORT_BASENAME}{REPORT_SUFFIX}: {last}")
        with open(matches[-1], encoding="utf-8", errors="replace") as f:
            text_report = f.read()

    try:
        payload = json.loads(text_report)
    except Exception as e:
        return fail("bad_report", f"не прочитан отчёт prowler: {e}",
                    exit_code=2)
    if not isinstance(payload, list):
        return fail("bad_report",
                    "ожидался массив OCSF Detection Finding, получено: "
                    + type(payload).__name__, exit_code=2)

    findings = []
    for item in payload:
        if isinstance(item, dict):
            findings.append(normalize(item))

    return ok({"provider": provider, "findings": findings,
               "count": len(findings)})


if __name__ == "__main__":
    sys.exit(main())