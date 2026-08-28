package core

// M5, пакет №5: относительные пути из pipeline input резолвятся от ДИРЕКТОРИИ
// ПЛАГИНА (cwd subprocess), а не от корня проекта — ядро обязано предупреждать
// про нерезолвящийся file_ref до запуска, а не отдавать сырое not_found плагина.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func filerefManifest(dir string) *Manifest {
	return &Manifest{
		ID:    "file_ref_echo",
		Dir:   dir,
		Input: map[string]Port{"path": {Type: "string", Format: "file_ref"}},
	}
}

func TestFileRefWarning_RootOnlyPathGetsHint(t *testing.T) {
	pluginDir := t.TempDir()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "data"), 0o755); err != nil {
		t.Fatal(err)
	}

	w := fileRefWarnings(filerefManifest(pluginDir),
		&Step{ID: "s"}, map[string]interface{}{"path": "data"}, root)

	if len(w) != 1 {
		t.Fatalf("ожидалось ровно 1 предупреждение, got: %v", w)
	}
	if !strings.Contains(w[0], "не найден от рабочей директории плагина") {
		t.Fatalf("нет основного посыла: %s", w[0])
	}
	if !strings.Contains(w[0], "КОРНЯ ПРОЕКТА") {
		t.Fatalf("путь существует от root — должен быть хинт, что имели в виду его: %s", w[0])
	}
	if !strings.Contains(w[0], pluginDir) {
		t.Fatalf("в тексте должна быть абсолютная директория плагина: %s", w[0])
	}
}

func TestFileRefWarning_SilentWhenResolvesFromPluginDir(t *testing.T) {
	pluginDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(pluginDir, "fixtures"), 0o755); err != nil {
		t.Fatal(err)
	}
	w := fileRefWarnings(filerefManifest(pluginDir),
		&Step{ID: "s"}, map[string]interface{}{"path": "fixtures"}, t.TempDir())
	if len(w) != 0 {
		t.Fatalf("путь резолвится от cwd плагина — предупреждений быть не должно: %v", w)
	}
}

func TestFileRefWarning_AbsoluteAndEmptySkipped(t *testing.T) {
	pluginDir := t.TempDir()
	abs := filepath.Join(pluginDir, "nowhere", "nope")
	for _, v := range []string{abs, ""} {
		w := fileRefWarnings(filerefManifest(pluginDir),
			&Step{ID: "s"}, map[string]interface{}{"path": v}, t.TempDir())
		if len(w) != 0 {
			t.Fatalf("значение %q не должно предупреждать (правила: абсолютные/пустые — зона плагина): %v", v, w)
		}
	}
}

func TestFileRefWarning_NonFileRefPortsIgnored(t *testing.T) {
	pluginDir := t.TempDir()
	m := &Manifest{
		ID:  "x",
		Dir: pluginDir,
		Input: map[string]Port{
			"path": {Type: "string"},             // без format
			"dir":  {Type: "string", Format: "text"}, // чужой формат
		},
	}
	w := fileRefWarnings(m, &Step{ID: "s"},
		map[string]interface{}{"path": "no/such", "dir": "no/such"}, t.TempDir())
	if len(w) != 0 {
		t.Fatalf("порты без format: file_ref ядро не трогает: %v", w)
	}
}

func TestFileRefWarning_MissingEverywhereWarnsWithoutHint(t *testing.T) {
	pluginDir := t.TempDir()
	w := fileRefWarnings(filerefManifest(pluginDir),
		&Step{ID: "s"}, map[string]interface{}{"path": "no_such_dir_zzz"}, t.TempDir())
	if len(w) != 1 {
		t.Fatalf("ожидалось 1 предупреждение, got: %v", w)
	}
	if strings.Contains(w[0], "КОРНЯ ПРОЕКТА") {
		t.Fatalf("хинт не должен появляться, если от root тоже нет: %s", w[0])
	}
}

// E2E, ровно сценарий тестера: pipeline input с путём «как из корня»,
// плагин живёт в своей директории → в журнале рана появляется file_ref_warning,
// и при этом ран НЕ падает (предупреждение, не ошибка).
func TestFileRefWarning_EndToEndIntoJournal(t *testing.T) {
	requirePython(t)
	pf := &PipelineFile{
		FormatVersion: "0.2",
		Pipeline: Pipeline{
			Name:  "t_fileref",
			Input: map[string]interface{}{"p": "definitely_missing_everywhere_q42"},
			Steps: []Step{{
				ID: "probe", Plugin: "testdata/plugins/file_ref_echo",
				OnError: "stop", Timeout: sec(5),
				Bind: map[string]string{"path": "input.p"},
			}},
		},
	}
	stats, err := Run(pf, NewEngine(), quietOpts(t))
	if err != nil {
		t.Fatalf("ран не должен падать из-за предупреждения: %v", err)
	}
	if stats.OK != 1 {
		t.Fatalf("статы: %+v", stats)
	}
	raw, err := os.ReadFile(filepath.Join(stats.RunDir, "journal.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "file_ref_warning") {
		t.Fatalf("в журнале рана должно быть событие file_ref_warning:\n%s", raw)
	}
}
