package core

import (
	"testing"
	"time"
)

// fx — манифест с минимумом полей для запуска фикстуры из testdata/plugins.
func fx(name string) *Manifest {
	return &Manifest{
		ID:      name,
		Dir:     "testdata/plugins/" + name,
		Runtime: Runtime{Type: "python", Entry: "main.py"},
	}
}

func TestExecOK(t *testing.T) {
	requirePython(t)
	res := execPlugin(fx("echo_ok"), []byte(`{"value":"hello"}`), 5*time.Second)
	if !res.OK() {
		t.Fatalf("ожидался ok, got %+v", res)
	}
	if res.Output["value"] != "hello" {
		t.Fatalf("эхо вернуло не то: %v", res.Output)
	}
}

func TestExecDomainError(t *testing.T) {
	requirePython(t)
	res := execPlugin(fx("failer"), []byte(`{"value":"bad"}`), 5*time.Second)
	if res.OK() || res.Platform {
		t.Fatalf("доменная ошибка не должна быть платформенной: %+v", res)
	}
	if res.ExitCode != 1 || res.ErrCode != "bad_value" || res.Retryable {
		t.Fatalf("поля ошибки: %+v", res)
	}
	if res.shouldRetry() {
		t.Fatal("не-retryable доменная ошибка не должна ретраиться")
	}
}

func TestExecRetryableFlag(t *testing.T) {
	requirePython(t)
	res := execPlugin(fx("failer"), []byte(`{"value":"flaky"}`), 5*time.Second)
	if !res.Retryable || !res.shouldRetry() {
		t.Fatalf("retryable доменная ошибка должна ретраиться: %+v", res)
	}
}

func TestExecCrashIsPlatform(t *testing.T) {
	requirePython(t)
	res := execPlugin(fx("crasher"), []byte(`{}`), 5*time.Second)
	if !res.Platform || res.ExitCode != 2 {
		t.Fatalf("exit 2 обязан быть платформенной ошибкой: %+v", res)
	}
	if res.shouldRetry() {
		t.Fatal("краш без таймаута не ретраится (ретраит только timeout/retryable)")
	}
}

func TestExecProtocolViolation(t *testing.T) {
	requirePython(t)
	res := execPlugin(fx("bad_proto"), []byte(`{}`), 5*time.Second)
	if !res.Platform || res.ErrCode != "protocol_violation" {
		t.Fatalf("не-JSON на stdout → protocol_violation: %+v", res)
	}
}

func TestExecTimeout(t *testing.T) {
	requirePython(t)
	start := time.Now()
	res := execPlugin(fx("sleeper"), []byte(`{}`), 200*time.Millisecond)
	if !res.TimedOut || !res.Platform || res.ErrCode != "timeout" {
		t.Fatalf("ожидался timeout-платформенный результат: %+v", res)
	}
	if time.Since(start) > 3*time.Second {
		t.Fatal("таймаут не убил процесс вовремя")
	}
	if res.shouldRetry() {
		// timeout → retryable — так задумано (PROTOCOL §6)
	} else {
		t.Fatal("таймаут должен помечаться ретраибельным")
	}
}

func TestBuildInput(t *testing.T) {
	m := &Manifest{Input: map[string]Port{
		"req":  {From: "input.a"},
		"opt":  {From: "input.missing", Optional: true},
		"from": {From: "steps.x.f"},
	}}
	ctx := NewCtx(map[string]interface{}{"a": "v"})
	ctx.SetStep("x", map[string]interface{}{"f": 42.0})

	in, err := buildInput(m, nil, ctx)
	if err != nil {
		t.Fatal(err)
	}
	if in["req"] != "v" || in["from"] != 42.0 {
		t.Fatalf("вход собран не по манифесту: %v", in)
	}
	if _, present := in["opt"]; present {
		t.Fatal("optional-вход при отсутствии данных не должен попадать в stdin")
	}
}

func TestBuildInputMissingRequired(t *testing.T) {
	m := &Manifest{Input: map[string]Port{"req": {From: "input.nope"}}}
	if _, err := buildInput(m, nil, NewCtx(nil)); err == nil {
		t.Fatal("отсутствующий обязательный вход обязан давать ошибку")
	}
}

func TestEnforceOutput(t *testing.T) {
	m := &Manifest{Output: map[string]Port{
		"value": {Type: "string"},
		"maybe": {Type: "string", Optional: true},
	}}

	// лишнее поле отбрасывается
	clean, dropped, err := EnforceOutput(m, map[string]interface{}{"value": "x", "junk": 1.0})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := clean["junk"]; ok || len(dropped) != 1 || dropped[0] != "junk" {
		t.Fatalf("необъявленное поле должно отбрасываться: clean=%v dropped=%v", clean, dropped)
	}

	// пропавшее обязательное — нарушение контракта
	if _, _, err := EnforceOutput(m, map[string]interface{}{}); err == nil {
		t.Fatal("пропавшее обязательное поле обязано стопить ран")
	}

	// optional можно не возвращать
	if _, _, err := EnforceOutput(m, map[string]interface{}{"value": "x"}); err != nil {
		t.Fatalf("optional-поле не обязано присутствовать: %v", err)
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("abc", 10); got != "abc" {
		t.Fatal(got)
	}
	long := make([]byte, 300)
	for i := range long {
		long[i] = 'x'
	}
	if got := truncate(string(long), 10); len(got) > 20 {
		t.Fatalf("truncate не работает: %d", len(got))
	}
}
