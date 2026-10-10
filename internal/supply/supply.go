// Package supply проверяет, что цепочка поставки собрана так, как о ней
// написано в документации: сторонние действия закреплены по SHA, а релизный
// воркфлоу действительно подписывает артефакты и проверяет подпись.
//
// Зачем. Подпись SHA256SUMS — это обещание, которое даёт документация, и
// обещания в этом проекте уже расходились с кодом (см. пакет verdoc про
// «документ обещает одно, репозиторий другое»). Здесь разрыв устроен так:
// удалить шаг подписи из release.yml не ломает НИ ОДИН существующий прогон —
// сборка зелёная, тесты зелёные, релиз зелёный, просто артефакты уезжают без
// подписи, и единственный след — дыра в цепочке, которой никто не замечает.
//
// Проверка НЕ проверяет криптографию: она смотрит на текст воркфлоу. Саму
// подпись проверяет cosign в release.yml (`cosign verify-blob`) и человек по
// инструкции из docs/supply-chain.md. Здесь ловится другое — исчезновение
// подписи. Поэтому обе проверки нужны вместе: убрать шаг целиком нельзя
// незаметно.
//
// Правило, которому пакет подчинён: отсутствие проверки должно быть заметно.
// Молча пропустить то, чего проверка не поняла, здесь нельзя — поэтому любой
// незнакомый `uses:` без `@` считается незакреплённым и валит шаг.
//
// Никаких зависимостей кроме gopkg.in/yaml.v3 (уже в go.mod): YAML берётся
// как дерево yaml.Node, а не как структура, — нужны номера строк, иначе
// сообщение «uses не закреплён» без указания, где именно, бесполезно.
package supply

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// workflowsDir — где лежат воркфлоу относительно корня репозитория.
const workflowsDir = ".github/workflows"

// releaseWorkflow — единственный воркфлоу, который выпускает артефакты.
const releaseWorkflow = "release.yml"

// Подписанные файлы. Список задан здесь, а не выводится из содержимого
// dist/: проверка «всё, что нашлось» проходит успехом и при нуле файлов,
// то есть подпись могла бы исчезнуть целиком, а шаг остался бы зелёным.
// Это тот же список, что в release.yml, и он специально продублирован:
// расхождение двух списков — повод упасть, а не повод одной строкой.
var signedFiles = []string{
	"dist/SHA256SUMS",
	"dist/sbom.spdx.json",
}

var (
	// shaPinRe — закрепляющий реф: ровно 40 hex-символов, полный SHA-1.
	// Короткая запись (`@v4`, `@main`) по определению не закрепление.
	shaPinRe = regexp.MustCompile(`^[0-9a-f]{40}$`)

	// digestPinRe — закрепление образа действия `docker://name@sha256:...`.
	digestPinRe = regexp.MustCompile(`@sha256:[0-9a-f]{64}$`)

	// localUseRe — переиспользование воркфлоу этого же репозитория
	// (`uses: ./.github/workflows/release.yml`). Оно не третье лицо, и
	// версионируется вместе с коммитом, а не тегом.
	localUseRe = regexp.MustCompile(`^\./`)
)

// useRef и runStep — найденные в воркфлоу элементы с номерами строк.
type useRef struct {
	value string
	line  int
}

type runStep struct {
	body string
	line int
}

// CheckRepo разбирает все воркфлоу и возвращает строку-сводку либо ошибку со
// списком нарушений. Каждое нарушение названо файлом и строкой.
func CheckRepo(repo string) (string, error) {
	dir := filepath.Join(repo, filepath.FromSlash(workflowsDir))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("не читается %s: %w", workflowsDir, err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if ext := filepath.Ext(e.Name()); ext == ".yml" || ext == ".yaml" {
			names = append(names, e.Name())
		}
	}
	if len(names) == 0 {
		return "", fmt.Errorf("в %s нет ни одного воркфлоу", workflowsDir)
	}
	sort.Strings(names)

	var problems []string
	pinned := 0
	for _, name := range names {
		path := filepath.Join(dir, name)
		uses, runs, err := parseWorkflow(path)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		for _, u := range uses {
			if ok, why := pinOK(u.value); !ok {
				problems = append(problems, fmt.Sprintf("%s:%d: %s (%s)", name, u.line, u.value, why))
			} else {
				pinned++
			}
		}
		if name == releaseWorkflow {
			problems = append(problems, checkSigning(name, uses, runs)...)
		}
	}
	if len(problems) > 0 {
		return "", fmt.Errorf("цепочка поставки: %d нарушений:\n  %s",
			len(problems), strings.Join(problems, "\n  "))
	}
	return fmt.Sprintf("%d воркфлоу, %d actions закреплены по SHA; %s подписывает %s и проверяет подпись в том же прогоне",
		len(names), pinned, releaseWorkflow, strings.Join(signedFiles, ", ")), nil
}

