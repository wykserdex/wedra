# Версионирование WEDRA

## Канонические оси

| Ось | Источник | Текущее значение | Правило |
|---|---|---|---|
| Product / application | `VERSION` | `0.32a` | буквенные инкременты внутри `X.Y`: `0.32a`, `0.32b`, … ; тег `v0.32a` |
| Pipeline и plugin protocol | `protocol/VERSION` | `0.2` | меняется только при изменении контракта |
| Registry schema | `registry.yaml` | `0.1` | формат записи реестра |
| Plugin component | `plugin.yaml:version` | semver плагина | версия конкретного компонента |
| Plugin compatibility | `plugin.yaml:platform_api` | `^0.1` | совместимость с protocol API |
| MCP wire protocol | MCP server | `2024-11-05` | внешний JSON-RPC контракт |
| Registry source pin | `version` + `commit` | entry-specific | tag источника и immutable SHA |

`format_version` в pipeline — это версия protocol, а не версия WEDRA.

## Product version

`VERSION` — единственный источник текущей версии приложения.

Схема нумерации: стабильная базовая версия `X.Y`, а инкременты внутри неё
идут буквенными суффиксами в алфавитном порядке — `0.32a`, `0.32b`, `0.32c`,
и так далее. Каждый инкремент выходит отдельным тегом `vX.YZ` и отдельным
релизом, поэтому любой шаг можно откатить точечно, не трогая соседние.
Когда буквы исчерпаны, база повышается на minor (`0.33a`).

Правила:

- Номер не уменьшается и не переиспользуется. Следующий инкремент выбирается
  строго после последнего опубликованного тега.
- `X.YZ` не является SemVer prerelease и не должен им разбираться как SemVer
  (например, semver-библиотека может отвергнуть `0.32a` как некорректный).
  `VERSION` — это строка, которую читает ядро, а не SemVer-constraint.
- Версия плагина (`plugin.yaml:version`) остаётся SemVer — это независимая ось.

Release workflow отклоняет tag, если он не совпадает с `VERSION`, и CI требует
заголовок `## <VERSION> ` в `CHANGELOG.md`. Перед релизом `VERSION` меняется с
рабочей версии на выбранную release-версию.

## Protocol version

`protocol/VERSION` и каталог `protocol/vX.Y/` описывают формат pipeline,
plugin manifest и ошибок. Product release может не менять protocol; изменение
protocol требует обновления protocol changelog, schema и conformance fixtures.

Поддерживаемые `format_version` перечисляются в коде и protocol documentation.
Неиспользуемые или экспериментальные номера не должны появляться в текущих
документах как active protocol.

## Registry pins

`registry.yaml` имеет собственную версию формата. Поле `version` в записи —
tag или branch источника, а `commit` — точный SHA для supply-chain pin. Они не
должны автоматически совпадать с product version: это разные системы
ссылок.

## Проверки

CI и release проверяют:

1. `VERSION` совпадает с верхним заголовком `CHANGELOG.md`.
2. `protocol/VERSION` совпадает с текущим заголовком protocol changelog.
3. `conformance/manifest.json` использует текущую protocol version.
4. Release tag совпадает с текущим `VERSION`.
5. Последний опубликованный tag не переиспользуется и не перемещается.

## Исторические обозначения

`v9`, `v10`, `v0.9` в старых сообщениях и `archive/` — исторические записи.
Они не являются альтернативными текущими версиями и не используются в новых
документах как источник version.
