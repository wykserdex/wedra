package journal

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
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

func TestValidateRunID(t *testing.T) {
	for _, runID := range []string{"run-1", "RUN_42", "a", strings.Repeat("a", 160)} {
		if err := ValidateRunID(runID); err != nil {
			t.Errorf("valid run_id %q rejected: %v", runID, err)
		}
	}
	for _, runID := range []string{
		"", ".", "..", "../outside", `..\outside`, "run/child", `run\child`,
		"/absolute", `\absolute`, "run id", "run.id", "é", strings.Repeat("a", 161),
	} {
		if err := ValidateRunID(runID); err == nil {
			t.Errorf("invalid run_id accepted: %q", runID)
		}
	}
}

func TestSafeRunDirRejectsSymlink(t *testing.T) {
	base := t.TempDir()
	outside := t.TempDir()
	target := filepath.Join(outside, "secret")
	if err := os.WriteFile(target, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "linked-run")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if got, err := SafeRunDir(base, "linked-run"); err == nil {
		t.Fatalf("SafeRunDir accepted symlink: %q", got)
	}
}

func TestSafeRunDirAllowsMissingRoot(t *testing.T) {
	base := filepath.Join(t.TempDir(), "not-created", "runs")
	if got, err := SafeRunDir(base, "safe-run"); err != nil || got == "" {
		t.Fatalf("SafeRunDir missing root: %q err=%v", got, err)
	}
}

func TestSafeRunDir(t *testing.T) {
	base := t.TempDir()
	root, err := filepath.Abs(base)
	if err != nil {
		t.Fatal(err)
	}
	got, err := SafeRunDir(base, "safe-run")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "safe-run")
	if got != want {
		t.Fatalf("SafeRunDir=%q, want %q", got, want)
	}
	for _, runID := range []string{"../outside", `..\outside`, "safe/child", `safe\child`, "safe/../outside", `safe\..\outside`} {
		got, err := SafeRunDir(base, runID)
		if err == nil {
			t.Errorf("SafeRunDir accepted %q: %q", runID, got)
		}
		if got != "" {
			t.Errorf("SafeRunDir returned %q for rejected %q", got, runID)
		}
	}
}

// Каталог прогонов по умолчанию выбирается по одному правилу everywhere: раньше
// `pipeline run` при отсутствии var/runs писал в runs/, а `runs list/show` искали
// строго var/runs — и только что записанный ран становился невидимым.
func TestDefaultRunsDirAt(t *testing.T) {
	t.Run("var/runs есть — выбирается он", func(t *testing.T) {
		base := t.TempDir()
		if err := os.MkdirAll(filepath.Join(base, "var", "runs"), 0o755); err != nil {
			t.Fatal(err)
		}
		if got, want := DefaultRunsDirAt(base), filepath.Join(base, "var", "runs"); got != want {
			t.Fatalf("DefaultRunsDirAt = %q, want %q", got, want)
		}
	})

	t.Run("var/runs нет — фолбэк на runs", func(t *testing.T) {
		base := t.TempDir()
		if err := os.MkdirAll(filepath.Join(base, "runs"), 0o755); err != nil {
			t.Fatal(err)
		}
		if got, want := DefaultRunsDirAt(base), filepath.Join(base, "runs"); got != want {
			t.Fatalf("DefaultRunsDirAt = %q, want %q", got, want)
		}
	})

	t.Run("пусто на диске — фолбэк всё равно runs", func(t *testing.T) {
		base := t.TempDir()
		if got, want := DefaultRunsDirAt(base), filepath.Join(base, "runs"); got != want {
			t.Fatalf("DefaultRunsDirAt = %q, want %q", got, want)
		}
	})

	t.Run("var/runs — файл, а не каталог", func(t *testing.T) {
		base := t.TempDir()
		if err := os.MkdirAll(filepath.Join(base, "var"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(base, "var", "runs"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if got := DefaultRunsDirAt(base); got != filepath.Join(base, "runs") {
			t.Fatalf("DefaultRunsDirAt = %q: не-каталог не должен выигрывать", got)
		}
	})
}

// Хранилище без явного baseDir обязано писать туда же, куда смотрит чтение:
// иначе прогон, созданный ядром, не находится командой чтения.
func TestStoreDefaultBaseDirMatchesReader(t *testing.T) {
	base := t.TempDir()
	if err := os.MkdirAll(filepath.Join(base, "var", "runs"), 0o755); err != nil {
		t.Fatal(err)
	}
	restore := chdirTemp(t, base)
	defer restore()

	if got, want := NewFilesystemStore("").BaseDir, DefaultRunsDir(); got != want {
		t.Fatalf("FilesystemStore.BaseDir = %q, want %q", got, want)
	}
	js := NewJsonStore("", "")
	if got, want := js.BaseDir, DefaultRunsDir(); got != want {
		t.Fatalf("JsonStore.BaseDir = %q, want %q", got, want)
	}
	if got, want := js.DBPath, filepath.Join(DefaultRunsDir(), "runs.db"); got != want {
		t.Fatalf("JsonStore.DBPath = %q, want %q", got, want)
	}
	if _, err := SafeRunDir("", "run-x"); err != nil {
		t.Fatalf("SafeRunDir(\"\") вернул ошибку на пустом baseDir: %v", err)
	}
}

// chdirTemp переводит процесс во временный каталог на время теста.
func chdirTemp(t *testing.T, dir string) func() {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	return func() {
		if err := os.Chdir(old); err != nil {
			t.Fatalf("вернуть рабочий каталог: %v", err)
		}
	}
}
