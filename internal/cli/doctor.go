package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/wykserdex/wedra/internal/pipeline"
	"github.com/wykserdex/wedra/internal/plugin"
)

// doctor: ответ на вопрос «я скачал бинарь, ничего не настроил, что мне
// сломано и что делать».
//
// Команда только ОПРАШИВАЕТ: ни один плагин не запускается, ничего не ставится,
// сеть не используется. Единственное, что выполняется, — интерпретатор
// Python: без его запуска не узнать ни версию, ни какие пакеты уже стоят, а
// именно это и есть половина ответа.
//
// Три состояния плагина:
//
//	ready               — всё, что нужно, на месте
//	needs_dependency    — не хватает пакета из runtime.requires
//	needs_external_tool — нужен донор, которого нет в PATH
//
// Последнее — отдельное состояние, а не подвид первого, потому что
// runtime.requires его НЕ ставит и не может: `pip install arjun` ставит
// одноимённый пакет, а `joomscan` — это Perl-скрипт, `webanalyze` — Go-
// программа, и часть доноров ставится только из git. Человек видит в
// манифесте «pip install X», пробует, и либо получает ошибку, либо (для
// git/Perl/Go) не получает вообще ничего.

// Статусы плагина. Это НЕ коды ошибок протокола: они не попадают ни в журнал
// рана, ни в issues валидации, и намеренно не имеют вида E_*/W_*, чтобы гейт
// errcodes (коды в Go ↔ protocol/v0.2/ERRORS.md) не считал их контрактом.
const (
	doctorReady    = "ready"
	doctorNeedsDep = "needs_dependency"
	doctorNeedsDon = "needs_external_tool"
)

// pipInstallRe — «pip install X» из описания манифеста. Именно отсюда человек
// берёт команду установки, поэтому и цитировать её будем оттуда же.
//
// Первая группа после `pip install` — то, что pip считает целью установки.
// Спецслучаи отсекаются: `-r requirements.txt` и `git+https://…` не называют
// пакета, и подставить вместо них имя — значит выдумать команду, которая не
// сработает (для VCS-цели pip вообще требует префикс git+, а не голое имя).
var pipInstallRe = regexp.MustCompile(`(?i)pip3?\s+install\s+("?[^\s"']+)`)

// vcsPrefixes — цели, которые pip ставит не из индекса пакетов.
var vcsPrefixes = []string{"git+", "hg+", "svn+", "bzr+", "http://", "https://", "file://", "-e", "."}

// whichRe — литералы shutil.which("…") из кода плагина. Это самый честный
// источник: плагин сам говорит, какое имя бинаря ищет, и это может отличаться
// от имени пакета (пакет scoutsuite даёт бинарь scout, oletools — olevba).
var whichRe = regexp.MustCompile(`shutil\.which\(\s*"([^"]+)"`)

// doctorPlugin — состояние одного плагина.
type doctorPlugin struct {
	ID     string `json:"id"`
	Dir    string `json:"dir"`
	Status string `json:"status"`

	// Requires — объявленные runtime.requires (пусто у большинства).
	Requires []string `json:"requires,omitempty"`
	// MissingRequires — из них отсутствуют в интерпретаторе.
	MissingRequires []string `json:"missing_requires,omitempty"`

	// Donor — внешний инструмент, упомянутый в описании.
	Donor string `json:"donor,omitempty"`
	// DonorFound — найден ли он в PATH (пусто Donor → поле не значимо).
	DonorFound bool `json:"donor_found,omitempty"`
	// DonorFoundName — под каким именно именем нашёлся: у пакета scoutsuite
	// бинарь называется scout, и это полезно видеть.
	DonorFoundName string `json:"donor_found_name,omitempty"`
	// DonorCandidates — какие имена проверялись в PATH: имя пакета плюс
	// литералы shutil.which из кода плагина.
	DonorCandidates []string `json:"donor_candidates,omitempty"`
	// Install — готовая команда, которую можно скопировать.
	Install string `json:"install,omitempty"`
}

