package cli

import "testing"

func TestRunIDFromDir(t *testing.T) {
	if got := runIDFromDir("var/runs/20260924-120000-demo"); got != "20260924-120000-demo" {
		t.Fatalf("run id = %q", got)
	}
	if got := runIDFromDir(""); got != "" {
		t.Fatalf("empty run id = %q", got)
	}
}
