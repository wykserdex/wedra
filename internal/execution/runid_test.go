package execution

import (
	"strings"
	"testing"
)

func TestNewRunIDIsUniqueASCIIAndReadable(t *testing.T) {
	first, err := NewRunID("Проверка входа")
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewRunID("Проверка входа")
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("run IDs must be unique")
	}
	if !strings.Contains(first, "Proverka-vhoda") {
		t.Fatalf("unexpected transliterated ID: %s", first)
	}
	for _, r := range first {
		if r > 127 {
			t.Fatalf("non-ASCII run ID: %s", first)
		}
	}
}

func TestSanitizeEmptyAndPunctuation(t *testing.T) {
	if got := Sanitize("  ***  "); got != "run" {
		t.Fatalf("got %q", got)
	}
}
