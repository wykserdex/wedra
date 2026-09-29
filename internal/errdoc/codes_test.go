package errdoc

// Тесты сверки кодов.
//
// Главный тест здесь последний: он проверяет настоящий репозиторий. Остальные
// работают на фикстурах, потому что показывают, что проверка ЛОВИТ расхождения —
// иначе она может быть зелёной просто потому, что ничего не нашла.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixtureRepo собирает минимальный репозиторий: объявления кодов, документ и
// пару файлов-эмиттеров.
type fixture struct {
	dir      string
	issueSrc string
	docSrc   string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dir := t.TempDir()
	for _, p := range []string{
		filepath.Join("internal", "pipeline"),
		filepath.Join("internal", "api"),
		filepath.Join("protocol", "v0.2"),
	} {
		if err := os.MkdirAll(filepath.Join(dir, p), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	f := &fixture{dir: dir}
	f.writeIssue("")
	f.writeDoc("| E_ALPHA | пример |")
	return f
}

func (f *fixture) writeIssue(src string) {
	f.issueSrc = src
	body := "package pipeline\n\nconst (\n" + src + ")\n"
	mustWrite(filepath.Join(f.dir, filepath.FromSlash(issueFile)), body)
}

func (f *fixture) writeDoc(rows string) {
	f.docSrc = "## Коды\n\n| Код | Когда |\n|---|---|\n" + rows + "\n"
	mustWrite(filepath.Join(f.dir, filepath.FromSlash(errorsDoc)), f.docSrc)
}

func (f *fixture) emit(name, body string) {
	mustWrite(filepath.Join(f.dir, "internal", "api", name), body)
}

func mustWrite(path, body string) {
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		panic(err)
	}
}

// Код используется и описан — норма.
func TestConsistentFixturePasses(t *testing.T) {
	f := newFixture(t)
	f.writeIssue("\tE_ALPHA = \"E_ALPHA\"\n")
	f.emit("a.go", "package api\n\nvar code = E_ALPHA\n")
	if _, err := CheckRepo(f.dir); err != nil {
		t.Fatalf("расхождений нет, а проверка ругается: %v", err)
	}
}

// Главный случай: код выдаётся, но в контракте его нет. Агент получает его и
// не находит в ERRORS.md.
func TestUndocumentedCodeIsReported(t *testing.T) {
	f := newFixture(t)
	f.writeIssue("\tE_ALPHA = \"E_ALPHA\"\n\tE_BETA = \"E_BETA\"\n")
	// E_ALPHA используется и описан — он чист и в отчёте появляться не должен.
	// (В первой версии фикстуры E_ALPHA был описан, но не использовался, и
	// попадал в «зарезервированные» — тест проверял не то.)
	f.emit("a.go", "package api\n\nvar one = E_BETA\n\nvar two = E_ALPHA\n")
	out, err := CheckRepo(f.dir)
	if err == nil {
		t.Fatal("недокументированный код должен провалить проверку")
	}
	if !strings.Contains(out, "E_BETA") {
		t.Errorf("в отчёте нет E_BETA:\n%s", out)
	}
	if strings.Contains(out, "E_ALPHA") {
		t.Errorf("E_ALPHA задран зря, он и описан, и используется:\n%s", out)
	}
}

// Обратная дыра: контракт обещает код, которого нет нигде.
func TestDocumentedButNeverEmittedIsReported(t *testing.T) {
	f := newFixture(t)
	f.writeIssue("\tE_ALPHA = \"E_ALPHA\"\n")
	f.writeDoc("| E_ALPHA | пример |\n| E_GHOST | обещан, но не выдаётся |")
	f.emit("a.go", "package api\n\nvar code = E_ALPHA\n")
	out, err := CheckRepo(f.dir)
	if err == nil {
		t.Fatal("обещанный, но не выдаваемый код должен провалить проверку")
	}
	if !strings.Contains(out, "E_GHOST") {
		t.Errorf("в отчёте нет E_GHOST:\n%s", out)
	}
}

// Мёртвая константа: объявлена, не описана, не используется. Это rot, который
// должен ловиться, иначе константы копятся вечно.
func TestDeadConstantIsReported(t *testing.T) {
	f := newFixture(t)
	f.writeIssue("\tE_ALPHA = \"E_ALPHA\"\n\tE_DEAD = \"E_DEAD\"\n")
	f.emit("a.go", "package api\n\nvar code = E_ALPHA\n")
	out, err := CheckRepo(f.dir)
	if err == nil {
		t.Fatal("мёртвая константа должна провалить проверку")
	}
	if !strings.Contains(out, "E_DEAD") {
		t.Errorf("в отчёте нет E_DEAD:\n%s", out)
	}
}

// Глоб в документе покрывает семейство. Иначе семь зарезервированных
// E_MANIFEST_* выглядели бы как семь мёртвых констант — и проверка вводила бы
// в заблуждение ровно там, где хочет помочь.
func TestGlobDocumentsWholeFamily(t *testing.T) {
	f := newFixture(t)
	f.writeIssue("\tE_ALPHA = \"E_ALPHA\"\n\tE_FAM_A = \"E_FAM_A\"\n\tE_FAM_B = \"E_FAM_B\"\n")
	f.writeDoc("| E_ALPHA | пример |\n| E_FAM_* | семейство зарезервировано |")
	f.emit("a.go", "package api\n\nvar code = E_ALPHA\n")
	out, err := CheckRepo(f.dir)
	if err != nil {
		t.Fatalf("зарезервированное семейство не поломка, а проверка ругается: %v\n%s", err, out)
	}
	if !strings.Contains(out, "E_FAM_A") || !strings.Contains(out, "зарезервировано") {
		t.Errorf("семейство должно быть показано как зарезервированное:\n%s", out)
	}
}

// ГЛАВНАЯ ЗАЩИТА ОТ ВАКУУМНО-ЗЕЛЁНОЙ ПРОВЕРКИ. Если разбор сломан и нашёл
// ноль кодов, зелёный результат не значит ничего. Поэтому пустой репозиторий
// обязан давать ошибку, а не «успех».
func TestEmptyRepoIsBrokenNotGreen(t *testing.T) {
	dir := t.TempDir()
	for _, p := range []string{
		filepath.Join("internal", "pipeline"),
		filepath.Join("protocol", "v0.2"),
	} {
		if err := os.MkdirAll(filepath.Join(dir, p), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite(filepath.Join(dir, filepath.FromSlash(issueFile)), "package pipeline\n")
	mustWrite(filepath.Join(dir, filepath.FromSlash(errorsDoc)), "ничего\n")
	if _, err := CheckRepo(dir); err == nil {
		t.Fatal("разбор нашёл ноль кодов — это поломка проверки, а не успех")
	}
}

// Объявление константы не считается её использованием. Иначе каждая константа
// «использована» сама собой и проверка мёртвых не срабатывает никогда —
// именно такой баг здесь и был в первой версии.
func TestDeclarationIsNotAUsage(t *testing.T) {
	f := newFixture(t)
	f.writeIssue("\tE_DEAD = \"E_DEAD\"\n")
	f.writeDoc("| что-то | без кодов |")
	_, err := CheckRepo(f.dir)
	if err == nil {
		t.Fatal("константа, на которую ссылается только её объявление, должна считаться мёртвой")
	}
}

// Упоминание кода в комментарии — не использование. Иначе ссылка на код в
// пояснении делала бы мёртвую константу живой, и проверка обесценивалась бы
// по мере того, как файлы покрываются комментариями.
func TestCommentMentionIsNotAUsage(t *testing.T) {
	f := newFixture(t)
	f.writeIssue("\tE_DEAD = \"E_DEAD\"\n")
	f.writeDoc("| что-то | без кодов |")
	f.emit("a.go", "package api\n\n// E_DEAD упоминается только здесь, в комментарии.\nvar x = 1\n")
	_, err := CheckRepo(f.dir)
	if err == nil {
		t.Fatal("упоминание в комментарии не должно считаться использованием")
	}
}

// Код ВНУТРИ строки тоже считается: fmt.Errorf с префиксом кода либо сырой
// JSON с полем code. Без этого такие коды выглядели бы невыдающимися.
func TestCodeInsideStringCounts(t *testing.T) {
	f := newFixture(t)
	f.writeIssue("\tE_ALPHA = \"E_ALPHA\"\n")
	f.writeDoc("| E_ALPHA | пример |\n| E_BUSY | MCP |")
	f.emit("a.go", "package api\n\nvar raw = `{\"error\":\"занято\",\"code\":\"E_BUSY\"}`\n")
	out, err := CheckRepo(f.dir)
	if err != nil {
		t.Fatalf("код внутри сырого JSON — это выдача, проверка не должна ругаться: %v\n%s", err, out)
	}
}

// Настоящий репозиторий должен сходиться. Если этот тест падает, значит либо
// появилось расхождение (и это правда), либо сломалась проверка (и это тоже
// правда, но чинить надо её).
func TestCheckRepoOnRealRepository(t *testing.T) {
	repo, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	out, err := CheckRepo(repo)
	if err != nil {
		t.Fatalf("коды в этом репозитории разошлись с контрактом:\n%s", out)
	}
	if !strings.Contains(out, "в коде") {
		t.Errorf("отчёт пуст — проверка, скорее всего, ничего не нашла:\n%s", out)
	}
}

func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}
