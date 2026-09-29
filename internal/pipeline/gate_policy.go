package pipeline

import (
	"strings"

	"wedra/internal/common"
)

// Требование одобрения человека перед опасным шагом.
//
// Проблема, из-за которой файл существует. Раньше MCP проверял только одно:
// «если в пайплайне есть human_gate, нужен ли канал к человеку». Обратное —
// «а если human_gate нет вовсе» — не проверялось, и агент мог отправить
// run_pipeline с пайплайном, где ни одного гейта нет. Ран исполнялся целиком,
// и ничего в нём не происходило без ведома человека. Формулировка «у агента нет
// кнопки approve» оказывалась верной только по счастливой случайности: гейт
// можно было просто не включить.
//
// Правило здесь одно и намеренно узкое: ПЕРВЫЙ шаг, который что-то опасное
// делает, обязан идти после human_gate. Опасность определяется по объявленным
// capabilities плагина (permissions манифеста) — это единственный источник
// правды о шаге, доступный до запуска.
//
// Чего правило НЕ делает (границы, а не мелочи):
//
//   - Оно не измеряет поведение кода. permissions — декларация, а не песочница:
//     плагин, заявивший filesystem: none, всё равно может писать куда угодно
//     правами пользователя. Экран ловит заявленное намерение, а не фактическое
//     действие. Против плагина, который врёт в манифесте, работает другое —
//     sandbox: untrusted и изолятор ОС (см. SECURITY.md).
//   - Оно не оценивает when/foreach: шаг за `when` может и не выполниться, но
//     статически это не известно, поэтому шаг считается выполняющимся. Лишний
//     гейт (он не понадобился) безопаснее пропущенного.

// Capabilities — объявленные права шага: то, что он может сделать с миром.
type Capabilities struct {
	// Network — плагин объявил permissions.network (выход в сеть).
	Network bool
	// DiskWrite — плагин объявил filesystem, дающий право записи.
	DiskWrite bool
	// Secrets — плагин объявил permissions.secrets и получает их в env.
	Secrets bool
	// Unknown — манифест не прочитан, поэтому права неизвестны. Считается
	// опасным: незнание прав — не доказательство их отсутствия.
	Unknown bool
}

// Dangerous — требует ли такой набор прав одобрения человека.
func (c Capabilities) Dangerous() bool {
	return c.Network || c.DiskWrite || c.Secrets || c.Unknown
}

// Why — человекочитаемый список прав одной строкой. Пусто, только если прав нет.
func (c Capabilities) Why() string {
	var parts []string
	if c.Network {
		parts = append(parts, "сеть")
	}
	if c.DiskWrite {
		parts = append(parts, "запись на диск")
	}
	if c.Secrets {
		parts = append(parts, "чтение секретов")
	}
	if c.Unknown {
		parts = append(parts, "неизвестно: манифест не прочитан")
	}
	return strings.Join(parts, ", ")
}

// Значения permissions.filesystem, которыми плагин получает право ПИСАТЬ.
// `read`, `none` и незаданное поле таким правом не обладают.
const (
	fsWorkspace = "workspace"
	fsWrite     = "write"
	fsReadWrite = "readwrite"
)

// FilesystemWrites — объявляет ли filesystem право записи.
//
// `workspace` включён рядом с write/readwrite не по симметрии, а по смыслу:
// объявление означает «доступ к рабочей папке», и собственный фикстурный
// плагин retry_flaky пишет `_counter` рядом с собой именно с
// `filesystem: workspace`. Считать это «чтением» значило бы приписать
// манифесту то, чего в нём нет.
func FilesystemWrites(fs string) bool {
	switch fs {
	case fsWorkspace, fsWrite, fsReadWrite:
		return true
	}
	return false
}

// StepCapabilities — объявленные права шага по манифесту его плагина.
// Встроенные модули (core/*) — доверенный код в процессе ядра: прав у них нет
// по построению, и пустой набор для них — правда, а не поблажка.
func StepCapabilities(st *Step, m *Manifest) Capabilities {
	if st == nil || m == nil {
		return Capabilities{Unknown: true}
	}
	if IsBuiltin(st.Plugin) {
		return Capabilities{}
	}
	return ManifestCapabilities(m)
}

