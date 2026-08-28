package core

import (
	"strings"
	"testing"
)

// Фейковые манифесты (без диска): Validate ходит через Engine.cache.
func syntaxManifest() *Manifest {
	return &Manifest{
		ID: "syntax",
		Input: map[string]Port{
			"email": {From: "input.item", Type: "string", Format: "email"},
		},
		Output: map[string]Port{
			"syntax": {Type: "boolean"},
			"mx":     {Type: "array"},
		},
	}
}

func mxConsumerManifest(optional bool) *Manifest {
	return &Manifest{
		ID: "mx_consumer",
		Input: map[string]Port{
			"mx": {From: "steps.syntax.mx", Type: "array", Optional: optional},
		},
		Output: map[string]Port{"done": {Type: "boolean"}},
	}
}

func engineWith(mans ...*Manifest) *Engine {
	eng := NewEngine()
	for _, m := range mans {
		eng.cache["fake/"+m.ID] = m
	}
	return eng
}

func validForeachPipeline() *PipelineFile {
	return &PipelineFile{
		FormatVersion: PlatformAPI,
		Pipeline: Pipeline{
			Name:        "ok",
			Input:       map[string]interface{}{"items": []interface{}{"a@b.c"}},
			Foreach:     "input.items",
			ForeachItem: "item",
			ItemType:    "string",
			ItemFormat:  "email",
			Steps: []Step{
				{ID: "syntax", Plugin: "fake/syntax", OnError: "stop"},
				{ID: "consume", Plugin: "fake/mx_consumer", OnError: "stop"},
			},
		},
	}
}

func expectErr(t *testing.T, errs []string, substr string) {
	t.Helper()
	for _, e := range errs {
		if strings.Contains(e, substr) {
			return
		}
	}
	t.Fatalf("ожидалась ошибка %q, получены: %v", substr, errs)
}

func TestValidateOK(t *testing.T) {
	pf := validForeachPipeline()
	errs, _ := Validate(pf, engineWith(syntaxManifest(), mxConsumerManifest(false)))
	if len(errs) != 0 {
		t.Fatalf("валидная цепочка не должна давать ошибок: %v", errs)
	}
}

func TestValidateForeachMissingArray(t *testing.T) {
	pf := validForeachPipeline()
	pf.Pipeline.Foreach = "input.nope"
	errs, _ := Validate(pf, engineWith(syntaxManifest(), mxConsumerManifest(false)))
	expectErr(t, errs, "foreach")
}

func TestValidateForeachItemWithoutFormat(t *testing.T) {
	// Исторический баг: элемент foreach без item_format не покрывает порт с format: email.
	pf := validForeachPipeline()
	pf.Pipeline.ItemFormat = ""
	errs, _ := Validate(pf, engineWith(syntaxManifest(), mxConsumerManifest(false)))
	expectErr(t, errs, "формат")
}

func TestValidateDuplicateIDs(t *testing.T) {
	pf := validForeachPipeline()
	pf.Pipeline.Steps[1].ID = "syntax"
	pf.Pipeline.Steps[1].Plugin = "fake/syntax"
	errs, _ := Validate(pf, engineWith(syntaxManifest()))
	expectErr(t, errs, "дублирующийся id")
}

func TestValidateUnknownPolicy(t *testing.T) {
	pf := validForeachPipeline()
	pf.Pipeline.Steps[0].OnError = "skipp"
	errs, _ := Validate(pf, engineWith(syntaxManifest(), mxConsumerManifest(false)))
	expectErr(t, errs, "stop|skip|retry")
}

func TestValidateTypeMismatch(t *testing.T) {
	cons := mxConsumerManifest(false)
	cons.Input["mx"] = Port{From: "steps.syntax.mx", Type: "object"} // producer даёт array
	pf := validForeachPipeline()
	errs, _ := Validate(pf, engineWith(syntaxManifest(), cons))
	expectErr(t, errs, "несовместим")
}

func TestValidateSkipSafety(t *testing.T) {
	// skip-able продюсер → потребитель обязан объявить вход optional
	pf := validForeachPipeline()
	pf.Pipeline.Steps[0].OnError = "skip"

	errs, _ := Validate(pf, engineWith(syntaxManifest(), mxConsumerManifest(false)))
	expectErr(t, errs, "optional")

	pf2 := validForeachPipeline()
	pf2.Pipeline.Steps[0].OnError = "skip"
	errs2, _ := Validate(pf2, engineWith(syntaxManifest(), mxConsumerManifest(true)))
	if len(errs2) != 0 {
		t.Fatalf("optional-потребитель skip-able продюсера должен быть валиден: %v", errs2)
	}
}

