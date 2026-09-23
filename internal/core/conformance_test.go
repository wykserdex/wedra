package core

import "testing"

// TestConformance — батарея ядра (см. conformance/README.md).
// CI запускает её как go test ./internal/core -run TestConformance.
func TestConformance(t *testing.T) {
	report := RunConformance("testdata/plugins")
	for _, c := range report.Checks {
		t.Logf("%s pass=%v %s", c.Name, c.Pass, c.Details)
		if !c.Pass {
			t.Errorf("conformance %s: %s", c.Name, c.Details)
		}
	}
	if !report.OK {
		t.Fatal("conformance failed")
	}
}