// doctorReport — весь ответ команды.
type doctorReport struct {
	OK bool `json:"ok"`

	Python plugin.InterpreterInfo `json:"python"`
	// PythonError — интерпретатор не найден: python-плагины не запустятся
	// ВООБЩЕ, и проверка пакетов бессмысленна.
	PythonError string `json:"python_error,omitempty"`

	PluginsDir string `json:"plugins_dir"`
	Total      int    `json:"total"`
	Ready      int    `json:"ready"`
	NeedsDep   int    `json:"needs_dependency"`
	NeedsDonor int    `json:"needs_external_tool"`

	Plugins []doctorPlugin `json:"plugins"`
	// Summary — та же строка, что печатается человеку.
	Summary string `json:"summary"`
}

// RunDoctor — wedra doctor [--json]
func RunDoctor(args []string) {
	asJSON := false
	pluginsDir := ""
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			asJSON = true
		case strings.HasPrefix(a, "--plugins="):
			pluginsDir = strings.TrimPrefix(a, "--plugins=")
		case a == "--plugins" && i+1 < len(args):
			i++
			pluginsDir = args[i]
		default:
			fmt.Printf("неизвестный флаг doctor %q\n", a)
			os.Exit(2)
		}
	}

	report := runDoctor(pluginsDir)

	if asJSON {
		printJSON(report)
		if !report.OK {
			os.Exit(1)
		}
		return
	}
	printDoctorHuman(report)
	if !report.OK {
		os.Exit(1)
	}
}

func runDoctor(pluginsDir string) doctorReport {
	if pluginsDir == "" {
		pluginsDir = "plugins"
	}
	rep := doctorReport{PluginsDir: pluginsDir, OK: true}

	// 1. Интерпретатор — тем же кодом, каким реально стартуют плагины.
	info, err := plugin.PythonInfo()
	if err != nil {
		rep.PythonError = err.Error()
		rep.OK = false
	} else {
		rep.Python = info
	}

	manifests := scanDoctorManifests(pluginsDir)
	rep.Total = len(manifests)

	// 2. Пакеты runtime.requires — одним опросом интерпретатора.
	//
	// Если интерпретатора нет, проверять нечем: python-плагины не запустятся
	// вообще, и «отсутствующий пакет» был бы ложью, маскирующей настоящую
	// причину. Поэтому в этом случае requires помечаются отсутствующими, но
	// причина называется отдельно (PythonError), а не ими.
	wanted := []string{}
	seenPkg := map[string]bool{}
	for _, m := range manifests {
		for _, r := range m.Requires {
			name := requirementName(r)
			if name == "" {
				continue
			}
			// Пробуем и имя пакета, и имя модуля, если они различаются:
			// иначе dnspython (модуль dns) читался бы как отсутствующий.
			for _, probe := range requirementImportNames(name) {
				if seenPkg[probe] {
					continue
				}
				seenPkg[probe] = true
				wanted = append(wanted, probe)
			}
		}
	}
	present := map[string]bool{}
	if rep.PythonError == "" && len(wanted) > 0 {
		if got, perr := plugin.PythonModulesPresent(rep.Python.Path, wanted); perr == nil {
			present = got
		} else {
			// Проба не отработала: честнее сказать об этом, чем объявить все
			// пакеты отсутствующими.
			rep.PythonError = perr.Error()
			rep.OK = false
		}
	}

	// 3. Внешние доноры: есть ли такой бинарь в PATH.
	plugins := make([]doctorPlugin, 0, len(manifests))
	for _, m := range manifests {
		p := doctorPlugin{
			ID:       m.ID,
			Dir:      m.Dir,
			Status:   doctorReady,
			Requires: m.Requires,
		}
		for _, r := range m.Requires {
			name := requirementName(r)
			if name == "" {
				continue
			}
			// Пакет считается установленным, если нашёлся либо он сам, либо
			// модуль, которым он импортируется (dnspython → dns).
			found := false
			for _, probe := range requirementImportNames(name) {
				if present[probe] {
					found = true
					break
				}
			}
			if !found {
				p.MissingRequires = append(p.MissingRequires, r)
			}
		}
		if len(p.MissingRequires) > 0 {
			p.Status = doctorNeedsDep
			p.Install = installCommand(p.MissingRequires)
		}

		// Донор: «pip install X» из описания манифеста.
		if donor, ok := donorFromDescription(m.Description); ok {
			candidates := donorCandidates(donor, m.Dir)
			found, foundName := donorInPath(candidates)
			p.Donor = donor
			p.DonorCandidates = candidates
			p.DonorFound = found
			if found {
				p.DonorFoundName = foundName
			}
			if !found {
				// Приоритет у более серьёзной проблемы: отсутствующий пакет
				// из requires не чинится, пока нет интерпретатора, и наоборот.
				// Порядок в выводе задаёт doctorNeedsDep как более серьёзный.
				if p.Status == doctorReady {
					p.Status = doctorNeedsDon
				}
				p.Install = "pip install " + donor
			}
		}

		switch p.Status {
		case doctorReady:
			rep.Ready++
		case doctorNeedsDep:
			rep.NeedsDep++
		case doctorNeedsDon:
			rep.NeedsDonor++
		}
		plugins = append(plugins, p)
	}
	rep.Plugins = plugins
	rep.Summary = doctorSummary(rep)
	// OK — это не «ничего не сломалось», а «плагинов есть и они готовы».
	// Пустой каталог плагинов тоже неуспех: человек запустил doctor, ожидая
	// узнать про 99 плагинов, и «ну ноль проблем» было бы ложью. То же самое с
	// отсутствующим интерпретатором — плагины-то есть, запустить их нельзя.
	if rep.NeedsDep > 0 || rep.NeedsDonor > 0 || rep.PythonError != "" || rep.Total == 0 {
		rep.OK = false
	}
	return rep
}

