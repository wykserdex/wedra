package execution

import (
	"testing"

	"wedra/internal/pipeline"
)

// netEngine отдаёт фиксированный манифест с заданным сетевым объявлением.
type netEngine struct {
	permissiveEngine
	net []pipeline.NetworkPermission
}

func (e netEngine) LoadManifest(ref string) (*pipeline.Manifest, error) {
	return &pipeline.Manifest{ID: ref, Permissions: pipeline.Permissions{Network: e.net}}, nil
}

func netPipeline(network string) *pipeline.PipelineFile {
	return &pipeline.PipelineFile{
		FormatVersion: "0.2",
		Pipeline: pipeline.Pipeline{
			Name:    "net",
			Network: network,
			Input:   map[string]interface{}{},
			Steps: []pipeline.Step{
				{ID: "s", Plugin: "acme/probe"},
			},
		},
	}
}

var structuredNet = []pipeline.NetworkPermission{{Host: "api.example.com", Port: 443}}

var blanketNet = []pipeline.NetworkPermission{{AnyHost: true}}

// The network gate as it behaves today. deny is the documented default,
// "allow" is an explicit opt-in, and only an any_host declaration is actually
// enforceable.
//
// Assertions use ASCII markers on purpose: the messages this gate produces are
// Russian, and a test matching on Russian literals was silently corrupted by an
// editor rewriting the file in another encoding, reddening a passing suite for
// the wrong reason.
//
// KNOWN GAP, asserted here so it cannot go unnoticed: a pipeline that omits the
// `network` field entirely runs neither branch and therefore gets no network
// check at all, which is fail-open even though the documentation says deny is
// the default. Fixing that means treating unset as deny, which refuses existing
// pipelines whose plugins declare a host, so it needs its own migration. If
// this row ever starts failing, that fix landed and the expectations below plus
// SECURITY.md need updating together.
func TestNetworkPolicyMatrix(t *testing.T) {
	cases := []struct {
		name     string
		network  string
		net      []pipeline.NetworkPermission
		wantDeny bool
		marker   string
	}{
		{"deny, host:port declared", "deny", structuredNet, true, "network: deny"},
		{"allow, host:port declared", "allow", structuredNet, true, "host:port"},
		{"allow, explicit any_host", "allow", blanketNet, false, ""},
		{"allow, no network declared", "allow", nil, false, ""},
		{"unset field, no network declared", "", nil, false, ""},
		// Known gap: unset means "no gate", so a declared host is not refused.
		{"unset field, host:port declared (known gap)", "", structuredNet, false, ""},
		{"unset field, any_host declared (known gap)", "", blanketNet, false, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pf := netPipeline(tc.network)
			_, err := Run(pf, netEngine{net: tc.net}, RunOptions{Quiet: true, RunsDir: t.TempDir()})
			msg := ""
			if err != nil {
				msg = err.Error()
			}
			if tc.wantDeny {
				if err == nil {
					t.Fatal("expected the network gate to refuse, but the run proceeded")
				}
				if !contains(msg, tc.marker) {
					t.Fatalf("expected refusal containing %q, got: %v", tc.marker, err)
				}
				return
			}
			// The gate let it through: nothing about network may appear in the
			// error. A later runtime failure is fine and not what is asserted.
			if contains(msg, "network") {
				t.Fatalf("the network gate should not have fired, got: %v", err)
			}
		})
	}
}
