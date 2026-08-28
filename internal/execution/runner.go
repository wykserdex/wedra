package execution

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"orchestrator/internal/context"
	"orchestrator/internal/gate"
	"orchestrator/internal/journal"
	"orchestrator/internal/pipeline"
	"orchestrator/internal/plugin"
)

type RunOptions struct {
	Yes     bool
	RunsDir string
	Quiet   bool
	Resume  string
	Store   string
	DBPath  string
}

func (o RunOptions) logf(format string, a ...interface{}) {
	if !o.Quiet {
		fmt.Printf(format+"\n", a...)
	}
}

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

type Engine interface {
	LoadManifest(ref string) (*pipeline.Manifest, error)
}

func Run(pf *pipeline.PipelineFile, eng Engine, opts RunOptions) (RunStats, error) {
	if opts.Store == "sqlite" {
		s := journal.NewSQLiteStore(opts.RunsDir, opts.DBPath)
		return runWithStore(pf, eng, opts, s)
	}
	return runWithStore(pf, eng, opts, journal.NewFilesystemStore(opts.RunsDir))
}

func runWithStore(pf *pipeline.PipelineFile, eng Engine, opts RunOptions, store journal.RunStore) (RunStats, error) {
	var stats RunStats
	if opts.RunsDir == "" {
		opts.RunsDir = "var/runs"
	}
	if fs, ok := store.(*journal.FilesystemStore); ok {
		if fs.BaseDir == "" {
			fs.BaseDir = opts.RunsDir
		} else {
			opts.RunsDir = fs.BaseDir
		}
	}
	if sq, ok := store.(*journal.SQLiteStore); ok {
		if sq.BaseDir == "" {
			sq.BaseDir = opts.RunsDir
		} else {
			opts.RunsDir = sq.BaseDir
		}
	}

	var ctx *context.Ctx
	var startItemIdx int
	var j *journal.Journal
	var err error

	if opts.Resume != "" {
		data, err := store.LoadContext(opts.Resume)
		if err != nil {
			return stats, fmt.Errorf("--resume %s: %w", opts.Resume, err)
		}
		ctx = &context.Ctx{Data: data}
		maxIdx, err := store.MaxItemIndex(opts.Resume)
		if err != nil {
			return stats, fmt.Errorf("--resume %s: не читается journal: %w", opts.Resume, err)
		}
		startItemIdx = maxIdx + 1
		j, err = store.OpenAppend(opts.Resume)
		if err != nil {
			return stats, err
		}
		stats.RunDir = j.Dir
		opts.logf("▶ resume %q с элемента %d (журнал: %s)", pf.Pipeline.Name, startItemIdx, j.Dir)
		j.Event("run_resumed", map[string]interface{}{"from_item": startItemIdx})
	} else {
		runID := time.Now().Format("20060102-150405") + "-" + sanitize(pf.Pipeline.Name)
		// use store.Create so SQLite DB gets entry
		j, err = store.Create(runID)
		if err != nil {
			return stats, err
		}
		defer j.Close()
		stats.RunDir = j.Dir
		opts.logf("▶ запуск %q  (журнал: %s)", pf.Pipeline.Name, j.Dir)
		j.Event("run_start", map[string]interface{}{"pipeline": pf.Pipeline.Name})
		// also append to store's event log for SQLite
		_ = store.AppendEvent(runID, "run_start", map[string]interface{}{"pipeline": pf.Pipeline.Name})
		ctx = context.NewCtx(pf.Pipeline.Input)
	}

	var items []interface{}
	var preSteps []*pipeline.Step
	var loopSteps []*pipeline.Step
	var postSteps []*pipeline.Step

	if pf.Pipeline.Foreach == "" {
		items = []interface{}{nil}
		for i := range pf.Pipeline.Steps {
			st := &pf.Pipeline.Steps[i]
			if st.AfterForeach {
				postSteps = append(postSteps, st)
			} else {
				loopSteps = append(loopSteps, st)
			}
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
			st := &pf.Pipeline.Steps[i]
			if st.AfterForeach {
				postSteps = append(postSteps, st)
			} else {
				loopSteps = append(loopSteps, st)
			}
		}
	} else if strings.HasPrefix(pf.Pipeline.Foreach, "steps.") {
		parts := strings.Split(pf.Pipeline.Foreach, ".")
		if len(parts) < 3 {
			return stats, fmt.Errorf("foreach: %s должен быть вида steps.<id>.<field>", pf.Pipeline.Foreach)
		}
		srcID := parts[1]
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
		for i := 0; i <= srcIdx; i++ {
			preSteps = append(preSteps, &pf.Pipeline.Steps[i])
		}
		for i := srcIdx + 1; i < len(pf.Pipeline.Steps); i++ {
			st := &pf.Pipeline.Steps[i]
			if st.AfterForeach {
				postSteps = append(postSteps, st)
			} else {
				loopSteps = append(loopSteps, st)
			}
		}
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

	if startItemIdx > 0 {
		if startItemIdx >= len(items) {
			opts.logf("  resume: все %d элементов уже пройдены", len(items))
			if len(postSteps) > 0 {
				opts.logf("  → фаза 3: post-foreach %d шагов", len(postSteps))
				for _, st := range postSteps {
					if _, err := runStep(eng, st, ctx, j, opts); err != nil {
						j.Event("run_failed", map[string]interface{}{"step": st.ID, "error": err.Error()})
						return stats, fmt.Errorf("шаг %s (post-foreach): %w", st.ID, err)
					}
				}
			}
			j.Event("run_end", map[string]interface{}{"ok": stats.OK, "aborted": stats.Aborted, "resumed": true})
			return stats, nil
		}
		opts.logf("  resume: пропускаем %d элементов, продолжаем с %d/%d", startItemIdx, startItemIdx+1, len(items))
	}

	agg := map[string][]interface{}{}
	if startItemIdx > 0 {
		if stepsMap, ok := ctx.Data["steps"].(map[string]interface{}); ok {
			for _, st := range loopSteps {
				if raw, ok := stepsMap[st.ID+"_all"]; ok {
					if arr, ok := raw.([]interface{}); ok {
						if len(arr) >= startItemIdx {
							agg[st.ID] = append([]interface{}{}, arr[:startItemIdx]...)
						} else {
							agg[st.ID] = append([]interface{}{}, arr...)
						}
					}
				}
			}
		}
	}

	for idx, it := range items {
		if idx < startItemIdx {
			continue
		}
		if pf.Pipeline.Foreach != "" {
			if len(preSteps) == 0 {
				ctx.ResetSteps()
			} else {
				if stepsMap, ok := ctx.Data["steps"].(map[string]interface{}); ok {
					for _, st := range loopSteps {
						delete(stepsMap, st.ID)
					}
				}
			}
			ctx.SetInput(itemKey, it)
			opts.logf("\n─ [%d/%d] %v", idx+1, len(items), it)
		}
		j.Event("item_start", map[string]interface{}{"item_index": idx, "item": it})

		itemStatus := "ok"
		for _, st := range loopSteps {
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
			if v, ok := ctx.Data["steps"].(map[string]interface{})[st.ID]; ok {
				agg[st.ID] = append(agg[st.ID], v)
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

	if len(postSteps) > 0 {
		opts.logf("\n▶ фаза 3: post-foreach %d шагов (агрегаты: %d)", len(postSteps), len(agg))
		if stepsMap, ok := ctx.Data["steps"].(map[string]interface{}); ok {
			for id, arr := range agg {
				stepsMap[id+"_all"] = arr
			}
		}
		j.Event("post_phase_start", map[string]interface{}{"steps": len(postSteps)})
		for _, st := range postSteps {
			action, err := runStep(eng, st, ctx, j, opts)
			if err != nil {
				j.Event("run_failed", map[string]interface{}{"step": st.ID, "error": err.Error()})
				j.Snapshot(ctx)
				return stats, fmt.Errorf("шаг %s (post-foreach): %w", st.ID, err)
			}
			if action == "abort_item" {
				stats.Aborted++
				break
			}
		}
		j.Snapshot(ctx)
		j.Event("post_phase_end", map[string]interface{}{})
	}

	j.Event("run_end", map[string]interface{}{"ok": stats.OK, "aborted": stats.Aborted})
	opts.logf("\n■ ран завершён: ok=%d aborted=%d → %s", stats.OK, stats.Aborted, j.Dir)
	return stats, nil
}

func runStep(eng Engine, st *pipeline.Step, ctx *context.Ctx, j *journal.Journal, opts RunOptions) (string, error) {
	if plugin.IsBuiltin(st.Plugin) {
		svc := gate.NewService()
		return svc.Run(st, ctx, j, gate.GateOptions{Yes: opts.Yes, Quiet: opts.Quiet}), nil
	}
	m, err := eng.LoadManifest(st.Plugin)
	if err != nil {
		return "", err
	}
	input, err := buildInput(m, st, ctx)
	if err != nil {
		return "", err
	}
	for _, w := range plugin.FileRefWarnings(m, input) {
		opts.logf("    ⚠ %s", w)
		j.Event("file_ref_warning", map[string]interface{}{"step": st.ID, "message": w})
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
	var res *plugin.ExecResult
	for attempt := 1; attempt <= attempts; attempt++ {
		opts.logf("  → %-12s (попытка %d/%d)", st.ID, attempt, attempts)
		j.Event("step_start", map[string]interface{}{"step": st.ID, "attempt": attempt})
		res = plugin.Exec(m, rawIn, timeout)
		j.Event("step_end", map[string]interface{}{
			"step": st.ID, "attempt": attempt, "exit_code": res.ExitCode,
			"duration_ms": res.Duration.Milliseconds(), "status": statusOf(res),
			"error": errOrNil(res), "stderr": truncate(res.Stderr, 500),
		})
		if res.OK() || !res.ShouldRetry() {
			break
		}
		delay := retryDelay(st, attempt)
		opts.logf("    … retry через %s (%s: %s)", delay, res.ErrCode, res.ErrMsg)
		time.Sleep(delay)
	}
	if res.OK() {
		out, dropped, err := plugin.EnforceOutput(m, res.Output)
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

func statusOf(r *plugin.ExecResult) string {
	if r.OK() {
		return "ok"
	}
	if r.Platform {
		return "platform_error"
	}
	return "domain_error"
}

func errOrNil(r *plugin.ExecResult) interface{} {
	if r.OK() {
		return nil
	}
	return map[string]interface{}{"code": r.ErrCode, "message": r.ErrMsg, "retryable": r.Retryable}
}

func buildInput(m *pipeline.Manifest, st *pipeline.Step, ctx *context.Ctx) (map[string]interface{}, error) {
	in := map[string]interface{}{}
	for name, port := range m.Input {
		from := pipeline.PortSource(name, port, st)
		v, ok := ctx.Get(from)
		if !ok {
			if port.Optional {
				continue
			}
			return nil, fmt.Errorf("вход %q: путь %s не найден в контексте", name, from)
		}
		in[name] = v
	}
	return in, nil
}

func retryDelay(st *pipeline.Step, attempt int) time.Duration {
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

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
