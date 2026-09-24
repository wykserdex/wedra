# Версионирование WEDRA

## Канонические оси

| Ось | Источник | Текущее значение | Правило |
|---|---|---|---|
| Product / application | `VERSION` | `0.30.0-dev` | SemVer; tag стабильного релиза `vX.Y.Z` |
| Pipeline и plugin protocol | `protocol/VERSION` | `0.2` | меняется только при изменении контракта |
| Registry schema | `registry.yaml` | `0.1` | формат записи реестра |
| Plugin component | `plugin.yaml:version` | semver плагина | версия конкретного компонента |
| Plugin compatibility | `plugin.yaml:platform_api` | `^0.1` | совместимость с protocol API |
| MCP wire protocol | MCP server | `2024-11-05` | внешний JSON-RPC контракт |
| Registry source pin | `version` + `commit` | entry-specific | tag источника и immutable SHA |

`format_version` в pipeline — это версия protocol, а не версия WEDRA.

## Product version

`VERSION` — единственный источник текущей версии приложения. В разработке
используется SemVer prerelease (`X.Y.Z-dev`); стабильный релиз публикуется
только как `X.Y.Z` и получает тег `vX.Y.Z`.

Номер не уменьшается и не переиспользуется. Если опубликованная версия выше
текущей development-версии, следующий релиз начинается с максимальной
опубликованной версии плюс один minor. Старые `v9`, `v10`, буквенные suffixes
и записи ниже `v0.29` считаются историческими обозначениями и не выбирают
номер следующего релиза.

Release workflow обязан отклонять tag, если он не совпадает с `VERSION`.
Перед релизом `VERSION` меняется с development-версии на стабильную.

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
4. Release tag совпадает со стабильным `VERSION`.
5. Последний опубликованный tag не переиспользуется и не перемещается.

## Исторические обозначения

`v9`, `v10`, `v0.9` в старых сообщениях и `archive/` — исторические записи.
Они не являются альтернативными текущими версиями и не используются в новых
документах как источник version.
