package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type RunOptions struct {
	Yes     bool   // auto-accept human_gate (CI/демо)
	RunsDir string // var/runs
	Quiet   bool   // без консольного вывода (тесты)
	Resume  string // run_id для --resume
}

func (o RunOptions) logf(format string, a ...interface{}) {
	if !o.Quiet {
		fmt.Printf(format+"\n", a...)
	}
}

// RunStats — итог рана; на него опирается exit-код CLI (см. PROTOCOL.md §6).
type RunStats struct {
	OK      int
	Aborted int
	RunDir  string
}

func sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		}
		return '-'
	}, s)
}

func Run(pf *PipelineFile, eng *Engine, opts RunOptions) (RunStats, error) {
	var stats RunStats
	if opts.RunsDir == "" {
		opts.RunsDir = "var/runs"
	}
	// --resume: грузим предыдущий контекст и продолжаем
	var ctx *Ctx
	var startItemIdx int
	var runID string
	var j *Journal
	var err error

	if opts.Resume != "" {
		// resume: используем старый runDir как базу, но пишем в новый? Для простоты — продолжаем в той же директории
		// v0.12: resume продолжает в том же runDir, дописывая journal
		prevDir := filepath.Join(opts.RunsDir, opts.Resume)
		// загружаем context.json
		raw, err := os.ReadFile(filepath.Join(prevDir, "context.json"))
		if err != nil {
			return stats, fmt.Errorf("--resume %s: не читается context.json: %w", opts.Resume, err)
		}
		var data map[string]interface{}
		if err := json.Unmarshal(raw, &data); err != nil {
			return stats, fmt.Errorf("--resume %s: битый context.json: %w", opts.Resume, err)
		}
		ctx = &Ctx{Data: data}
		// находим последний item_end
		eventsRaw, _ := os.ReadFile(filepath.Join(prevDir, "journal.jsonl"))
		lines := strings.Split(string(eventsRaw), "\n")
		maxIdx := -1
		for _, line := range lines {
			if strings.Contains(line, "\"type\":\"item_end\"") || strings.Contains(line, "\"type\": \"item_end\"") {
				// парсим item_index
				var ev map[string]interface{}
				if err := json.Unmarshal([]byte(line), &ev); err == nil {
					if idx, ok := ev["item_index"].(float64); ok {
						if int(idx) > maxIdx {
							maxIdx = int(idx)
						}
					}
				}
			}
		}
		startItemIdx = maxIdx + 1
		runID = opts.Resume
		j, err = OpenJournalAppend(prevDir)
		if err != nil {
			return stats, err
		}
		stats.RunDir = j.Dir
		opts.logf("▶ resume %q с элемента %d (журнал: %s)", pf.Pipeline.Name, startItemIdx, j.Dir)
		j.Event("run_resumed", map[string]interface{}{"from_item": startItemIdx})
	} else {
		runID = time.Now().Format("20060102-150405") + "-" + sanitize(pf.Pipeline.Name)
		j, err = NewJournal(filepath.Join(opts.RunsDir, runID))
		if err != nil {
			return stats, err
		}
		defer j.Close()
		stats.RunDir = j.Dir
		opts.logf("▶ запуск %q  (журнал: %s)", pf.Pipeline.Name, j.Dir)
		j.Event("run_start", map[string]interface{}{"pipeline": pf.Pipeline.Name})
		ctx = NewCtx(pf.Pipeline.Input)
	}

	// v0.12: foreach может быть input.* ИЛИ steps.<id>.<field>
	var items []interface{}
	var preSteps []*Step
	var foreachSteps []*Step

	if pf.Pipeline.Foreach == "" {
		items = []interface{}{nil}
		for i := range pf.Pipeline.Steps {
			foreachSteps = append(foreachSteps, &pf.Pipeline.Steps[i])
		}
	} else if strings.HasPrefix(pf.Pipeline.Foreach, "input.") {
		v, ok := ctx.Get(pf.Pipeline.Foreach)
		if !ok {
			return stats, fmt.Errorf("foreach: путь %s не найден", pf.Pipeline.Foreach)
		}
		arr, ok := v.([]interface{})
		if !ok {
			return stats, fmt.Errorf("foreach: %s не массив", pf.Pipeline.Foreach)
		}
		items = arr
		for i := range pf.Pipeline.Steps {
			foreachSteps = append(foreachSteps, &pf.Pipeline.Steps[i])
		}
	} else if strings.HasPrefix(pf.Pipeline.Foreach, "steps.") {
		// steps.<id>.<field> — двухфазный ран
		parts := strings.Split(pf.Pipeline.Foreach, ".")
		if len(parts) < 3 {
			return stats, fmt.Errorf("foreach: %s должен быть вида steps.<id>.<field>", pf.Pipeline.Foreach)
		}
		srcID := parts[1]
		// находим индекс srcID
		srcIdx := -1
		for i, st := range pf.Pipeline.Steps {
			if st.ID == srcID {
				srcIdx = i
				break
			}
		}
		if srcIdx == -1 {
			return stats, fmt.Errorf("foreach: шаг %s не найден", srcID)
		}
		// preSteps = 0..srcIdx
		for i := 0; i <= srcIdx; i++ {
			preSteps = append(preSteps, &pf.Pipeline.Steps[i])
		}
		for i := srcIdx + 1; i < len(pf.Pipeline.Steps); i++ {
			foreachSteps = append(foreachSteps, &pf.Pipeline.Steps[i])
		}
		// если не resume — прогоняем preSteps один раз
		if opts.Resume == "" {
			opts.logf("  → фаза 1: получение массива %s из %d шагов", pf.Pipeline.Foreach, len(preSteps))
			for _, st := range preSteps {
				action, err := runStep(eng, st, ctx, j, opts)
				if err != nil {
					j.Event("run_failed", map[string]interface{}{"step": st.ID, "error": err.Error()})
					j.Snapshot(ctx)
					return stats, fmt.Errorf("шаг %s (pre-foreach): %w", st.ID, err)
				}
				if action == "abort_item" {
					return stats, fmt.Errorf("foreach: pre-фаза остановлена на шаге %s", st.ID)
				}
			}
			j.Snapshot(ctx)
		}
		// теперь массив должен быть в контексте
		v, ok := ctx.Get(pf.Pipeline.Foreach)
		if !ok {
			return stats, fmt.Errorf("foreach: после pre-фазы путь %s не найден", pf.Pipeline.Foreach)
		}
		arr, ok := v.([]interface{})
		if !ok {
			return stats, fmt.Errorf("foreach: %s не массив (после pre-фазы)", pf.Pipeline.Foreach)
		}
		items = arr
	} else {
		return stats, fmt.Errorf("foreach: путь должен начинаться с input. или steps., got %s", pf.Pipeline.Foreach)
	}

	itemKey := pf.Pipeline.ForeachItem
	if itemKey == "" {
		itemKey = "item"
	}

	// если resume — пропускаем уже пройденные элементы
	if startItemIdx > 0 {
		if startItemIdx >= len(items) {
			opts.logf("  resume: все %d элементов уже пройдены", len(items))
			j.Event("run_end", map[string]interface{}{"ok": stats.OK, "aborted": stats.Aborted, "resumed": true})
			return stats, nil
		}
		opts.logf("  resume: пропускаем %d элементов, продолжаем с %d/%d", startItemIdx, startItemIdx+1, len(items))
	}

	for idx, it := range items {
		if idx < startItemIdx {
			continue
		}
		if pf.Pipeline.Foreach != "" {
			if len(preSteps) == 0 {
				// input-foreach: scope per-item, сбрасываем steps каждый элемент
				ctx.ResetSteps()
			} else {
				// steps-foreach: preSteps остаются, сбрасываем только foreach-шаги (если были)
				// для простоты — удаляем из ctx.Data["steps"] все кроме preSteps
				if stepsMap, ok := ctx.Data["steps"].(map[string]interface{}); ok {
					for _, st := range foreachSteps {
						delete(stepsMap, st.ID)
					}
				}
			}
			ctx.SetInput(itemKey, it)
			opts.logf("\n─ [%d/%d] %v", idx+1, len(items), it)
		}
		j.Event("item_start", map[string]interface{}{"item_index": idx, "item": it})

		itemStatus := "ok"
		stepsToRun := foreachSteps
		if pf.Pipeline.Foreach == "" {
			stepsToRun = foreachSteps
		}
		// для steps-foreach: если preSteps есть, они уже выполнены, бежим только foreachSteps
		// для input-foreach: foreachSteps = все шаги
		for _, st := range stepsToRun {
			action, err := runStep(eng, st, ctx, j, opts)
			if err != nil {
				j.Event("run_failed", map[string]interface{}{"step": st.ID, "error": err.Error()})
				j.Snapshot(ctx)
				return stats, fmt.Errorf("шаг %s: %w", st.ID, err)
			}
			if action == "abort_item" {
				itemStatus = "aborted"
				j.Event("item_aborted", map[string]interface{}{"item_index": idx, "at_step": st.ID})
				break
			}
		}
		j.Snapshot(ctx)
		j.Event("item_end", map[string]interface{}{"item_index": idx, "status": itemStatus})
		if itemStatus == "aborted" {
			stats.Aborted++
		} else {
			stats.OK++
		}
	}

	j.Event("run_end", map[string]interface{}{"ok": stats.OK, "aborted": stats.Aborted})
	opts.logf("\n■ ран завершён: ok=%d aborted=%d → %s", stats.OK, stats.Aborted, j.Dir)
	return stats, nil
}

