# Conformance tests

Проверяют, что плагин и ядро соблюдают PROTOCOL, а не только что manifest валиден.

Фикстуры из `conformance/fixtures/v0.2/` (в development checkout также доступны в `internal/core/testdata/plugins/`):
- `echo_ok` — корректный ok
- `failer` — доменная ошибка exit 1
- `crasher` — краш exit 2
- `bad_proto` — мусор в stdout → protocol_violation
- `contract_breaker` — не возвращает обязательное поле
- `leaker` — возвращает незадекларированное поле
- `type_drifter` — дрейф типа
- `file_ref_echo` — file_ref
- `sleeper` — отмена
- `retry_flaky` — retryable
- `chatter` — oversized stdout
- `big_stderr` — большой stderr без нарушения протокола

Проверки:
- корректный handshake (stdin JSON → stdout JSON)
- timeout и отмена
- аварийное завершение (exit >=2) → platform:<code>
- мусор в stdout → protocol_violation
- лимит stdout иstderr
- golden issue codes

Запуск: `go test ./internal/core/ -run 'TestPluginTest|TestExec' -v -count=1`
или батарея CLI: `wedra plugin test --conformance --json`
или одна фикстура: `wedra plugin test conformance/fixtures/v0.2/<name>`
