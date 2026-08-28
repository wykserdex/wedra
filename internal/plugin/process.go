package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"orchestrator/internal/pipeline"
)

type Manifest = pipeline.Manifest

// ExecResult — результат запуска плагина, теперь в internal/plugin
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

func (r *ExecResult) OK() bool { return r.ExitCode == 0 && r.ErrCode == "" }

func (r *ExecResult) ShouldRetry() bool { return r.Retryable }

// Exec — запуск процесса плагина (вынесено из core/executor.go)
func Exec(m *Manifest, inputJSON []byte, timeout time.Duration) *ExecResult {
	start := time.Now()
	res := &ExecResult{}
	if m.Dir == "" {
		res.Platform = true
		res.ErrCode = "manifest_no_dir"
		res.ErrMsg = "у манифеста нет Dir"
		res.ExitCode = 2
		return res
	}
	entry := filepath.Join(m.Dir, m.Runtime.Entry)
	var cmd *exec.Cmd
	switch m.Runtime.Type {
	case "python":
		py := pythonInterpreter()
		cmd = exec.Command(py, entry)
	default:
		cmd = exec.Command(entry)
	}
	cmd.Dir = m.Dir
	cmd.Stdin = bytes.NewReader(inputJSON)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	// Go 1.22: CommandContext
	cmd = exec.CommandContext(ctx, cmd.Path, cmd.Args[1:]...)
	cmd.Dir = m.Dir
	cmd.Stdin = bytes.NewReader(inputJSON)
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()
	res.Duration = time.Since(start)
	res.Stderr = errBuf.String()
	if ctx.Err() == context.DeadlineExceeded {
		res.Platform = true
		res.ErrCode = "timeout"
		res.ErrMsg = fmt.Sprintf("таймаут %s", timeout)
		res.ExitCode = 2
		return res
	}
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			res.ExitCode = exitErr.ExitCode()
		} else {
			res.ExitCode = 2
			res.Platform = true
			res.ErrCode = "exec_failed"
			res.ErrMsg = err.Error()
			return res
		}
	}
	// парсим stdout как конверт
	var env struct {
		Status string                 `json:"status"`
		Output map[string]interface{} `json:"output"`
		Error  *struct {
			Code      string `json:"code"`
			Message   string `json:"message"`
			Retryable bool   `json:"retryable"`
		} `json:"error"`
	}
	if err := json.Unmarshal(outBuf.Bytes(), &env); err != nil {
		res.Platform = true
		res.ErrCode = "protocol_violation"
		res.ErrMsg = fmt.Sprintf("stdout не JSON: %v, output: %s", err, truncate(outBuf.String(), 500))
		if res.ExitCode == 0 {
			res.ExitCode = 2
		}
		return res
	}
	if env.Status == "ok" {
		res.Output = env.Output
		res.ExitCode = 0
		return res
	}
	if env.Status == "error" && env.Error != nil {
		res.ErrCode = env.Error.Code
		res.ErrMsg = env.Error.Message
		res.Retryable = env.Error.Retryable
		if res.ExitCode == 0 {
			res.ExitCode = 1
		}
		// exit >=2 → platform
		if res.ExitCode >= 2 {
			res.Platform = true
		}
		return res
	}
	res.Platform = true
	res.ErrCode = "protocol_violation"
	res.ErrMsg = "неизвестный status в конверте"
	if res.ExitCode == 0 {
		res.ExitCode = 2
	}
	return res
}

func pythonInterpreter() string {
	if _, err := os.Stat("/usr/bin/python3"); err == nil {
		return "python3"
	}
	return "python"
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
