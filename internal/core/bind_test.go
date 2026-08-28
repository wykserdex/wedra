package core

// Контракт v0.2: bind в YAML-шаге перекрывает дефолтный from манифеста.
// Мотивация (M5, фидбек): ценность = композиция; а композиция требует
// «один плагин дважды в цепочке» и «разводка живёт в пайплайне».

import (
	"path/filepath"
	"strings"
	"testing"
)

// ── валидация ─────────────────────────────────────────────────────────────

func TestValidateBindOverridesManifest(t *testing.T) {
	// манифест читает input.item, мы перебиндили на input.email
	m := &Manifest{
		ID: "checker",
		Input: map[string]Port{
			"email": {From: "input.item", Type: "string", Format: "email"},
		},
		Output: map[string]Port{"ok": {Type: "boolean"}},
	}
	pf := &PipelineFile{
		FormatVersion: "0.2",
		Pipeline: Pipeline{
			Name:  "t_bind",
			Input: map[string]interface{}{"email": "a@b.c"}, // input.item НЕТ — но есть bind
			Steps: []Step{
				{ID: "s", Plugin: "fake/checker", Bind: map[string]string{"email": "input.email"}},
			},
		},
	}
	errs, _ := Validate(pf, engineWith(m))
	if len(errs) != 0 {
		t.Fatalf("bind должен перекрывать from манифеста: %v", errs)
	}
}

func TestValidateBindUnknownPort(t *testing.T) {
	m := syntaxManifest()
	pf := validForeachPipeline()
	pf.Pipeline.Steps = []Step{
		{ID: "syntax", Plugin: "fake/syntax",
			Bind: map[string]string{"emial": "input.item"}}, // опечатка в имени порта
	}
	errs, _ := Validate(pf, engineWith(m))
	expectErr(t, errs, "несуществующий порт")
}

func TestValidateMissingBindingIsError(t *testing.T) {
	// v0.2: манифест без from легален, но пайплайн ОБЯЗАН дать bind
	m := &Manifest{
		ID:     "wirefree",
		Input:  map[string]Port{"x": {Type: "string"}}, // from опущен
		Output: map[string]Port{"done": {Type: "boolean"}},
	}
	pf := &PipelineFile{
		FormatVersion: "0.2",
		Pipeline: Pipeline{
			Name:  "t_nowire",
			Input: map[string]interface{}{"v": "s"},
			Steps: []Step{{ID: "w", Plugin: "fake/wirefree"}},
		},
	}
	errs, _ := Validate(pf, engineWith(m))
	expectErr(t, errs, "нет привязки")

	// с bind — валидно
	pf.Pipeline.Steps[0].Bind = map[string]string{"x": "input.v"}
	errs, _ = Validate(pf, engineWith(m))
	if len(errs) != 0 {
		t.Fatalf("с bind цепочка обязана быть валидной: %v", errs)
	}
}

func TestValidateBindSkipSafetyStillApplies(t *testing.T) {
	// skip-безопасность через bind: читаем из skip-able шага необязательным забывчиком
	consumer := &Manifest{
		ID:     "consumer",
		Input:  map[string]Port{"data": {Type: "array"}}, // без from, порт обязательный
		Output: map[string]Port{"done": {Type: "boolean"}},
	}
	pf := validForeachPipeline()
	pf.Pipeline.Steps[0].OnError = "skip"
	pf.Pipeline.Steps = []Step{
		pf.Pipeline.Steps[0], // syntax, skip-able
		{ID: "c", Plugin: "fake/consumer", Bind: map[string]string{"data": "steps.syntax.mx"}},
	}
	errs, _ := Validate(pf, engineWith(syntaxManifest(), consumer))
	expectErr(t, errs, "optional")
}

