#!/usr/bin/env python3
"""Mock CLI olevba для контракт-тестов (без пакета oletools и без офисных файлов).

Имитирует `olevba -j -a <file>`: печатает в stdout поток JSON-объектов через
запятую — сначала MetaInformation (как print_json с _json_is_first=True),
затем объект по файлу. Никаких настоящих документов и значений секретов.

Режимы env: MOCK_SLEEP=N (тест wall_timeout), MOCK_NO_REPORT=1 (пустой stdout),
MOCK_BAD_REPORT=1 (битый JSON), MOCK_FAIL=1 (ненулевой код выхода),
MOCK_NO_MACROS=1 (документ без макросов), MOCK_BROKEN_FILE=1
(json_conversion_successful=false).
"""
import json
import os
import sys
import time

if os.environ.get("MOCK_SLEEP"):
    time.sleep(float(os.environ["MOCK_SLEEP"]))
if os.environ.get("MOCK_FAIL") == "1":
    print("olevba 0.60.1 - mock: unable to open the file",
          file=sys.stderr)
    sys.exit(6)

args = sys.argv[1:]
filename = args[-1] if args else "sample.doc"
analysis_only = "-a" in args or "--analysis" in args

if os.environ.get("MOCK_BAD_REPORT") == "1":
    sys.stdout.write('{"script_name": "olevba", "type": "MetaInformation"},\n'
                     '      {"file": "sample.doc", "macros": [oops}\n')
    sys.exit(0)
if os.environ.get("MOCK_NO_REPORT") == "1":
    sys.exit(0)

meta = {
    "script_name": "olevba",
    "version": "0.60.1",
    "python_version": [3, 12, 0],
    "url": "http://decalage.info/python/oletools",
    "type": "MetaInformation",
}

if os.environ.get("MOCK_NO_MACROS") == "1":
    macros, analysis = [], None
elif os.environ.get("MOCK_BROKEN_FILE") == "1":
    macros = [{"vba_filename": "Module1", "subfilename": os.path.basename(filename),
               "ole_stream": "VBA/dir", "code": None}]
    analysis = None
else:
    code = None if analysis_only else "Attribute VB_Name = \"Module1\"\n"
    macros = [
        {"vba_filename": "Module1", "subfilename": os.path.basename(filename),
         "ole_stream": "VBA/dir", "code": code},
        {"vba_filename": "ThisDocument",
         "subfilename": os.path.basename(filename),
         "ole_stream": "VBA/ThisDocument", "code": code},
    ]
    analysis = [
        {"type": "AutoExec", "keyword": "AutoOpen",
         "description": "Auto-executable macro"},
        {"type": "Suspicious", "keyword": "Shell",
         "description": "May start a process"},
        {"type": "IOC", "keyword": "http://malware.example.invalid/drop",
         "description": "URL"},
    ]

result = {
    "container": None,
    "file": filename,
    "json_conversion_successful": not os.environ.get("MOCK_BROKEN_FILE") == "1",
    "analysis": analysis,
    "code_deobfuscated": None,
    "do_deobfuscate": False,
    "show_pcode": False,
    "type": "OpenXML",
    "macros": macros,
}

sys.stdout.write("      " + json.dumps(meta, indent=4).splitlines()[0] + "\n")
for line in json.dumps(meta, indent=4).splitlines()[1:]:
    sys.stdout.write("      " + line.rstrip() + "\n")
sys.stdout.write(",     " + json.dumps(result, indent=4).splitlines()[0] + "\n")
for line in json.dumps(result, indent=4).splitlines()[1:]:
    sys.stdout.write("      " + line.rstrip() + "\n")
print("olevba 0.60.1 - analysis done", file=sys.stderr)