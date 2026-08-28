package core

import "testing"

func TestCtxGetSet(t *testing.T) {
	c := NewCtx(map[string]interface{}{"value": "hello", "nested": map[string]interface{}{"x": 1.0}})

	if v, ok := c.Get("input.value"); !ok || v != "hello" {
		t.Fatalf("input.value: got %v ok=%v", v, ok)
	}
	if v, ok := c.Get("input.nested.x"); !ok || v != 1.0 {
		t.Fatalf("input.nested.x: got %v ok=%v", v, ok)
	}
	if _, ok := c.Get("input.missing"); ok {
		t.Fatal("input.missing должен отсутствовать")
	}
	if _, ok := c.Get("input.value.deep"); ok {
		t.Fatal("путь сквозь скаляр должен не резолвиться")
	}

	c.SetStep("a", map[string]interface{}{"f": "v"})
	if v, ok := c.Get("steps.a.f"); !ok || v != "v" {
		t.Fatalf("steps.a.f: got %v ok=%v", v, ok)
	}
}

func TestCtxNamespacesPerStep(t *testing.T) {
	c := NewCtx(nil)
	c.SetStep("one", map[string]interface{}{"mx": "a"})
	c.SetStep("two", map[string]interface{}{"mx": "b"})

	v1, _ := c.Get("steps.one.mx")
	v2, _ := c.Get("steps.two.mx")
	if v1 != "a" || v2 != "b" {
		t.Fatalf("неймспейсы шагов пересеклись: %v / %v", v1, v2)
	}
}

func TestCtxResetSteps(t *testing.T) {
	c := NewCtx(map[string]interface{}{"item": "x"})
	c.SetStep("a", map[string]interface{}{"f": 1.0})
	c.ResetSteps()
	if _, ok := c.Get("steps.a.f"); ok {
		t.Fatal("после ResetSteps steps обязаны быть пусты")
	}
	if v, _ := c.Get("input.item"); v != "x" {
		t.Fatal("ResetSteps не должен трогать input")
	}
}
