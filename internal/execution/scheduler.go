package execution

import "time"

type RetryPolicy struct {
	Attempts int
	Delay    time.Duration
	Backoff  string
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

type ExecutionGraph struct {
	Nodes []string
	Edges map[string][]string
}

func BuildGraph() *ExecutionGraph {
	return &ExecutionGraph{Edges: map[string][]string{}}
}
