# ssl_info

Снимает TLS-сертификат с `host:port`: CN субъекта и издателя,
`notAfter`, дней до протухания, флаги expired/self-signed.
`MOCK=1` — ответ без сети.

Для разбора DER-сертификата требуется `cryptography==50.0.2`:
`python3 -m pip install -r requirements.lock`.
