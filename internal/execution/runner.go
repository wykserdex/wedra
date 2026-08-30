package execution

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"wedra/internal/common"
	"wedra/internal/context"
	"wedra/internal/gate"
	"wedra/internal/journal"
	"wedra/internal/pipeline"
	"wedra/internal/plugin"
)

type RunOptions struct {
	Yes     bool
	RunsDir string
	Quiet   bool
	Resume  string
	Store   string
	DBPath  string
	// v0.24: GUI — ID рана известен до старта (ответ API, карточка гейта)
	RunID string
	// v0.24: фабрика не-терминального ввода гейта (GUI/API). Вызывается на
	// каждом встроенном gate-шаге; nil — терминальный stdin (дефолт).
	GateUI func(*pipeline.Step) gate.GateUI
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

// Sanitize — публичная обёртка (v0.24: GUI генерирует runID тем же форматом).
func Sanitize(s string) string { return sanitize(s) }

type Engine interface {
	LoadManifest(ref string) (*pipeline.Manifest, error)
}

func Run(pf *pipeline.PipelineFile, eng Engine, opts RunOptions) (RunStats, error) {
	if opts.Store == "json" {
		s := journal.NewJsonStore(opts.RunsDir, opts.DBPath)
		return runWithStore(pf, eng, opts, s)
	}
	return runWithStore(pf, eng, opts, journal.NewFilesystemStore(opts.RunsDir))
}

func runWithStore(pf *pipeline.PipelineFile, eng Engine, opts RunOptions, store journal.RunStore) (RunStats, error) {
	var stats RunStats
	// v0.16: secrets — до любого эффекта: не запустим ран без ключей
	var missingSecrets []string
	for _, k := range pf.Pipeline.Secrets {
		if os.Getenv(k) == "" {
			missingSecrets = append(missingSecrets, k)
		}
	}
	if len(missingSecrets) > 0 {
		return stats, fmt.Errorf("secrets: не заданы переменные окружения: %s (export перед запуском, значения в YAML не живут)", strings.Join(missingSecrets, ", "))
	}
	// v0.17: network: deny — до любого эффекта, не доверяем validate
	if pf.Pipeline.Network == "deny" {
		for i := range pf.Pipeline.Steps {
			st := &pf.Pipeline.Steps[i]
			if plugin.IsBuiltin(st.Plugin) {
				continue
			}
			m, err := eng.LoadManifest(st.Plugin)
			if err != nil {
				continue // ошибка резолвинга всплывает ниже
			}
			if len(m.Permissions.Network) > 0 {
				return stats, fmt.Errorf("network: шаг %s (плагин %s) заявил сеть (%s), а пайплайн запрещает (network: deny)", st.ID, st.Plugin, pipeline.NetworkHosts(m))
			}
		}
	}
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
	if sq, ok := store.(*journal.JsonStore); ok {
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
		runID := opts.RunID
		if runID == "" {
			runID = time.Now().Format("20060102-150405") + "-" + sanitize(pf.Pipeline.Name)
		}
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
			preRefs := make([]*pipeline.Step, 0, len(preSteps))
			for i := range preSteps {
				preRefs = append(preRefs, preSteps[i])
			}
			acts, err := runParallelSegments(eng, pf, preRefs, ctx, j, opts, true)
			if err != nil {
				j.Event("run_failed", map[string]interface{}{"error": err.Error()})
				j.Snapshot(ctx)
				return stats, fmt.Errorf("pre-foreach: %w", err)
			}
			for _, a := range acts {
				if a == "abort_item" {
					return stats, fmt.Errorf("foreach: pre-фаза остановлена")
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
					if _, err := runStep(eng, pf, st, ctx, j, opts); err != nil {
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
						delete(stepsMap, st.ID+"_all")
					}
				}
			}
			ctx.SetInput(itemKey, it)
			opts.logf("\n─ [%d/%d] %v", idx+1, len(items), it)
		}
		j.Event("item_start", map[string]interface{}{"item_index": idx, "item": it})

		itemStatus := "ok"
		loopRefs := make([]*pipeline.Step, 0, len(loopSteps))
		for i := range loopSteps {
			loopRefs = append(loopRefs, loopSteps[i])
		}
		acts, err := runParallelSegments(eng, pf, loopRefs, ctx, j, opts, true)
		if err != nil {
			j.Event("run_failed", map[string]interface{}{"error": err.Error()})
			j.Snapshot(ctx)
			return stats, fmt.Errorf("в цикле: %w", err)
		}
		for i, action := range acts {
			st := loopRefs[i]
			if action == "abort_item" {
				itemStatus = "aborted"
				j.Event("item_aborted", map[string]interface{}{"item_index": idx, "at_step": st.ID})
				break
			}
			// v0.20: у шага с foreach агрегируем его внутренний _all, не последний выход
			srcKey := st.ID
			if st.Foreach != "" {
				srcKey = st.ID + "_all"
			}
			if v, ok := ctx.Data["steps"].(map[string]interface{})[srcKey]; ok {
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
		postRefs := make([]*pipeline.Step, 0, len(postSteps))
		for i := range postSteps {
			postRefs = append(postRefs, postSteps[i])
		}
		acts, err := runParallelSegments(eng, pf, postRefs, ctx, j, opts, true)
		if err != nil {
			j.Event("run_failed", map[string]interface{}{"error": err.Error()})
			j.Snapshot(ctx)
			return stats, fmt.Errorf("post-foreach: %w", err)
		}
		for _, a := range acts {
			if a == "abort_item" {
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

// stepSegment — фрагмент списка шагов: одиночный шаг или параллельная группа.
type stepSegment struct {
	parallel bool
	group    string
	steps    []*pipeline.Step
}

// segmentSteps — разбивает список шагов на одиночные шаги и смежные блоки
// с одинаковым parallel_group (v0.20). Группа из одного шага — обычный шаг
// (валидатор уже предупреждает).
func segmentSteps(steps []*pipeline.Step) []stepSegment {
	var segs []stepSegment
	for i := 0; i < len(steps); {
		g := steps[i].ParallelGroup
		if g != "" {
			j := i
			for j < len(steps) && steps[j].ParallelGroup == g {
				j++
			}
			segs = append(segs, stepSegment{parallel: j-i > 1, group: g, steps: steps[i:j]})
			i = j
			continue
		}
		segs = append(segs, stepSegment{steps: steps[i : i+1]})
		i++
	}
	return segs
}

// runParallelSegments — исполняет сегменты (шаги/группы) последовательно,
// параллельную группу — goroutine'ами с барьером. Возвращает actions в
// порядке списка шагов. stopOnAbort: при on_error=stop в одиночном шаге
// последующие шаги не исполняются (abort_item присутствует в actions).
// Внутри параллельной группы stop = остановка рана (барьер не терпит половин).
func runParallelSegments(eng Engine, pf *pipeline.PipelineFile, steps []*pipeline.Step, ctx *context.Ctx, j *journal.Journal, opts RunOptions, stopOnAbort bool) ([]string, error) {
	var actions []string
	for _, seg := range segmentSteps(steps) {
		if seg.parallel {
			acts, err := runParallelGroup(eng, pf, seg, ctx, j, opts)
			if err != nil {
				return actions, err
			}
			actions = append(actions, acts...)
			continue
		}
		st := seg.steps[0]
		action, err := runStepFlow(eng, pf, st, ctx, j, opts)
		if err != nil {
			return actions, err
		}
		actions = append(actions, action)
		if stopOnAbort && action == "abort_item" {
			return actions, nil
		}
	}
	return actions, nil
}

// runParallelGroup — v0.20: ветки группы исполняются параллельно, каждая на
// своей копии контекста; после барьера выходы сливаются в общий контекст в
// порядке списка (детерминизм). Платформенная ошибка или on_error=stop в
// любой ветке останавливает ран. human_gate и foreach в группах запрещены
// валидатором.
func runParallelGroup(eng Engine, pf *pipeline.PipelineFile, seg stepSegment, ctx *context.Ctx, j *journal.Journal, opts RunOptions) ([]string, error) {
	ids := make([]string, 0, len(seg.steps))
	for _, st := range seg.steps {
		ids = append(ids, st.ID)
	}
	start := time.Now()
	j.Event("parallel_start", map[string]interface{}{"group": seg.group, "steps": ids})
	opts.logf("  ‖ группа %q параллельно: %s", seg.group, strings.Join(ids, ", "))

	type branchOut struct {
		st     *pipeline.Step
		out    map[string]interface{}
		action string
		err    error
	}
	outs := make([]branchOut, len(seg.steps))
	var wg sync.WaitGroup
	for i, st := range seg.steps {
		wg.Add(1)
		go func(i int, st *pipeline.Step) {
			defer wg.Done()
			bctx := cloneCtx(ctx)
			action, err := runStepFlow(eng, pf, st, bctx, j, opts)
			bo := branchOut{st: st, action: action, err: err}
			if err == nil {
				if m, ok := bctx.Data["steps"].(map[string]interface{}); ok {
					if v, ok := m[st.ID].(map[string]interface{}); ok {
						bo.out = v
					}
				}
			}
			outs[i] = bo
		}(i, st)
	}
	wg.Wait()

	for _, o := range outs {
		if o.err != nil {
			return nil, fmt.Errorf("параллельная группа %q, шаг %s: %w", seg.group, o.st.ID, o.err)
		}
		if o.action == "abort_item" {
			return nil, fmt.Errorf("параллельная группа %q, шаг %s: on_error=stop — ран остановлен", seg.group, o.st.ID)
		}
	}
	var actions []string
	statuses := map[string]string{}
	for _, o := range outs {
		if o.out != nil {
			ctx.SetStep(o.st.ID, o.out)
			statuses[o.st.ID] = "ok"
			actions = append(actions, "ok")
		} else {
			statuses[o.st.ID] = "skipped"
			actions = append(actions, "skipped")
		}
	}
	j.Snapshot(ctx)
	j.Event("parallel_end", map[string]interface{}{"group": seg.group, "statuses": statuses, "duration_ms": time.Since(start).Milliseconds()})
	return actions, nil
}

// cloneCtx — глубокая копия контекста (JSON-roundtrip; значения контекста
// всегда JSON-безопасны).
func cloneCtx(ctx *context.Ctx) *context.Ctx {
	b, err := json.Marshal(ctx.Data)
	if err != nil {
		panic("context: не сериализуется: " + err.Error())
	}
	var data map[string]interface{}
	if err := json.Unmarshal(b, &data); err != nil {
		panic("context: не десериализуется: " + err.Error())
	}
	return &context.Ctx{Data: data}
}

// runStepFlow — обёртка над runStep с управляющим потоком v0.20:
// when-условие (skip) и foreach на уровне шага (per-item мини-цикл).
func runStepFlow(eng Engine, pf *pipeline.PipelineFile, st *pipeline.Step, ctx *context.Ctx, j *journal.Journal, opts RunOptions) (string, error) {
	if st.When.IsSet() {
		ok, err := pipeline.EvaluateWhen(st.When, ctx.Data)
		if err != nil {
			j.Event("step_skipped", map[string]interface{}{"step": st.ID, "reason": "when", "error": err.Error()})
			return "", fmt.Errorf("шаг %s: when: %w", st.ID, err)
		}
		if !ok {
			opts.logf("  → %-12s (условие не выполнено: %s — skipped)", st.ID, st.When.String())
			j.Event("step_skipped", map[string]interface{}{"step": st.ID, "reason": "when", "condition": st.When.String()})
			return "ok", nil
		}
	}
	if st.Foreach != "" {
		return runStepForeach(eng, pf, st, ctx, j, opts)
	}
	return runStep(eng, pf, st, ctx, j, opts)
}

// runStepForeach — v0.20: шаг по каждому элементу массива из пути.
// input.<foreach_item> перезаписывается на итерацию; steps.<id> — выход
// последней итерации; steps.<id>_all — массив всех выходов.
// В отличие от pipeline-foreach, «item» здесь не существует: on_error=stop
// останавливает весь ран.
func runStepForeach(eng Engine, pf *pipeline.PipelineFile, st *pipeline.Step, ctx *context.Ctx, j *journal.Journal, opts RunOptions) (string, error) {
	v, ok := ctx.Get(st.Foreach)
	if !ok {
		return "", fmt.Errorf("foreach: путь %s не найден в контексте", st.Foreach)
	}
	arr, ok := v.([]interface{})
	if !ok {
		return "", fmt.Errorf("foreach: %s не массив (шаг %s)", st.Foreach, st.ID)
	}
	itemKey := st.ForeachItem
	if itemKey == "" {
		itemKey = "item"
	}
	var results []interface{}
	for i, it := range arr {
		j.Event("foreach_item_start", map[string]interface{}{"step": st.ID, "index": i, "item": it})
		ctx.SetInput(itemKey, it)
		action, err := runStep(eng, pf, st, ctx, j, opts)
		if err != nil {
			j.Event("foreach_item_failed", map[string]interface{}{"step": st.ID, "index": i, "error": err.Error()})
			return "", fmt.Errorf("foreach шаг %s: элемент %d/%d: %w", st.ID, i+1, len(arr), err)
		}
		if action == "abort_item" {
			j.Event("foreach_item_failed", map[string]interface{}{"step": st.ID, "index": i, "reason": "on_error=stop"})
			return "", fmt.Errorf("foreach шаг %s: элемент %d/%d упал (on_error=stop) — ран остановлен", st.ID, i+1, len(arr))
		}
		if out, ok := ctx.Data["steps"].(map[string]interface{})[st.ID]; ok {
			results = append(results, out)
		}
		j.Event("foreach_item_end", map[string]interface{}{"step": st.ID, "index": i})
	}
	if stepsMap, ok := ctx.Data["steps"].(map[string]interface{}); ok {
		stepsMap[st.ID+"_all"] = results
	}
	return "ok", nil
}

func runStep(eng Engine, pf *pipeline.PipelineFile, st *pipeline.Step, ctx *context.Ctx, j *journal.Journal, opts RunOptions) (string, error) {
	if plugin.IsBuiltin(st.Plugin) {
		var svc *gate.Service
		if opts.GateUI != nil {
			svc = gate.NewServiceWithUI(opts.GateUI(st))
		} else {
			svc = gate.NewService()
		}
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
	// v0.17: declare-now — subprocess получает контракт сети (deny → честный плагин откажется от сети)
	netEnv := "allow"
	if pf.Pipeline.Network == "deny" {
		netEnv = "deny"
	}
	var res *plugin.ExecResult
	for attempt := 1; attempt <= attempts; attempt++ {
		opts.logf("  → %-12s (попытка %d/%d)", st.ID, attempt, attempts)
		j.Event("step_start", map[string]interface{}{
			"step": st.ID, "attempt": attempt,
			"network": netEnv, "network_declared": pipeline.NetworkHostList(m),
		})
		res = plugin.ExecWithEnv(m, rawIn, timeout, []string{"WEDRA_NETWORK=" + netEnv})
		j.Event("step_end", map[string]interface{}{
			"step": st.ID, "attempt": attempt, "exit_code": res.ExitCode,
			"duration_ms": res.Duration.Milliseconds(), "status": statusOf(res),
			"error": errOrNil(res), "stderr": common.Truncate(res.Stderr, 500),
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
		j.Event("step_skipped", map[string]interface{}{"step": st.ID, "reason": "on_error", "code": res.ErrCode, "message": res.ErrMsg})
		// v0.20: skip = шаг не дал выход — чистим неймспейс, иначе
		// значение предыдущей итерации утёкнет в foreach-агрегацию
		if stepsMap, ok := ctx.Data["steps"].(map[string]interface{}); ok {
			delete(stepsMap, st.ID)
		}
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
		// v0.23: симметричный контракт — вход тоже проверяется (тип+формат),
		// иначе сломанный upstream утекает дальше
		if err := plugin.CheckValue(name, fmt.Sprintf("вход плагина %s", m.ID), port, v); err != nil {
			return nil, err
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
