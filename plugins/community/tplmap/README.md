# tplmap

Поиск SSTI (server-side template injection) по одной явно заданной цели
(epinna/tplmap), обёртка над CLI. Только детект: флаги эксплуатации
(`--os-cmd`, `--os-shell`, `--upload`, `--download`, `--bind-shell`) не
передаются. Отчёт — текстовый stdout, разбирается построчно.

Установка внешнего инструмента: `git clone https://github.com/epinna/tplmap &&
pip install PyYAML requests` (на PyPI пакета `tplmap` нет, а
`pip install -r requirements.txt` из клона не собирается — там зажаты
`PyYAML==5.1.2`, `requests==2.22.0` и пакет `wsgiref==0.1.2`, которого на
Python 3 нет).
Путь к `tplmap.py` переопределяется env `TPLMAP_BIN`.

Ещё два нюанса самого tplmap 0.5, из-за которых он не стартует на современном
Python 3 «как есть»: `core/plugin.py` обращается к `collections.Mapping`
(удалён в 3.10) и зовёт `.replace()` на булевом результате рендера. На Python
≤3.9 работает; на новее нужен чуть пропатченный checkout. На интерфейс обёртки
это не влияет — она разбирает только stdout.