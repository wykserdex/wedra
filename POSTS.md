# Посты для dev-чатов + чек-лист заливки репы (v9)

Каналы под реальные ресурсы: dev-чаты + GitHub-репа. Без бюджета. Цель — ещё 3–5 пакетов фидбека, не звёзды.

## Чек-лист перед постингом (10 мин)

1. Отозвать GitHub token (ghp_...), если ещё не отозвал — был в чате, использован для push, не сохранён в .git/config.
2. GitHub Release `v0.2-m5-v9`: tag `v0.2-m5-v9`, title `v0.2-m5-v9: 4 тестера, 3 критичных бага закрыто`, attach `orchestrator-mvp-2026-08-28-v9.zip` — холодным не надо собирать Go.
3. В About репы: `Оркестратор цепочек из плагинов с человеком в петле. Контракт stdin JSON → stdout JSON, без SDK. 93 теста, 4 внешних автора, 7 плагинов. Ищу тестеров на написание своего плагина.`
4. Topics: `cli`, `orchestrator`, `plugin-system`, `human-in-the-loop`, `golang`, `python`
5. Проверить README шапка: 4 автора, 7 плагинов, 8.5/10, 93 теста — актуально.

## Пост 1 — короткий (основной, для Python automation / Go CLI чатов)

```
Привет. Делаю CLI-оркестратор цепочек из плагинов с человеком в петле.

Плагин = обычный скрипт: stdin JSON → stdout JSON, без SDK.
Ядро собирает вход из input.* / steps.*, валидирует типы/форматы/привязки ДО запуска, пишет в steps.<id>.

Что проверено снаружи (M5):
- 4 внешних автора, 7 плагинов из 5 ниш (текст, файлы, OSINT sorter/correlator, csv/file_ref, email triage)
- 4/4 написали свой плагин с первого захода по TUTORIAL_PLUGINS.md, 0 провалов воронки
- ядро 8.5/10 для MVP (цитаты: «каждый кусок можно независимо написать, протестировать и заменить», «ошибки говорят буквально какой порт и почему несовместим»)
- 93 теста зелёных, CI, gofmt чистый
- последний внешний автор нашёл 3 критичных бага (гейт терял данные при basename-коллизии, optional без привязки падал, exit>=2 терял code) — закрыто в v9

Ищу 2–3 человека, кто попробует написать свой маленький плагин (15–30 мин) и скажет где бесит.

Репа: https://github.com/wykserdex/wedra
Release zip без сборки: v0.2-m5-v9 (bin/linux/darwin/win)

Старт: TUTORIAL_PLUGINS.md → tool plugin create --example array → plugin test → validate → run

Идеальный фидбек: что хотел сделать, где застрял, validate/test/run output, что бесит.
```

## Пост 2 — чуть глубже (для automation / n8n / Airflow / shell чатов)

Всё из поста 1 + абзац:

```
Чем отличается от n8n/Airflow: это не no-code замена, а thin CLI-контракт для своих скриптов. 
Ступень = обычный скрипт, независимо тестируется (plugin test 23ms), заменяется, между ступенями — human_gate с правкой и материализацией в steps.<gate>.*.

Фичи выросли из фидбека, не из головы: contains по массивам, enforce типов, bind в пайплайне (один плагин дважды), file_ref warning до запуска с подсказкой «есть от корня», --example array с runtime-guard, отказ format на не-string, qualified ключи при коллизии basename в gate, optional без привязки = warning, platform:code на exit>=2.

Что НЕ умеет честно: нет GUI (отложен до ≥2 запросов «без UI не пойду»), нет resume, нет foreach по steps.* (только input.* — главный недостающий кусок для батчей из файла, кандидат M6).

Демо одной командой: ./demo.sh (mock, без ключей) — create → test → validate → run с гейтом.
```

## Пост 3 — OSINT/infosec (только как один из use-case)

```
Привет. Делаю не OSINT-сканер, а CLI-оркестратор цепочек из обычных плагинов.

Для OSINT это выглядит как:
collector #1 ─┐
collector #2 ─┼─→ normalizer/correlator → human review/report
collector #3 ─┘

Плагин = stdin JSON → stdout JSON, без SDK, можно без сети/секретов/файлов.
Уже было 2 внешних OSINT-теста: sorter для Maigret-результатов и identity-correlator (дедуп, слияние идентификаторов между источниками, confidence, multi-source alerts) — оба validate/test/run прошли, оценка ядра 9/10.

Нужен 1 человек, кто попробует написать маленький processor/normalizer под свои уже собранные наблюдения и скажет где контракт тупит. Без агрессивного сбора.

Репа: https://github.com/wykserdex/wedra
Начать с TUTORIAL_PLUGINS.md — там есть --example array.
```

## Правила ответов в чатах

- «а зачем, есть n8n/Airflow?» → «Да, не замена. Ставка на локальный CLI и scripts-as-plugins без SDK: написал stdin JSON → stdout JSON, описал manifest, получил validate/test/run/human gate. Ступень независимо тестируется и заменяется.»
- «где GUI?» → «GUI отложил. Сейчас проверяю контракт и plugin authoring. Если 2+ холодных скажут без UI не пойду — верну в приоритет.»
- «слишком сыро» → «Да, это M5/MVP. Мне нужен фидбек не красиво/некрасиво, а где ломается путь create → test → validate → run.»
- Если хочет помочь → «Лучшее: возьми свою мини-задачу и оформи как plugin. Не demo ради demo, а свой processor/checker/converter.»

## Категории чатов (порядок)

1. **Python automation/scripting** — первый приоритет, плагин = обычный Python скрипт. Просить написать свой плагин 15–30 мин.
2. **Go/CLI/open-source feedback** — фидбек по CLI, протоколу, release zip, структуре репы.
3. **Automation/no-code/ops/data wrangling** (n8n, Airflow, Make/Zapier-adjacent, shell) — понимают pipelines, акцент «thin contract».
4. **OSINT/infosec** — только как один slice, просить писать processor/normalizer/correlator, без сети/секретов.
5. **LLM/local automation** — показывать llm_same_provider.yaml: LLM draft → human edit → same provider refine.

Не спамить 20 чатов сразу. День 1: 2 чата + 3 DM. Ждать 24ч. Фиксировать в OUTREACH_TRACKER.md и M5_FEEDBACK.md.

Каждый пакет фидбека → максимум одна фича/фикс → новый zip. Не плодить фичи без боли.
