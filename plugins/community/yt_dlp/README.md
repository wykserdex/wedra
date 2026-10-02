# yt_dlp

Метаданные видео по URL (yt-dlp/yt-dlp), обёртка над CLI в режиме «только
метаданные»: `-J/--dump-single-json` + `--skip-download` + `--no-playlist`
(`--yes-playlist`, если `playlist: true`) + `--ignore-config`. Из мегабайтного
info-репорта плагин отдаёт только title, uploader, duration, extractor,
webpage_url и число форматов.

Установка внешнего инструмента: `pip install yt-dlp`.
Путь к бинарю переопределяется env `YT_DLP_BIN` (единственный путь для тестов).
