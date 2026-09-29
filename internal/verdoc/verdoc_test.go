package verdoc

// Тесты проверки версий. Ключевой сценарий — расхождение: именно его эта
// проверка и поймала (VERSION 0.33a против 0.32c в README), поэтому тест без
// расхождения ничего не проверял бы.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// repoFixture — синтетический репозиторий, повторяющий формы, в которых
// версия и минимум Go названы по-настоящему. Собирается на каждый тест, чтобы
// правки одного случая не ломали другой.
type repoFixture struct {
	dir     string
	version string
}

func newRepo(t *testing.T, version string) *repoFixture {
	t.Helper()
	dir := t.TempDir()
	r := &repoFixture{dir: dir, version: version}
	r.write(t, "VERSION", version+"\n")
	r.write(t, "go.mod", "module wedra\n\ngo 1.26\n")
	r.write(t, "README.md", strings.Join([]string{
		"# WEDRA",
		"",
		"Требования: Go 1.26+ нужен только для сборки из исходников.",
		"",
		"## Что умеет",
		"",
		"| Возможность | Как выглядит |",
		"|---|---|",
		"| Агенты (MCP) | 8 инструментов: `list_plugins`, `describe_plugin`, `validate_pipeline`, `plan_pipeline`, `run_pipeline`, `get_run`, `cancel_run`, `exec_plugin` |",
		"",
		"## Релизы",
		"",
		"Тег релиза совпадает с [`VERSION`](VERSION) — сейчас `" + version + "`;",
		"protocol version — [`protocol/VERSION`](protocol/VERSION) (`0.2`).",
		"",
	}, "\n"))
	r.write(t, "README.en.md", strings.Join([]string{
		"# WEDRA",
		"",
		"## Current contract",
		"",
		"- Product version: `" + version + "` from [`VERSION`](VERSION)",
		"- Pipeline/plugin protocol: `0.2` from [`protocol/VERSION`](protocol/VERSION)",
		"",
		"## Build and run",
		"",
		"Building from source needs Go 1.26 or newer.",
		"",
	}, "\n"))
	r.write(t, "CONTRIBUTING.md", strings.Join([]string{
		"# Contributing",
		"",
		"1. Use Go 1.26 or newer and Python 3 for plugin fixtures.",
		"",
	}, "\n"))
	r.write(t, "docs/versioning.md", strings.Join([]string{
		"# Версионирование WEDRA",
		"",
		"| Ось | Источник | Текущее значение |",
		"|---|---|---|",
		"| Product / application | `VERSION` | `" + version + "` |",
		"| Pipeline and plugin protocol | `protocol/VERSION` | `0.2` |",
		"",
	}, "\n"))
	r.write(t, "docs/architecture.md", strings.Join([]string{
		"# Архитектура",
		"",
		"## Версии",
		"",
		"- `VERSION` (корень) — единственная версия приложения; текущий релиз —",
		"  `" + version + "`, а release tag обязан совпадать с `VERSION`.",
		"",
	}, "\n"))
	r.write(t, mcpServerFile, mcpSource)
	return r
}

// mcpSource — настоящая форма callTool: восемь инструментов, восьмой за
// opt-in флагом. Ровно та же разметка, что в internal/mcp/server.go.
const mcpSource = `package mcp

type Server struct{ trust bool }

func (s *Server) callTool(name string) (string, bool) {
	switch name {
	case "list_plugins":
		return "", false
	case "describe_plugin":
		return "", false
	case "validate_pipeline":
		return "", false
	case "plan_pipeline":
		return "", false
	case "run_pipeline":
		return "", false
	case "get_run":
		return "", false
	case "cancel_run":
		return "", false
	case "exec_plugin":
		return "", false
	default:
		return "", true
	}
}
`

