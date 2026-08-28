# Protocol

Протокол — контракт между ядром и плагинами. Раньше был только `PROTOCOL.md` как «вечный документ», теперь разделён на версии и машинно-читаемые схемы.

## Структура

- `versions/v0.1.md` — протокол v0.1 (изначальный, from в манифесте)
- `versions/v0.2.md` — протокол v0.2 (bind в пайплайне, manifest без from легален)
- `schemas/v0.1/` и `v0.2/` — JSON Schema для manifest, request, response, event
- `examples/` — примеры конвертов

## Версионирование

- `protocol_version` в конверте (пока не обязателен, но планируется в v0.3)
- `platform_api` в plugin.yaml — semver диапазон совместимости с ядром (`^0.1`)
- `format_version` в pipeline.yaml — `0.1` или `0.2`
- Правила обратной совместимости: v0.2 обратно совместим с v0.1 (from остаётся дефолтом), v0.3 будет требовать envelope с protocol_version

## Envelope (план v0.3)

```json
{
  "protocol_version": "0.2",
  "type": "invoke",
  "request_id": "req-123",
  "payload": {"email": "a@b.com"}
}
```

Сейчас транспорт — простой JSON на stdin/stdout (один объект), без envelope. В v0.3 планируется JSONL с envelope для cancel, handshake, streaming.

## Lifecycle плагина (v0.2)

1. Ядро резолвит `plugin.yaml` → `Engine.LoadManifest`
2. Статическая валидация `Validate` (типы, форматы, bind, skip-безопасность)
3. `fileRefWarningsForRun` — проверка file_ref до запуска
4. Запуск процесса: cwd = директория плагина, stdin = JSON вход, stdout = JSON конверт, stderr = логи
5. Exit code: 0 ok, 1 domain error (on_error), >=2 platform error (всегда stop)
6. EnforceOutput — только объявленные поля попадают в `steps.<id>`
7. Journal: `run_start`, `item_start`, `step_start`, `step_end`, `gate_decision`, `item_end`, `run_end`

## Ограничения

- Размер сообщения: stdout должен быть одним JSON-объектом, без мусора (иначе protocol_violation)
- Таймауты: дефолт 60s, задаётся в шаге `timeout: 10s`
- Формат ошибок: `{"status":"error","error":{"code":"...","message":"...","retryable":bool}}`
- stdout — только протокол, stderr — логи (ядро складывает в journal)
