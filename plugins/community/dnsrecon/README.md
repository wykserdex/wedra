# dnsrecon

DNS-разведка домена (A/NS/MX/SOA/SPF/CNAME/AXFR, опционально брутфорс
поддоменов по словарю) — обёртка над CLI dnsrecon, читает JSON-репорт `-j`.

Установка внешнего инструмента: `pip install dnsrecon`.
Путь к бинарю переопределяется env `DNSRECON_BIN` (по умолчанию `python3 -m dnsrecon`).