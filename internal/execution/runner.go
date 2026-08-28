package execution

import (
	"fmt"
	"path/filepath"
	"time"

	"orchestrator/internal/context"
	"orchestrator/internal/journal"
	"orchestrator/internal/pipeline"
	"orchestrator/internal/plugin"
)

// RunOptions — опции рана, теперь в internal/execution
type RunOptions struct {
	Yes     bool
	RunsDir string
	Quiet   bool
}

func (o RunOptions) Logf(format string, a ...interface{}) {
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
	// копия из core/runner.go
	res := ""
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			res += string(r)
		default:
			res += "-"
		}
	}
	return res
}

// Runner — оркестратор выполнения пайплайна (вынесено из core/runner.go)
// Для MVP делегирует в core.Run, в M6 будет полностью здесь
type Runner struct {
	Engine  *plugin.Engine
	Journal *journal.Journal
}

func NewRunner(eng *plugin.Engine) *Runner {
	return &Runner{Engine: eng}
}

func (r *Runner) Run(pf *pipeline.PipelineFile, opts RunOptions) (RunStats, error) {
	var stats RunStats
	if opts.RunsDir == "" {
		opts.RunsDir = "var/runs"
	}
	runID := time.Now().Format("20060102-150405") + "-" + sanitize(pf.Pipeline.Name)
	j, err := journal.NewJournal(filepath.Join(opts.RunsDir, runID))
	if err != nil {
		return stats, err
	}
	defer j.Close()
	stats.RunDir = j.Dir
	opts.Logf("▶ запуск %q (журнал: %s)", pf.Pipeline.Name, j.Dir)
	j.Event("run_start", map[string]interface{}{"pipeline": pf.Pipeline.Name})

	ctx := context.NewCtx(pf.Pipeline.Input)
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
			opts.Logf("\n─ [%d/%d] %v", idx+1, len(items), it)
		}
		j.Event("item_start", map[string]interface{}{"item_index": idx, "item": it})
		// TODO: вынести runStep в scheduler
		// для MVP — заглушка, реальный ран пока в core.Run
		j.Event("item_end", map[string]interface{}{"item_index": idx, "status": "ok"})
		stats.OK++
	}
	j.Event("run_end", map[string]interface{}{"ok": stats.OK, "aborted": stats.Aborted})
	opts.Logf("\n■ ран завершён: ok=%d aborted=%d → %s", stats.OK, stats.Aborted, j.Dir)
	return stats, nil
}
