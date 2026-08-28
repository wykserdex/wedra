package core

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

type RunOptions struct {
	Yes     bool // auto-accept human_gate (CI/демо)
	RunsDir string
	Quiet   bool // без консольного вывода (тесты)
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
		// fallback для совместимости со старым runs/
	}
	runID := time.Now().Format("20060102-150405") + "-" + sanitize(pf.Pipeline.Name)
	j, err := NewJournal(filepath.Join(opts.RunsDir, runID))
	if err != nil {
		return stats, err
	}
	defer j.Close()
	stats.RunDir = j.Dir

	opts.logf("▶ запуск %q  (журнал: %s)", pf.Pipeline.Name, j.Dir)
	j.Event("run_start", map[string]interface{}{"pipeline": pf.Pipeline.Name})

	ctx := NewCtx(pf.Pipeline.Input)

	// foreach: прогон цепочки по элементам массива (PROTOCOL.md §6 — scope per-item)
	var items []interface{}
	if pf.Pipeline.Foreach != "" {
		v, ok := ctx.Get(pf.Pipeline.Foreach)
		if !ok {
			return stats, fmt.Errorf("foreach: путь %s не найден", pf.Pipeline.Foreach)
		}
		arr, ok := v.([]interface{})
		if !ok {
			return stats, fmt.Errorf("foreach: %s не массив", pf.Pipeline.Foreach)
		}
		items = arr
	} else {
		items = []interface{}{nil}
	}
	itemKey := pf.Pipeline.ForeachItem
	if itemKey == "" {
		itemKey = "item"
	}

	for idx, it := range items {
		if pf.Pipeline.Foreach != "" {
			ctx.ResetSteps()
			ctx.SetInput(itemKey, it)
			opts.logf("\n─ [%d/%d] %v", idx+1, len(items), it)
		}
		j.Event("item_start", map[string]interface{}{"item_index": idx, "item": it})

		itemStatus := "ok"
		for i := range pf.Pipeline.Steps {
			st := &pf.Pipeline.Steps[i]
			action, err := runStep(eng, st, ctx, j, opts)
			if err != nil { // платформенная ошибка — стоп всего рана (§3)
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
	// M5 пакет №5: относительный file_ref, не резолвящийся от cwd плагина, —
	// предупреждаем до запуска, а не ждём not_found изнутри плагина.
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

	// доменная ошибка — политика шага из YAML (§6)
	switch st.OnError {
	case "skip":
		opts.logf("    ! пропущен по on_error=skip: %s", res.ErrMsg)
		j.Event("step_skipped", map[string]interface{}{"step": st.ID, "code": res.ErrCode, "message": res.ErrMsg})
		return "ok", nil
	default: // stop (сюда же падает исчерпанный retry)
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

// buildInput: плагин получает ровно те поля, что объявил в input (PROTOCOL.md §4);
// источник каждого порта — bind шага (v0.2) или дефолтный from манифеста.
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
