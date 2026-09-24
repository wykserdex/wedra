package journal

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestJsonStoreIndexesEachJournalEventOnce(t *testing.T) {
	dir := t.TempDir()
	store := NewJsonStore(dir, filepath.Join(dir, "runs.db"))
	j, err := store.Create("run-1")
	if err != nil {
		t.Fatal(err)
	}
	j.Event("run_start", map[string]interface{}{"pipeline": "demo"})
	j.Event("item_end", map[string]interface{}{"item_index": 0, "status": "ok"})
	j.Close()

	raw, err := os.ReadFile(filepath.Join(dir, "runs.db"))
	if err != nil {
		t.Fatal(err)
	}
	var db dbFile
	if err := json.Unmarshal(raw, &db); err != nil {
		t.Fatal(err)
	}
	if len(db.Events) != 2 {
		t.Fatalf("want 2 indexed events, got %d", len(db.Events))
	}
	starts := 0
	for _, event := range db.Events {
		if event.Type == "run_start" {
			starts++
		}
	}
	if starts != 1 {
		t.Fatalf("want one run_start, got %d", starts)
	}
	idx, err := store.MaxItemIndex("run-1")
	if err != nil || idx != 0 {
		t.Fatalf("MaxItemIndex=%d err=%v", idx, err)
	}
}

func TestJsonStoreMaxItemIndexIgnoresAbortedItem(t *testing.T) {
	dir := t.TempDir()
	store := NewJsonStore(dir, filepath.Join(dir, "runs.db"))
	j, err := store.Create("run-2")
	if err != nil {
		t.Fatal(err)
	}
	j.Event("item_end", map[string]interface{}{"item_index": 0, "status": "aborted"})
	j.Close()
	idx, err := store.MaxItemIndex("run-2")
	if err != nil || idx != -1 {
		t.Fatalf("MaxItemIndex=%d err=%v", idx, err)
	}
}