func TestValidateForwardReference(t *testing.T) {
	// потребитель идёт РАНЬШЕ продюсера
	pf := validForeachPipeline()
	pf.Pipeline.Steps[0], pf.Pipeline.Steps[1] = pf.Pipeline.Steps[1], pf.Pipeline.Steps[0]
	errs, _ := Validate(pf, engineWith(syntaxManifest(), mxConsumerManifest(false)))
	expectErr(t, errs, "не найден выше по цепочке")
}

func TestValidateMissingInputField(t *testing.T) {
	m := syntaxManifest()
	m.Input["email"] = Port{From: "input.email", Type: "string"} // нет такого поля в input
	pf := validForeachPipeline()
	errs, _ := Validate(pf, engineWith(m, mxConsumerManifest(false)))
	expectErr(t, errs, "нет поля email")
}

func TestValidateFormatNarrowing(t *testing.T) {
	producer := &Manifest{
		ID:     "p",
		Output: map[string]Port{"f": {Type: "string", Format: "email"}},
	}
	consumerFor := func(format string) *Manifest {
		return &Manifest{
			ID:     "c",
			Input:  map[string]Port{"x": {From: "steps.p.f", Type: "string", Format: format}},
			Output: map[string]Port{"done": {Type: "boolean"}},
		}
	}
	mkPF := func(c *Manifest) *PipelineFile {
		return &PipelineFile{
			FormatVersion: PlatformAPI,
			Pipeline: Pipeline{
				Name: "fmt", Input: map[string]interface{}{},
				Steps: []Step{
					{ID: "p", Plugin: "fake/p"},
					{ID: "c", Plugin: "fake/c"},
				},
			},
		}
	}

	// producer email → consumer text: сужение, разрешено
	if errs, _ := Validate(mkPF(consumerFor("text")), engineWith(producer, consumerFor("text"))); len(errs) != 0 {
		t.Fatalf("email → text должно быть совместимо: %v", errs)
	}
	// producer email → consumer url: разные ветки, запрещено
	errs, _ := Validate(mkPF(consumerFor("url")), engineWith(producer, consumerFor("url")))
	expectErr(t, errs, "формат")
	// producer без формата → consumer email: запрещено (не хватает гарантий)
	bare := &Manifest{ID: "p", Output: map[string]Port{"f": {Type: "string"}}}
	errs, _ = Validate(mkPF(consumerFor("email")), engineWith(bare, consumerFor("email")))
	expectErr(t, errs, "формат")
}

func TestValidateLiteralInputFormat(t *testing.T) {
	// одиночный ран: литеральный input проверяется по формату порта ДО запуска
	m := &Manifest{
		ID:     "syntax_lit",
		Input:  map[string]Port{"email": {From: "input.item", Type: "string", Format: "email"}},
		Output: map[string]Port{"syntax": {Type: "boolean"}},
	}
	mk := func(email string) *PipelineFile {
		return &PipelineFile{
			FormatVersion: PlatformAPI,
			Pipeline: Pipeline{
				Name:  "single",
				Input: map[string]interface{}{"item": email},
				Steps: []Step{{ID: "s", Plugin: "fake/syntax_lit"}},
			},
		}
	}
	if errs, _ := Validate(mk("a@b.c"), engineWith(m)); len(errs) != 0 {
		t.Fatalf("валидный литерал email должен проходить: %v", errs)
	}
	errs, _ := Validate(mk("bad-email"), engineWith(m))
	expectErr(t, errs, "не соответствует формату")
}

func TestValidatePluginDirFixtures(t *testing.T) {
	if errs := ValidatePluginDir("testdata/plugins/echo_ok"); len(errs) != 0 {
		t.Fatalf("echo_ok должен быть валиден: %v", errs)
	}
	if errs := ValidatePluginDir("testdata/plugins/no_such_dir"); len(errs) == 0 {
		t.Fatal("несуществующая папка обязана давать ошибку")
	}
}
