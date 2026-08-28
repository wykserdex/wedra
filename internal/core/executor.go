package core

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
)

// ExecResult — результат одного запуска плагина (см. PROTOCOL.md §3).
type ExecResult struct {
	ExitCode  int
	Status    string // "ok" | "error"
	Output    map[string]interface{}
	ErrCode   string
	ErrMsg    string
	Retryable bool
	Platform  bool // краш/таймаут/нарушение протокола — не подчиняется on_error, стопит ран
	TimedOut  bool
	Stderr    string
	Duration  time.Duration
}

func (r *ExecResult) OK() bool { return !r.Platform && r.ExitCode == 0 && r.Status == "ok" }

// shouldRetry: таймауты и доменные ошибки с явным retryable.
func (r *ExecResult) shouldRetry() bool {
	if r.TimedOut {
		return true
	}
	return r.Status == "error" && !r.Platform && r.Retryable
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

func execPlugin(m *Manifest, input []byte, timeout time.Duration) *ExecResult {
	return execPluginEnv(m, input, timeout, nil)
}

// execPluginEnv — то же + проброс env (tool plugin test: секреты, mock-режимы).
// extraEnv переопределяет унаследованные переменные.
func execPluginEnv(m *Manifest, input []byte, timeout time.Duration, extraEnv []string) *ExecResult {
	res := &ExecResult{ExitCode: -1}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var cmd *exec.Cmd
	switch m.Runtime.Type {
	// entry → абсолютный путь: cmd.Dir меняет cwd дочернего процесса,
	// относительный путь до entry сломался бы
	case "python":
		py, err := pythonInterpreter()
		if err != nil {
			res.Platform, res.ErrCode, res.ErrMsg = true, "runtime_missing", err.Error()
			return res
		}
		entry, err := filepath.Abs(filepath.Join(m.Dir, m.Runtime.Entry))
		if err != nil {
			res.Platform, res.ErrCode, res.ErrMsg = true, "spawn_failed", err.Error()
			return res
		}
		cmd = exec.CommandContext(ctx, py, entry)
	case "binary":
		entry, err := filepath.Abs(filepath.Join(m.Dir, m.Runtime.Entry))
		if err != nil {
			res.Platform, res.ErrCode, res.ErrMsg = true, "spawn_failed", err.Error()
			return res
		}
		cmd = exec.CommandContext(ctx, entry)
	default:
		res.Platform, res.ErrCode, res.ErrMsg = true, "runtime_unknown", "runtime.type: "+m.Runtime.Type
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
		res.Platform, res.TimedOut = true, true
		res.ErrCode, res.ErrMsg = "timeout", "плагин превысил таймаут "+timeout.String()
		return res
	}
	if runErr != nil {
		var ee *exec.ExitError
		if errors.As(runErr, &ee) {
			res.ExitCode = ee.ExitCode()
		} else {
			res.Platform, res.ErrCode, res.ErrMsg = true, "spawn_failed", runErr.Error()
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
			res.ErrMsg = fmt.Sprintf("на stdout не JSON по протоколу (exit %d): %s",
				res.ExitCode, truncate(string(outTrim), 200))
			return res
		}
	}

	switch {
	case res.ExitCode == 0:
		if wr.Status != "ok" {
			res.Platform, res.ErrCode = true, "protocol_violation"
			res.ErrMsg = "exit 0, но status != ok"
			return res
		}
		res.Status = "ok"
		res.Output = wr.Output
		if res.Output == nil {
			res.Output = map[string]interface{}{}
		}
	case res.ExitCode == 1:
		res.Status = "error"
		res.ErrCode, res.ErrMsg = "plugin_error", "доменная ошибка без описания"
		if wr.Error != nil {
			res.ErrCode = wr.Error.Code
			res.ErrMsg = wr.Error.Message
			res.Retryable = wr.Error.Retryable
		}
	default: // exit >= 2 — платформенная ошибка (краш, нарушение контракта)
		// фикс №3 (внешний автор, 2026-08-28): раньше конверт плагина выбрасывался,
		// error.code всегда был crash, и автор не мог различить bad_input vs assert.
		// Теперь парсим конверт и на exit>=2: ErrCode = platform:<code> (fallback crash),
		// Platform остаётся true — ран всё равно стопится, но триаж возможен.
		res.Platform, res.Status = true, "error"
		if wr.Error != nil && wr.Error.Code != "" {
			res.ErrCode = "platform:" + wr.Error.Code
			res.ErrMsg = wr.Error.Message
			if res.ErrMsg == "" {
				res.ErrMsg = fmt.Sprintf("exit %d: %s", res.ExitCode, truncate(string(outTrim), 200))
			}
			res.Retryable = wr.Error.Retryable
		} else {
			res.ErrCode, res.ErrMsg = "crash",
				fmt.Sprintf("exit %d: %s", res.ExitCode, truncate(string(outTrim), 200))
		}
	}
	return res
}

// EnforceOutput: в контекст попадают только объявленные поля (§5);
// пропущенное обязательное поле — нарушение контракта.
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

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
