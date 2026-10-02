#!/usr/bin/env python3
"""sslyze — анализ TLS/SSL цели (обёртка над CLI sslyze).

Вход (stdin JSON): target, commands[] (опц., ["certinfo"]), wall_timeout
(опц., 300).

Вызов: <SSLYZE_BIN|python3 -m sslyze> --json_out <tmp>/sslyze_report.json
       --quiet [--<команда> ...] <target>   (cwd = временная папка).

У sslyze отличный машинный вывод: --json_out ПРИНИМАЕТ ИМЯ ФАЙЛА (--quiet
только чтобы не мусорить в stdout), поэтому репорт читается из временного
каталога. Формат: {"server_scan_results": [ {server_location{hostname,port},
scan_status, connectivity_error_trace, scan_result{<команда>: {status,
result}}, ...} ]}. findings — по одной записи на каждый server × выполненную
проверку ({target: "host:port", command: <ключ в scan_result>, status: <status
попытки>}); ключи, начинающиеся с "_", и не-словари пропускаются. Если
scan_status сервера не SUCCESSFUL, добавляется запись command="connectivity" с
этим статусом. Пустой server_scan_results — нормальный результат (findings:
[], scanned: 0), не ошибка; отсутствие ключа server_scan_results — bad_report.

Выход (stdout JSON): {target, findings[{target, command, status}], scanned}.

Доменные ошибки: empty_target, sslyze_not_installed, timeout (retryable),
no_report, tool_failed. Платформенные (exit 2): битый JSON входа, commands не
массив/с неизвестным флагом, нечитаемый JSON-репорт.
"""
import json
import os
import shutil
import subprocess
import sys
import tempfile

DEFAULT_WALL = 300
DEFAULT_COMMANDS = ["certinfo"]

KNOWN_COMMANDS = {
    "certinfo", "heartbleed", "openssl_ccs", "reneg", "resum",
    "session_tickets", "compression", "early_data", "elliptic_curves",
    "robot", "ems", "alpn", "session_renegotiation", "sni_support",
    "tlsv1", "tlsv1_1", "tlsv1_2", "tlsv1_3", "sslv2",
}


def ok(output):
    print(json.dumps({"status": "ok", "output": output}))
    return 0


def fail(code, message, retryable=False, exit_code=1):
    print(json.dumps({"status": "error",
                      "error": {"code": code, "message": message,
                                "retryable": retryable}}))
    return exit_code


def server_label(location):
    if not isinstance(location, dict):
        return ""
    host = str(location.get("hostname") or "").strip()
    port = location.get("port")
    if not host:
        return ""
    if port in (None, ""):
        return host
    return "%s:%s" % (host, port)


def extract_findings(report):
    if not isinstance(report, dict) or "server_scan_results" not in report:
        raise ValueError("в отчёте нет server_scan_results")
    servers = report.get("server_scan_results")
    if not isinstance(servers, list):
        raise ValueError("server_scan_results не массив")
    findings = []
    for server in servers:
        if not isinstance(server, dict):
            continue
        label = server_label(server.get("server_location"))
        status = str(server.get("scan_status") or "").strip()
        if status and status.upper() != "SUCCESSFUL":
            findings.append({"target": label, "command": "connectivity",
                             "status": status})
        scan_result = server.get("scan_result")
        if not isinstance(scan_result, dict):
            continue
        for command in sorted(scan_result):
            if command.startswith("_"):
                continue
            attempt = scan_result.get(command)
            if not isinstance(attempt, dict):
                continue
            attempt_status = str(attempt.get("status") or "").strip()
            findings.append({"target": label, "command": command,
                             "status": attempt_status})
    return findings, len(servers)


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

    target = str(data.get("target") or "").strip().lower().rstrip(".")
    if not target:
        return fail("empty_target", "target пуст")

    commands = data.get("commands")
    if commands is None:
        commands = list(DEFAULT_COMMANDS)
    if not isinstance(commands, list):
        return fail("bad_commands", "commands обязан быть массивом", exit_code=2)
    commands = [str(c).strip().lstrip("-").replace("-", "_") for c in commands]
    commands = [c for c in commands if c]
    unknown = [c for c in commands if c not in KNOWN_COMMANDS]
    if unknown:
        return fail("bad_commands",
                    "неизвестные проверки sslyze: " + ",".join(unknown),
                    exit_code=2)
    if not commands:
        commands = list(DEFAULT_COMMANDS)

    try:
        wall = float(data.get("wall_timeout") or DEFAULT_WALL)
    except (TypeError, ValueError):
        return fail("bad_wall_timeout", "wall_timeout обязан быть числом",
                    exit_code=2)

    bin_env = os.environ.get("SSLYZE_BIN", "").strip()
    if bin_env:
        if "/" in bin_env or "\\" in bin_env:
            cmd = [os.path.abspath(bin_env)]
        else:
            cmd = [shutil.which(bin_env) or os.path.abspath(bin_env)]
    else:
        cmd = [sys.executable, "-m", "sslyze"]
    if len(cmd) == 1 and cmd[0].lower().endswith(".py"):
        cmd = [sys.executable] + cmd

    with tempfile.TemporaryDirectory() as td:
        report_path = os.path.join(td, "sslyze_report.json")
        cmd += ["--json_out", report_path, "--quiet"]
        for command in commands:
            cmd += ["--" + command]
        cmd += [target]

        try:
            proc = subprocess.run(cmd, capture_output=True, text=True,
                                  cwd=td, timeout=wall)
        except FileNotFoundError:
            return fail("sslyze_not_installed",
                        "sslyze не найден: pip install sslyze "
                        "(или укажите SSLYZE_BIN)")
        except subprocess.TimeoutExpired:
            return fail("timeout",
                        f"sslyze не уложился в {wall:.0f}s: уменьшите commands "
                        "или увеличьте wall_timeout", retryable=True)
        if proc.stderr:
            sys.stderr.write(proc.stderr)
        if proc.stdout:
            sys.stderr.write(proc.stdout)

        if not os.path.exists(report_path):
            tail = (proc.stderr or proc.stdout or "").strip().splitlines()
            last = tail[-1] if tail else ""
            if proc.returncode != 0:
                return fail("tool_failed",
                            f"sslyze упал с кодом {proc.returncode}: {last}")
            return fail("no_report",
                        f"sslyze не дал JSON-репорт: {last or '--json_out пуст'}")

        try:
            with open(report_path, encoding="utf-8") as f:
                report = json.load(f)
        except Exception as e:
            return fail("bad_report", f"не прочитан JSON sslyze: {e}",
                        exit_code=2)

    try:
        findings, scanned = extract_findings(report)
    except ValueError as e:
        return fail("bad_report", f"неожиданный формат репорта sslyze: {e}",
                    exit_code=2)

    return ok({"target": target, "findings": findings, "scanned": scanned})


if __name__ == "__main__":
    sys.exit(main())