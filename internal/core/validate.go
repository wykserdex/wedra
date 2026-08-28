package core

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Ранги форматов: consumer принимает producer не слабее себя.
// string ⊇ text ⊇ {email, url, ip}; ветки между собой несовместимы.
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
	Step    *Step        // nil для input.*
	Literal interface{}  // значение-литерал для input.* (для статической проверки формата)
}

var formatCheckers = map[string]func(string) bool{
	"email": func(s string) bool {
		return regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`).MatchString(s)
	},
	"url": func(s string) bool { return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") },
	"ip":  func(s string) bool { return net.ParseIP(s) != nil },
	"text": func(s string) bool { return true },
}

// scalarMatchesFormat: константное input-значение обязано удовлетворять формату порта —
// опечатку в email ловим ещё до запуска.
func scalarMatchesFormat(lit interface{}, format string) bool {
	chk, ok := formatCheckers[format]
	if !ok {
		return true // неизвестный/enum-формат — не проверяем статически
	}
	s, ok := lit.(string)
	if !ok {
		return false
	}
	return chk(s)
}

// resolveSource проверяет путь from и возвращает информацию об источнике.
func resolveSource(path string, prior map[string]priorStep, pf *PipelineFile) (srcInfo, string) {
	parts := strings.Split(path, ".")
	if len(parts) < 2 {
		return srcInfo{}, "путь слишком короткий: " + path
	}
	p := pf.Pipeline
	switch parts[0] {
	case "input":
		key := parts[1]
		itemKey := p.ForeachItem
		if itemKey == "" {
			itemKey = "item"
		}
		if key == itemKey && p.Foreach != "" {
			// элемент foreach: тип объявляется в пайплайне (item_type/item_format)
			return srcInfo{Name: path, Type: p.ItemType, Format: p.ItemFormat}, ""
		}
		val, ok := p.Input[key]
		if !ok {
			return srcInfo{}, "в input пайплайна нет поля " + key
		}
		return srcInfo{Name: path, Type: kindOf(val), Literal: val}, ""
	case "steps":
		if len(parts) < 3 {
			return srcInfo{}, "ожидается путь steps.<step_id>.<поле>: " + path
		}
		sid, field := parts[1], parts[2]
		ps, ok := prior[sid]
		if !ok {
			return srcInfo{}, "шаг " + sid + " не найден выше по цепочке"
		}
		if IsBuiltin(ps.step.Plugin) {
			// human_gate материализует поля с типом "всё что угодно" — wildcard
			return srcInfo{Name: path, Step: ps.step, Format: "any"}, ""
		}
		port, ok := ps.manifest.Output[field]
		if !ok {
			return srcInfo{}, fmt.Sprintf("плагин %s не объявляет выход %q", ps.manifest.ID, field)
		}
		return srcInfo{Name: path, Type: port.Type, Format: port.Format, Step: ps.step}, ""
	default:
		return srcInfo{}, "путь должен начинаться с input. или steps.: " + path
	}
}

// Validate — статические проверки цепочки до запуска (PROTOCOL.md §6).
// errs блокируют запуск, warns — нет.
func Validate(pf *PipelineFile, eng *Engine) (errs, warns []string) {
	if pf.FormatVersion != "0.1" && pf.FormatVersion != "0.2" {
		warns = append(warns, fmt.Sprintf("format_version %q не из списка поддерживаемых: 0.1, 0.2", pf.FormatVersion))
	}
	p := &pf.Pipeline

	if p.Foreach != "" {
		key := strings.TrimPrefix(p.Foreach, "input.")
		if key == p.Foreach {
			errs = append(errs, "foreach: путь должен начинаться с input.")
		} else if _, ok := p.Input[key]; !ok {
			errs = append(errs, "foreach: массив "+p.Foreach+" не найден в input")
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

		switch st.OnError {
		case "", "stop", "skip", "retry":
		default:
			errs = append(errs, "шаг "+st.ID+": on_error="+st.OnError+", ожидается stop|skip|retry")
		}
		if st.OnError == "retry" && st.Retry != nil && st.Retry.Attempts < 1 {
			errs = append(errs, "шаг "+st.ID+": retry.attempts < 1")
		}

		if IsBuiltin(st.Plugin) {
			if len(st.Bind) > 0 {
				errs = append(errs, "шаг "+st.ID+": human_gate не принимает bind — его контракт это form")
			}
			switch st.OnReject {
			case "", "stop", "continue":
			default:
				errs = append(errs, "шаг "+st.ID+": on_reject="+st.OnReject+", ожидается stop|continue")
			}
			for _, f := range st.Form {
				if src, e := resolveSource(f.Field, prior, pf); e != "" {
					// если поле читает из skip-able шага — это warning, а не ошибка
					warns = append(warns, fmt.Sprintf("шаг %s, form: %s — поле может отсутствовать в рантайме", st.ID, e))
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

		// bind: только на существующие порты плагина
		for b := range st.Bind {
			if _, ok := m.Input[b]; !ok {
				errs = append(errs, fmt.Sprintf("шаг %s: bind указывает на несуществующий порт %q (порты плагина: %s)",
					st.ID, b, portNames(m.Input)))
			}
		}

		for portName, port := range m.Input {
			srcPath := portSource(portName, port, st)
			if srcPath == "" {
				// v0.2: from можно опустить, но тогда пайплайн ОБЯЗАН дать bind
				errs = append(errs, fmt.Sprintf("шаг %s, порт %s: нет привязки (ни bind в шаге, ни from в манифесте)", st.ID, portName))
				continue
			}
			src, perr := resolveSource(srcPath, prior, pf)
			if perr != "" {
				if port.Optional {
					warns = append(warns, fmt.Sprintf("шаг %s, порт %s: %s (optional — допустимо)", st.ID, portName, perr))
				} else {
					errs = append(errs, fmt.Sprintf("шаг %s, порт %s: %s", st.ID, portName, perr))
				}
				continue
			}
			src.Name = srcPath
			if src.Type != "" && port.Type != "" && src.Type != port.Type {
				errs = append(errs, fmt.Sprintf("шаг %s, порт %s: тип %s несовместим с выходом %q (%s)",
					st.ID, portName, port.Type, src.Name, src.Type))
			}
			// Формат: для литерального input — прямая проверка значения
			// (опечатку в email ловим до запуска); иначе — сужение рангов.
			literalChecked := false
			if src.Literal != nil && port.Format != "" {
				if s, isStr := src.Literal.(string); isStr {
					literalChecked = true
					if !scalarMatchesFormat(s, port.Format) {
						errs = append(errs, fmt.Sprintf("шаг %s, порт %s: значение input %q не соответствует формату %q",
							st.ID, portName, s, port.Format))
					}
				}
			}
			if !literalChecked && !formatsCompatible(src.Format, port.Format) {
				errs = append(errs, fmt.Sprintf("шаг %s, порт %s: формат источника %q не покрывает требуемый %q",
					st.ID, portName, src.Format, port.Format))
			}
			// Skip-безопасность: читать из skip-able шага можно только optional-входом
			if src.Step != nil && src.Step.OnError == "skip" && !port.Optional {
				errs = append(errs, fmt.Sprintf("шаг %s, порт %s: читает из skip-able шага %s — объявите вход optional: true",
					st.ID, portName, src.Step.ID))
			}
		}
		prior[st.ID] = priorStep{step: st, manifest: m}
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

// ValidatePluginDir — заготовка `tool plugin validate` из SDK.
// checkPortFormats — M5, тестер №3 (второй круг): манифест глотал
// `format: json` на порте с `type: array`, и автор получал наказание только на
// уровне пайплайна чужим языком («формат источника "" не покрывает "json"»).
// format — подтип СТРОК; проверяем на месте, в plugin validate:
//  1. format на не-string порте → ошибка с объяснением и путём исправления;
//  2. неизвестный format → ошибка со списком допустимых.
func checkPortFormats(pfx, name string, port Port, errs []string) []string {
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
	var errs []string
	m, err := NewEngine().LoadManifest(dir)
	if err != nil {
		return append(errs, err.Error())
	}
	if m.Version == "" {
		errs = append(errs, "нет version")
	}
	if m.PlatformAPI == "" {
		errs = append(errs, "нет platform_api")
	}
	switch m.Runtime.Type {
	case "python", "binary":
	default:
		errs = append(errs, "runtime.type неизвестен: "+m.Runtime.Type)
	}
	if m.Runtime.Entry == "" {
		errs = append(errs, "runtime.entry пуст")
	} else if _, err := os.Stat(filepath.Join(dir, m.Runtime.Entry)); err != nil {
		errs = append(errs, "entry не найден: "+m.Runtime.Entry)
	}
	for name, port := range m.Input {
		// from может быть опущен (v0.2): тогда привязку обязан дать пайплайн через bind
		if port.Type == "" {
			errs = append(errs, "input "+name+": нет type")
		}
		errs = checkPortFormats("input", name, port, errs)
	}
	for name, port := range m.Output {
		errs = checkPortFormats("output", name, port, errs)
	}
	if len(m.Output) == 0 {
		errs = append(errs, "output пуст — плагин ничего не пишет в контекст")
	}
	return errs
}
