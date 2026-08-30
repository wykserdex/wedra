package gate

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"orchestrator/internal/common"
	"orchestrator/internal/context"
	"orchestrator/internal/journal"
	"orchestrator/internal/pipeline"
)

type GateOptions struct {
	Yes   bool
	Quiet bool
}

// GateUI — канал ввода human_gate.
// v0.15: шов для GUI/API (M6) — терминал текущая реализация, подставить
// свою можно без изменения Service.
type GateUI interface {
	ReadLine() (string, error)
}

// StdinUI — читает из os.Stdin (дефолт).
// Один bufio.Reader на весь гейт: новый на каждый ReadLine съел бы
// забференный остаток предыдущей строки.
type StdinUI struct {
	reader *bufio.Reader
}

func NewStdinUI() *StdinUI { return &StdinUI{reader: bufio.NewReader(os.Stdin)} }

func (u *StdinUI) ReadLine() (string, error) {
	if u.reader == nil {
		u.reader = bufio.NewReader(os.Stdin)
	}
	return u.reader.ReadString('\n')
}

type Service struct {
	UI GateUI
}

func NewService() *Service { return &Service{UI: NewStdinUI()} }

// NewServiceWithUI — для GUI/API: заменить терминал своим вводом.
func NewServiceWithUI(ui GateUI) *Service {
	if ui == nil {
		return NewService()
	}
	return &Service{UI: ui}
}

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
				fmt.Printf("  %s %s = %s\n", mark, f.Field, common.Truncate(string(b), 500))
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

	ui := s.UI
	if ui == nil {
		ui = NewStdinUI()
	}
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
		line, rerr := ui.ReadLine()
		if rerr != nil {
			fmt.Println("  ! ввод закрыт (EOF) — перехожу к действию")
			break
		}
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
	action := ""
	for attempt := 0; attempt < 5; attempt++ {
		fmt.Printf("  действие [%s]: ", strings.Join(keys, "/"))
		ans, rerr := ui.ReadLine()
		if rerr != nil {
			// v0.23: EOF — не «accept». Гейт — это человек; закрытый ввод
			// трактуем как остановку рана, не как молчаливое одобрение.
			fmt.Println("  ! ввод закрыт (EOF) — гейт: стоп")
			j.Event("gate_decision", map[string]interface{}{"step": st.ID, "action": "stop", "reason": "EOF (ввод закрыт)"})
			return "abort_item"
		}
		ans = strings.ToLower(strings.TrimSpace(ans))
		if ans == "" {
			fmt.Println("  ! пусто — введи одно из: " + strings.Join(actions, "/"))
			continue
		}
		matched := false
		for i, a := range actions {
			if ans == a || ans == keys[i] {
				action = a
				matched = true
				break
			}
		}
		if !matched {
			fmt.Printf("  ! %q не из списка — введи: %s\n", ans, strings.Join(actions, "/"))
			continue
		}
		break
	}
	if action == "" {
		// 5 мусорных попыток — тоже не «accept»
		fmt.Println("  ! не удалось распознать действие — гейт: стоп")
		j.Event("gate_decision", map[string]interface{}{"step": st.ID, "action": "stop", "reason": "нераспознанный ввод (5 попыток)"})
		return "abort_item"
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