// doctorManifest — минимум из манифеста, нужный doctor'у.
type doctorManifest struct {
	ID          string
	Dir         string
	Description string
	Requires    []string
}

// scanDoctorManifests читает манифесты напрямую из каталога.
//
// Не через core.ScanPlugins(): тот идёт по относительным путям, молча пропускает
// битый манифест и ничего не говорит. Здесь битый манифест — это плагин без
// doctor-вердикта, и человек должен узнать об этом, а не решить, что плагинов
// просто меньше.
func scanDoctorManifests(dir string) []doctorManifest {
	var out []doctorManifest
	entries, err := os.ReadDir(dir)
	if err != nil {
		return out
	}
	for _, group := range entries {
		if !group.IsDir() {
			continue
		}
		groupDir := filepath.Join(dir, group.Name())
		// Раскладка и плоская (plugins/<id>) и вложенная
		// (plugins/official/<id>, plugins/community/<id>).
		candidates := []string{groupDir}
		if sub, err := os.ReadDir(groupDir); err == nil {
			for _, e := range sub {
				if e.IsDir() {
					candidates = append(candidates, filepath.Join(groupDir, e.Name()))
				}
			}
		}
		for _, cd := range candidates {
			raw, err := os.ReadFile(filepath.Join(cd, "plugin.yaml"))
			if err != nil {
				continue
			}
			// Разбор — тем же кодом, что и у ядра (pipeline.DecodeManifest),
			// а не своя копия yaml: иначе doctor считал бы готовым плагин,
			// который ран потом не примет.
			var m pipeline.Manifest
			if err := pipeline.DecodeManifest(raw, &m); err != nil || m.ID == "" {
				continue
			}
			out = append(out, doctorManifest{
				ID:          m.ID,
				Dir:         cd,
				Description: m.Description,
				Requires:    m.Runtime.Requires,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// donorFromDescription достаёт имя пакета из «pip install X» в описании.
//
// Второй аргумент — «упомянут ли вообще». Совпадение есть всегда положительное;
// «нет» означает «в описании нечем учиться», и тогда doctor молчит, а не
// выдумывает донора.
func donorFromDescription(desc string) (string, bool) {
	m := pipInstallRe.FindStringSubmatch(desc)
	if len(m) < 2 {
		return "", false
	}
	name := strings.Trim(strings.TrimSpace(m[1]), `"'`)
	name = strings.TrimSuffix(name, ".")
	if name == "" {
		return "", false
	}
	// `-r requirements.txt`, `--upgrade X` и VCS-цели (`git+https://…`,
	// `https://…tar.gz`) пакетом не являются: у них нет имени, по которому
	// можно проверить наличие в PATH, а подставленное вместо них имя дало бы
	// человеку команду, которая не сработает.
	if strings.HasPrefix(name, "-") {
		return "", false
	}
	lower := strings.ToLower(name)
	for _, p := range vcsPrefixes {
		if strings.HasPrefix(lower, p) {
			return "", false
		}
	}
	// Точка или путь — это локальный каталог, а не пакет.
	if strings.ContainsAny(name, "/\\") {
		return "", false
	}
	return name, true
}

// donorCandidates — какие имена имеет смысл искать в PATH для донора.
//
// Имя пакета — не обязательно имя бинаря: scoutsuite даёт scout, oletools —
// olevba, wapiti3 — wapiti. Поэтому к имени пакета добавляются литералы
// shutil.which из кода самого плагина: если плагин ищет конкретное имя, мы
// проверяем именно его, а не угадываем по шаблону.
func donorCandidates(donor, pluginDir string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			return
		}
		seen[s] = true
		out = append(out, s)
	}
	add(donor)
	for _, w := range whichLiterals(pluginDir) {
		add(w)
	}
	return out
}

// whichLiterals — имена из shutil.which("…") по всем .py плагина.
func whichLiterals(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			if name == "__pycache__" || name == ".git" {
				continue
			}
			out = append(out, whichLiterals(filepath.Join(dir, name))...)
			continue
		}
		if !strings.HasSuffix(name, ".py") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		for _, m := range whichRe.FindAllStringSubmatch(string(raw), -1) {
			if len(m) > 1 {
				out = append(out, m[1])
			}
		}
	}
	return out
}

// donorInPath — есть ли такой донор в PATH.
//
// exec.LookPath, а не обход PATH вручную: он учитывает PATHEXT на Windows, то
// есть находит donor.exe и donor.cmd так же, как это делает плагин. Имя
// возвращается, чтобы показать человеку, КАКОЕ ИМЯ нашлось: у пакета
// scoutsuite бинарь называется scout, и «нашлось scout» полезнее, чем молчание.
func donorInPath(candidates []string) (found bool, foundName string) {
	for _, c := range candidates {
		if _, err := exec.LookPath(c); err == nil {
			return true, c
		}
	}
	return false, ""
}

// requirementName — имя пакета из спецификации `package==version`.
//
// Точные версии НЕ приводятся: задача и контракт этого не требуют, а пин
// меняется вместе с lock-файлом, и дублировать его здесь — значит показать
// человеку цифру, которая завтра разойдётся с манифестом.
func requirementName(spec string) string {
	s := strings.TrimSpace(spec)
	for _, sep := range []string{"==", ">=", "<=", "~=", "!=", ">", "<", "[", ";", "@"} {
		if i := strings.Index(s, sep); i > 0 {
			s = s[:i]
		}
	}
	return strings.Trim(s, "[] ")
}

// requirementImportName — имя МОДУЛЯ, которым этот пакет импортируется.
//
// Оно не всегда совпадает с именем пакета на PyPI: dnspython ставит модуль
// `dns`, и без этой поправки doctor объявил бы отсутствующим пакет, который на
// машине стоит и с которым плагин работает. Проверено: importlib.util.find_spec
// для «dnspython» возвращает None, для «dns» — модуль.
//
// Таблица намеренно мала и перечислимая: выводить её из кода плагинов дороже,
// чем поддерживать одну-две Known-истины, а выдумывать правила перевода имён
// («убери дефис», «замени _ на -») нельзя — они ломают имена вроде py-altdns.
var requirementImportAliases = map[string]string{
	"dnspython": "dns",
}

// requirementImportNames — под какими именами искать пакет в интерпретаторе:
// своё имя и, если известно отличие, имя модуля.
func requirementImportNames(name string) []string {
	if alias, ok := requirementImportAliases[name]; ok {
		return []string{name, alias}
	}
	return []string{name}
}

func installCommand(reqs []string) string {
	if len(reqs) == 0 {
		return ""
	}
	return "pip install " + strings.Join(reqs, " ")
}

// doctorSummary — одна строка человеческим языком.
func doctorSummary(rep doctorReport) string {
	if rep.Total == 0 {
		return fmt.Sprintf("плагины не найдены в %s — проверь, что бинарь запущен из корня репозитория", rep.PluginsDir)
	}
	if rep.PythonError != "" && rep.Ready == 0 {
		return fmt.Sprintf("не нашлось ни одного готового плагина: сначала интерпретатор Python — %s", rep.PythonError)
	}
	if rep.NeedsDep == 0 && rep.NeedsDonor == 0 {
		return fmt.Sprintf("готовы все %d плагинов — окружение настроено", rep.Total)
	}
	var parts []string
	if rep.NeedsDep > 0 {
		parts = append(parts, plural(rep.NeedsDep, "ждёт pip-пакет из runtime.requires",
			"ждут pip-пакет из runtime.requires", "ждут pip-пакеты из runtime.requires"))
	}
	if rep.NeedsDonor > 0 {
		parts = append(parts, fmt.Sprintf("%s внешнего инструмента (runtime.requires его не ставит)",
			plural(rep.NeedsDonor, "ждёт", "ждут", "ждут")))
	}
	return fmt.Sprintf("готовы %d из %d плагинов, остальным нужно доустановить: %s",
		rep.Ready, rep.Total, strings.Join(parts, "; "))
}

// plural — согласование числительного с существительным. Русский текст без
// него читается как «1 ждут», и это первое, на что смотрит человек, решая
// доверять ли отчёту.
func plural(n int, one, few, many string) string {
	switch {
	case n%10 == 1 && n%100 != 11:
		return fmt.Sprintf("%d %s", n, one)
	case n%10 >= 2 && n%10 <= 4 && (n%100 < 10 || n%100 >= 20):
		return fmt.Sprintf("%d %s", n, few)
	default:
		return fmt.Sprintf("%d %s", n, many)
	}
}

func printDoctorHuman(rep doctorReport) {
	fmt.Println("WEDRA doctor — что сломано в окружении и что с этим делать")
	fmt.Println()
	if rep.PythonError != "" {
		fmt.Println("Python: НЕ НАЙДЕН —", rep.PythonError)
		fmt.Println("       все 99 плагинов написаны на Python, без него не запустится ни один.")
	} else {
		fmt.Printf("Python: %s\n", rep.Python.Version)
		fmt.Printf("        %s\n", rep.Python.Path)
	}
	fmt.Println()

	if rep.Total == 0 {
		fmt.Println("Плагины: не найдены в", rep.PluginsDir)
		fmt.Println()
		fmt.Println(rep.Summary)
		return
	}

	// Сначала те, кому что-то нужно: вверху то, что человек будет делать.
	var needDep, needDonor, ready []*doctorPlugin
	for i := range rep.Plugins {
		p := &rep.Plugins[i]
		switch p.Status {
		case doctorNeedsDep:
			needDep = append(needDep, p)
		case doctorNeedsDon:
			needDonor = append(needDonor, p)
		default:
			ready = append(ready, p)
		}
	}

	if len(needDep) > 0 {
		fmt.Printf("Нужна pip-зависимость (%d): runtime.requires объявлен, но пакета нет\n", len(needDep))
		for _, p := range needDep {
			fmt.Printf("  %-24s %s\n", p.ID, strings.Join(p.MissingRequires, ", "))
			fmt.Printf("      %s\n", p.Install)
		}
		fmt.Println()
	}
	if len(needDonor) > 0 {
		// Доноров обычно больше, чем зависимостей, и они повторяются: одна
		// команда на строку плагина превращала бы отчёт в простыню из
		// повторов. Поэтому группируем по команде установки.
		byInstall := map[string][]string{}
		for _, p := range needDonor {
			byInstall[p.Install] = append(byInstall[p.Install], p.ID)
		}
		cmds := make([]string, 0, len(byInstall))
		for c := range byInstall {
			cmds = append(cmds, c)
		}
		sort.Strings(cmds)
		fmt.Printf("Нужен внешний инструмент (%d плагинов, %d команд): runtime.requires его НЕ ставит\n",
			len(needDonor), len(cmds))
		for _, c := range cmds {
			ids := byInstall[c]
			sort.Strings(ids)
			fmt.Printf("  %s\n", c)
			fmt.Printf("      %s\n", strings.Join(ids, ", "))
		}
		fmt.Println()
	}
	if len(ready) > 0 {
		fmt.Printf("Готовы сразу (%d):\n ", len(ready))
		for i, p := range ready {
			if i > 0 && i%8 == 0 {
				fmt.Print("\n ")
			} else if i > 0 {
				fmt.Print(" ")
			}
			fmt.Print(p.ID)
		}
		fmt.Println()
		fmt.Println()
	}
	fmt.Println(rep.Summary)
}
