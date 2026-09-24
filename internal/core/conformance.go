package core

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"wedra/internal/pipeline"
	"wedra/internal/plugin"
)

// ConformanceCheck — одна проверка ядра/протокола.
type ConformanceCheck struct {
	Name    string `json:"name"`
	Pass    bool   `json:"pass"`
	Details string `json:"details,omitempty"`
}

// ConformanceReport — машиночитаемый отчёт для CI (+ бейдж).
type ConformanceReport struct {
	OK     bool               `json:"ok"`
	Checks []ConformanceCheck `json:"checks"`
}

func confPass(name string) ConformanceCheck { return ConformanceCheck{Name: name, Pass: true} }
func confFail(name, details string) ConformanceCheck {
	return ConformanceCheck{Name: name, Pass: false, Details: details}
}

func defaultConformanceFixturesDir(fixturesDir string) string {
	if strings.TrimSpace(fixturesDir) != "" {
		return fixturesDir
	}
	var candidates []string
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates,
			filepath.Join(cwd, "conformance", "fixtures", "v0.2"),
			filepath.Join(cwd, "internal", "core", "testdata", "plugins"),
		)
	}
	if _, file, _, ok := runtime.Caller(0); ok {
		candidates = append(candidates, filepath.Join(filepath.Dir(file), "testdata", "plugins"))
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(dir, "conformance", "fixtures", "v0.2"),
			filepath.Join(dir, "internal", "core", "testdata", "plugins"),
		)
	}
	for _, candidate := range candidates {
		if fi, err := os.Stat(candidate); err == nil && fi.IsDir() {
			return candidate
		}
	}
	return filepath.Join("internal", "core", "testdata", "plugins")
}

// RunConformance — батарея ядра по фикстурам fixturesDir
// (по умолчанию conformance/fixtures/v0.2, затем development testdata):
// handshake, big_stdout 17MB→protocol_violation, big_stderr 2MB→ok,
// cancel (sleeper+отмена → cancelled, не timeout), error_codes (golden Issue).
func RunConformance(fixturesDir string) ConformanceReport {
	fixturesDir = defaultConformanceFixturesDir(fixturesDir)
	var checks []ConformanceCheck

	load := func(name string) *Manifest {
		eng := NewEngine()
		m, err := eng.LoadManifest(filepath.Join(fixturesDir, name))
		if err != nil {
			return nil
		}
		return m
	}

	// 1. handshake: echo_ok
	if m := load("echo_ok"); m == nil {
		checks = append(checks, confFail("handshake", "echo_ok не загрузился"))
	} else {
		res := plugin.Exec(m, []byte("{}"), 10*time.Second)
		if res.OK() {
			checks = append(checks, confPass("handshake"))
		} else {
			checks = append(checks, confFail("handshake", "code="+res.ErrCode+" msg="+res.ErrMsg))
		}
	}

	// 2. big_stdout 17MB → protocol_violation
	if m := load("chatter"); m == nil {
		checks = append(checks, confFail("big_stdout", "chatter не загрузился"))
	} else {
		res := plugin.Exec(m, []byte("{}"), 30*time.Second)
		if res.Platform && res.ErrCode == "protocol_violation" {
			checks = append(checks, confPass("big_stdout"))
		} else {
			checks = append(checks, confFail("big_stdout", "want protocol_violation, got "+res.ErrCode))
		}
	}

	// 3. big_stderr 2MB → ok
	if m := load("big_stderr"); m == nil {
		checks = append(checks, confFail("big_stderr", "big_stderr не загрузился"))
	} else {
		res := plugin.Exec(m, []byte("{}"), 30*time.Second)
		if res.OK() {
			checks = append(checks, confPass("big_stderr"))
		} else {
			checks = append(checks, confFail("big_stderr", "want ok, got "+res.ErrCode))
		}
	}

	// 4. cancel: sleeper + отмена через 300ms → cancelled, не timeout
	if m := load("sleeper"); m == nil {
		checks = append(checks, confFail("cancel", "sleeper не загрузился"))
	} else {
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan *plugin.ExecResult, 1)
		go func() { done <- plugin.ExecWithEnvCtx(ctx, m, []byte("{}"), 30*time.Second, nil) }()
		time.Sleep(300 * time.Millisecond)
		cancel()
		select {
		case res := <-done:
			if res.Cancelled && res.ErrCode == "cancelled" {
				checks = append(checks, confPass("cancel"))
			} else {
				checks = append(checks, confFail("cancel", "want cancelled, got "+res.ErrCode))
			}
		case <-time.After(15 * time.Second):
			checks = append(checks, confFail("cancel", "не завершился за 15с после cancel"))
		}
	}

	// 5. error_codes: golden Issue-коды (контракт для агентов)
	eng := NewEngine()
	mkBad := &pipeline.PipelineFile{
		FormatVersion: "0.2",
		Pipeline: pipeline.Pipeline{
			Name:  "bad",
			Input: map[string]interface{}{},
			Steps: []pipeline.Step{{ID: "s", Plugin: "fake/nope", Bind: map[string]string{"x": "input.nope"}}},
		},
	}
	issues := pipeline.ValidateIssues(mkBad, eng)
	found := false
	for _, is := range issues {
		if is.Code == pipeline.E_PLUGIN_LOAD || is.Code == pipeline.E_PORT_SOURCE {
			found = true
			break
		}
	}
	if found {
		checks = append(checks, confPass("error_codes"))
	} else {
		codes := []string{}
		for _, is := range issues {
			codes = append(codes, is.Code)
		}
		checks = append(checks, confFail("error_codes", "нет E_PLUGIN_LOAD/E_PORT_SOURCE"))
	}

	ok := true
	for _, c := range checks {
		if !c.Pass {
			ok = false
			break
		}
	}
	return ConformanceReport{OK: ok, Checks: checks}
}
