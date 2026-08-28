package execution

import "time"

// Scheduler — политика retry/backoff, таймауты, параллельность (план M6)
// Сейчас — заглушка, логика в core/runner.go retryDelay

type RetryPolicy struct {
	Attempts int
	Delay    time.Duration
	Backoff  string // fixed | exponential
}

func (p *RetryPolicy) DelayFor(attempt int) time.Duration {
	d := p.Delay
	if d == 0 {
		d = time.Second
	}
	if p.Backoff == "exponential" {
		return d << (attempt - 1)
	}
	return d
}

// ExecutionGraph — DAG шагов для будущего параллельного выполнения независимых веток
type ExecutionGraph struct {
	Nodes []string
	Edges map[string][]string // step_id → depends_on
}

func BuildGraph() *ExecutionGraph {
	return &ExecutionGraph{Edges: map[string][]string{}}
}