func TestValidateBindTypeMismatchCaught(t *testing.T) {
	producer := &Manifest{ID: "p", Output: map[string]Port{"f": {Type: "boolean"}}}
	consumer := &Manifest{
		ID:     "c",
		Input:  map[string]Port{"x": {Type: "string"}}, // хочет string
		Output: map[string]Port{"done": {Type: "boolean"}},
	}
	pf := &PipelineFile{
		FormatVersion: "0.2",
		Pipeline: Pipeline{
			Name: "t_types", Input: map[string]interface{}{},
			Steps: []Step{
				{ID: "p", Plugin: "fake/p"},
				{ID: "c", Plugin: "fake/c", Bind: map[string]string{"x": "steps.p.f"}},
			},
		},
	}
	errs, _ := Validate(pf, engineWith(producer, consumer))
	expectErr(t, errs, "несовместим")
}

func TestValidateGateRejectsBind(t *testing.T) {
	pf := validForeachPipeline()
	pf.Pipeline.Steps = append(pf.Pipeline.Steps, Step{
		ID: "g", Plugin: "core/human_gate", Bind: map[string]string{"x": "input.item"},
	})
	errs, _ := Validate(pf, engineWith(syntaxManifest(), mxConsumerManifest(false)))
	expectErr(t, errs, "human_gate")
}

// ── рантайм: главный сценарий v0.2 — один плагин дважды в цепочке ───────

func TestBindSamePluginTwiceEndToEnd(t *testing.T) {
	requirePython(t)
	t.Setenv("LLM_MOCK", "1")
	absLLM := filepath.Join("..", "..", "plugins", "official", "llm_gemini")

	pf := &PipelineFile{
		FormatVersion: "0.2",
		Pipeline: Pipeline{
			Name: "t_same_provider",
			Input: map[string]interface{}{
				"topic":         "арбузы",
				"draft_system":  "пиши",
				"refine_system": "правь",
			},
			Steps: []Step{
				{ID: "draft", Plugin: absLLM, OnError: "stop", Timeout: sec(10)},
				{ID: "review", Plugin: "core/human_gate",
					Form: []FormField{{Field: "steps.draft.text", Editable: true, Type: "string"}}},
				{ID: "refine", Plugin: absLLM, OnError: "stop", Timeout: sec(10),
					Bind: map[string]string{
						"prompt": "steps.review.text",
						"system": "input.refine_system",
					}},
			},
		},
	}
	opts := quietOpts(t)
	opts.Yes = true
	stats, err := Run(pf, NewEngine(), opts)
	if err != nil {
		t.Fatalf("цепочка с bind упала: %v", err)
	}
	if stats.OK != 1 {
		t.Fatalf("статы: %+v", stats)
	}

	snap := readContextSnapshot(t, stats.RunDir)
	steps := snap["steps"].(map[string]interface{})
	draft := steps["draft"].(map[string]interface{})["text"].(string)
	review := steps["review"].(map[string]interface{})["text"].(string)
	refine := steps["refine"].(map[string]interface{})["text"].(string)

	if review != draft {
		t.Fatalf("гейт не материализовал: review=%q", review)
	}
	// refine получил prompt ИЗ ГЕЙТА (мок возвращает эхо промпта):
	// в тексте refine дважды встречается mock-тег — вложенный черновик
	if n := strings.Count(refine, "[mock:"); n != 2 {
		t.Fatalf("refine получил не review.text (ожидалось 2 mock-тега): %q", refine)
	}
	if !strings.Contains(refine, draft) {
		t.Fatalf("refine не содержит текст черновика: %q", refine)
	}
}

// ── рантайм: bind на вход, не существующий в контексте ─────────────────

func TestBindMissingPathRuntimeError(t *testing.T) {
	requirePython(t)
	pf := &PipelineFile{
		FormatVersion: "0.2",
		Pipeline: Pipeline{
			Name: "t_bindgap",
			Input: map[string]interface{}{
				"topic": "x",
			},
			Steps: []Step{
				{ID: "d", Plugin: filepath.Join("..", "..", "plugins", "official", "llm_gemini"),
					OnError: "stop", Timeout: sec(5),
					Bind: map[string]string{"prompt": "input.not_here"}},
			},
		},
	}
	_, err := Run(pf, NewEngine(), quietOpts(t))
	if err == nil || !strings.Contains(err.Error(), "input.not_here") {
		t.Fatalf("ожидалась ошибка про несуществующий путь, got: %v", err)
	}
}
