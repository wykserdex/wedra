package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestJournalEventsAndSnapshot(t *testing.T) {
	dir := t.TempDir()
	j, err := NewJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()

	j.Event("run_start", map[string]interface{}{"pipeline": "t"})
	j.Event("step_end", map[string]interface{}{"step": "a", "exit_code": 0})

	ctx := NewCtx(map[string]interface{}{"value": "hello"})
	ctx.SetStep("a", map[string]interface{}{"mx": []interface{}{"mx.yandex.ru"}})
	j.Snapshot(ctx)

	events := readEvents(t, dir)
	if len(events) != 2 || events[0]["type"] != "run_start" || events[1]["type"] != "step_end" {
		t.Fatalf("события журнала: %v", events)
	}
	if _, ok := events[0]["ts"]; !ok {
		t.Fatal("у события нет timestamp")
	}

	snap, err := os.ReadFile(filepath.Join(dir, "context.json"))
	if err != nil {
		t.Fatal(err)
	}
	// снапшот обязан быть валидным JSON с нашей структурой
	var parsed map[string]interface{}
	if err := json.Unmarshal(snap, &parsed); err != nil {
		t.Fatalf("context.json не JSON: %v", err)
	}
	steps, ok := parsed["steps"].(map[string]interface{})
	if !ok {
		t.Fatal("в снапшоте нет steps")
	}
	if _, ok := steps["a"]; !ok {
		t.Fatal("в снапшоте нет неймспейса шага a")
	}
}
