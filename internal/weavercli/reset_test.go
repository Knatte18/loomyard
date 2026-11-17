// reset_test.go pins the reset verb's envelopes and the weaver module's pre-run over fake seams.

package weavercli

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/preflight"
	"github.com/Knatte18/loomyard/internal/runplan"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
	"github.com/Knatte18/loomyard/internal/weaverengine"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// stubGit answers the one git question Reset asks; every other method of the embedded interface is nil.
type stubGit struct{ websterengine.Git }

func (stubGit) MergeInProgress(string) (bool, error) { return false, nil }

// stubRunner is a weaverengine.StrandRunner that finds no run and stops nothing.
type stubRunner struct{}

func (stubRunner) StartGated(shuttleengine.Spec, shuttleengine.GateSpec) (*shuttleengine.Run, error) {
	return nil, nil
}

func (stubRunner) ProbeGated(shuttleengine.Spec, shuttleengine.GateSpec) (*shuttleengine.Run, bool, error) {
	return nil, false, nil
}

func (stubRunner) StopStrand(string) error { return nil }

// resetDeps returns ResetDeps over temp directories.
// When begun is set, its run has recorded a begun batch starting at start.
func resetDeps(t *testing.T, begun bool) weaverengine.ResetDeps {
	t.Helper()
	root := t.TempDir()
	geom := weaverengine.Geometry{
		AnchorRoot:   root,
		WorktreeRoot: root,
		RunDir:       weaverengine.Dir(root),
		ScratchDir:   weaverengine.ScratchDir(root),
		Git:          stubGit{},
	}
	if begun {
		if err := os.MkdirAll(geom.RunDir, 0o755); err != nil {
			t.Fatal(err)
		}
		st := &weaverengine.State{RunGUID: "guid"}
		st.Batches = map[int]*runplan.BatchState{1: {Slug: "one", StartSHA: "start-sha", Kind: "strand"}}
		if err := weaverengine.SaveState(geom.RunDir, geom.ScratchDir, st); err != nil {
			t.Fatal(err)
		}
	}
	return weaverengine.ResetDeps{
		Geom:          geom,
		Runner:        stubRunner{},
		ResetCode:     func(string, []string) error { return nil },
		RecordPending: func() (bool, error) { return false, nil },
		Commit:        func(string) error { return nil },
		DirtyTracked:  func() ([]string, error) { return nil, nil },
	}
}

func TestResetVerb(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	t.Run("OkEnvelopeCarriesTheStartCommitThePlanCommitAndTheArchivedNames", func(t *testing.T) {
		t.Parallel()
		var out bytes.Buffer

		code := resetVerb(&out, resetDeps(t, true), false)

		var envelope map[string]any
		if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
			t.Fatalf("envelope %q: %v", out.String(), err)
		}
		archived, _ := envelope["archived"].([]any)
		if code != 0 || envelope["ok"] != true || envelope["start_sha"] != "start-sha" || envelope["plan_commit"] != "" || len(archived) != 1 {
			t.Errorf("resetVerb() = %d, %v; want ok with start sha %q and the archived state file", code, envelope, "start-sha")
		}
	})

	t.Run("RefusalIsAnErrEnvelopeCarryingItsWayForward", func(t *testing.T) {
		t.Parallel()
		var out bytes.Buffer

		code := resetVerb(&out, resetDeps(t, false), false)

		var envelope map[string]any
		if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
			t.Fatalf("envelope %q: %v", out.String(), err)
		}
		message, _ := envelope["error"].(string)
		if code != 1 || envelope["ok"] != false || !strings.Contains(message, "no Weaver run has begun") || !strings.Contains(message, "way forward") {
			t.Errorf("resetVerb() = %d, %v; want an err envelope naming the way forward", code, envelope)
		}
	})

	hubLocation := func(string) (*lymcwd.Location, preflight.Mode, error) {
		return &lymcwd.Location{}, preflight.ModeHub, nil
	}

	t.Run("StopStrandFlagReachesReset", func(t *testing.T) {
		t.Parallel()
		for _, args := range [][]string{{"reset"}, {"reset", "--stop-strand"}} {
			var got []bool
			c := &weaverCLI{
				resolve:      hubLocation,
				resetDepsFor: func(*lymcwd.Location) (weaverengine.ResetDeps, error) { return weaverengine.ResetDeps{}, nil },
				runReset: func(_ io.Writer, _ weaverengine.ResetDeps, stop bool) int {
					got = append(got, stop)
					return 0
				},
			}

			if code := clihelp.ExecuteIn(newCommand(c), filepath.Join(t.TempDir()), io.Discard, args); code != 0 {
				t.Fatalf("%v = exit %d; want 0", args, code)
			}

			if want := len(args) == 2; len(got) != 1 || got[0] != want {
				t.Errorf("%v reached Reset with stop %v; want [%v]", args, got, want)
			}
		}
	})

	t.Run("PreRunOutsideAHubRefusesWithTheOutsideHubRefusal", func(t *testing.T) {
		t.Parallel()
		var out bytes.Buffer
		ran := false
		c := &weaverCLI{
			resolve: func(string) (*lymcwd.Location, preflight.Mode, error) { return nil, preflight.ModeStandalone, nil },
			resetDepsFor: func(*lymcwd.Location) (weaverengine.ResetDeps, error) {
				ran = true
				return weaverengine.ResetDeps{}, nil
			},
			runReset: func(io.Writer, weaverengine.ResetDeps, bool) int { ran = true; return 0 },
		}

		code := clihelp.ExecuteIn(newCommand(c), t.TempDir(), &out, []string{"reset"})

		if code != 1 || !strings.Contains(out.String(), outsideHubRefusal) || ran {
			t.Errorf("reset outside a hub = exit %d, ran %v, output %q; want exit 1 with the outside-hub refusal and no reset", code, ran, out.String())
		}
	})
}
