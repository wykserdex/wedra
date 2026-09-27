package common

import (
	"reflect"
	"strings"
	"unicode/utf8"
)

// Встроенные модули. В отличие от плагинов это доверенный код, исполняемый
// в процессе ядра: никакой subprocess-модели, манифеста на диске и песочницы.
// Поэтому список короткий и закрытый — добавлять сюда модуль с доступом к
// файлам или сети нельзя, это осознанная граница (см. SECURITY.md).
const (
	HumanGatePluginRef = "core/human_gate"
	TextStatsPluginRef = "core/text_stats"
)

// builtinRefs — закрытый реестр встроенных модулей. Порядок не важен.
var builtinRefs = map[string]bool{
	HumanGatePluginRef: true,
	TextStatsPluginRef: true,
}

func CanonicalBuiltinRef(ref string) (string, bool) {
	normalized := strings.ReplaceAll(strings.TrimSpace(ref), `\`, "/")
	if builtinRefs[normalized] {
		return normalized, true
	}
	return "", false
}

func IsBuiltinRef(ref string) bool {
	_, ok := CanonicalBuiltinRef(ref)
	return ok
}

func IsBuiltinNamespace(ref string) bool {
	normalized := strings.ReplaceAll(strings.TrimSpace(ref), `\`, "/")
	return normalized == "core" || strings.HasPrefix(normalized, "core/")
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

func Basename(path string) string {
	parts := strings.Split(path, ".")
	return parts[len(parts)-1]
}

// Truncate — обрезает строку до n байт, не разрывая UTF-8 символ.
// v0.15: rune-safe (было s[:n] — на кириллице могло срезать символ пополам).
func Truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n] + "…"
}

func DeepEqual(a, b interface{}) bool {
	return reflect.DeepEqual(a, b)
}

func ExtractStepID(path string) string {
	// steps.<id>.<field> → <id>
	parts := strings.Split(path, ".")
	if len(parts) >= 2 && parts[0] == "steps" {
		return parts[1]
	}
	return ""
}
