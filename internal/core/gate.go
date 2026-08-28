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
func gateMaterialize(st *Step, ctx *Ctx, edits map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for _, f := range st.Form {
		name := basename(f.Field)
		if v, ok := edits[name]; ok {
			out[name] = v
			continue
		}
		if v, ok := ctx.Get(f.Field); ok {
			out[name] = v
		}
	}
	return out
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
				fmt.Printf("  %s %s = %s\n", mark, f.Field, truncate(string(b), 120))
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

	// правки editable-полей
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
		if f.Type != "" && kindOf(v) != f.Type {
			fmt.Printf("    ! тип %s не подходит под %s — правка пропущена\n", kindOf(v), f.Type)
			continue
		}
		edits[basename(f.Field)] = v // пишется под неймспейсом гейта, источник не затирается
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
