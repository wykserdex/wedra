package core

// M5, тестер №3 (круг 2): «results как array + format: json» проходил plugin
// validate и взрывался только на pipeline validate чужим сообщением про
// «формат источника ""». Теперь манифест отклоняет такое на месте.

import (
	"strings"
	"testing"
)

func TestManifest_FormatOnArrayRejected(t *testing.T) {
	// дословно кейс тестера: результаты Maigret-стайл чекера
	m := &Manifest{
		ID:     "osint_result_sorter",
		Input:  map[string]Port{"results": {Type: "array", Format: "json"}},
		Output: map[string]Port{"by_status": {Type: "object"}},
	}
	var errs []string
	errs = checkPortFormats("input", "results", m.Input["results"], errs)
	if len(errs) != 1 {
		t.Fatalf("ожидалась 1 ошибка, got: %v", errs)
	}
	for _, want := range []string{"json", "array", "type: string", "уберите format"} {
		if !strings.Contains(errs[0], want) {
			t.Fatalf("ошибка должна объяснять и лечить (нет %q): %s", want, errs[0])
		}
	}
}

func TestManifest_UnknownFormatRejected(t *testing.T) {
	errs := checkPortFormats("input", "x", Port{Type: "string", Format: "xml"}, nil)
	if len(errs) != 1 || !strings.Contains(errs[0], "file_ref") {
		t.Fatalf("неизвестный формат должен перечислять допустимые: %v", errs)
	}
}

func TestManifest_LegalFormatsPass(t *testing.T) {
	for _, p := range []Port{
		{Type: "string", Format: "email"},
		{Type: "string", Format: "file_ref"},
		{Type: "string"},                      // без формата
		{Type: "array"},                       // массив без формата — легально
		{Type: "object"},
		{Type: "number"},
	} {
		if errs := checkPortFormats("input", "x", p, nil); len(errs) != 0 {
			t.Fatalf("легальный порт %+v не должен ругаться: %v", p, errs)
		}
	}
}

func TestManifest_FormatCheckedOnOutputsToo(t *testing.T) {
	errs := checkPortFormats("output", "res", Port{Type: "array", Format: "json"}, nil)
	if len(errs) != 1 || !strings.Contains(errs[0], "output res") {
		t.Fatalf("выходы проверяются так же, с ярлыком output: %v", errs)
	}
}
