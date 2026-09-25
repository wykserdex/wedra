# START HERE — тест за 5 минут

Это ранняя версия оркестратора WEDRA: CLI, локально, цепочки из YAML + человек в петле. Тестировать идеально — не хваля: нас интересует, **где ты споткнулся**.

## Требования

- Python 3.9+ в PATH (`python3 --version` или `py --version`). Go НЕ нужен — бинарники приложены.
- Опционально: `pip install dnspython` (иначе MX-проверка в паке A тихо деградирует — это ok).

## Шаг 0 — бинарь под твою ОС

```
wedra-windows-amd64.exe      ← Windows (SmartScreen может ругнуться на неподписанный бинарь — «Подробнее → Выполнить»)
wedra-linux-amd64            ← Linux
wedra-darwin-arm64           ← macOS Apple Silicon (при Gatekeeper: xattr -d com.apple.quarantine ./wedra-darwin-arm64)
wedra-darwin-amd64           ← macOS Intel
```

Дальше пишем `wedra` — подставь свой путь. Работай из корня распакованной папки (пути в пайплайнах относительны).

## 5-минутный сценарий

**1. Статическая проверка цепочки (покажет, что контракты ловятся до запуска):**
```bash
wedra pipeline validate examples/email_check.yaml
```

**2. Пак A — проверка email-списка, человек в петле:**
```bash
wedra pipeline run examples/email_check.yaml
```
Прогонит 3 email (третий — заведомо битый, смотри журнал событий). На паузе `human_gate`: Enter (без правки) → `a` (принять) или `r` (отклонить).

**3. Пак B — текстовый LLM-конвейер в mock-режиме:**
```bash
# Windows (cmd):    set LLM_MOCK=1 && set GEMINI_API_KEY=mock && set LLM_OAI_API_KEY=mock && wedra.exe pipeline run examples\llm_text_chain.yaml
# PowerShell:       $env:LLM_MOCK=1; $env:GEMINI_API_KEY="mock"; $env:LLM_OAI_API_KEY="mock"; .\wedra.exe pipeline run examples\llm_text_chain.yaml
# Linux/macOS:      LLM_MOCK=1 GEMINI_API_KEY=mock LLM_OAI_API_KEY=mock ./wedra pipeline run examples/llm_text_chain.yaml
```
На гейте попробуй ввести правку (JSON-строка в кавычках) и нажми `a` — refine-шаг должен получить именно твою правку. Ключи всё равно должны существовать: preflight проверяет `pipeline.secrets`, даже когда mock-плагин не использует сеть. С настоящими ключами (`GEMINI_API_KEY`, `LLM_OAI_API_KEY`) то же самое по-настоящему.

**4. Собери свой плагин за минуту:**
```bash
wedra plugin create plugins/proba
# отредактируй plugins/proba/main.py (там урок протокола в комментариях)
wedra plugin test plugins/proba      # контракт-тесты — должны быть зелёными из коробки
```

**5. Загляни в журнал:** `runs/<последняя папка>/journal.jsonl` — все события рана.

## Что прислать в ответ (это и есть данные M5)

1. Дошёл ли до рабочей цепочки **без моих подсказок**? Если нет — на каком шаге завяз?
2. Сколько минут от распаковки до первого зелёного прогона? (замерь честно)
3. Что было непонятно в терминах: «плагин», «цепочка», «human_gate», «контракт»?
4. `wedra plugin create` → свой плагин: получилось? сколько минут?
5. Один абзац: стал бы ты таким пользоваться и для какой своей задачи?

Любой traceback, неловкая ошибка, «а чего оно молчит» — присылай скрином/текстом, это золото.

## Известные косяки (честно)

- Windows: unsigned exe → SmartScreen; требуется Python (плагины на нём).
- GUI, `--resume` и MCP работают; resume требует тот же pipeline identity и журнал.
- Нет OS-песочницы для community-плагинов: они запускаются с правами текущего пользователя.
- Секреты — только env-переменные; `permissions` — декларация и аудит, не изоляция.
- SMTP-плагин (сырой 25-й порт) сознательно НЕ в витрине: у большинства провайдеров порт закрыт.
