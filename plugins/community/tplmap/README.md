# tplmap

Поиск SSTI (server-side template injection) по одной явно заданной цели
(epinna/tplmap), обёртка над CLI. Только детект: флаги эксплуатации
(`--os-cmd`, `--os-shell`, `--upload`, `--download`, `--bind-shell`) не
передаются. Отчёт — текстовый stdout, разбирается построчно.

Установка внешнего инструмента: `git clone https://github.com/epinna/tplmap &&
pip install -r requirements.txt` (на PyPI пакета `tplmap` нет).
Путь к `tplmap.py` переопределяется env `TPLMAP_BIN`.