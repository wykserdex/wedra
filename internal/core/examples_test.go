package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"wedra/internal/pipeline"
)

// Примеры из examples/ — документация, которую копируют. Дрейф между ними и
// рантаймом стоил реальных правок: пустое поле network стало означать deny, и
// шесть примеров с сетевыми плагинами перестали запускаться, при том что ни
// один тест их не проверял. Этот тест — именно та проверка, которой не было.
func TestExamplesAreValid(t *testing.T) {
	// Пути плагинов в примерах относительные, а тест живёт в internal/core.
	// Модуль на go1.22, поэтому t.Chdir недоступен — меняем каталог руками.
	// Тесты пакета не параллелятся (t.Parallel не используется), так что это
	// безопасно.
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(filepath.Join("..", "..")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })

	entries, err := os.ReadDir("examples")
	if err != nil {
		t.Fatalf("examples недоступны: %v", err)
	}
	eng := NewEngine()
	checked := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		path := filepath.Join("examples", e.Name())
		pf, err := pipeline.LoadPipelineFile(path)
		if err != nil {
			t.Errorf("%s: не читается: %v", e.Name(), err)
			continue
		}
		checked++
		for _, is := range pipeline.ValidateIssues(pf, eng) {
			if is.Severity == "error" {
				t.Errorf("%s: %s: %s (подсказка: %s)", e.Name(), is.Code, is.Message, is.Hint)
			}
		}
	}
	if checked == 0 {
		t.Fatal("не найдено ни одного примера — тест ничего не проверяет")
	}
	t.Logf("проверено примеров: %d", checked)
}
