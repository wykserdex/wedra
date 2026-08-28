package core

import (
	"orchestrator/internal/execution"
)

type RunOptions = execution.RunOptions
type RunStats = execution.RunStats

func Run(pf *PipelineFile, eng *Engine, opts RunOptions) (RunStats, error) {
	return execution.Run(pf, eng, execution.RunOptions{
		Yes:     opts.Yes,
		RunsDir: opts.RunsDir,
		Quiet:   opts.Quiet,
		Resume:  opts.Resume,
	})
}
