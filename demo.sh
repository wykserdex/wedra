#!/usr/bin/env bash
# Детерминированная демка весь цикл (для записи/скринкаста и поста в чат).
# Никаких ключей API не нужно: LLM-плагины работают в mock-режиме.
set -e
cd "$(dirname "$0")"

# бинарник: собранный ./tool | готовый из bin/ (после unzip может потерять +x) | собрать из исходников
TOOL=./tool
[ -x "$TOOL" ] || { [ -f ./bin/linux-amd64/tool ] && chmod +x ./bin/linux-amd64/tool && TOOL=./bin/linux-amd64/tool; }
[ -x "$TOOL" ] || { echo "▶ сборка из исходников"; go build -o tool ./cmd/tool; TOOL=./tool; }

run() { echo; echo "── $ $*"; "$@"; }

echo "▶ 1/4  plugin create: скелет плагина, сразу зелёный"
rm -rf /tmp/orch_demo_plugin
$TOOL plugin create /tmp/orch_demo_plugin --author demo --description "Demo-плагин" --example array | head -5

echo; echo "▶ 2/4  plugin test: контрактные тесты автора плагина"
$TOOL plugin test /tmp/orch_demo_plugin

echo; echo "▶ 3/4  validate: статическая проверка цепочки до запуска"
$TOOL validate pipelines/llm_same_provider.yaml

echo '── $ LLM_MOCK=1 ./tool run pipelines/llm_same_provider.yaml   (в гейте правим текст)'
echo "▶ 4/4  run: Gemini-черновик → человек правит → ТОТ ЖЕ плагин дорабатывает правку (bind, v0.2)"
printf '"Правка человека: сжато и по делу."\na\n' | LLM_MOCK=1 $TOOL run pipelines/llm_same_provider.yaml

echo; echo "── итог: create → test → validate → run с гейтом человека. Один бинарник, без SDK."
