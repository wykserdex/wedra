package core

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

func kindOf(v interface{}) string {
	switch v.(type) {
	case bool:
		return "boolean"
	case float64:
		return "number"
	case string:
		return "string"
	case []interface{}:
		return "array"
	case map[string]interface{}:
		return "object"
	case nil:
		return "null"
	}
	return "unknown"
}

// gateMaterialize: на accept все поля формы материализуются под неймспейсом
// гейта (с правками поверх) — downstream ВСЕГДА читает из steps.<gate_id>,
// а не гадает, были правки или нет (PROTOCOL.md §7).
// Фикс M5-фидбек №1 (внешний автор, 2026-08-28): два поля с одинаковым basename
// (steps.before.file_count и steps.after.file_count) раньше затирали друг друга.
// Теперь при коллизии basename ключ = <step_id>_<basename> (before_file_count),
// что сохраняет данные и делает коллизию видимой. Без коллизии — старый flat-ключ.
func gateMaterialize(st *Step, ctx *Ctx, edits map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	// считаем коллизии basename
	bnCount := map[string]int{}
	for _, f := range st.Form {
		bnCount[basename(f.Field)]++
	}
	for _, f := range st.Form {
		bn := basename(f.Field)
		key := bn
		if bnCount[bn] > 1 {
			// квалифицируем именем источника: steps.<step_id>.<...>
			parts := strings.Split(f.Field, ".")
			if len(parts) >= 3 && parts[0] == "steps" {
				key = parts[1] + "_" + bn
			} else {
				// fallback для input.* или неожиданных путей
				key = strings.ReplaceAll(f.Field, ".", "_")
			}
		}
		if v, ok := edits[key]; ok {
			out[key] = v
			continue
		}
		// обратная совместимость: правка могла прийти под старым basename (до фикса)
		if v, ok := edits[bn]; ok && bnCount[bn] == 1 {
			out[key] = v
			continue
		}
		if v, ok := ctx.Get(f.Field); ok {
			out[key] = v
		}
	}
	return out
}

func extractStepID(field string) string {
	parts := strings.Split(field, ".")
	if len(parts) >= 3 && parts[0] == "steps" {
		return parts[1]
	}
	return ""
}

// runGate — встроенная нода core/human_gate, CLI-версия (PROTOCOL.md §7).
// Возвращает действие для раннера: "ok" (продолжить) или "abort_item".
func runGate(st *Step, ctx *Ctx, j *Journal, opts RunOptions) string {
	if !opts.Quiet {
		fmt.Printf("\n══ human_gate · %s ══\n", st.ID)
		for _, f := range st.Form {
			mark := " "
			if f.Editable {
				mark = "*"
			}
			if v, ok := ctx.Get(f.Field); ok {
				b, _ := json.Marshal(v)
				// v9.1: было 120 — резало reasons/массивы, теперь 500 (фидбек внешнего автора №4)
				fmt.Printf("  %s %s = %s\n", mark, f.Field, truncate(string(b), 500))
			} else {
				fmt.Printf("  %s %s = <нет данных>\n", mark, f.Field)
			}
		}
	}

	if opts.Yes {
		if !opts.Quiet {
			fmt.Println("  [--yes] auto-accept")
		}
		m := gateMaterialize(st, ctx, nil)
		if len(m) > 0 {
			ctx.SetStep(st.ID, m)
		}
		j.Event("gate_decision", map[string]interface{}{"step": st.ID, "action": "accept", "auto": true, "materialized": m})
		return "ok"
	}

	reader := bufio.NewReader(os.Stdin)

	// правки editable-полей — считаем коллизии, чтобы не затирать (фикс №1)
	bnCountForEdits := map[string]int{}
	for _, f := range st.Form {
		bnCountForEdits[basename(f.Field)]++
	}
	edits := map[string]interface{}{}
	for _, f := range st.Form {
		if !f.Editable {
			continue
		}
		fmt.Printf("  (*) новое значение для %s (JSON, Enter — оставить): ", f.Field)
		line, _ := reader.ReadString('\n')
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var v interface{}
		if err := json.Unmarshal([]byte(line), &v); err != nil {
			fmt.Println("    ! не JSON — правка пропущена")
			continue
		}
		// v0.12 fix #19: выводим тип из источника, если в form Type пустой
		expectedType := f.Type
		if expectedType == "" {
			if srcVal, ok := ctx.Get(f.Field); ok {
				expectedType = kindOf(srcVal)
			}
		}
		if expectedType != "" && kindOf(v) != expectedType {
			fmt.Printf("    ! тип %s не подходит под %s (выведен из %s) — правка пропущена\n", kindOf(v), expectedType, f.Field)
			continue
		}
		bn := basename(f.Field)
		key := bn
		if bnCountForEdits[bn] > 1 {
			parts := strings.Split(f.Field, ".")
			if len(parts) >= 3 && parts[0] == "steps" {
				key = parts[1] + "_" + bn
			} else {
				key = strings.ReplaceAll(f.Field, ".", "_")
			}
		}
		edits[key] = v // пишется под неймспейсом гейта, источник не затирается
	}

	actions := st.Actions
	if len(actions) == 0 {
		actions = []string{"accept", "reject"}
	}
	keys := make([]string, len(actions))
	for i, a := range actions {
		if a != "" {
			keys[i] = strings.ToLower(a[:1])
		}
	}
	fmt.Printf("  действие [%s]: ", strings.Join(keys, "/"))
	ans, _ := reader.ReadString('\n')
	ans = strings.ToLower(strings.TrimSpace(ans))
	action := actions[0]
	for i, a := range actions {
		if ans == a || ans == keys[i] {
			action = a
			break
		}
	}

	if action == "reject" {
		j.Event("gate_decision", map[string]interface{}{"step": st.ID, "action": "reject"})
		if st.OnReject == "" || st.OnReject == "stop" {
			return "abort_item"
		}
		return "ok"
	}
	m := gateMaterialize(st, ctx, edits)
	if len(m) > 0 {
		ctx.SetStep(st.ID, m)
	}
	j.Event("gate_decision", map[string]interface{}{"step": st.ID, "action": "accept", "edits": edits, "materialized": m})
	return "ok"
}

func basename(path string) string {
	parts := strings.Split(path, ".")
	return parts[len(parts)-1]
}
