package core

import (
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

const pipelineDoc = `
format_version: "0.1"
pipeline:
  name: t
  input:
    emails: ["a@b.c"]
  foreach: input.emails
  foreach_item: item
  item_type: string
  item_format: email
  steps:
    - id: a
      plugin: plugins/x
      on_error: retry
      timeout: 5s
      retry: { attempts: 4, delay: 250ms, backoff: exponential }
    - id: g
      plugin: core/human_gate
      form:
        - { field: steps.a.mx, editable: true, type: boolean }
      actions: [accept, reject]
      on_reject: continue
`

func TestPipelineUnmarshal(t *testing.T) {
	var pf PipelineFile
	if err := yaml.Unmarshal([]byte(pipelineDoc), &pf); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	p := pf.Pipeline
	if pf.FormatVersion != "0.1" || p.Name != "t" {
		t.Fatalf("заголовок не распарсился: %+v", pf)
	}
	if p.Foreach != "input.emails" || p.ForeachItem != "item" {
		t.Fatalf("foreach не распарсился: %+v", p)
	}
	if p.ItemType != "string" || p.ItemFormat != "email" {
		t.Fatalf("item_type/format не распарсились: %+v", p)
	}

	st := p.Steps[0]
	if st.OnError != "retry" || st.Timeout.Duration != 5*time.Second {
		t.Fatalf("шаг a: %+v", st)
	}
	if st.Retry == nil || st.Retry.Attempts != 4 ||
		st.Retry.Delay.Duration != 250*time.Millisecond || st.Retry.Backoff != "exponential" {
		t.Fatalf("retry не распарсился: %+v", st.Retry)
	}

	gate := p.Steps[1]
	if gate.Form[0].Field != "steps.a.mx" || !gate.Form[0].Editable || gate.OnReject != "continue" {
		t.Fatalf("гейт не распарсился: %+v", gate)
	}
}

func TestDurationInvalid(t *testing.T) {
	var st Step
	err := yaml.Unmarshal([]byte("id: x\ntimeout: 5лет\n"), &st)
	if err == nil {
		t.Fatal("невалидная длительность обязана давать ошибку парсинга")
	}
}
