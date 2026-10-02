package core

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Регресс: публичный корпус conformance и фикстуры внутренних тестов — это две
// копии одного и того же набора, и они уже один раз разъехались.
//
// Фикстура retry_flaky в conformance/fixtures/v0.2 осталась на варианте, который
// пишет счётчик попыток рядом с main.py, а в internal/core/testdata — на
// исправленном (счётчик вынесен за пределы каталога плагина). Причина правки
// принципиальна: после инверсии доверия запись в собственный каталог меняет хэш
// содержимого и отзывает доверие у только что отработавшего плагина. То есть
// публичный корпус — то, что получают внешние авторы плагинов — показывал
// именно тот пример, который сами разработчики признали негодным.
//
// Копии остаются (conformance/ публикуется в release-ассете как отдельный zip и
// должен читаться без репозитория), но теперь расхождение невозможно: любая
// правка обязана попасть в оба места, иначе тест красный. Симлинк здесь не
// подходит: репозиторий собирают и на Windows, где симлинки в чекауте не всегда
// разворачиваются.
func TestConformanceFixturesMatchTestdata(t *testing.T) {
	public := filepath.Join(repoRoot(t), "conformance", "fixtures", "v0.2")
	internal := filepath.Join(repoRoot(t), "internal", "core", "testdata", "plugins")

	pub := hashTree(t, public)
	inn := hashTree(t, internal)

	for name := range pub {
		if _, ok := inn[name]; !ok {
			t.Errorf("фикстура %s есть в публичном корпусе, но нет в internal/core/testdata", name)
		}
	}
	for name := range inn {
		if _, ok := pub[name]; !ok {
			t.Errorf("фикстура %s есть в internal/core/testdata, но нет в публичном корпусе", name)
		}
	}
	for name, h := range inn {
		if ph, ok := pub[name]; ok && ph != h {
			t.Errorf("содержимое фикстуры %s расходится между conformance/fixtures/v0.2 и internal/core/testdata/plugins:\n"+
				"  публичный корпус: %s\n  внутренние тесты: %s\n"+
				"правьте оба места (это одна и та же фикстура)", name, ph, h)
		}
	}
}

// hashTree возвращает отпечаток содержимого каталога фикстур:
// относительный путь → sha256 файла.
func hashTree(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		if d.IsDir() {
			// Служебные каталоги, которые появляются в процессе прогона,
			// к содержимому фикстуры не относятся.
			if d.Name() == "__pycache__" || d.Name() == ".git" {
				return fs.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") || strings.HasSuffix(d.Name(), ".pyc") {
			return nil
		}
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		sum := sha256.Sum256(raw)
		out[filepath.ToSlash(rel)] = hex.EncodeToString(sum[:12])
		return nil
	})
	if err != nil {
		t.Fatalf("обход %s: %v", root, err)
	}
	if len(out) == 0 {
		t.Fatalf("в %s не нашлось ни одной фикстуры — тест не проверил ничего", root)
	}
	return out
}

// repoRoot — корень репозитория от каталога пакета. Если его нет (пакет собран
// вне чекаута), тест обязан упасть, а не пропуститься: «пропуск неотличим от
// успеха» — ровно тот дефект, против которого этот тест и написан.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("корень репозитория (go.mod) не найден — тест синхронизации фикстур не проверил ничего")
		}
		dir = parent
	}
}
