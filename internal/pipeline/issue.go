package pipeline

// Severity — уровень проблемы: error блокирует запуск, warning — нет.
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
)

// Fix — машинная подсказка для агента: что делать с проблемой.
// Op: "bind" — перепривязать порт на один из Candidates,
//
//	"set" — выставить значение Target,
//	"declare" — объявить недостающее (secrets, поле input).
//
// Target: "steps.<id>.<port>" | "input.<field>" | "pipeline.<field>".
// Candidates: отсортированный список допустимых значений.
type Fix struct {
	Op         string   `json:"op"`
	Target     string   `json:"target,omitempty"`
	Candidates []string `json:"candidates,omitempty"`
}

// Issue — структурная ошибка валидации. Code — стабильный публичный
// контракт (см. protocol/v0.2/ERRORS.md): тексты Message можно менять,
// коды нельзя. Path — JSON-путь проблемы: "steps.syntax.syntax",
// "input.emails[3]", "pipeline.foreach".
type Issue struct {
	Code     string   `json:"code"`
	Severity Severity `json:"severity"`
	Step     string   `json:"step,omitempty"`
	Port     string   `json:"port,omitempty"`
	Path     string   `json:"path,omitempty"`
	Message  string   `json:"message"`
	Hint     string   `json:"hint,omitempty"`
	Fix      *Fix     `json:"fix,omitempty"`
}

// IsError — удобный фильтр.
func (i Issue) IsError() bool { return i.Severity == SeverityError }

// SplitIssues — совместимость со старым API (errs, warns []string):
// GUI-редактор и старые тесты продолжают работать на строках.
func SplitIssues(issues []Issue) (errs, warns []string) {
	for _, is := range issues {
		if is.Severity == SeverityError {
			errs = append(errs, is.Message)
		} else {
			warns = append(warns, is.Message)
		}
	}
	return errs, warns
}

// FilterCode — все проблемы с данным кодом (для тестов TestIssueCodes).
func FilterCode(issues []Issue, code string) []Issue {
	var out []Issue
	for _, is := range issues {
		if is.Code == code {
			out = append(out, is)
		}
	}
	return out
}

// Коды ошибок валидации — публичный контракт (protocol/v0.2/ERRORS.md).
// Тексты Message менять можно, коды — нельзя.
const (
	E_FORMAT_VERSION         = "E_FORMAT_VERSION"
	E_CYCLE                  = "E_CYCLE"
	E_FOREACH_PATH           = "E_FOREACH_PATH"
	E_FOREACH_NOT_FOUND      = "E_FOREACH_NOT_FOUND"
	E_FOREACH_SHAPE          = "E_FOREACH_SHAPE"
	E_STEP_ID_EMPTY          = "E_STEP_ID_EMPTY"
	E_STEP_ID_DUP            = "E_STEP_ID_DUP"
	E_WHEN_OP                = "E_WHEN_OP"
	E_WHEN_PATH              = "E_WHEN_PATH"
	E_WHEN_FORWARD_REF       = "E_WHEN_FORWARD_REF"
	E_STEP_FOREACH_PATH      = "E_STEP_FOREACH_PATH"
	E_STEP_FOREACH_SHAPE     = "E_STEP_FOREACH_SHAPE"
	E_STEP_FOREACH_NOT_FOUND = "E_STEP_FOREACH_NOT_FOUND"
	E_FOREACH_COMBO          = "E_FOREACH_COMBO"
	E_FOREACH_ITEM_NAME      = "E_FOREACH_ITEM_NAME"
	E_GATE_FOREACH           = "E_GATE_FOREACH"
	E_GATE_PARALLEL          = "E_GATE_PARALLEL"
	E_GATE_BIND              = "E_GATE_BIND"
	E_GATE_ACTIONS           = "E_GATE_ACTIONS"
	E_ON_ERROR               = "E_ON_ERROR"
	E_RETRY_ATTEMPTS         = "E_RETRY_ATTEMPTS"
	E_ON_REJECT              = "E_ON_REJECT"
	E_PLUGIN_LOAD            = "E_PLUGIN_LOAD"
	E_BIND_UNKNOWN_PORT      = "E_BIND_UNKNOWN_PORT"
	E_NETWORK_DENIED         = "E_NETWORK_DENIED"
	E_PORT_UNBOUND           = "E_PORT_UNBOUND"
	E_PORT_SOURCE            = "E_PORT_SOURCE"
	E_TYPE_MISMATCH          = "E_TYPE_MISMATCH"
	E_FORMAT_INPUT           = "E_FORMAT_INPUT"
	E_FORMAT_MISMATCH        = "E_FORMAT_MISMATCH"
	E_OPTIONAL_REQUIRED      = "E_OPTIONAL_REQUIRED"
	E_PARALLEL_SPLIT         = "E_PARALLEL_SPLIT"
	E_FILE_REF_NOT_FOUND     = "E_FILE_REF_NOT_FOUND"
	E_MANIFEST_VERSION       = "E_MANIFEST_VERSION"
	E_MANIFEST_PLATFORM_API  = "E_MANIFEST_PLATFORM_API"
	E_MANIFEST_RUNTIME       = "E_MANIFEST_RUNTIME"
	E_MANIFEST_ENTRY         = "E_MANIFEST_ENTRY"
	E_MANIFEST_INPUT_TYPE    = "E_MANIFEST_INPUT_TYPE"
	E_MANIFEST_FORMAT        = "E_MANIFEST_FORMAT"
	E_MANIFEST_OUTPUT_EMPTY  = "E_MANIFEST_OUTPUT_EMPTY"
	E_APPROVAL_VALUE         = "E_APPROVAL_VALUE"
	E_GATES_VALUE            = "E_GATES_VALUE"

	W_FORMAT_VERSION_MISSING = "W_FORMAT_VERSION_MISSING"
	W_SECRETS_MISSING_ENV    = "W_SECRETS_MISSING_ENV"
	W_FOREACH_ITEM_TYPE      = "W_FOREACH_ITEM_TYPE"
	W_FOREACH_ITEM_FORMAT    = "W_FOREACH_ITEM_FORMAT"
	W_GATE_FORM_COLLISION    = "W_GATE_FORM_COLLISION"
	W_GATE_FORM_MISSING      = "W_GATE_FORM_MISSING"
	W_GATE_FORM_SKIP         = "W_GATE_FORM_SKIP"
	W_NETWORK_DECLARED       = "W_NETWORK_DECLARED"
	W_PORT_OPTIONAL_UNBOUND  = "W_PORT_OPTIONAL_UNBOUND"
	W_PORT_OPTIONAL_SOURCE   = "W_PORT_OPTIONAL_SOURCE"
	W_SECRETS_UNUSED         = "W_SECRETS_UNUSED"
	W_SECRETS_UNDECLARED     = "W_SECRETS_UNDECLARED"
	W_PARALLEL_SINGLE        = "W_PARALLEL_SINGLE"
	W_FILE_REF_ROOT          = "W_FILE_REF_ROOT"
)
