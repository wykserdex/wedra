# binwalk

Поиск и извлечение встроенных данных из бинарников (ReFirmLabs/binwalk),
обёртка над CLI: JSON-отчёт `--log`, опционально извлечение в каталог плагина.

Установка внешнего инструмента: нужен именно binwalk v3 (Rust) — `cargo install
binwalk` либо Docker-образ из wiki; у версии 2 нет ни `--log`, ни `--directory`.
`pip install binwalk` НЕ подходит: на PyPI лежит только нерабочий sdist 2.1.0
от 2015 года (нет пакета `binwalk.core`, `binwalk --help` падает).
Путь к бинарю переопределяется env `BINWALK_BIN` (в тестах — `mock_binwalk.py`).
Сеть не нужна; внешние распаковщики (7zip, unsquashfs и т. п.) — по желанию.
