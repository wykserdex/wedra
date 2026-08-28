package gate

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"orchestrator/internal/context"
	"orchestrator/internal/journal"
	"orchestrator/internal/pipeline"
)

type GateOptions struct {
	Yes   bool
	Quiet bool
}

type Service struct{}

func NewService() *Service { return &Service{} }

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

func basename(path string) string {
	parts := strings.Split(path, ".")
	return parts[len(parts)-1]
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

func (s *Service) Materialize(form []pipeline.FormField, ctx *context.Ctx, edits map[string]interface{}) map[string]interface{} {
	return gateMaterialize(form, ctx, edits)
}

func gateMaterialize(form []pipeline.FormField, ctx *context.Ctx, edits map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	bnCount := map[string]int{}
	for _, f := range form {
		bnCount[basename(f.Field)]++
	}
	for _, f := range form {
		bn := basename(f.Field)
		key := bn
		if bnCount[bn] > 1 {
			parts := strings.Split(f.Field, ".")
			if len(parts) >= 3 && parts[0] == "steps" {
				key = parts[1] + "_" + bn
			} else {
				key = strings.ReplaceAll(f.Field, ".", "_")
			}
		}
		if v, ok := edits[key]; ok {
			out[key] = v
			continue
		}
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

func (s *Service) Run(st *pipeline.Step, ctx *context.Ctx, j *journal.Journal, opts GateOptions) string {
	if !opts.Quiet {
		fmt.Printf("\n══ human_gate · %s ══\n", st.ID)
		for _, f := range st.Form {
			mark := " "
			if f.Editable {
				mark = "*"
			}
			if v, ok := ctx.Get(f.Field); ok {
				b, _ := json.Marshal(v)
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
		m := gateMaterialize(st.Form, ctx, nil)
		if len(m) > 0 {
			ctx.SetStep(st.ID, m)
		}
		j.Event("gate_decision", map[string]interface{}{"step": st.ID, "action": "accept", "auto": true, "materialized": m})
		return "ok"
	}

	reader := bufio.NewReader(os.Stdin)
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
		edits[key] = v
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
	m := gateMaterialize(st.Form, ctx, edits)
	if len(m) > 0 {
		ctx.SetStep(st.ID, m)
	}
	j.Event("gate_decision", map[string]interface{}{"step": st.ID, "action": "accept", "edits": edits, "materialized": m})
	return "ok"
}
