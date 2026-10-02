# pyelftools

Разбор ELF-файла (eliben/pyelftools): класс (`ELF32`/`ELF64`), секции и
подключаемые библиотеки `DT_NEEDED`. У pyelftools нет CLI — плагин запускает
тот же интерпретатор с фиксированным сниппетом (`python -c …`), который
импортирует библиотеку и печатает JSON.

Установка внешнего инструмента: `pip install pyelftools`.
Путь к донору переопределяется env `PYELFTOOLS_BIN`.