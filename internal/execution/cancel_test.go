package execution

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"wedra/internal/journal"
	"wedra/internal/pipeline"
	"wedra/internal/runctx"
)

func writeSleeper(t *testing.T, dir string) string {
	t.Helper()
	d := filepath.Join(dir, "sleeper")
	if err := os.MkdirAll(d, 0755); err != nil {
		t.Fatal(err)
	}
	manifest := map[string]interface{}{
		"id": "sleeper", "version": "0.1", "platform_api": "0.1",
		"runtime": map[string]interface{}{"type": "python", "entry": "main.py"},
		"input":   map[string]interface{}{},
		"output":  map[string]interface{}{"done": map[string]interface{}{"type": "boolean"}},
	}
	raw, _ := json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(d, "plugin.yaml"), raw, 0644); err != nil {
		t.Fatal(err)
	}
	// спит 30с без дочерних (на Unix группа убивается вместе с внуком —
	// см. TestExecTimeoutKillsProcessGroup; на Windows Job Object нет,
	// поэтому cancel-тест без внука, иначе Wait висит на унаследованном пайпе).
	py := "import sys,json,time\njson.load(sys.stdin)\ntime.sleep(30)\njson.dump({'status':'ok','output':{'done':True}},sys.stdout)\n"
	if err := os.WriteFile(filepath.Join(d, "main.py"), []byte(py), 0644); err != nil {
		t.Fatal(err)
	}
	return d
}

type mapEngine struct {
	dirs map[string]string
}

type permissiveEngine struct{}

func (permissiveEngine) LoadManifest(ref string) (*pipeline.Manifest, error) {
	return &pipeline.Manifest{ID: ref}, nil
}

func (m *mapEngine) LoadManifest(ref string) (*pipeline.Manifest, error) {
	// ref — абсолютный путь к директории
	raw, err := os.ReadFile(filepath.Join(ref, "plugin.yaml"))
	if err != nil {
		return nil, err
	}
	var manifest pipeline.Manifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil, err
	}
	manifest.Dir = ref
	return &manifest, nil
}

// TestCancelSleeper — отмена через 200ms: группа умирает <5с, статус cancelled, не timeout.
func TestCancelSleeper(t *testing.T) {
	dir := t.TempDir()
	sleeperDir := writeSleeper(t, filepath.Join(dir, "plugins"))
	runsDir := filepath.Join(dir, "runs")
	pf := &pipeline.PipelineFile{
		FormatVersion: "0.2",
		Pipeline: pipeline.Pipeline{
			Name:  "cancel_demo",
			Input: map[string]interface{}{},
			Steps: []pipeline.Step{{ID: "sleep", Plugin: sleeperDir, Timeout: pipeline.Duration{}}},
		},
	}
	eng := &mapEngine{}
	runCtx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	var stats RunStats
	start := time.Now()
	go func() {
		var err error
		stats, err = Run(pf, eng, RunOptions{Quiet: true, RunsDir: runsDir, Ctx: runCtx})
		done <- err
	}()
	time.Sleep(200 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		dur := time.Since(start)
		if err == nil {
			t.Fatalf("ожидалась отмена, ран завершился ok=%v", stats)
		}
		if dur > 10*time.Second {
			t.Fatalf("отмена заняла %s (want <10s)", dur)
		}
		msg := err.Error()
		if !(contains(msg, "cancelled") || contains(msg, "отмен")) {
			t.Fatalf("статус не cancelled: %v", err)
		}
		// журнал: run_cancelled, затем snapshot; --resume должен поднимать
		rd := journal.NewReader(stats.RunDir)
		events, _ := rd.Events()
		sawCancel := false
		for _, e := range events {
			if e["type"] == "run_cancelled" {
				sawCancel = true
				if code, _ := e["code"].(string); code != "cancelled" {
					t.Fatalf("run_cancelled без code=cancelled: %v", e)
				}
			}
			if e["type"] == "run_failed" {
				if code, _ := e["code"].(string); code == "timeout" {
					t.Fatalf("отмена превратилась в timeout")
				}
			}
		}
		if !sawCancel {
			types := []string{}
			for _, e := range events {
				if ty, _ := e["type"].(string); ty != "" {
					types = append(types, ty)
				}
			}
			t.Fatalf("нет run_cancelled, события: %v", types)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("ран не завершился за 30с после cancel")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	}())
}

func TestRetryDelayIsBounded(t *testing.T) {
	st := &pipeline.Step{Retry: &pipeline.Retry{Delay: pipeline.Duration{Duration: time.Hour}, Backoff: "exponential"}}
	if got := retryDelay(st, 10); got != pipeline.MaxRetryDelay {
		t.Fatalf("retry delay=%s, want cap %s", got, pipeline.MaxRetryDelay)
	}
}

func TestRunRejectsReservedBuiltin(t *testing.T) {
	pf := &pipeline.PipelineFile{
		FormatVersion: "0.2",
		Pipeline: pipeline.Pipeline{
			Name:  "reserved",
			Input: map[string]interface{}{},
			Steps: []pipeline.Step{{ID: "s", Plugin: "core/does_not_exist"}},
		},
	}
	_, err := Run(pf, permissiveEngine{}, RunOptions{Yes: true, Quiet: true, RunsDir: t.TempDir()})
	if err == nil || !contains(err.Error(), "неизвестный встроенный модуль") {
		t.Fatalf("expected reserved builtin error, got %v", err)
	}
}

func TestCloneCtxRejectsNaN(t *testing.T) {
	ctx := &runctx.Ctx{Data: map[string]interface{}{"value": math.NaN()}}
	if _, err := cloneCtx(ctx); err == nil {
		t.Fatal("cloneCtx accepted NaN")
	}
}

func TestRunParallelRejectsNaNContext(t *testing.T) {
	pf := &pipeline.PipelineFile{
		FormatVersion: "0.2",
		Pipeline: pipeline.Pipeline{
			Name:  "nan_context",
			Input: map[string]interface{}{"value": math.NaN()},
			Steps: []pipeline.Step{
				{ID: "a", Plugin: "fake", ParallelGroup: "g"},
				{ID: "b", Plugin: "fake", ParallelGroup: "g"},
			},
		},
	}
	_, err := Run(pf, permissiveEngine{}, RunOptions{Quiet: true, RunsDir: t.TempDir()})
	if err == nil || !contains(err.Error(), "context") {
		t.Fatalf("expected context serialization error, got %v", err)
	}
}

func TestResumeCursorDoesNotLoopOnHugeIndex(t *testing.T) {
	dir := t.TempDir()
	j, err := journal.NewJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := j.Event("item_end", map[string]interface{}{"item_index": 1 << 30, "status": "ok"}); err != nil {
		t.Fatal(err)
	}
	j.Close()
	result := make(chan struct {
		idx   int
		stats RunStats
		err   error
	}, 1)
	go func() {
		idx, stats, err := resumeCursor(dir)
		result <- struct {
			idx   int
			stats RunStats
			err   error
		}{idx, stats, err}
	}()
	select {
	case got := <-result:
		if got.err != nil || got.idx != 0 {
			t.Fatalf("resumeCursor: idx=%d stats=%+v err=%v", got.idx, got.stats, got.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("resumeCursor did not stop on a sparse huge index")
	}
}
