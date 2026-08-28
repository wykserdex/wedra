package core

import (
	"time"

	"orchestrator/internal/plugin"
)

type ExecResult struct {
	ExitCode  int
	Status    string
	Output    map[string]interface{}
	ErrCode   string
	ErrMsg    string
	Retryable bool
	Platform  bool
	TimedOut  bool
	Stderr    string
	Duration  time.Duration
}

func (r *ExecResult) OK() bool {
	return !r.Platform && r.ExitCode == 0 && r.Status == "ok"
}

func (r *ExecResult) shouldRetry() bool {
	if r.TimedOut {
		return true
	}
	return r.Status == "error" && !r.Platform && r.Retryable
}

func fromPluginRes(pr *plugin.ExecResult) *ExecResult {
	res := &ExecResult{
		ExitCode:  pr.ExitCode,
		Output:    pr.Output,
		ErrCode:   pr.ErrCode,
		ErrMsg:    pr.ErrMsg,
		Retryable: pr.Retryable,
		Platform:  pr.Platform,
		Stderr:    pr.Stderr,
		Duration:  pr.Duration,
	}
	if pr.OK() {
		res.Status = "ok"
	} else {
		res.Status = "error"
	}
	if pr.ErrCode == "timeout" {
		res.TimedOut = true
	}
	return res
}

func execPlugin(m *Manifest, input []byte, timeout time.Duration) *ExecResult {
	pr := plugin.Exec(m, input, timeout)
	return fromPluginRes(pr)
}

func execPluginEnv(m *Manifest, input []byte, timeout time.Duration, extraEnv []string) *ExecResult {
	pr := plugin.ExecWithEnv(m, input, timeout, extraEnv)
	return fromPluginRes(pr)
}

func EnforceOutput(m *Manifest, out map[string]interface{}) (map[string]interface{}, []string, error) {
	return plugin.EnforceOutput(m, out)
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

func buildInput(m *Manifest, st *Step, ctx *Ctx) (map[string]interface{}, error) {
	in := map[string]interface{}{}
	for name, port := range m.Input {
		from := PortSource(name, port, st)
		v, ok := ctx.Get(from)
		if !ok {
			if port.Optional {
				continue
			}
			return nil, &errorString{msg: "вход " + name + ": путь " + from + " не найден"}
		}
		in[name] = v
	}
	return in, nil
}

type errorString struct{ msg string }

func (e *errorString) Error() string { return e.msg }
