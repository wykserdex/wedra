#!/usr/bin/env python3
"""Фикстура: рожает дочерний sleep и выходит (тест process-group kill, v0.23).
pid дочернего — в файл $SPID_FILE."""
import os
import subprocess

child = subprocess.Popen(["sleep", "30"])
with open(os.environ["SPID_FILE"], "w") as f:
    f.write(str(child.pid))
