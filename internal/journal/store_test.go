package journal

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestJsonStoreUsesJournalAsSourceOfTruth(t *testing.T) {
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
	if len(db.Events) != 0 {
		t.Fatalf("normal journal events must not trigger JSON-array rewrites: %d", len(db.Events))
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

func TestParseItemIndexRejectsInvalidValues(t *testing.T) {
	for _, value := range []interface{}{math.NaN(), math.Inf(1), math.Inf(-1), -1, 1.5, float64(1 << 63), uint(^uint(0)), "1", true} {
		if got, ok := ParseItemIndex(value); ok {
			t.Fatalf("invalid index accepted: value=%v got=%d", value, got)
		}
	}
	for _, value := range []interface{}{float64(0), float64(7), int64(8), json.Number("9")} {
		if _, ok := ParseItemIndex(value); !ok {
			t.Fatalf("valid index rejected: %v", value)
		}
	}
}
