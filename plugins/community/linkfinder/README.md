# linkfinder

Извлечение эндпоинтов и ссылок из JavaScript (GerbenJavado/LinkFinder),
обёртка над скриптом `linkfinder.py` в режиме `-o cli` (результат — stdout).

Установка внешнего инструмента: пакета `linkfinder` на PyPI нет, поэтому
`git clone https://github.com/GerbenJavado/LinkFinder && pip install -r requirements.txt`.
Путь к скрипту переопределяется env `LINKFINDER_BIN` (в тестах — `mock_linkfinder.py`).
Сеть нужна инструменту: цель — реальный сайт, ключей не требует.
