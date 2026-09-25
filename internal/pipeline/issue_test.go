package pipeline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stubEngine — фейковые манифесты без диска.
type stubEngine struct {
	mans map[string]*Manifest
}

func (e *stubEngine) LoadManifest(ref string) (*Manifest, error) {
	if m, ok := e.mans[ref]; ok {
		return m, nil
	}
	return nil, &stubErr{msg: "плагин " + ref + " не найден (stub)"}
}

type stubErr struct{ msg string }

func (e *stubErr) Error() string { return e.msg }

func stubBase() *stubEngine {
	syntax := &Manifest{
		ID: "fake/syntax",
		Input: map[string]Port{
			"email": {From: "input.item", Type: "string", Format: "email"},
		},
		Output: map[string]Port{
			"syntax": {Type: "boolean"},
			"email":  {Type: "string", Format: "email"},
		},
	}
	needTwo := &Manifest{
		ID: "fake/need_two",
		Input: map[string]Port{
			"a": {Type: "string"},
			"b": {Type: "string"},
		},
		Output: map[string]Port{"done": {Type: "boolean"}},
	}
	arrProducer := &Manifest{
		ID:     "fake/arr_producer",
		Output: map[string]Port{"arr": {Type: "array"}},
		Input:  map[string]Port{},
	}
	objProducer := &Manifest{
		ID:     "fake/obj_producer",
		Output: map[string]Port{"obj": {Type: "object"}},
		Input:  map[string]Port{},
	}
	objConsumer := &Manifest{
		ID: "fake/obj_consumer",
		Input: map[string]Port{
			"x": {From: "steps.p.arr", Type: "object"},
		},
		Output: map[string]Port{"done": {Type: "boolean"}},
	}
	return &stubEngine{mans: map[string]*Manifest{
		"fake/syntax":       syntax,
		"fake/need_two":     needTwo,
		"fake/arr_producer": arrProducer,
		"fake/obj_producer": objProducer,
		"fake/obj_consumer": objConsumer,
	}}
}

func loadFixture(t *testing.T, name string) *PipelineFile {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "issues", name))
	if err != nil {
		t.Fatalf("fixture %s: %v", name, err)
	}
	pf, err := LoadPipelineFileFromBytes(raw)
	if err != nil {
		t.Fatalf("fixture %s parse: %v", name, err)
	}
	return pf
}

// TestIssueCodes — по одной фикстуре на код: ожидаемый код ровно один.
func TestIssueCodes(t *testing.T) {
	eng := stubBase()
	cases := []struct {
		file     string
		code     string
		severity Severity
		wantFix  bool
	}{
		{"E_BIND_UNKNOWN_PORT.yaml", E_BIND_UNKNOWN_PORT, SeverityError, true},
		{"E_PORT_UNBOUND.yaml", E_PORT_UNBOUND, SeverityError, true},
		{"E_PORT_SOURCE.yaml", E_PORT_SOURCE, SeverityError, true},
		{"E_TYPE_MISMATCH.yaml", E_TYPE_MISMATCH, SeverityError, true},
		{"W_FOREACH_ITEM_FORMAT.yaml", W_FOREACH_ITEM_FORMAT, SeverityWarning, false},
	}
	for _, tc := range cases {
		t.Run(tc.code, func(t *testing.T) {
			pf := loadFixture(t, tc.file)
			issues := ValidateIssues(pf, eng)
			got := FilterCode(issues, tc.code)
			if len(got) != 1 {
				kinds := []string{}
				for _, is := range issues {
					kinds = append(kinds, is.Code+":"+is.Message)
				}
				t.Fatalf("ожидался ровно 1 %s, получено %d: %v", tc.code, len(got), kinds)
			}
			if got[0].Severity != tc.severity {
				t.Fatalf("%s: severity %q, а ожидалась %q", tc.code, got[0].Severity, tc.severity)
			}
			if got[0].Message == "" || got[0].Path == "" {
				t.Fatalf("%s: пустые Message/Path: %+v", tc.code, got[0])
			}
			if tc.wantFix {
				if got[0].Fix == nil || len(got[0].Fix.Candidates) == 0 {
					t.Fatalf("%s: нет Fix.candidates: %+v", tc.code, got[0])
				}
				if got[0].Hint == "" {
					t.Fatalf("%s: нет Hint", tc.code)
				}
			}
		})
	}
}

