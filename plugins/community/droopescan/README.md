# droopescan

Сканер CMS — Drupal, WordPress, Silverstripe, частично Joomla и Moodle (SamJoan/droopescan), обёртка над CLI.
Цель уходит в `droopescan scan <cms> -u <url> --output json`; из JSON-вывода берётся версия и найденные модули/темы.

Установка внешнего инструмента: `pip install droopescan`.
Путь к бинарю переопределяется env `DROOPESCAN_BIN`.