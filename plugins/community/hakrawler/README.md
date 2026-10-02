# hakrawler

Веб-краулер для OSINT и веб-разведки (hakluke/hakrawler), обёртка над CLI.
Цель уходит в stdin (как требует сам инструмент), находки читаются из stdout в режиме `-json`.

Установка внешнего инструмента: `go install github.com/hakluke/hakrawler@latest`.
Путь к бинарю переопределяется env `HAKRAWLER_BIN`.