// TestIssueCodesTable — остальные коды без YAML: конструкция в коде.
func TestIssueCodesTable(t *testing.T) {
	eng := stubBase()
	mk := func(mut func(*PipelineFile)) *PipelineFile {
		pf := &PipelineFile{
			FormatVersion: "0.2",
			Pipeline: Pipeline{
				Name:  "t",
				Input: map[string]interface{}{"email": "a@b.c"},
				Steps: []Step{{ID: "s", Plugin: "fake/syntax", Bind: map[string]string{"email": "input.email"}}},
			},
		}
		mut(pf)
		return pf
	}
	cases := []struct {
		name string
		mut  func(*PipelineFile)
		code string
	}{
		{"format_version", func(pf *PipelineFile) { pf.FormatVersion = "9.9" }, E_FORMAT_VERSION},
		{"cycle", func(pf *PipelineFile) {
			pf.Pipeline.Steps = []Step{
				{ID: "a", Plugin: "fake/syntax", Bind: map[string]string{"email": "steps.b.email"}},
				{ID: "b", Plugin: "fake/syntax", Bind: map[string]string{"email": "steps.a.email"}},
			}
		}, E_CYCLE},
		{"foreach_path", func(pf *PipelineFile) { pf.Pipeline.Foreach = "oops" }, E_FOREACH_PATH},
		{"foreach_not_found", func(pf *PipelineFile) { pf.Pipeline.Foreach = "input.nope" }, E_FOREACH_NOT_FOUND},
		{"step_id_empty", func(pf *PipelineFile) { pf.Pipeline.Steps[0].ID = "" }, E_STEP_ID_EMPTY},
		{"step_id_dup", func(pf *PipelineFile) {
			pf.Pipeline.Steps = append(pf.Pipeline.Steps, Step{ID: "s", Plugin: "fake/syntax", Bind: map[string]string{"email": "input.email"}})
		}, E_STEP_ID_DUP},
		{"when_op", func(pf *PipelineFile) {
			pf.Pipeline.Steps[0].When.Op = "nope"
			pf.Pipeline.Steps[0].When.Path = "input.email"
		}, E_WHEN_OP},
		{"when_forward", func(pf *PipelineFile) {
			pf.Pipeline.Steps[0].When.Op = "truthy"
			pf.Pipeline.Steps[0].When.Path = "steps.s.email"
		}, E_WHEN_FORWARD_REF},
		{"on_error", func(pf *PipelineFile) { pf.Pipeline.Steps[0].OnError = "nope" }, E_ON_ERROR},
		{"gate_bind", func(pf *PipelineFile) {
			pf.Pipeline.Steps[0].Plugin = "core/human_gate"
			pf.Pipeline.Steps[0].Bind = map[string]string{"x": "input.email"}
		}, E_GATE_BIND},
		{"gate_actions", func(pf *PipelineFile) {
			pf.Pipeline.Steps[0].Plugin = "core/human_gate"
			pf.Pipeline.Steps[0].Actions = []string{"<img src=x onerror=alert(1)>"}
		}, E_GATE_ACTIONS},
		{"plugin_load", func(pf *PipelineFile) { pf.Pipeline.Steps[0].Plugin = "fake/nope" }, E_PLUGIN_LOAD},
		{"format_input", func(pf *PipelineFile) {
			pf.Pipeline.Input["email"] = "bad-email"
		}, E_FORMAT_INPUT},
		{"approval_value", func(pf *PipelineFile) {
			pf.Pipeline.Steps[0].Approval = "nope"
		}, E_APPROVAL_VALUE},
		{"gates_value", func(pf *PipelineFile) {
			pf.Pipeline.Gates = "nope"
		}, E_GATES_VALUE},
		{"parallel_split", func(pf *PipelineFile) {
			pf.Pipeline.Steps = []Step{
				{ID: "a", Plugin: "fake/syntax", Bind: map[string]string{"email": "input.email"}, ParallelGroup: "g"},
				{ID: "b", Plugin: "fake/syntax", Bind: map[string]string{"email": "input.email"}},
				{ID: "c", Plugin: "fake/syntax", Bind: map[string]string{"email": "input.email"}, ParallelGroup: "g"},
			}
		}, E_PARALLEL_SPLIT},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			issues := ValidateIssues(mk(tc.mut), eng)
			if got := FilterCode(issues, tc.code); len(got) != 1 {
				all := []string{}
				for _, is := range issues {
					all = append(all, is.Code)
				}
				t.Fatalf("ожидался ровно 1 %s, получено %d (все: %v)", tc.code, len(got), all)
			}
		})
	}
}

