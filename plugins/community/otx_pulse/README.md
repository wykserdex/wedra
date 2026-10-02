# otx_pulse

Публичные пульсы AlienVault OTX по индикатору (домен/хост, IP, URL, md5/sha1/sha256):
сколько пульсов и какие (`id`, `name`, `created`). Защитная проверка: только
чтение открытых данных OTX.

Внешнего пакета нет: запрос делает дочерний процесс со сниппетом на stdlib-urllib
(GET `otx.alienvault.com/api/v1/indicators/<type>/<indicator>/general`).
Публичные пульсы отдаются без ключа, поэтому `permissions.secrets` пуст.
Путь к донору переопределяется env `OTX_PULSE_BIN` — тесты идут через
`mock_otx_pulse.py`, без сети.