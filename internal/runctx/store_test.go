package runctx

import (
	"encoding/json"
	"errors"
	"testing"
)

func mustDecode(t *testing.T, raw string) map[string]interface{} {
	t.Helper()
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		t.Fatal(err)
	}
	return data
}

// F-01: валидный контек.json проходит проверку формы без изменений.
func TestNormalizeKeepsValidContext(t *testing.T) {
	data := mustDecode(t, `{"input":{"item":"x"},"steps":{"a":{"f":1}}}`)
	got, err := Normalize(data)
	if err != nil {
		t.Fatalf("валидный контекст отвергнут: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("состав контекста изменился: %v", got)
	}
	steps, ok := got["steps"].(map[string]interface{})
	if !ok {
		t.Fatalf("steps не объект: %#v", got["steps"])
	}
	step, ok := steps["a"].(map[string]interface{})
	if !ok || step["f"] != 1.0 {
		t.Fatalf("данные шага потеряны: %#v", steps)
	}
}

// F-01: nil допускается и нормализуется (отсутствующий input/steps).
func TestNormalizeFillsMissingNamespaces(t *testing.T) {
	cases := []string{`{}`, `null`, `{"input":null,"steps":null}`, `{"steps":{"a":1}}`}
	for _, raw := range cases {
		data := mustDecode(t, raw)
		got, err := Normalize(data)
		if err != nil {
			t.Fatalf("%s: ожидалась нормализация, получено %v", raw, err)
		}
		if got == nil {
			t.Fatalf("%s: карта контекста не создана", raw)
		}
		for _, ns := range []string{"input", "steps"} {
			if _, ok := got[ns].(map[string]interface{}); !ok {
				t.Fatalf("%s: %s не объект после нормализации: %#v", raw, ns, got[ns])
			}
		}
	}
}

// F-01: steps: 5 / input: [] — типизированная ошибка, не паника.
func TestNormalizeRejectsForeignShape(t *testing.T) {
	cases := []struct {
		raw   string
		field string
		kind  string
	}{
		{`{"steps":5}`, "steps", "number"},
		{`{"input":5,"steps":{}}`, "input", "number"},
		{`{"steps":[]}`, "steps", "array"},
		{`{"input":"nope"}`, "input", "string"},
		{`{"steps":true}`, "steps", "boolean"},
	}
	for _, tc := range cases {
		_, err := Normalize(mustDecode(t, tc.raw))
		if err == nil {
			t.Fatalf("%s: битая форма принята", tc.raw)
		}
		var shape *ShapeError
		if !errors.As(err, &shape) {
			t.Fatalf("%s: ожидался *ShapeError, получено %T (%v)", tc.raw, err, err)
		}
		if shape.Field != tc.field {
			t.Fatalf("%s: поле %q, ожидалось %q", tc.raw, shape.Field, tc.field)
		}
		if kind := jsonKind(shape.Got); kind != tc.kind {
			t.Fatalf("%s: тип %q, ожидался %q", tc.raw, kind, tc.kind)
		}
	}
}

// F-01: accessors не паникуют на битом/чужом контексте.
func TestAccessorsDoNotPanicOnForeignShape(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("accessors упали на битом контексте: %v", r)
		}
	}()
	c := &Ctx{Data: map[string]interface{}{"input": 5, "steps": 5}}
	c.SetInput("item", "x")
	c.SetStep("a", map[string]interface{}{"f": 1.0})
	c.ResetSteps()
	c.SetInput("item", "y")
	c.SetStep("a", map[string]interface{}{"f": 2.0})
	if v, ok := c.Get("input.item"); !ok || v != "y" {
		t.Fatalf("SetInput не восстановил input: %v %v", v, ok)
	}
	if v, ok := c.Get("steps.a.f"); !ok || v != 2.0 {
		t.Fatalf("SetStep не восстановил steps: %v %v", v, ok)
	}
	if len(c.Input()) != 1 || len(c.Steps()) != 1 {
		t.Fatalf("accessors вернули не ту форму: input=%v steps=%v", c.Input(), c.Steps())
	}
}

// F-01: пустой/отсутствующий Data и nil-контекст — без паники.
func TestAccessorsDoNotPanicOnEmptyContext(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("accessors упали на пустом контексте: %v", r)
		}
	}()
	var nilCtx *Ctx
	nilCtx.SetInput("item", "x")
	nilCtx.SetStep("a", map[string]interface{}{})
	nilCtx.ResetSteps()
	if _, ok := nilCtx.Get("input.item"); ok {
		t.Fatal("nil-контекст не должен резолвить пути")
	}
	if nilCtx.Steps() != nil {
		t.Fatal("nil-контекст не должен выдавать карту")
	}

	c := &Ctx{}
	c.SetInput("item", "x")
	c.SetStep("a", map[string]interface{}{"f": 1.0})
	if v, ok := c.Get("steps.a.f"); !ok || v != 1.0 {
		t.Fatalf("пустой Data должен самовосстановиться: %v %v", v, ok)
	}
}
