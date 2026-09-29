package main

// Генератор встроенного allow-list (internal/plugin/trustseed_gen.go).
//
// Запуск (из корня репозитория):
//
//	go run ./internal/plugin/cmd/genseed            # перезаписать
//	go run ./internal/plugin/cmd/genseed -check     # только проверить (CI)
//
// Откуда берутся хэши — по одному на КАЖДУЮ запись реестра, и это важно.
//
// Доверие даётся не по имени, а по хэшу содержимого, поэтому у плагина может
// быть столько доверенных состояний, сколько у него выпусков. Источник этих
// состояний — пины реестра (registry.yaml: version + commit). Для каждой записи
// дерево по пину разворачивается во временный каталог и считается ContentDigest.
//
// Именно по пину, а не по HEAD: registry.yaml — это то, что реально уедет
// пользователю, и доверенным должен быть ровно тот байт-в-байт код, который
// развернёт `wedra plugin install`. Хэш HEAD-плагина был бы доверием к
// содержимому, которого в реестре нет.
//
// Клон НЕ делается по сети: пины лежат в истории этого же репозитория, и CI
// специально checkout-ит с fetch-depth: 0 (см. .github/workflows/ci.yml).
// Генератор читает их через `git archive` — операция без состояния и без
// сети, поэтому один и тот же пин даёт один и тот же хэш на любой машине.
//
// Что НЕ попадает в seed:
//   - тестовые фикстуры (internal/core/testdata, conformance/fixtures) — это не
//     плагины продукта, и их доверие выдаётся явно в тестах;
//   - community-плагины, которых нет в registry.yaml: доверенным может быть
//     только то, за что реестр отвечает.
//
// Файл перезаписывается целиком. Правки руками теряются — это генератор.

import (
	"archive/tar"
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"wedra/internal/plugin"
	"wedra/internal/registry"
)

const outPath = "internal/plugin/trustseed_gen.go"

func main() {
	repo := flag.String("repo", ".", "корень репозитория")
	check := flag.Bool("check", false, "только проверить, что файл совпадает (для CI)")
	flag.Parse()

	reg, err := registry.Load(filepath.Join(*repo, registry.RegistryFile))
	if err != nil {
		fail("загрузить реестр: %v", err)
	}
	defer reg.Close()

	names := reg.PluginNames()
	sort.Strings(names)

	tmp, err := os.MkdirTemp("", "wedra-genseed-*")
	if err != nil {
		fail("временный каталог: %v", err)
	}
	defer os.RemoveAll(tmp)

	seeds := map[string][]string{}
	for _, name := range names {
		entry, _ := reg.GetPlugin(name)
		if err := registry.ValidateEntry(entry, true); err != nil {
			fail("запись %s: %v", name, err)
		}
		ref := entry.Commit
		if ref == "" {
			// Локальный source без пина: доверенного выпуска не существует, и
			// придумывать его нельзя. Пропускаем молча — запись без commit
			// ValidateEntry выше уже отверг для удалённых source.
			continue
		}
		dir := filepath.Join(tmp, name)
		if err := extractAt(*repo, ref, dir); err != nil {
			fail("развернуть %s@%s: %v", name, ref, err)
		}
		digest, err := plugin.ContentDigest(filepath.Join(dir, filepath.FromSlash(entry.Path)))
		if err != nil {
			fail("хеш %s@%s (%s): %v", name, ref, entry.Path, err)
		}
		seeds[name] = append(seeds[name], digest)
	}

	var b strings.Builder
	b.WriteString(header)
	b.WriteString("var builtinTrusted = map[string][]string{\n")
	for _, id := range sortedKeys(seeds) {
		fmt.Fprintf(&b, "\t%q: {\n", id)
		for _, d := range dedupe(seeds[id]) {
			fmt.Fprintf(&b, "\t\t%q,\n", d)
		}
		b.WriteString("\t},\n")
	}
	b.WriteString("}\n")

	target := filepath.Join(*repo, outPath)
	if *check {
		cur, err := os.ReadFile(target)
		if err != nil {
			fail("прочитать %s: %v", outPath, err)
		}
		if string(cur) != b.String() {
			fail("%s устарел: перегенерируй (go run ./internal/plugin/cmd/genseed)", outPath)
		}
		fmt.Println("seed совпадает с пинами реестра")
		return
	}
	if err := os.WriteFile(target, []byte(b.String()), 0o644); err != nil {
		fail("записать %s: %v", outPath, err)
	}
	fmt.Printf("%s: %d плагинов, %d хэшей\n", outPath, len(seeds), countHashes(seeds))
}

// extractAt — развернуть поддерево репозитория по пину в dst.
//
// `git archive` вместо checkout: не трогает состояние рабочего копии вызывающей
// машины, не требует сети и не зависит от настроек core.autocrlf — а значит,
// хэш не «поплывёт» от того, где и как запустили генератор.
func extractAt(repo, ref, dst string) error {
	cmd := exec.Command("git", "-C", repo, "archive", "--format=tar", ref)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git archive %s: %v: %s", ref, err, strings.TrimSpace(stderr.String()))
	}
	return untar(bytes.NewReader(stdout.Bytes()), dst)
}

// untar — распаковать tar в dst. Права не восстанавливаются намеренно: хэш
// содержимого от них не зависит, а молчаливое изменение прав при распаковке
// сделало бы генератор зависимым от umask машины.
func untar(r io.Reader, dst string) error {
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		name := filepath.Clean(filepath.FromSlash(hdr.Name))
		if name == "." || strings.HasPrefix(name, "..") {
			continue
		}
		target := filepath.Join(dst, name)
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return err
			}
			if err := f.Close(); err != nil {
				return err
			}
		default:
			// symlink и прочее пропускаем: ContentDigest такой каталог всё
			// равно отверг бы, а распаковка symlink в temp — лишний риск.
		}
	}
}

func sortedKeys(m map[string][]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, v := range in {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}

func countHashes(m map[string][]string) int {
	n := 0
	for _, v := range m {
		n += len(v)
	}
	return n
}

func fail(format string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, "genseed: "+format+"\n", a...)
	os.Exit(1)
}

var header = `package plugin

// Code generated by internal/plugin/cmd/genseed. DO NOT EDIT.
//
// Встроенный allow-list: плагины, содержимое которых совпадает с пин-ом
// реестра (registry.yaml), доверены без записи в конфиге оператора. Он нужен
// для совместимости: до инверсии плагин считался доверенным по умолчанию, и без
// засева реестровые плагины стали бы untrusted, а вне Linux (где изолятора
// нет) перестали бы работать вовсе.
//
// Что это значит для пользователя: доверенность проверяется по хэшу
// содержимого, поэтому подмена файла в установленном плагине понижает
// доверие, даже если id остался прежним. Community-плагин, которого нет в
// registry.yaml, доверен не будет — для него нужна запись в
// wedra-trust.yaml.

`
