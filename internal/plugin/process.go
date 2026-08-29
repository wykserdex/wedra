package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"orchestrator/internal/common"
	"orchestrator/internal/pipeline"
)

type Manifest = pipeline.Manifest

type ExecResult struct {
	Output    map[string]interface{}
	ErrCode   string
	ErrMsg    string
	Retryable bool
	Platform  bool
	ExitCode  int
	Stderr    string
	Duration  time.Duration
}

func (r *ExecResult) OK() bool { return !r.Platform && r.ExitCode == 0 && r.ErrCode == "" }
func (r *ExecResult) ShouldRetry() bool {
	if r.ErrCode == "timeout" {
		return true
	}
	return r.Retryable
}

type wireResponse struct {
	Status string                 `json:"status"`
	Output map[string]interface{} `json:"output"`
	Error  *struct {
		Code      string `json:"code"`
		Message   string `json:"message"`
		Retryable bool   `json:"retryable"`
	} `json:"error"`
}

func pythonInterpreter() (string, error) {
	for _, name := range []string{"python3", "python"} {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("не найден интерпретатор python (python3/python)")
}

func Exec(m *Manifest, inputJSON []byte, timeout time.Duration) *ExecResult {
	return ExecWithEnv(m, inputJSON, timeout, nil)
}

func ExecWithEnv(m *Manifest, inputJSON []byte, timeout time.Duration, extraEnv []string) *ExecResult {
	return execPluginEnv(m, inputJSON, timeout, extraEnv)
}

func execPluginEnv(m *Manifest, input []byte, timeout time.Duration, extraEnv []string) *ExecResult {
	res := &ExecResult{ExitCode: -1}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var cmd *exec.Cmd
	switch m.Runtime.Type {
	case "python":
		py, err := pythonInterpreter()
		if err != nil {
			res.Platform, res.ErrCode, res.ErrMsg = true, "runtime_missing", err.Error()
			res.ExitCode = 2
			return res
		}
		entry, err := filepath.Abs(filepath.Join(m.Dir, m.Runtime.Entry))
		if err != nil {
			res.Platform, res.ErrCode, res.ErrMsg = true, "spawn_failed", err.Error()
			res.ExitCode = 2
			return res
		}
		cmd = exec.CommandContext(ctx, py, entry)
	case "binary":
		entry, err := filepath.Abs(filepath.Join(m.Dir, m.Runtime.Entry))
		if err != nil {
			res.Platform, res.ErrCode, res.ErrMsg = true, "spawn_failed", err.Error()
			res.ExitCode = 2
			return res
		}
		cmd = exec.CommandContext(ctx, entry)
	default:
		res.Platform, res.ErrCode, res.ErrMsg = true, "runtime_unknown", "runtime.type: "+m.Runtime.Type
		res.ExitCode = 2
		return res
	}

	cmd.Dir = m.Dir
	cmd.Stdin = bytes.NewReader(input)
	if len(extraEnv) > 0 {
		cmd.Env = mergeEnv(os.Environ(), extraEnv)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr

	start := time.Now()
	runErr := cmd.Run()
	res.Duration = time.Since(start)
	res.Stderr = stderr.String()

	if ctx.Err() == context.DeadlineExceeded {
		res.Platform = true
		res.ErrCode, res.ErrMsg = "timeout", "плагин превысил таймаут "+timeout.String()
		res.ExitCode = 2
		return res
	}
	if runErr != nil {
		var ee *exec.ExitError
		if errors.As(runErr, &ee) {
			res.ExitCode = ee.ExitCode()
		} else {
			res.Platform, res.ErrCode, res.ErrMsg = true, "spawn_failed", runErr.Error()
			res.ExitCode = 2
			return res
		}
	} else {
		res.ExitCode = 0
	}

	var wr wireResponse
	outTrim := bytes.TrimSpace(stdout.Bytes())
	if len(outTrim) > 0 {
		if err := json.Unmarshal(outTrim, &wr); err != nil {
			res.Platform, res.ErrCode = true, "protocol_violation"
			res.ErrMsg = fmt.Sprintf("на stdout не JSON по протоколу (exit %d): %s", res.ExitCode, common.Truncate(string(outTrim), 200))
			if res.ExitCode == 0 {
				res.ExitCode = 2
			}
			return res
		}
	}

	switch {
	case res.ExitCode == 0:
		if wr.Status != "ok" {
			res.Platform, res.ErrCode = true, "protocol_violation"
			res.ErrMsg = "exit 0, но status != ok"
			res.ExitCode = 2
			return res
		}
		res.Output = wr.Output
		if res.Output == nil {
			res.Output = map[string]interface{}{}
		}
	case res.ExitCode == 1:
		res.ErrCode, res.ErrMsg = "plugin_error", "доменная ошибка без описания"
		if wr.Error != nil {
			res.ErrCode = wr.Error.Code
			res.ErrMsg = wr.Error.Message
			res.Retryable = wr.Error.Retryable
		}
	default:
		res.Platform = true
		if wr.Error != nil && wr.Error.Code != "" {
			res.ErrCode = "platform:" + wr.Error.Code
			res.ErrMsg = wr.Error.Message
			if res.ErrMsg == "" {
				res.ErrMsg = fmt.Sprintf("exit %d: %s", res.ExitCode, common.Truncate(string(outTrim), 200))
			}
			res.Retryable = wr.Error.Retryable
		} else {
			res.ErrCode, res.ErrMsg = "crash", fmt.Sprintf("exit %d: %s", res.ExitCode, common.Truncate(string(outTrim), 200))
		}
	}
	return res
}

func mergeEnv(base, extra []string) []string {
	m := map[string]string{}
	for _, kv := range base {
		if i := indexByte(kv, '='); i >= 0 {
			m[kv[:i]] = kv[i+1:]
		}
	}
	for _, kv := range extra {
		if i := indexByte(kv, '='); i >= 0 {
			m[kv[:i]] = kv[i+1:]
		}
	}
	out := make([]string, 0, len(m))
	for k, v := range m {
		out = append(out, k+"="+v)
	}
	return out
}

func indexByte(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}

func EnforceOutput(m *Manifest, out map[string]interface{}) (clean map[string]interface{}, dropped []string, err error) {
	clean = map[string]interface{}{}
	for k, v := range out {
		if _, declared := m.Output[k]; declared {
			clean[k] = v
		} else {
			dropped = append(dropped, k)
		}
	}
	for name, port := range m.Output {
		if _, ok := clean[name]; !ok && !port.Optional {
			return nil, dropped, fmt.Errorf("нарушение контракта: плагин %s не вернул обязательное поле %q", m.ID, name)
		}
	}
	return clean, dropped, nil
}
