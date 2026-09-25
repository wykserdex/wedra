package pipeline

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"wedra/internal/common"
)

var formatRank = map[string]int{
	"": 0, "string": 0,
	"text": 1, "file_ref": 1,
	"email": 2, "url": 3, "ip": 4,
}

func formatsCompatible(producer, consumer string) bool {
	if consumer == "" || producer == "any" {
		return true
	}
	p, pok := formatRank[producer]
	c, cok := formatRank[consumer]
	if !pok || !cok {
		return producer == consumer
	}
	return p >= c
}

type priorStep struct {
	step     *Step
	manifest *Manifest
}

type srcInfo struct {
	Name    string
	Type    string
	Format  string
	Step    *Step
	Literal interface{}
}

var formatCheckers = map[string]func(string) bool{
	"email": func(s string) bool {
		return regexp.MustCompile(`^[^@\s]+@[^@\s]+$`).MatchString(s)
	},
	"url":  func(s string) bool { return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") },
	"ip":   func(s string) bool { return net.ParseIP(s) != nil },
	"text": func(s string) bool { return true },
}

func scalarMatchesFormat(lit interface{}, format string) bool {
	chk, ok := formatCheckers[format]
	if !ok {
		return true
	}
	s, ok := lit.(string)
	if !ok {
		return false
	}
	return chk(s)
}

func resolveSource(path string, prior map[string]priorStep, pf *PipelineFile, st *Step) (srcInfo, string) {
	parts := strings.Split(path, ".")
	if len(parts) < 2 {
		return srcInfo{}, "путь слишком короткий: " + path
	}
	p := pf.Pipeline
	switch parts[0] {
	case "input":
		key := parts[1]
		// v0.20: input.<foreach_item> шага с foreach — динамическая переменная элемента
		if st != nil && st.Foreach != "" {
			sitemKey := st.ForeachItem
			if sitemKey == "" {
				sitemKey = "item"
			}
			if key == sitemKey {
				return srcInfo{Name: path, Type: "string", Format: "text"}, ""
			}
		}
		itemKey := p.ForeachItem
		if itemKey == "" {
			itemKey = "item"
		}
		if key == itemKey && p.Foreach != "" {
			if len(parts) > 2 {
				return srcInfo{Name: path, Type: "string", Format: "text"}, ""
			}
			return srcInfo{Name: path, Type: p.ItemType, Format: p.ItemFormat}, ""
		}
		val, ok := p.Input[key]
		if !ok {
			return srcInfo{}, "в input пайплайна нет поля " + key
		}
		if len(parts) > 2 {
			return srcInfo{Name: path, Type: "", Format: ""}, ""
		}
		return srcInfo{Name: path, Type: KindOf(val), Literal: val}, ""
	case "steps":
		if len(parts) == 2 && strings.HasSuffix(parts[1], "_all") {
			base := strings.TrimSuffix(parts[1], "_all")
			ps, ok := prior[base]
			if !ok {
				return srcInfo{}, "шаг " + base + " не найден выше по цепочке (агрегат " + path + ")"
			}
			return srcInfo{Name: path, Type: "array", Format: "any", Step: ps.step}, ""
		}
		if len(parts) < 3 {
			return srcInfo{}, "ожидается путь steps.<step_id>.<поле>: " + path
		}
		sid, field := parts[1], parts[2]
		if strings.HasSuffix(sid, "_all") {
			base := strings.TrimSuffix(sid, "_all")
			ps, ok := prior[base]
			if !ok {
				return srcInfo{}, "шаг " + base + " не найден выше по цепочке"
			}
			return srcInfo{Name: path, Type: "array", Format: "any", Step: ps.step}, ""
		}
		ps, ok := prior[sid]
		if !ok {
			return srcInfo{}, "шаг " + sid + " не найден выше по цепочке"
		}
		if IsBuiltin(ps.step.Plugin) {
			return srcInfo{Name: path, Step: ps.step, Format: "any"}, ""
		}
		port, ok := ps.manifest.Output[field]
		if !ok {
			if strings.HasSuffix(field, "_all") {
				baseField := strings.TrimSuffix(field, "_all")
				if _, ok2 := ps.manifest.Output[baseField]; ok2 {
					return srcInfo{Name: path, Type: "array", Format: "any", Step: ps.step}, ""
				}
			}
			return srcInfo{}, fmt.Sprintf("плагин %s не объявляет выход %q", ps.manifest.ID, field)
		}
		return srcInfo{Name: path, Type: port.Type, Format: port.Format, Step: ps.step}, ""
	default:
		return srcInfo{}, "путь должен начинаться с input. или steps.: " + path
	}
}

func IsBuiltin(ref string) bool {
	return common.IsBuiltinRef(ref)
}

func IsBuiltinNamespace(ref string) bool {
	return common.IsBuiltinNamespace(ref)
}

type Engine interface {
	LoadManifest(ref string) (*Manifest, error)
}

func Validate(pf *PipelineFile, eng Engine) (errs, warns []string) {
	return SplitIssues(ValidateIssues(pf, eng))
}

func validateLegacy(pf *PipelineFile, eng Engine) (errs, warns []string) {
	if pf.FormatVersion != "0.1" && pf.FormatVersion != "0.2" {
		if pf.FormatVersion != "" {
			errs = append(errs, fmt.Sprintf("format_version %q не из списка поддерживаемых: 0.1, 0.2", pf.FormatVersion))
		} else {
			warns = append(warns, fmt.Sprintf("format_version %q не из списка поддерживаемых: 0.1, 0.2", pf.FormatVersion))
		}
	}
	// v0.16: secrets — предупреждение до запуска, ошибка будет в раннере
	for _, k := range pf.Pipeline.Secrets {
		if os.Getenv(k) == "" {
			warns = append(warns, fmt.Sprintf("secrets: переменная окружения %s не задана (нужна для запуска)", k))
		}
	}
	if cycle := DetectCycle(pf); cycle != "" {
		errs = append(errs, cycle)
	}
	p := &pf.Pipeline
	if p.Foreach != "" {
		if strings.HasPrefix(p.Foreach, "input.") {
			key := strings.TrimPrefix(p.Foreach, "input.")
			if value, ok := p.Input[key]; !ok {
				errs = append(errs, "foreach: массив "+p.Foreach+" не найден в input")
			} else if arr, ok := value.([]interface{}); ok && len(arr) > MaxForeachItems {
				errs = append(errs, fmt.Sprintf("foreach: input.%s содержит %d элементов, максимум %d", key, len(arr), MaxForeachItems))
			}
		} else if strings.HasPrefix(p.Foreach, "steps.") {
			parts := strings.Split(p.Foreach, ".")
			if len(parts) < 3 {
				errs = append(errs, "foreach: steps.* должен быть вида steps.<id>.<field>")
			} else {
				found := false
				for _, st := range p.Steps {
					if st.ID == parts[1] {
						found = true
						break
					}
				}
				if !found {
					errs = append(errs, fmt.Sprintf("foreach: шаг %s не найден в пайплайне", parts[1]))
				}
			}
		} else {
			errs = append(errs, "foreach: путь должен начинаться с input. или steps.")
		}
	}
	seen := map[string]bool{}
	prior := map[string]priorStep{}
	for i := range p.Steps {
		st := &p.Steps[i]
		if st.ID == "" {
			errs = append(errs, fmt.Sprintf("шаг #%d: пустой id", i+1))
			continue
		}
		if seen[st.ID] {
			errs = append(errs, "шаг "+st.ID+": дублирующийся id")
		}
		seen[st.ID] = true
		if !IsBuiltin(st.Plugin) && IsBuiltinNamespace(st.Plugin) {
			errs = append(errs, fmt.Sprintf("шаг %s: неизвестный встроенный модуль: %s", st.ID, st.Plugin))
			continue
		}
		// v0.20: управляющий поток на уровне шага
		if st.When.IsSet() {
			if !WhenOps[st.When.Op] {
				errs = append(errs, fmt.Sprintf("шаг %s: when: неизвестный оператор %q (допускаются: truthy, exists, missing, eq, neq, gt, gte, lt, lte, contains)", st.ID, st.When.Op))
			}
			if !strings.HasPrefix(st.When.Path, "input.") && !strings.HasPrefix(st.When.Path, "steps.") {
				errs = append(errs, fmt.Sprintf("шаг %s: when: путь должен начинаться с input. или steps. (got %s)", st.ID, st.When.Path))
			} else if parts := strings.Split(st.When.Path, "."); strings.HasPrefix(st.When.Path, "steps.") {
				if len(parts) < 3 {
					errs = append(errs, fmt.Sprintf("шаг %s: when: steps.* должен быть вида steps.<id>.<field>", st.ID))
				} else if _, ok := prior[parts[1]]; !ok {
					errs = append(errs, fmt.Sprintf("шаг %s: when: читает из шага %s, который ещё не выполняется", st.ID, parts[1]))
				}
			}
		}
		if st.Foreach != "" {
			if !strings.HasPrefix(st.Foreach, "input.") && !strings.HasPrefix(st.Foreach, "steps.") {
				errs = append(errs, fmt.Sprintf("шаг %s: foreach: путь должен начинаться с input. или steps. (got %s)", st.ID, st.Foreach))
			} else if strings.HasPrefix(st.Foreach, "steps.") {
				parts := strings.Split(st.Foreach, ".")
				if len(parts) < 3 {
					errs = append(errs, fmt.Sprintf("шаг %s: foreach: steps.* должен быть вида steps.<id>.<field>", st.ID))
				} else if _, ok := prior[parts[1]]; !ok {
					errs = append(errs, fmt.Sprintf("шаг %s: foreach: шаг %s не найден или ещё не выполняется", st.ID, parts[1]))
				}
			} else if key := strings.TrimPrefix(st.Foreach, "input."); !strings.Contains(key, ".") {
				if _, ok := p.Input[key]; !ok {
					errs = append(errs, fmt.Sprintf("шаг %s: foreach: массив %s не найден в input", st.ID, st.Foreach))
				}
			}
			if st.AfterForeach {
				errs = append(errs, fmt.Sprintf("шаг %s: foreach и after_foreach не сочетаются", st.ID))
			}
			if st.ParallelGroup != "" {
				errs = append(errs, fmt.Sprintf("шаг %s: foreach не сочетается с parallel_group", st.ID))
			}
			if st.ForeachItem != "" && strings.ContainsAny(st.ForeachItem, ". \t\"'") {
				errs = append(errs, fmt.Sprintf("шаг %s: foreach_item должно быть простым именем (got %q)", st.ID, st.ForeachItem))
			}
			if IsBuiltin(st.Plugin) {
				errs = append(errs, fmt.Sprintf("шаг %s: human_gate не принимает foreach", st.ID))
			}
		}
		if st.ParallelGroup != "" && IsBuiltin(st.Plugin) {
			errs = append(errs, fmt.Sprintf("шаг %s: human_gate нельзя ставить в параллельную группу %q (гейты сериализуют терминал)", st.ID, st.ParallelGroup))
		}
		switch st.OnError {
		case "", "stop", "skip", "retry":
		default:
			errs = append(errs, "шаг "+st.ID+": on_error="+st.OnError+", ожидается stop|skip|retry")
		}
		if st.OnError == "retry" && st.Retry != nil && st.Retry.Attempts < 1 {
			errs = append(errs, "шаг "+st.ID+": retry.attempts < 1")
		}
		if st.OnError == "retry" && st.Retry != nil && st.Retry.Attempts > MaxRetryAttempts {
			errs = append(errs, fmt.Sprintf("шаг %s: retry.attempts=%d, максимум %d", st.ID, st.Retry.Attempts, MaxRetryAttempts))
		}
		if IsBuiltin(st.Plugin) {
			if len(st.Bind) > 0 {
				errs = append(errs, "шаг "+st.ID+": human_gate не принимает bind")
			}
			switch st.OnReject {
			case "", "stop", "continue":
			default:
				errs = append(errs, "шаг "+st.ID+": on_reject="+st.OnReject+", ожидается stop|continue")
			}
			for _, action := range st.Actions {
				if action != "accept" && action != "reject" {
					errs = append(errs, "шаг "+st.ID+": actions="+action+", допустимы accept|reject")
				}
			}
			bnSeen := map[string][]string{}
			for _, f := range st.Form {
				bn := Basename(f.Field)
				bnSeen[bn] = append(bnSeen[bn], f.Field)
			}
			for bn, fields := range bnSeen {
				if len(fields) > 1 {
					warns = append(warns, fmt.Sprintf("шаг %s, form: базовое имя %q встречается в %v — будут ключи вида <step_id>_%s", st.ID, bn, fields, bn))
				}
			}
			for _, f := range st.Form {
				if src, e := resolveSource(f.Field, prior, pf, nil); e != "" {
					warns = append(warns, fmt.Sprintf("шаг %s, form: %s — поле может отсутствовать", st.ID, e))
				} else if src.Step != nil && src.Step.OnError == "skip" {
					warns = append(warns, fmt.Sprintf("шаг %s, form: %s читает из skip-able шага %s", st.ID, f.Field, src.Step.ID))
				}
			}
			prior[st.ID] = priorStep{step: st, manifest: &Manifest{ID: "core/human_gate"}}
			continue
		}
		m, err := eng.LoadManifest(st.Plugin)
		if err != nil {
			errs = append(errs, "шаг "+st.ID+": "+err.Error())
			continue
		}
		for b := range st.Bind {
			if _, ok := m.Input[b]; !ok {
				errs = append(errs, fmt.Sprintf("шаг %s: bind указывает на несуществующий порт %q (порты: %s)", st.ID, b, portNames(m.Input)))
			}
		}
		// v0.17: declare-now — плагин заявил сеть в манифесте
		if len(m.Permissions.Network) > 0 {
			hosts := NetworkHosts(m)
			if p.Network == "deny" {
				errs = append(errs, fmt.Sprintf("шаг %s: плагин %s заявил сеть (%s), а пайплайн запрещает (network: deny)", st.ID, st.Plugin, hosts))
			} else {
				warns = append(warns, fmt.Sprintf("шаг %s: плагин заявил сеть: %s (declare-now, аудит — журнал)", st.ID, hosts))
			}
		}
		for portName, port := range m.Input {
			srcPath := PortSource(portName, port, st)
			if srcPath == "" {
				if port.Optional {
					warns = append(warns, fmt.Sprintf("шаг %s, порт %s: нет привязки (optional)", st.ID, portName))
					continue
				}
				errs = append(errs, fmt.Sprintf("шаг %s, порт %s: нет привязки", st.ID, portName))
				continue
			}
			src, perr := resolveSource(srcPath, prior, pf, st)
			if perr != "" {
				if port.Optional {
					warns = append(warns, fmt.Sprintf("шаг %s, порт %s: %s (optional)", st.ID, portName, perr))
				} else {
					errs = append(errs, fmt.Sprintf("шаг %s, порт %s: %s", st.ID, portName, perr))
				}
				continue
			}
			src.Name = srcPath
			if src.Type != "" && port.Type != "" && src.Type != port.Type {
				errs = append(errs, fmt.Sprintf("шаг %s, порт %s: тип %s несовместим с выходом %q (%s)", st.ID, portName, port.Type, src.Name, src.Type))
			}
			literalChecked := false
			if src.Literal != nil && port.Format != "" {
				if s, isStr := src.Literal.(string); isStr {
					literalChecked = true
					if !scalarMatchesFormat(s, port.Format) {
						errs = append(errs, fmt.Sprintf("шаг %s, порт %s: значение input %q не соответствует формату %q", st.ID, portName, s, port.Format))
					}
				}
			}
			if !literalChecked && !formatsCompatible(src.Format, port.Format) {
				errs = append(errs, fmt.Sprintf("шаг %s, порт %s: формат источника %q не покрывает %q", st.ID, portName, src.Format, port.Format))
			}
			if src.Step != nil && src.Step.OnError == "skip" && !port.Optional {
				errs = append(errs, fmt.Sprintf("шаг %s, порт %s: читает из skip-able шага %s — объявите optional", st.ID, portName, src.Step.ID))
			}
		}
		prior[st.ID] = priorStep{step: st, manifest: m}
	}
	// v0.20: parallel_group — шаги группы должны быть смежными в списке
	groupLast := map[string]int{}
	groupSize := map[string]int{}
	for i := range p.Steps {
		g := p.Steps[i].ParallelGroup
		if g == "" {
			continue
		}
		groupSize[g]++
		if last, ok := groupLast[g]; ok && i != last+1 {
			errs = append(errs, fmt.Sprintf("parallel_group %q: шаги группы должны быть рядом в списке (шаг %s отделён от группы)", g, p.Steps[i].ID))
		}
		groupLast[g] = i
	}
	for g, n := range groupSize {
		if n == 1 {
			warns = append(warns, fmt.Sprintf("parallel_group %q: один шаг — параллелизм бессмыслен", g))
		}
		if n > MaxParallelWidth {
			errs = append(errs, fmt.Sprintf("parallel_group %q: %d шагов, максимум %d", g, n, MaxParallelWidth))
		}
	}
	// v0.17: кросс-проверка secrets — pipeline.secrets ↔ permissions.secrets манифестов
	pluginSecrets := map[string]bool{}
	for _, ps := range prior {
		for _, s := range ps.manifest.Permissions.Secrets {
			pluginSecrets[s] = true
		}
	}
	pipelineSecrets := map[string]bool{}
	for _, k := range p.Secrets {
		pipelineSecrets[k] = true
	}
	for _, k := range p.Secrets {
		if !pluginSecrets[k] {
			warns = append(warns, fmt.Sprintf("secrets: пайплайн объявляет %s, но ни один плагин не заявляет её в permissions.secrets", k))
		}
	}
	var undeclared []string
	for s := range pluginSecrets {
		if !pipelineSecrets[s] {
			undeclared = append(undeclared, s)
		}
	}
	sort.Strings(undeclared)
	for _, s := range undeclared {
		warns = append(warns, fmt.Sprintf("secrets: плагину нужен ключ %s — объявите в pipeline secrets (иначе может не быть в env при запуске)", s))
	}
	return errs, warns
}

func portNames(m map[string]Port) string {
	if len(m) == 0 {
		return "—"
	}
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return strings.Join(ks, ", ")
}

func CheckPortFormats(pfx, name string, port Port, errs []string) []string {
	if port.Format == "" {
		return errs
	}
	if port.Type != "" && port.Type != "string" {
		return append(errs, fmt.Sprintf(
			"%s %s: format %q задан на type: %s — format применим только к строкам (type: string); "+
				"для %s уберите format: валидатор покроет форму типом, а состав элементов проверяйте внутри плагина",
			pfx, name, port.Format, port.Type, port.Type))
	}
	if _, ok := formatRank[port.Format]; !ok {
		return append(errs, fmt.Sprintf(
			"%s %s: неизвестный format %q — есть: text, email, url, ip, file_ref",
			pfx, name, port.Format))
	}
	return errs
}

func ValidatePluginDir(dir string) []string {
	raw, err := os.ReadFile(filepath.Join(dir, "plugin.yaml"))
	if err != nil {
		return []string{err.Error()}
	}
	var m Manifest
	if err := unmarshalYAML(raw, &m); err != nil {
		return []string{err.Error()}
	}
	m.Dir = dir
	var errs []string
	if err := ValidateManifest(&m); err != nil {
		errs = append(errs, err.Error())
	}
	if len(m.Output) == 0 {
		errs = append(errs, "output пуст")
	}
	if safeManifestEntry(m.Runtime.Entry) {
		if err := validateManifestEntryFile(dir, m.Runtime.Entry); err != nil {
			errs = append(errs, "entry: "+err.Error())
		}
	}
	return errs
}

func KindOf(v interface{}) string {
	switch v.(type) {
	case string:
		return "string"
	case float64, int, int64:
		return "number"
	case bool:
		return "boolean"
	case []interface{}:
		return "array"
	case map[string]interface{}:
		return "object"
	default:
		return "string"
	}
}

func Basename(p string) string {
	parts := strings.Split(p, ".")
	return parts[len(parts)-1]
}

var (
	manifestIDPattern          = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)
	manifestNamePattern        = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	manifestVersionPattern     = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$|^0\.[0-9]+$`)
	manifestHostPattern        = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9.-]*[A-Za-z0-9])?$`)
	environmentNamePattern     = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	manifestRequirementPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*==[A-Za-z0-9][A-Za-z0-9_.+-]*$`)
)

var manifestPortTypes = map[string]bool{
	"string": true, "number": true, "boolean": true, "array": true, "object": true,
}

var manifestFormats = map[string]bool{
	"text": true, "email": true, "url": true, "ip": true, "file_ref": true,
}

func ValidateManifest(m *Manifest) error {
	if m == nil {
		return fmt.Errorf("манифест пуст")
	}
	var problems []string
	add := func(format string, args ...interface{}) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}
	if m.ID == "" {
		add("id обязателен")
	} else if !manifestIDPattern.MatchString(m.ID) {
		add("id %q: допустимы только строчные буквы, цифры и _", m.ID)
	}
	if m.Version == "" {
		add("version обязателен")
	} else if !manifestVersionPattern.MatchString(m.Version) {
		add("version %q: ожидается SemVer X.Y.Z", m.Version)
	}
	if m.PlatformAPI == "" {
		add("platform_api обязателен")
	} else if !platformAPICompatible(m.PlatformAPI) {
		add("platform_api %q несовместим с текущим API %s", m.PlatformAPI, PlatformAPI)
	}
	if m.Runtime.Type != "python" && m.Runtime.Type != "binary" {
		add("runtime.type %q: ожидается python или binary", m.Runtime.Type)
	}
	if !safeManifestEntry(m.Runtime.Entry) {
		add("runtime.entry %q: нужен относительный путь внутри плагина без ..", m.Runtime.Entry)
	} else if m.Dir != "" {
		candidate := filepath.Join(m.Dir, filepath.FromSlash(m.Runtime.Entry))
		if _, statErr := os.Stat(candidate); statErr == nil {
			if err := validateManifestEntryFile(m.Dir, m.Runtime.Entry); err != nil {
				add("runtime.entry: %v", err)
			}
		} else if !os.IsNotExist(statErr) {
			add("runtime.entry: %v", statErr)
		}
	}
	for i, requirement := range m.Runtime.Requires {
		if strings.TrimSpace(requirement) == "" {
			add("runtime.requires[%d] пуст", i)
		} else if !manifestRequirementPattern.MatchString(requirement) {
			add("runtime.requires[%d] %q: ожидается package==version", i, requirement)
		}
	}
	if len(m.Runtime.Requires) > 0 && m.Dir != "" {
		if err := validateManifestRequirements(m.Dir, m.Runtime.Requires); err != nil {
			add("runtime.requires: %v", err)
		}
	}
	if err := validateManifestPorts("input", m.Input); err != nil {
		problems = append(problems, strings.Split(err.Error(), "; ")...)
	}
	if err := validateManifestPorts("output", m.Output); err != nil {
		problems = append(problems, strings.Split(err.Error(), "; ")...)
	}
	if m.Permissions.Filesystem != "" {
		switch m.Permissions.Filesystem {
		case "none", "read", "workspace", "write", "readwrite":
		default:
			add("permissions.filesystem %q: допустимы none|read|workspace|write|readwrite", m.Permissions.Filesystem)
		}
	}
	seenSecrets := map[string]bool{}
	for i, secret := range m.Permissions.Secrets {
		if !environmentNamePattern.MatchString(secret) {
			add("permissions.secrets[%d] %q: ожидается имя env-переменной", i, secret)
		} else if seenSecrets[secret] {
			add("permissions.secrets[%d] %q: дубликат", i, secret)
		}
		seenSecrets[secret] = true
	}
	seenNetwork := map[string]bool{}
	for i, permission := range m.Permissions.Network {
		if permission.Port < 0 || permission.Port > 65535 {
			add("permissions.network[%d].port %d: ожидается 0..65535", i, permission.Port)
		}
		if permission.AnyHost {
			if permission.Host != "" {
				add("permissions.network[%d]: any_host и host нельзя указывать вместе", i)
			}
		} else if !validManifestHost(permission.Host) {
			add("permissions.network[%d].host %q: ожидается DNS-host или IP без схемы", i, permission.Host)
		}
		key := permission.Host + ":" + strconv.Itoa(permission.Port) + ":" + strconv.FormatBool(permission.AnyHost)
		if seenNetwork[key] {
			add("permissions.network[%d]: дубликат разрешения", i)
		}
		seenNetwork[key] = true
	}
	if len(problems) > 0 {
		return fmt.Errorf("%s", strings.Join(problems, "; "))
	}
	return nil
}

func validateManifestRequirements(dir string, requirements []string) error {
	raw, err := os.ReadFile(filepath.Join(dir, "requirements.lock"))
	if err != nil {
		return fmt.Errorf("требуется requirements.lock с exact pins")
	}
	expected := map[string]bool{}
	for _, requirement := range requirements {
		expected[requirement] = true
	}
	seen := map[string]bool{}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !expected[line] {
			return fmt.Errorf("requirements.lock содержит незаявленную зависимость %q", line)
		}
		seen[line] = true
	}
	for requirement := range expected {
		if !seen[requirement] {
			return fmt.Errorf("requirements.lock не содержит %q", requirement)
		}
	}
	return nil
}

func validateManifestPorts(kind string, ports map[string]Port) error {
	var problems []string
	for name, port := range ports {
		if !manifestNamePattern.MatchString(name) {
			problems = append(problems, fmt.Sprintf("%s %q: имя должно быть строчными буквами, цифрами и _", kind, name))
		}
		if port.Type == "" {
			problems = append(problems, fmt.Sprintf("%s %s: type обязателен", kind, name))
		} else if !manifestPortTypes[port.Type] {
			problems = append(problems, fmt.Sprintf("%s %s: type %q неизвестен", kind, name, port.Type))
		}
		if port.Format != "" {
			if !manifestFormats[port.Format] {
				problems = append(problems, fmt.Sprintf("%s %s: format %q неизвестен", kind, name, port.Format))
			}
			if port.Type != "string" {
				problems = append(problems, fmt.Sprintf("%s %s: format применим только к string", kind, name))
			}
		}
		if port.From != "" && !validManifestSource(port.From) {
			problems = append(problems, fmt.Sprintf("%s %s: from %q должен быть input.<field> или steps.<id>.<field>", kind, name, port.From))
		}
	}
	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf("%s", strings.Join(problems, "; "))
}

func validManifestSource(source string) bool {
	parts := strings.Split(source, ".")
	if len(parts) < 2 || parts[0] != "input" {
		if len(parts) < 3 || parts[0] != "steps" {
			return false
		}
	}
	for _, part := range parts {
		if part == "" {
			return false
		}
	}
	return true
}

func validManifestHost(host string) bool {
	if host == "" || host == "*" || strings.ContainsAny(host, "/\\?#@ \t\r\n") {
		return false
	}
	if net.ParseIP(strings.Trim(host, "[]")) != nil {
		return true
	}
	return manifestHostPattern.MatchString(host)
}

func safeManifestEntry(entry string) bool {
	if entry == "" || strings.ContainsRune(entry, 0) {
		return false
	}
	normalized := strings.ReplaceAll(entry, "\\", "/")
	if strings.HasPrefix(normalized, "/") || regexp.MustCompile(`^[A-Za-z]:[\\/]`).MatchString(normalized) {
		return false
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(normalized)))
	return clean != "." && clean != ".." && !strings.HasPrefix(clean, "../")
}

func validateManifestEntryFile(dir, entry string) error {
	root, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	candidate := filepath.Join(root, filepath.FromSlash(entry))
	rel, err := filepath.Rel(root, candidate)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("путь выходит за пределы каталога плагина")
	}
	info, err := os.Stat(candidate)
	if err != nil {
		return fmt.Errorf("файл не найден: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("entry указывает на каталог")
	}
	rootReal, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	candidateReal, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return err
	}
	realRel, err := filepath.Rel(rootReal, candidateReal)
	if err != nil || realRel == ".." || strings.HasPrefix(realRel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("symlink выходит за пределы каталога плагина")
	}
	return nil
}

func parseManifestVersion(value string) ([3]int, bool) {
	value = strings.TrimSpace(value)
	value = strings.SplitN(value, "+", 2)[0]
	value = strings.SplitN(value, "-", 2)[0]
	parts := strings.Split(value, ".")
	if len(parts) == 2 {
		parts = append(parts, "0")
	}
	if len(parts) != 3 {
		return [3]int{}, false
	}
	var parsed [3]int
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return [3]int{}, false
		}
		parsed[i] = n
	}
	return parsed, true
}

func compareManifestVersions(a, b [3]int) int {
	for i := range a {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	return 0
}

func platformAPICompatible(spec string) bool {
	current, ok := parseManifestVersion(PlatformAPI + ".0")
	if !ok {
		return false
	}
	spec = strings.TrimSpace(spec)
	if strings.HasPrefix(spec, "^") {
		required, ok := parseManifestVersion(strings.TrimPrefix(spec, "^"))
		if !ok {
			return false
		}
		if required[0] == 0 {
			return current[0] == 0 && current[1] == required[1]
		}
		return current[0] == required[0]
	}
	terms := strings.Fields(spec)
	if len(terms) == 0 {
		return false
	}
	for _, term := range terms {
		op := "="
		for _, candidate := range []string{">=", "<=", ">", "<", "="} {
			if strings.HasPrefix(term, candidate) {
				op = candidate
				term = strings.TrimSpace(strings.TrimPrefix(term, candidate))
				break
			}
		}
		required, ok := parseManifestVersion(term)
		if !ok {
			return false
		}
		comparison := compareManifestVersions(current, required)
		switch op {
		case ">=":
			if comparison < 0 {
				return false
			}
		case ">":
			if comparison <= 0 {
				return false
			}
		case "<=":
			if comparison > 0 {
				return false
			}
		case "<":
			if comparison >= 0 {
				return false
			}
		case "=":
			if comparison != 0 {
				return false
			}
		}
	}
	return true
}

func DecodeManifest(raw []byte, m *Manifest) error {
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	decoder.KnownFields(true)
	if err := decoder.Decode(m); err != nil {
		return err
	}
	var extra interface{}
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("манифест содержит несколько YAML-документов")
		}
		return err
	}
	return nil
}

func unmarshalYAML(raw []byte, m *Manifest) error {
	return DecodeManifest(raw, m)
}

func Lint(pf *PipelineFile, eng Engine, projectRoot string) (errs, warns []string) {
	errs, warns = Validate(pf, eng)
	if projectRoot == "" {
		projectRoot, _ = os.Getwd()
	}
	for _, st := range pf.Pipeline.Steps {
		if IsBuiltin(st.Plugin) {
			continue
		}
		m, err := eng.LoadManifest(st.Plugin)
		if err != nil {
			continue
		}
		for portName, port := range m.Input {
			if port.Format != "file_ref" {
				continue
			}
			srcPath := PortSource(portName, port, &st)
			if srcPath == "" {
				continue
			}
			if !strings.HasPrefix(srcPath, "input.") {
				continue
			}
			key := strings.TrimPrefix(srcPath, "input.")
			if idx := strings.Index(key, "."); idx >= 0 {
				key = key[:idx]
			}
			rawVal, ok := pf.Pipeline.Input[key]
			if !ok {
				continue
			}
			s, ok := rawVal.(string)
			if !ok || s == "" {
				continue
			}
			if filepath.IsAbs(s) {
				if _, err := os.Stat(s); err != nil {
					errs = append(errs, fmt.Sprintf("шаг %s, порт %s (file_ref): файл %q не найден (abs): %v", st.ID, portName, s, err))
				}
				continue
			}
			pluginAbs, _ := filepath.Abs(st.Plugin)
			if _, err := os.Stat(filepath.Join(pluginAbs, s)); err == nil {
				continue
			}
			if _, err := os.Stat(filepath.Join(projectRoot, s)); err == nil {
				warns = append(warns, fmt.Sprintf("шаг %s, порт %s (file_ref): %q найден от корня проекта, но не от плагина (%s) — укажите путь относительно плагина или абсолютный", st.ID, portName, s, pluginAbs))
				continue
			}
			errs = append(errs, fmt.Sprintf("шаг %s, порт %s (file_ref): файл %q не найден ни от плагина (%s) ни от корня (%s)", st.ID, portName, s, pluginAbs, projectRoot))
		}
	}
	return errs, warns
}

func DetectCycle(pf *PipelineFile) string {
	adj := map[string][]string{}
	nodes := map[string]bool{}
	for _, st := range pf.Pipeline.Steps {
		nodes[st.ID] = true
		adj[st.ID] = []string{}
	}
	for _, st := range pf.Pipeline.Steps {
		deps := map[string]bool{}
		for _, v := range st.Bind {
			if strings.HasPrefix(v, "steps.") {
				parts := strings.Split(v, ".")
				if len(parts) >= 2 {
					deps[parts[1]] = true
				}
			}
		}
		for _, f := range st.Form {
			if strings.HasPrefix(f.Field, "steps.") {
				parts := strings.Split(f.Field, ".")
				if len(parts) >= 2 {
					id := parts[1]
					if strings.HasSuffix(id, "_all") {
						id = strings.TrimSuffix(id, "_all")
					}
					deps[id] = true
				}
			}
		}
		for dep := range deps {
			if dep == st.ID {
				return fmt.Sprintf("цикл: шаг %s зависит от самого себя (%v)", st.ID, st.Bind)
			}
			if _, ok := nodes[dep]; ok {
				adj[dep] = append(adj[dep], st.ID)
			}
		}
	}
	visited := map[string]int{}
	var stack []string
	var cyclePath []string

	var dfs func(string) bool
	dfs = func(u string) bool {
		visited[u] = 1
		stack = append(stack, u)
		for _, v := range adj[u] {
			if visited[v] == 0 {
				if dfs(v) {
					return true
				}
			} else if visited[v] == 1 {
				idx := -1
				for i, n := range stack {
					if n == v {
						idx = i
						break
					}
				}
				if idx >= 0 {
					cyclePath = append([]string{}, stack[idx:]...)
					cyclePath = append(cyclePath, v)
				} else {
					cyclePath = []string{v, u, v}
				}
				return true
			}
		}
		visited[u] = 2
		stack = stack[:len(stack)-1]
		return false
	}

	for id := range nodes {
		if visited[id] == 0 {
			if dfs(id) {
				return fmt.Sprintf("цикл в DAG: %s", strings.Join(cyclePath, " → "))
			}
		}
	}
	return ""
}
