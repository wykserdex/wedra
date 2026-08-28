package core

import (
	"encoding/json"
	"testing"
)

func testJournal(t *testing.T) *Journal {
	t.Helper()
	j, err := NewJournal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(j.Close)
	return j
}

func TestGateAutoAccept(t *testing.T) {
	ctx := NewCtx(map[string]interface{}{})
	st := &Step{ID: "review", Plugin: "core/human_gate"}
	j := testJournal(t)

	if got := runGate(st, ctx, j, RunOptions{Yes: true, Quiet: true}); got != "ok" {
		t.Fatalf("auto-accept вернул %q", got)
	}
	events := readEvents(t, j.Dir)
	if countEvents(events, "gate_decision") != 1 || events[0]["auto"] != true {
		t.Fatalf("auto-accept должен фиксироваться в журнале: %v", events)
	}
}

func TestGateRejectAbortsItem(t *testing.T) {
	newStdin(t, "r\n")
	ctx := NewCtx(map[string]interface{}{})
	st := &Step{ID: "review", Plugin: "core/human_gate"}

	if got := runGate(st, ctx, testJournal(t), RunOptions{Quiet: true}); got != "abort_item" {
		t.Fatalf("reject при on_reject=stop вернул %q", got)
	}
}

func TestGateRejectContinue(t *testing.T) {
	newStdin(t, "r\n")
	ctx := NewCtx(map[string]interface{}{})
	st := &Step{ID: "review", Plugin: "core/human_gate", OnReject: "continue"}

	if got := runGate(st, ctx, testJournal(t), RunOptions{Quiet: true}); got != "ok" {
		t.Fatalf("reject при on_reject=continue вернул %q", got)
	}
}

func TestGateEditWritesToGateNamespace(t *testing.T) {
	newStdin(t, "\"patched\"\na\n")
	ctx := NewCtx(map[string]interface{}{})
	ctx.SetStep("echo", map[string]interface{}{"value": "orig"})

	st := &Step{
		ID:     "review",
		Plugin: "core/human_gate",
		Form:   []FormField{{Field: "steps.echo.value", Editable: true, Type: "string"}},
	}
	if got := runGate(st, ctx, testJournal(t), RunOptions{Quiet: true}); got != "ok" {
		t.Fatalf("accept после правки вернул %q", got)
	}

	// правка легла под неймспейс ГЕЙТА, источник не затёрт
	if v, _ := ctx.Get("steps.review.value"); v != "patched" {
		t.Fatalf("steps.review.value = %v", v)
	}
	if v, _ := ctx.Get("steps.echo.value"); v != "orig" {
		t.Fatalf("источник затёрт! steps.echo.value = %v", v)
	}
}

func TestGateEditWrongTypeSkipped(t *testing.T) {
	newStdin(t, "\"definitely not a bool\"\na\n")
	ctx := NewCtx(map[string]interface{}{})
	ctx.SetStep("x", map[string]interface{}{"flag": false})

	st := &Step{
		ID:     "review",
		Plugin: "core/human_gate",
		Form:   []FormField{{Field: "steps.x.flag", Editable: true, Type: "boolean"}},
	}
	runGate(st, ctx, testJournal(t), RunOptions{Quiet: true})

	// кривая правка отклонена — но accept материализует ИСХОДНОЕ значение
	v, ok := ctx.Get("steps.review.flag")
	if !ok || v != false {
		t.Fatalf("ожидалось исходное значение (правка отклонена), got %v ok=%v", v, ok)
	}
}

func TestGatePassThroughMaterializesFields(t *testing.T) {
	// accept БЕЗ правок: поле всё равно материализуется под неймспейсом гейта —
	// downstream читает steps.<gate>.* независимо от того, правил человек или нет
	newStdin(t, "a\n")
	ctx := NewCtx(map[string]interface{}{})
	ctx.SetStep("draft", map[string]interface{}{"text": "черновик"})

	st := &Step{
		ID:     "review",
		Plugin: "core/human_gate",
		Form:   []FormField{{Field: "steps.draft.text", Editable: false, Type: "string"}},
	}
	if got := runGate(st, ctx, testJournal(t), RunOptions{Quiet: true}); got != "ok" {
		t.Fatalf("accept вернул %q", got)
	}
	if v, _ := ctx.Get("steps.review.text"); v != "черновик" {
		t.Fatalf("поле не материализовано в гейт: steps.review.text = %v", v)
	}
}

func TestGateAutoAcceptAlsoMaterializes(t *testing.T) {
	ctx := NewCtx(map[string]interface{}{})
	ctx.SetStep("draft", map[string]interface{}{"text": "t"})
	st := &Step{
		ID:     "review",
		Plugin: "core/human_gate",
		Form:   []FormField{{Field: "steps.draft.text"}},
	}
	runGate(st, ctx, testJournal(t), RunOptions{Yes: true, Quiet: true})
	if v, _ := ctx.Get("steps.review.text"); v != "t" {
		t.Fatalf("auto-accept не материализовал поле: %v", v)
	}
}

func TestGateDecisionJournaledWithEdits(t *testing.T) {
	newStdin(t, "true\na\n")
	ctx := NewCtx(map[string]interface{}{})
	ctx.SetStep("x", map[string]interface{}{"flag": false})

	j := testJournal(t)
	st := &Step{
		ID:     "review",
		Plugin: "core/human_gate",
		Form:   []FormField{{Field: "steps.x.flag", Editable: true, Type: "boolean"}},
	}
	runGate(st, ctx, j, RunOptions{Quiet: true})

	events := readEvents(t, j.Dir)
	if countEvents(events, "gate_decision") != 1 {
		t.Fatalf("нет gate_decision: %v", events)
	}
	raw, _ := json.Marshal(events[0])
	var ev map[string]interface{}
	json.Unmarshal(raw, &ev)
	edits, _ := ev["edits"].(map[string]interface{})
	if edits["flag"] != true {
		t.Fatalf("правка не попала в журнал: %v", ev)
	}
}

func TestKindOf(t *testing.T) {
	cases := []struct {
		v    interface{}
		want string
	}{
		{true, "boolean"},
		{1.5, "number"},
		{"s", "string"},
		{[]interface{}{}, "array"},
		{map[string]interface{}{}, "object"},
		{nil, "null"},
	}
	for _, c := range cases {
		if got := kindOf(c.v); got != c.want {
			t.Fatalf("kindOf(%v) = %q, want %q", c.v, got, c.want)
		}
	}
}

func TestBasename(t *testing.T) {
	if got := basename("steps.echo.value"); got != "value" {
		t.Fatal(got)
	}
	if got := basename("value"); got != "value" {
		t.Fatal(got)
	}
}