func runStep(eng *Engine, st *Step, ctx *Ctx, j *Journal, opts RunOptions) (string, error) {
	if IsBuiltin(st.Plugin) {
		return runGate(st, ctx, j, opts), nil
	}

	m, err := eng.LoadManifest(st.Plugin)
	if err != nil {
		return "", err
	}
	input, err := buildInput(m, st, ctx)
	if err != nil {
		return "", err
	}
	for _, w := range fileRefWarningsForRun(m, st, input) {
		opts.logf("    ⚠ %s", w)
		j.Event("file_ref_warning", map[string]interface{}{"step": st.ID, "message": formatHintForLog(w)})
	}
	rawIn, _ := json.Marshal(input)

	timeout := st.Timeout.Duration
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	attempts := 1
	if st.OnError == "retry" {
		attempts = 3
		if st.Retry != nil && st.Retry.Attempts > 0 {
			attempts = st.Retry.Attempts
		}
	}

	var res *ExecResult
	for attempt := 1; attempt <= attempts; attempt++ {
		opts.logf("  → %-12s (попытка %d/%d)", st.ID, attempt, attempts)
		j.Event("step_start", map[string]interface{}{"step": st.ID, "attempt": attempt})
		res = execPlugin(m, rawIn, timeout)
		j.Event("step_end", map[string]interface{}{
			"step": st.ID, "attempt": attempt, "exit_code": res.ExitCode,
			"duration_ms": res.Duration.Milliseconds(), "status": statusOf(res),
			"error": errOrNil(res), "stderr": truncate(res.Stderr, 500),
		})
		if res.OK() || !res.shouldRetry() {
			break
		}
		delay := retryDelay(st, attempt)
		opts.logf("    … retry через %s (%s: %s)", delay, res.ErrCode, res.ErrMsg)
		time.Sleep(delay)
	}

	if res.OK() {
		out, dropped, err := EnforceOutput(m, res.Output)
		if len(dropped) > 0 {
			j.Event("contract_warning", map[string]interface{}{"step": st.ID, "dropped_fields": dropped})
		}
		if err != nil {
			return "", err
		}
		ctx.SetStep(st.ID, out)
		return "ok", nil
	}

	if res.Platform {
		return "", fmt.Errorf("платформенная ошибка (%s): %s", res.ErrCode, res.ErrMsg)
	}

	switch st.OnError {
	case "skip":
		opts.logf("    ! пропущен по on_error=skip: %s", res.ErrMsg)
		j.Event("step_skipped", map[string]interface{}{"step": st.ID, "code": res.ErrCode, "message": res.ErrMsg})
		return "ok", nil
	default:
		opts.logf("    × %s: %s — элемент остановлен", res.ErrCode, res.ErrMsg)
		j.Event("step_failed", map[string]interface{}{"step": st.ID, "code": res.ErrCode, "message": res.ErrMsg})
		return "abort_item", nil
	}
}

func statusOf(r *ExecResult) string {
	if r.OK() {
		return "ok"
	}
	if r.Platform {
		return "platform_error"
	}
	return "domain_error"
}

func errOrNil(r *ExecResult) interface{} {
	if r.OK() {
		return nil
	}
	return map[string]interface{}{"code": r.ErrCode, "message": r.ErrMsg, "retryable": r.Retryable}
}

func buildInput(m *Manifest, st *Step, ctx *Ctx) (map[string]interface{}, error) {
	in := map[string]interface{}{}
	for name, port := range m.Input {
		from := portSource(name, port, st)
		v, ok := ctx.Get(from)
		if !ok {
			if port.Optional {
				continue
			}
			return nil, fmt.Errorf("вход %q: путь %s не найден в контексте (должна была поймать статическая валидация)", name, from)
		}
		in[name] = v
	}
	return in, nil
}

func retryDelay(st *Step, attempt int) time.Duration {
	d := time.Second
	if st.Retry != nil {
		if st.Retry.Delay.Duration > 0 {
			d = st.Retry.Delay.Duration
		}
		if st.Retry.Backoff == "exponential" {
			d = d << (attempt - 1)
		}
	}
	return d
}
