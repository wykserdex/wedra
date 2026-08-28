# Conformance tests

Проверяют, что плагин и ядро соблюдают PROTOCOL, а не только что manifest валиден.

Фикстуры из `internal/core/testdata/plugins/`:
- `echo_ok` — корректный ok
- `failer` — доменная ошибка exit 1
- `crasher` — краш exit 2
- `bad_proto` — мусор в stdout → protocol_violation
- `contract_breaker` — не возвращает обязательное поле
- `leaker` — возвращает незадекларированное поле
- `type_drifter` — дрейф типа
- `file_ref_echo` — file_ref
- `sleeper` — таймаут
- `retry_flaky` — retryable

Проверки:
- корректный handshake (stdin JSON → stdout JSON)
- неизвестный message type (status != ok/error)
- невалидный input (bad JSON)
- timeout
- cancel (не реализован, план v0.3)
- аварийное завершение (exit >=2) → platform:<code>
- мусор в stdout → protocol_violation
- большой output (пока нет лимита, план)
- повторный request_id (план v0.3 envelope)
- несовместимую версию (platform_api)
- graceful shutdown

Запуск: `go test ./internal/core -run TestConformance` или `tool plugin test <fixture>`