// ManifestCapabilities — объявленные права по одному манифесту плагина.
//
// Существует отдельно от StepCapabilities, потому что то же самое нужно там,
// где шага нет вовсе: инструмент MCP exec_plugin исполняет плагин напрямую,
// минуя пайплайн. Определение «что опасно» должно быть ОДНО: если каждая
// проверка объявит своё, они разъедутся, и разъедутся тихо — одна из них
// начнёт считать безопасным то, что другая считает опасным.
func ManifestCapabilities(m *Manifest) Capabilities {
	if m == nil {
		return Capabilities{Unknown: true}
	}
	return Capabilities{
		Network:   len(m.Permissions.Network) > 0,
		DiskWrite: FilesystemWrites(m.Permissions.Filesystem),
		Secrets:   len(m.Permissions.Secrets) > 0,
	}
}

// GateRequirement — вердикт проверки «нужно ли одобрение человека».
type GateRequirement struct {
	// Required — первый опасный шаг не защищён гейтом.
	Required bool
	// Step, Plugin — опасный шаг, из-за которого понадобилось одобрение.
	Step   string
	Plugin string
	// Why — права этого шага одной строкой (для сообщения агенту).
	Why string
	// GateID — id гейта, если он в пайплайне есть, но стоит ПОСЛЕ опасного
	// шага. Такой гейт не защищает: человек увидит результат, а не намерение.
	GateID string
	// GateAfter — GateID стоит после опасного шага (а не до и не вовсе).
	GateAfter bool
}

// IsHumanGate — встроенный модуль ИМЕННО гейт человека.
//
// Отдельно от IsBuiltin, который истинен и для core/text_stats: text_stats —
// не гейт. Считать его гейтом значило бы признать опасный пайплайн
// одобренным из-за шага, который ничего не одобряет.
func IsHumanGate(ref string) bool {
	canonical, ok := common.CanonicalBuiltinRef(ref)
	return ok && canonical == common.HumanGatePluginRef
}

// EvaluateGateApproval — нужен ли гейт перед первым опасным шагом пайплайна.
//
// Решение принимает ПЕРВЫЙ опасный шаг: гейт после него ничего не решает.
// Если опасных шагов нет, одобрение не требуется — иначе правило было бы
// слишком широким (агент не смог бы запустить, например, подсчёт слов).
func EvaluateGateApproval(pf *PipelineFile, eng Engine) GateRequirement {
	if pf == nil {
		return GateRequirement{}
	}
	// Гейт ищется по всему списку, а не «до первого опасного»: гейт ПОСЛЕ
	// опасного шага ничего не одобряет, и агенту полезно сказать об этом
	// прямо («переставь гейт выше»), а не просто «гейта нет».
	firstGate := -1
	for i := range pf.Pipeline.Steps {
		if IsHumanGate(pf.Pipeline.Steps[i].Plugin) {
			firstGate = i
			break
		}
	}
	for i := range pf.Pipeline.Steps {
		st := &pf.Pipeline.Steps[i]
		if IsHumanGate(st.Plugin) {
			continue
		}
		caps := stepCapsOrUnknown(st, eng)
		if !caps.Dangerous() {
			continue
		}
		// Гейт выше по списку отработал бы до этого шага — одобрение получено.
		if firstGate >= 0 && firstGate < i {
			return GateRequirement{}
		}
		req := GateRequirement{Required: true, Step: st.ID, Plugin: st.Plugin, Why: caps.Why()}
		if firstGate > i {
			req.GateID = pf.Pipeline.Steps[firstGate].ID
			req.GateAfter = true
		}
		return req
	}
	return GateRequirement{}
}

// stepCapsOrUnknown — права шага, а при нечитаемом манифесте — «неизвестно».
// Отказ по умолчанию: не удалось доказать, что шаг безобиден.
func stepCapsOrUnknown(st *Step, eng Engine) Capabilities {
	if eng == nil {
		return Capabilities{Unknown: true}
	}
	m, err := eng.LoadManifest(st.Plugin)
	if err != nil {
		return Capabilities{Unknown: true}
	}
	return StepCapabilities(st, m)
}
