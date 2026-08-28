package common

import (
	"reflect"
	"strings"
)

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

func Truncate(s string, n int) string {
	if len(s) <= n {
		return s
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
