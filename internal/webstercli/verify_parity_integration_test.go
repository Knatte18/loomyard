//go:build integration

// verify_parity_integration_test.go asserts the Gate Self-Check Parity Invariant for the verify gates:
// `lyx webster verify`, loomshed.NewVerifyGate and websterengine.NewVerifyGate must reach the same three-way verdict over one fixture.
// It is integration-tagged because every side spawns git and a shell.
// The shape follows loomcli's TestGateParity_*.

package webstercli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// verifyParityVerdict is the three-valued outcome each side maps onto before comparison.
type verifyParityVerdict string

const (
	verifyVerdictDone  verifyParityVerdict = "done"
	verifyVerdictStuck verifyParityVerdict = "stuck"
	verifyVerdictError verifyParityVerdict = "error"
)

// gateVerdict maps a gate closure's raw result onto a verdict: an error is error, a failed result is stuck, a passed one is done.
func gateVerdict(result shuttleengine.GateResult, err error) verifyParityVerdict {
	if err != nil {
		return verifyVerdictError
	}
	if !result.Passed {
		return verifyVerdictStuck
	}
	return verifyVerdictDone
}

// verbVerdict maps a decoded verb envelope onto a verdict: ok is done, not ok with a findings key is stuck, not ok without one is error.
func verbVerdict(env envelope.Envelope) verifyParityVerdict {
	if env.OK {
		return verifyVerdictDone
	}
	if _, hasFindings := env.Raw["findings"]; hasFindings {
		return verifyVerdictStuck
	}
	return verifyVerdictError
}

func TestGateParity_VerifyGate(t *testing.T) {
	cases := []struct {
		name    string
		command string
		dirty   bool
		want    verifyParityVerdict
	}{
		{name: "passing command", command: "exit 0", want: verifyVerdictDone},
		{name: "failing command", command: "exit 1", want: verifyVerdictStuck},
		{name: "dirty tree", command: "exit 0", dirty: true, want: verifyVerdictStuck},
		{name: "no verify section", command: "", want: verifyVerdictDone},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Each side gets its own fixture, so one side's record cannot skip another side's run.
			prepare := func() *verifyFixture {
				fx := newVerifyFixture(t, tc.command)
				if tc.dirty {
					if err := os.WriteFile(filepath.Join(fx.Worktree, "stray.txt"), []byte("x"), 0o644); err != nil {
						t.Fatalf("write stray file: %v", err)
					}
				}
				return fx
			}

			verbFx := prepare()
			_, env := verbFx.run(t)
			verb := verbVerdict(env)

			burlerFx := prepare()
			geom := burlerFx.CLI.geom
			burler := gateVerdict(loomshed.NewVerifyGate(geom.WorktreeRoot, geom.VerifyDir, "Webster-Burler gate", func() (string, error) { return tc.command, nil }, nil, nil)())

			websterFx := prepare()
			wgeom := websterFx.CLI.geom
			if err := os.MkdirAll(wgeom.WebsterDir, 0o755); err != nil {
				t.Fatalf("mkdir webster dir: %v", err)
			}
			if err := os.WriteFile(websterengine.OutcomePath(wgeom.WebsterDir), []byte("outcome: done\nstuck_reason: null\nbatches_done: 1\n"), 0o644); err != nil {
				t.Fatalf("write outcome.yaml: %v", err)
			}
			gate, _ := websterengine.NewVerifyGate(wgeom, 3, nil, nil, "", "/prompts/verify-fix.md")
			webster := gateVerdict(gate())

			if verb != tc.want || burler != tc.want || webster != tc.want {
				t.Errorf("verdicts: verb %q, Webster-Burler gate %q, webster gate %q; want all %q", verb, burler, webster, tc.want)
			}
		})
	}
}