// pinOK решает, закреплено ли действие. Второе значение — причина отказа,
// она попадает в сообщение пользователю.
func pinOK(value string) (bool, string) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return false, "пустая ссылка на действие"
	}
	if localUseRe.MatchString(trimmed) {
		return true, ""
	}
	at := strings.LastIndex(trimmed, "@")
	if at < 0 {
		return false, "нет @рефа: тег или ветку можно переписать молча"
	}
	ref := trimmed[at+1:]
	if shaPinRe.MatchString(ref) {
		return true, ""
	}
	if strings.HasPrefix(trimmed, "docker://") && digestPinRe.MatchString(ref) {
		return true, ""
	}
	return false, "реф не полный 40-символьный SHA"
}

// checkSigning отвечает на вопрос «подпись в воркфлоу вообще есть». Это не
// проверка криптографии, а проверка того, что шаг не удалён: без неё удаление
// подписи не ломает ни один прогон.
func checkSigning(name string, uses []useRef, runs []runStep) []string {
	var problems []string
	at := func(what string) string { return name + ": " + what }

	installer := false
	for _, u := range uses {
		if strings.HasPrefix(u.value, "sigstore/cosign-installer@") {
			installer = true
			break
		}
	}
	if !installer {
		problems = append(problems, at("нет шага sigstore/cosign-installer: подписывать нечем"))
	}

	// Шаг подписи и шаг проверки ищутся по буквальному тексту команд. Это
	// намеренно грубо: зато исчезновение шага сразу видно, а не требует
	// разбора YAML-графа шагов.
	var signer, verifier *runStep
	for i := range runs {
		if strings.Contains(runs[i].body, "cosign sign-blob") {
			signer = &runs[i]
		}
		if strings.Contains(runs[i].body, "cosign verify-blob") {
			verifier = &runs[i]
		}
	}
	switch {
	case signer == nil:
		problems = append(problems, at("нет `cosign sign-blob`: артефакты уедут без подписи"))
	default:
		for _, f := range signedFiles {
			if !strings.Contains(signer.body, f) {
				problems = append(problems, fmt.Sprintf(
					"%s: шаг подписи (run:%d) не упоминает %s: подписан не весь список",
					name, signer.line, f))
			}
		}
	}
	switch {
	case verifier == nil:
		problems = append(problems, at("нет `cosign verify-blob`: подпись никогда не проверяется"))
	default:
		for _, f := range signedFiles {
			if !strings.Contains(verifier.body, f) {
				problems = append(problems, fmt.Sprintf(
					"%s: шаг проверки (run:%d) не упоминает %s: файл остался бы непроверенным",
					name, verifier.line, f))
			}
		}
	}
	// Fail-closed на случай, когда cosign не установлен. Без этой строки
	// шаг упал бы с «command not found» — это тоже падение, но с сообщением
	// про оболочку, а не про отсутствие подписи; в коде документации важно,
	// что причина называется прямо.
	guarded := false
	for _, r := range runs {
		if strings.Contains(r.body, "command -v cosign") {
			guarded = true
			break
		}
	}
	if !guarded {
		problems = append(problems, at(
			"нет `command -v cosign`: без cosign шаг молча пройдёт мимо подписи"))
	}
	return problems
}

// parseWorkflow читает YAML как дерево и достаёт все `uses:` и все `run:`.
func parseWorkflow(path string) ([]useRef, []runStep, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, nil, fmt.Errorf("YAML не разбирается: %w", err)
	}
	var uses []useRef
	var runs []runStep
	walk(&doc, func(key string, val *yaml.Node) {
		switch key {
		case "uses":
			if val.Kind == yaml.ScalarNode {
				uses = append(uses, useRef{value: val.Value, line: val.Line})
			}
		case "run":
			if val.Kind == yaml.ScalarNode {
				runs = append(runs, runStep{body: val.Value, line: val.Line})
			}
		}
	})
	return uses, runs, nil
}

// walk обходит дерево YAML и зовёт fn на каждую пару ключ→значение
// отображения. Обход рекурсивный и по значениям тоже: `uses:` может лежать
// внутри with:, а не только на верхнем уровне шага.
func walk(n *yaml.Node, fn func(key string, val *yaml.Node)) {
	if n == nil {
		return
	}
	switch n.Kind {
	case yaml.DocumentNode, yaml.SequenceNode:
		for _, c := range n.Content {
			walk(c, fn)
		}
	case yaml.MappingNode:
		for i := 0; i+1 < len(n.Content); i += 2 {
			fn(n.Content[i].Value, n.Content[i+1])
			walk(n.Content[i+1], fn)
		}
	}
}
