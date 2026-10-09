package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wykserdex/wedra/internal/plugin"
)

// doctorFixture кладёт плагин с заданными манифестом и кодом и возвращает
// путь к каталогу plugins.
//
// Своя фикстура, а не plugins/ из репозитория: тест не должен зависеть от
// витрины (она меняется) и не должен видеть плагины, установленные на машине
// того, кто тест запускает.
func doctorFixture(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, body := range files {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// doctorManifestText — манифест минимального валидного плагина.
//
// Описание берётся в одинарных кавычках: реальные манифесты пишут его свёрнутым
// блоком (`description: >`), а в кавычках строка с двоеточием остаётся строкой.
// Без кавычек «Обёртка: pip install X» YAML считает началом отображения, и
// манифест молча не парсится — а плагин пропадает из отчёта.
func doctorManifestText(id, desc string, requires []string) string {
	req := "[]"
	if len(requires) > 0 {
		quoted := make([]string, 0, len(requires))
		for _, r := range requires {
			quoted = append(quoted, `"`+r+`"`)
		}
		req = "[" + strings.Join(quoted, ", ") + "]"
	}
	safe := strings.ReplaceAll(desc, "'", "''")
	return "id: " + id + `
version: 0.1.0
platform_api: "^0.1"
description: '` + safe + `'
author: "test"

runtime:
  type: python
  entry: main.py
  requires: ` + req + `

input:
  value:
    from: input.value
    type: string
output:
  value:
    type: string
permissions:
  network: []
  filesystem: none
  secrets: []
`
}

// doctorOne — отчёт ровно на один плагин, с внятной ошибкой вместо паника.
func doctorOne(t *testing.T, root string) doctorPlugin {
	t.Helper()
	rep := runDoctor(root)
	if len(rep.Plugins) != 1 {
		t.Fatalf("в отчёте плагинов %d, ждали 1 (каталог %s)", len(rep.Plugins), root)
	}
	return rep.Plugins[0]
}

// Нормальный случай: плагин на чистой стандартной библиотеке и без донора
// обязан быть готов. Это регрессия на «doctor считает готовым всё подряд» и
// наоборот — на «doctor флагует штатные плагины».
func TestDoctorReadyWhenNothingIsNeeded(t *testing.T) {
	root := doctorFixture(t, map[string]string{
		"clean/plugin.yaml": doctorManifestText("clean", "Плагин на stdlib, донора нет", nil),
		"clean/main.py":     "import json, sys\nprint(json.dumps({'status': 'ok', 'output': {'value': 'x'}}))\n",
	})
	rep := runDoctor(root)

	if rep.Total != 1 {
		t.Fatalf("найдено плагинов %d, ждали 1 — фикстура не прочитана", rep.Total)
	}
	if rep.Ready != 1 {
		t.Fatalf("готовых %d из 1: статус %q (%+v)", rep.Ready, rep.Plugins[0].Status, rep.Plugins[0])
	}
	if !rep.OK {
		t.Errorf("OK=false при полностью готовом окружении: %s", rep.Summary)
	}
	if rep.Plugins[0].Donor != "" {
		t.Errorf("донер выдуман там, где его нет: %q", rep.Plugins[0].Donor)
	}
	if !strings.Contains(rep.Summary, "готовы все 1 плагинов") {
		t.Errorf("итоговая строка не похожа на «всё готово»: %q", rep.Summary)
	}
}

// Случай с недостающей зависимостью: runtime.requires объявлен, но имён,
// которых наверняка нет на машине, в нём быть не должно — значит проверка
// «отсутствует» обязана сработать. Ничего не устанавливаем.
func TestDoctorReportsMissingDependency(t *testing.T) {
	const absent = "wedra_absent_pkg_doctor_probe"
	root := doctorFixture(t, map[string]string{
		"needy/plugin.yaml": doctorManifestText("needy", "Плагин с зависимостью",
			[]string{absent + "==9.9.9"}),
		"needy/main.py": "import json\nprint(json.dumps({'status': 'ok', 'output': {'value': 'x'}}))\n",
	})
	rep := runDoctor(root)

	if rep.NeedsDep != 1 {
		t.Fatalf("нужна зависимость для %d плагинов, ждали 1 (итог: %s)", rep.NeedsDep, rep.Summary)
	}
	p := doctorOne(t, root)
	if p.Status != doctorNeedsDep {
		t.Errorf("статус = %q, ждали %q", p.Status, doctorNeedsDep)
	}
	if len(p.MissingRequires) != 1 || !strings.Contains(p.MissingRequires[0], absent) {
		t.Errorf("missing_requires = %v, ждали один элемент с %s", p.MissingRequires, absent)
	}
	// Готовая команда обязана быть пригодна к копированию.
	if !strings.Contains(p.Install, "pip install") || !strings.Contains(p.Install, absent) {
		t.Errorf("команда установки не готова к копированию: %q", p.Install)
	}
	if rep.OK {
		t.Error("OK=true при отсутствующей зависимости")
	}
}

// Случай с отсутствующим внешним инструментом: донор назван в описании, его
// нет в PATH. Проверяем через подмену PATH на пустой каталог, чтобы результат
// не зависел от того, что установлено у машины.
func TestDoctorReportsMissingExternalTool(t *testing.T) {
	root := doctorFixture(t, map[string]string{
		"donor/plugin.yaml": doctorManifestText("donor",
			"Обёртка над CLI. Ставится отдельно: pip install wedra-absent-tool.", nil),
		"donor/main.py": "import json\nprint(json.dumps({'status': 'ok', 'output': {'value': 'x'}}))\n",
	})
	// Пустой PATH: ни интерпретатора, ни донора. Интерпретатор нужен и для
	// проверки requires, поэтому отдельно восстанавливаем его через
	// абсолютный путь — иначе тест проверял бы не то.
	t.Setenv("PATH", t.TempDir())

	rep := runDoctor(root)

	if rep.Total != 1 {
		t.Fatalf("найдено плагинов %d, ждали 1", rep.Total)
	}
	p := doctorOne(t, root)
	if p.Donor != "wedra-absent-tool" {
		t.Fatalf("донер = %q, ждали wedra-absent-tool", p.Donor)
	}
	if p.Status != doctorNeedsDon {
		t.Errorf("статус = %q, ждали %q (отсутствие интерпретатора не должно маскировать проблему)", p.Status, doctorNeedsDon)
	}
	if p.Install != "pip install wedra-absent-tool" {
		t.Errorf("команда = %q", p.Install)
	}
	if rep.NeedsDonor != 1 {
		t.Errorf("needs_external_tool = %d, ждали 1", rep.NeedsDonor)
	}
}

// Донор может называться в коде иначе, чем пакет: scoutsuite ставит бинарь
// scout. Если такое имя есть в PATH, плагин готов — иначе doctor ругался бы на
// плагин, который работает.
func TestDoctorAcceptsDonorFoundUnderCodeName(t *testing.T) {
	// Сначала узнаём настоящий интерпретатор: PATH будет подменён, а он нужен
	// для проверки requires.
	interp, err := doctorRealInterpreter()
	if err != nil {
		t.Skipf("на этой машине нет интерпретатора: %v", err)
	}
	binDir := t.TempDir()
	stub := filepath.Join(binDir, "scout")
	if runtimeIsWindows() {
		stub += ".bat"
	}
	if err := os.WriteFile(stub, []byte("@echo off\r\nexit /b 0\r\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	root := doctorFixture(t, map[string]string{
		"scoutpkg/plugin.yaml": doctorManifestText("scoutpkg",
			"Обёртка: pip install scoutsuite", nil),
		"scoutpkg/main.py": "import json, shutil\n" +
			"cmd = shutil.which(\"scout\")\n" +
			"print(json.dumps({'status': 'ok', 'output': {'value': str(cmd)}}))\n",
	})
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+filepath.Dir(interp))

	rep := runDoctor(root)
	p := doctorOne(t, root)
	if !p.DonorFound {
		t.Fatalf("донор scout не найден, хотя положен в PATH: %+v", p)
	}
	if !rep.OK {
		t.Errorf("OK=false, хотя единственный плагин запускается: %s", rep.Summary)
	}
	if p.DonorFoundName != "scout" {
		t.Errorf("найден под именем %q, ждали scout", p.DonorFoundName)
	}
	if p.Status != doctorReady {
		t.Errorf("статус = %q, ждали %q: плагин запускается, ругаться не на что", p.Status, doctorReady)
	}
}

// Имя пакета на PyPI и имя модуля не обязаны совпадать: dnspython ставит
// модуль `dns`. Без этой поправки doctor объявляет отсутствующим пакет, который
// стоит. Реальный кейс репозитория — official/syntax_mx_checker.
func TestDoctorAcceptsPackageWhoseModuleDiffers(t *testing.T) {
	if _, ok := requirementImportAliases["dnspython"]; !ok {
		t.Fatal("поправка на dnspython→dns пропала: она нужна, иначе syntax_mx_checker читается как сломанный")
	}
	names := requirementImportNames("dnspython")
	if len(names) != 2 || names[1] != "dns" {
		t.Fatalf("requirementImportNames(dnspython) = %v, ждали [dnspython dns]", names)
	}
	// Обычный пакет даёт одно имя.
	if got := requirementImportNames("holehe"); len(got) != 1 || got[0] != "holehe" {
		t.Errorf("requirementImportNames(holehe) = %v", got)
	}
}

// `pip install -r requirements.txt` и `pip install git+https://…` не называют
// пакета. Подставлять вместо них имя — значит выдумать нерабочую команду.
func TestDoctorIgnoresPipFlagsAndURLs(t *testing.T) {
	cases := []struct{ desc, donor string }{
		{"Ставится: pip install -r requirements.txt.", ""},
		{"Установка: pip install git+https://github.com/x/y.git", ""},
		{"Ставится отдельно: pip install arjun.", "arjun"},
	}
	for _, c := range cases {
		got, ok := donorFromDescription(c.desc)
		if ok != (c.donor != "") {
			t.Errorf("donorFromDescription(%q): ok=%v, ждали %v", c.desc, ok, c.donor != "")
		}
		if got != c.donor {
			t.Errorf("donorFromDescription(%q) = %q, ждали %q", c.desc, got, c.donor)
		}
	}
}

// Версии из пина не должны попадать в имя пакета: иначе проба ищет модуль
// «holehe==1.61», которого не бывает, и находит его только у отсутствующего.
func TestRequirementNameStripsPin(t *testing.T) {
	cases := map[string]string{
		"holehe==1.61":     "holehe",
		"dnspython==2.8.0": "dnspython",
		"holehe":           "holehe",
		"pkg>=1.0":         "pkg",
		"":                 "",
	}
	for in, want := range cases {
		if got := requirementName(in); got != want {
			t.Errorf("requirementName(%q) = %q, ждали %q", in, got, want)
		}
	}
}

// Пустой каталог плагинов — не «всё готово», а отсутствие витрины. Итоговая
// строка обязана называть причину, иначе человек решит, что у него всё в
// порядке.
func TestDoctorEmptyPluginsDirExplainsItself(t *testing.T) {
	rep := runDoctor(t.TempDir())
	if rep.Total != 0 {
		t.Fatalf("Total = %d при пустом каталоге", rep.Total)
	}
	if rep.OK {
		t.Error("OK=true при пустом каталоге плагинов")
	}
	if !strings.Contains(rep.Summary, "не найдены") {
		t.Errorf("итог не объясняет, что плагинов нет: %q", rep.Summary)
	}
}

// JSON-вывод обязан разбираться и нести те же счётчики, что и человеческий:
// иначе автоматизация и человек видят разное.
func TestDoctorJSONShape(t *testing.T) {
	root := doctorFixture(t, map[string]string{
		"a/plugin.yaml": doctorManifestText("a", "Плагин A на stdlib", nil),
		"a/main.py":     "import json\nprint('{}')\n",
	})
	rep := runDoctor(root)
	raw, err := json.Marshal(rep)
	if err != nil {
		t.Fatal(err)
	}
	var back map[string]interface{}
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"ok", "python", "plugins_dir", "total", "ready", "summary", "plugins"} {
		if _, ok := back[key]; !ok {
			t.Errorf("в JSON нет поля %q", key)
		}
	}
	list, ok := back["plugins"].([]interface{})
	if !ok || len(list) != 1 {
		t.Fatalf("plugins в JSON = %v, ждали массив из 1", back["plugins"])
	}
	entry := list[0].(map[string]interface{})
	if entry["status"] != doctorReady {
		t.Errorf("status в JSON = %v, ждали %q", entry["status"], doctorReady)
	}
}

// Согласование числительного: «1 ждёт», а не «1 ждут».
func TestDoctorSummaryPlural(t *testing.T) {
	cases := []struct {
		n          int
		wantSubstr string
	}{
		{1, "1 ждёт"},
		{2, "2 ждут"},
		{5, "5 ждут"},
		{11, "11 ждут"},
		{21, "21 ждёт"},
	}
	for _, c := range cases {
		got := plural(c.n, "ждёт", "ждут", "ждут")
		if got != c.wantSubstr {
			t.Errorf("plural(%d) = %q, ждали %q", c.n, got, c.wantSubstr)
		}
	}
}

// doctorRealInterpreter — путь к интерпретатору до подмены PATH: тесты,
// подставляющие пустой PATH, всё равно должны находить python тем же кодом,
// каким это делает ран.
func doctorRealInterpreter() (string, error) {
	info, err := plugin.PythonInfo()
	if err != nil {
		return "", err
	}
	return info.Path, nil
}

func runtimeIsWindows() bool { return os.PathListSeparator == ';' }