func (r *repoFixture) write(t *testing.T, rel, body string) {
	t.Helper()
	path := filepath.Join(r.dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (r *repoFixture) read(t *testing.T, rel string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(r.dir, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func (r *repoFixture) patch(t *testing.T, rel, old, new string) {
	t.Helper()
	body := r.read(t, rel)
	if !strings.Contains(body, old) {
		t.Fatalf("в %s нет фрагмента %q", rel, old)
	}
	r.write(t, rel, strings.Replace(body, old, new, 1))
}

// Главный сценарий: сходится.
func TestCheckRepoPassesOnConsistentRepo(t *testing.T) {
	r := newRepo(t, "0.33a")
	out, err := CheckRepo(r.dir)
	if err != nil {
		t.Fatalf("согласованный репозиторий не прошёл: %v\n%s", err, out)
	}
	if !strings.Contains(out, "0.33a") {
		t.Fatalf("в сводке нет версии: %q", out)
	}
}

// Реальный репозиторий обязан проходить: иначе проверка зелёная на синтетике
// и красная на том, ради чего написана.
func TestCheckRepoPassesOnRealRepository(t *testing.T) {
	repo, err := findRepoRoot()
	if err != nil {
		t.Skipf("репозиторий не найден: %v", err)
	}
	out, err := CheckRepo(repo)
	if err != nil {
		t.Fatalf("настоящий репозиторий не прошёл проверку версий: %v\n%s", err, out)
	}
}

// Расхождение, которое проверка и ловила: VERSION 0.33a, README 0.32c.
func TestCheckRepoFailsOnReadmeVersionDrift(t *testing.T) {
	r := newRepo(t, "0.33a")
	r.patch(t, "README.md", "сейчас `0.33a`", "сейчас `0.32c`")
	r.patch(t, "README.en.md", "Product version: `0.33a`", "Product version: `0.32c`")

	out, err := CheckRepo(r.dir)
	if err == nil {
		t.Fatalf("расхождение версий не поймано:\n%s", out)
	}
	if !strings.Contains(out, "0.32c") || !strings.Contains(out, "0.33a") {
		t.Fatalf("в отчёте нет ни одной из версий:\n%s", out)
	}
	for _, rel := range []string{"README.md", "README.en.md"} {
		if !strings.Contains(out, rel) {
			t.Fatalf("%s не назван в отчёте:\n%s", rel, out)
		}
	}
}

// Расхождение в docs/ — того же класса, что и в README.
func TestCheckRepoFailsOnDocsVersionDrift(t *testing.T) {
	r := newRepo(t, "0.33a")
	r.patch(t, "docs/versioning.md", "| `VERSION` | `0.33a` |", "| `VERSION` | `0.32c` |")
	r.patch(t, "docs/architecture.md", "  `0.33a`,", "  `0.32c`,")

	out, err := CheckRepo(r.dir)
	if err == nil {
		t.Fatalf("расхождение в docs не поймано:\n%s", out)
	}
	if !strings.Contains(out, "docs/versioning.md") || !strings.Contains(out, "docs/architecture.md") {
		t.Fatalf("оба документа должны быть названы:\n%s", out)
	}
}

// Примеры в правилах нумерации — не расхождение: `0.32a`/`0.32b` в том же
// блоке про VERSION обязаны оставаться в покое.
func TestCheckRepoIgnoresNumberingExamples(t *testing.T) {
	r := newRepo(t, "0.33a")
	r.patch(t, "docs/versioning.md", "| `VERSION` | `0.33a` |",
		"| `VERSION` | `0.33a` | инкременты `0.32a`, `0.32b`, `0.32c`; тег `v0.32a` |")
	if out, err := CheckRepo(r.dir); err != nil {
		t.Fatalf("примеры нумерации приняты за расхождение: %v\n%s", err, out)
	}
}

// Минимум Go: README обещает 1.22, go.mod требует 1.26.
func TestCheckRepoFailsOnGoFloorDrift(t *testing.T) {
	r := newRepo(t, "0.33a")
	r.patch(t, "README.md", "Go 1.26+", "Go 1.22+")
	r.patch(t, "README.en.md", "Go 1.26 or newer", "Go 1.22 or newer")
	r.patch(t, "CONTRIBUTING.md", "Go 1.26 or newer", "Go 1.22 or newer")

	out, err := CheckRepo(r.dir)
	if err == nil {
		t.Fatalf("расхождение минимума Go не поймано:\n%s", out)
	}
	if !strings.Contains(out, "go.mod требует go 1.26") {
		t.Fatalf("в отчёте нет обещания go.mod:\n%s", out)
	}
}

// Отсутствие директивы `go` — поломка проверки, а не успех: иначе «зелёный CI"
// означал бы «проверка не нашла чего сравнивать».
func TestCheckRepoFailsWhenGoDirectiveMissing(t *testing.T) {
	r := newRepo(t, "0.33a")
	r.write(t, "go.mod", "module wedra\n")
	out, err := CheckRepo(r.dir)
	if err == nil || !strings.Contains(err.Error(), "директива `go X.Y`") {
		t.Fatalf("пустой go.mod обязан ломать проверку, а не проходить её: %v\n%s", err, out)
	}
}

// Число инструментов MCP в README против dispatch-таблицы callTool.
func TestCheckRepoFailsOnMCPToolCountDrift(t *testing.T) {
	r := newRepo(t, "0.33a")
	// Та самая ошибка, что была в репозитории: восьмой инструмент принимается,
	// но не назван.
	r.patch(t, "README.md", ", `cancel_run`, `exec_plugin` |", ", `cancel_run` |")

	out, err := CheckRepo(r.dir)
	if err == nil {
		t.Fatalf("не названный инструмент не пойман:\n%s", out)
	}
	if !strings.Contains(out, "exec_plugin") {
		t.Fatalf("в отчёте нет имени инструмента:\n%s", out)
	}
}

// Лишний инструмент в README — тоже расхождение: README обещает то, чего
// вызов не принимает.
func TestCheckRepoFailsOnUnknownReadmeTool(t *testing.T) {
	r := newRepo(t, "0.33a")
	r.patch(t, "README.md", "`cancel_run`", "`cancel_run`, `teleport_run`")

	out, err := CheckRepo(r.dir)
	if err == nil {
		t.Fatalf("инструмент, которого нет в callTool, не пойман:\n%s", out)
	}
	if !strings.Contains(out, "teleport_run") {
		t.Fatalf("в отчёте нет имени инструмента:\n%s", out)
	}
}

// Строка таблицы MCP исчезла — проверка обязана это заметить, а не молча
// признать, что сверять нечего.
func TestCheckRepoFailsWhenMCPRowRemoved(t *testing.T) {
	r := newRepo(t, "0.33a")
	r.patch(t, "README.md", "| Агенты (MCP) | 8 инструментов: `list_plugins`, `describe_plugin`, `validate_pipeline`, `plan_pipeline`, `run_pipeline`, `get_run`, `cancel_run`, `exec_plugin` |\n", "")

	out, err := CheckRepo(r.dir)
	if err == nil || !strings.Contains(err.Error(), "сверять нечего") {
		t.Fatalf("удалённая строка таблицы обязана ломать проверку: %v\n%s", err, out)
	}
}

// callTool исчез или потерял switch — проверка не должна превращаться в
// вакуумно-зелёную.
func TestCheckRepoFailsWhenDispatchTableEmpty(t *testing.T) {
	r := newRepo(t, "0.33a")
	r.write(t, mcpServerFile, "package mcp\n\ntype Server struct{}\n")

	out, err := CheckRepo(r.dir)
	if err == nil || !strings.Contains(err.Error(), "проверка сломана") {
		t.Fatalf("пустая dispatch-таблица обязана ломать проверку: %v\n%s", err, out)
	}
}

// findRepoRoot — корень репозитория по go.mod вверх от рабочего каталога.
func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}