// TestValidateBackwardCompat — старые строки совпадают с SplitIssues.
func TestValidateBackwardCompat(t *testing.T) {
	eng := stubBase()
	pf := loadFixture(t, "E_PORT_SOURCE.yaml")
	errs1, warns1 := Validate(pf, eng)
	issues := ValidateIssues(pf, eng)
	errs2, warns2 := SplitIssues(issues)
	if strings.Join(errs1, "|") != strings.Join(errs2, "|") || strings.Join(warns1, "|") != strings.Join(warns2, "|") {
		t.Fatalf("Validate != SplitIssues(ValidateIssues): %v/%v vs %v/%v", errs1, warns1, errs2, warns2)
	}
}

// TestIssueJSON — golden: код и fix сериализуются для агента.
func TestIssueJSON(t *testing.T) {
	eng := stubBase()
	pf := loadFixture(t, "E_BIND_UNKNOWN_PORT.yaml")
	issues := ValidateIssues(pf, eng)
	got := FilterCode(issues, E_BIND_UNKNOWN_PORT)
	if len(got) != 1 {
		t.Fatalf("нет E_BIND_UNKNOWN_PORT")
	}
	b, _ := json.Marshal(got[0])
	s := string(b)
	for _, want := range []string{`"code":"E_BIND_UNKNOWN_PORT"`, `"fix"`, `"candidates"`, `"severity":"error"`} {
		if !strings.Contains(s, want) {
			t.Fatalf("JSON без %s: %s", want, s)
		}
	}
}

func TestResourceLimitIssues(t *testing.T) {
	arr := make([]interface{}, MaxForeachItems+1)
	for i := range arr {
		arr[i] = i
	}
	pf := &PipelineFile{
		FormatVersion: "0.2",
		Pipeline: Pipeline{
			Name:    "limits",
			Input:   map[string]interface{}{"items": arr},
			Foreach: "input.items",
			Steps:   []Step{{ID: "s", Plugin: "fake/syntax", OnError: "retry", Retry: &Retry{Attempts: MaxRetryAttempts + 1}}},
		},
	}
	issues := ValidateIssues(pf, stubBase())
	for _, code := range []string{E_FOREACH_LIMIT, E_RETRY_LIMIT} {
		if len(FilterCode(issues, code)) != 1 {
			t.Fatalf("missing %s in %+v", code, issues)
		}
	}
}

func TestReservedBuiltinRejectedByValidators(t *testing.T) {
	eng := &stubEngine{mans: map[string]*Manifest{
		"core/does_not_exist": {ID: "core/does_not_exist"},
	}}
	pf := &PipelineFile{
		FormatVersion: "0.2",
		Pipeline: Pipeline{
			Name:  "reserved",
			Steps: []Step{{ID: "s", Plugin: "core/does_not_exist"}},
		},
	}
	errs, _ := Validate(pf, eng)
	if len(errs) != 1 || !strings.Contains(errs[0], "неизвестный встроенный модуль") {
		t.Fatalf("Validate errors: %v", errs)
	}
	issues := ValidateIssues(pf, eng)
	if got := FilterCode(issues, E_PLUGIN_LOAD); len(got) != 1 {
		t.Fatalf("ValidateIssues errors: %+v", issues)
	}
}
