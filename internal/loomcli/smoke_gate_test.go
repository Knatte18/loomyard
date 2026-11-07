// smoke_gate_test.go is the live-substrate guard for the gate re-prompt loop in shuttleengine's Wait.
// Hermetic test fakes evaluate a gate closure exactly once, so they never run the send-and-poll loop;
// this test runs that loop against the real wait loop.
//
// It drives the real substrate: a hub, a reed session, a tmux pane, and a shuttleengine.Runner writing run.json.
// It also runs a real loomshed.NewDiscussionGate closure, and it spawns no provider subprocess.
// A stub Engine's Prepare launches a plain shell script in place of a provider.
// The script writes a deliberately invalid first artifact.
// It then blocks reading its next turn from the pane, which is where a real agent blocks between turns;
// it writes a valid artifact only once that turn arrives.
// This proves the gate's re-prompt text was typed into a real pane and read back out of it;
// no fake Send can show that.
//
// The test lives in this package because it reuses the fixtures the package's smoke tests already build:
// the hub fixture, the tmux skip, the reed engine probe, and the hermetic git TestMain.
//
// The build tag excludes this file from untagged runs and from the repo-wide done gate.
// Its only automatic guard is a compile check under the tag: `go test -tags tmux -run '^$' ./internal/loomcli/...`.
// An operator runs the test by hand against a real substrate.

package loomcli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/planglyph"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/shuttlefake"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// gateRepromptReadEngine is a shuttleengine.Engine that launches a shell script in place of a re-prompted agent.
// ComposeSend types text into the pane and submits it, which unblocks the script's `read`.
//
// Every other method is the embedded shuttlefake.Engine's inert answer, as the suite never reaches them.
// It is used through a pointer, since the embedded fake holds a mutex.
type gateRepromptReadEngine struct {
	shuttlefake.Engine
	// quietSeconds is how long the script sleeps before its first, deliberately invalid artifact.
	// The delay keeps the run in flight, not already complete, when Wait's first tick runs.
	quietSeconds int
}

// Prepare writes a script that waits quietSeconds, then writes an invalid decision record and an existing support log;
// it appends a turn-end event and blocks on a `read` of its next turn, as a real agent's pane does between turns.
// Once that turn arrives, the script overwrites the decision record with a valid one.
// It then appends a second turn-end event and sits idle.
//
// The `read` line makes this a live-substrate proof rather than a restatement of the gate contract.
// The shell's tty driver echoes the re-prompt text into the pane's visible viewport;
// the run loop requires that same evidence of a real agent's pane, and the script waits until that delivery lands.
func (e *gateRepromptReadEngine) Prepare(runDir string, spec shuttleengine.Spec, _ shuttleengine.Config) (shuttleengine.Launch, error) {
	if len(spec.OutputFiles) != 2 {
		return shuttleengine.Launch{}, fmt.Errorf("gateRepromptReadEngine: want exactly 2 output files (decision record, support log), got %d", len(spec.OutputFiles))
	}
	decisionRecordPath, supportLogPath := spec.OutputFiles[0], spec.OutputFiles[1]
	eventsPath := filepath.Join(runDir, "events.jsonl")

	invalidDecisionRecord := "# Decision Record\n\nThis first draft carries none of the required sections.\n"
	validDecisionRecord := "" +
		"# Decision Record\n\n" +
		"## Goal\nSmoke the gate re-prompt loop.\n\n" +
		"## Scope\nOne gated Discussion-Write row.\n\n" +
		"## Decisions\nFix the record once re-prompted.\n\n" +
		"## Constraints\nNone beyond the smoke harness.\n\n" +
		"## Auto-mode assumptions\nNone.\n\n" +
		"## Open risks\nNone.\n\n" +
		"## Acceptance criteria\nThe gate passes on the second attempt.\n"

	var script strings.Builder
	fmt.Fprintf(&script, "#!/bin/sh\n")
	fmt.Fprintf(&script, "sleep %d\n", e.quietSeconds)
	fmt.Fprintf(&script, "cat > '%s' <<'DECISIONRECORD_INVALID'\n%sDECISIONRECORD_INVALID\n", decisionRecordPath, invalidDecisionRecord)
	fmt.Fprintf(&script, "printf 'support log\\n' > '%s'\n", supportLogPath)
	fmt.Fprintf(&script, "printf 'turn-end\\n' >> '%s'\n", eventsPath)
	// Block for this run's next turn, exactly where a real agent's pane blocks between turns. The
	// re-prompt text Wait sends via Send lands here, submitted with an Enter that unblocks read.
	fmt.Fprintf(&script, "read _reprompt_line\n")
	fmt.Fprintf(&script, "cat > '%s' <<'DECISIONRECORD_VALID'\n%sDECISIONRECORD_VALID\n", decisionRecordPath, validDecisionRecord)
	fmt.Fprintf(&script, "printf 'turn-end\\n' >> '%s'\n", eventsPath)
	fmt.Fprintf(&script, "sleep 600\n")

	scriptPath := filepath.Join(runDir, "smoke-gate-launch.sh")
	if err := os.WriteFile(scriptPath, []byte(script.String()), 0o755); err != nil {
		return shuttleengine.Launch{}, fmt.Errorf("write smoke gate launch script: %w", err)
	}
	return shuttleengine.Launch{Cmd: "sh " + scriptPath, SessionID: "smoke-gate-reprompt"}, nil
}

// ParseEvents maps any non-empty events.jsonl content onto a single turn-end event.
func (e *gateRepromptReadEngine) ParseEvents(data []byte) ([]shuttleengine.Event, error) {
	if len(data) == 0 {
		return nil, nil
	}
	return []shuttleengine.Event{{Kind: shuttleengine.EventStop, Raw: data}}, nil
}

// ComposeSend types text into the pane and submits it with an Enter, which unblocks the script's `read`.
func (e *gateRepromptReadEngine) ComposeSend(text string) []shuttleengine.PaneInput {
	return []shuttleengine.PaneInput{{Text: text, Submit: true}}
}

// TestSmokeGate_RepromptsThroughARealPaneAndFixesTheArtifact proves that a real agent fixes its artifact.
// The agent is re-prompted through a real pane after an invalid first attempt, and the row reports Done.
//
// It runs loomshed.NewDiscussionGate over a real shuttleengine.Runner's RunGated.
// gateRepromptReadEngine stands in for the provider, as shellLaunchEngine does in smoke_attachprobe_test.go.
// It asserts the run reaches shuttleengine.OutcomeDone with a passed gate.
// It also asserts that the gate's reported Attempts is at least 1.
// The artifact on disk must also pass the same gate closure afterwards.
func TestSmokeGate_RepromptsThroughARealPaneAndFixesTheArtifact(t *testing.T) {
	tiergate.Require(t, 2)
	tmuxBinaryPath(t)
	_, loc, worktree, _ := newWiredPairFixture(t)
	registerBootstrapTeardown(t, loc, worktree)

	reedEngine := probeReedEngine(t, loc)
	if _, err := reedEngine.Up(); err != nil {
		t.Fatalf("reed up: %v", err)
	}

	runDir := filepath.Join(loomengine.LoomReviewsDir(loc), "gate-smoke")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", runDir, err)
	}
	decisionRecordPath := filepath.Join(runDir, "decision-record.md")
	supportLogPath := filepath.Join(runDir, "support-log.md")

	shuttleCfg, err := shuttleengine.LoadConfig(configengine.Dirs{Anchor: loc.AnchorPath()}, "shuttle")
	if err != nil {
		t.Fatalf("load shuttle config: %v", err)
	}
	reedGeom, err := hubgeom.ReedGeometry(loc)
	if err != nil {
		t.Fatalf("reed geometry: %v", err)
	}
	runner := shuttleengine.NewRunner(reedEngine, shuttlefake.EngineSet(&gateRepromptReadEngine{quietSeconds: 3}), reedGeom.AnchorPath, reedGeom.WorktreeRoot, shuttleCfg)

	spec := shuttleengine.Spec{
		Prompt:      "smoke: stand in for a re-prompted discussion writer",
		OutputFiles: []string{decisionRecordPath, supportLogPath},
		Role:        "discussion",
		Round:       "1",
		Timeout:     3 * time.Minute,
	}

	// resolveGateSpec builds this same closure for a row's "gates" entry named "discussion".
	// See internal/shedrecipe/entries_gate.go.
	gate := loomshed.NewDiscussionGate(loomshed.DiscussionCheck{
		RecordPath:     decisionRecordPath,
		SupportLogPath: supportLogPath,
		RelevantPath:   filepath.Join(filepath.Dir(supportLogPath), "relevant.md"),
		WorktreeRoot:   filepath.Dir(supportLogPath),
		Index:          planglyph.NewIndex(fabricengine.NewReferenceRule()),
		History:        func() ([][]byte, error) { return nil, nil },
		Citations:      true,
	})

	// RunGated blocks until Wait reaches a terminal outcome, bounded by spec.Timeout;
	// the test needs no separate deadline.
	result, err := runner.RunGated(spec, shuttleengine.GateSpec{{Gate: gate, Attempts: 3}})
	if err != nil {
		t.Fatalf("RunGated() error = %v; want nil", err)
	}

	if result.Outcome != shuttleengine.OutcomeDone {
		t.Errorf("RunGated() Outcome = %q; want %q -- the writer fixes its artifact once re-prompted, so the row must report Done", result.Outcome, shuttleengine.OutcomeDone)
	}
	if result.Gate == nil {
		t.Fatal("RunGated() Result.Gate = nil; want a populated GateOutcome for a gated run that reached Done")
	}
	if !result.Gate.Passed {
		t.Errorf("RunGated() Result.Gate.Passed = false; want true once the re-prompted turn fixed the artifact")
	}
	if result.Gate.Attempts < 1 {
		t.Errorf("RunGated() Result.Gate.Attempts = %d; want at least 1 -- the first, invalid attempt must have burned one re-prompt", result.Gate.Attempts)
	}

	// The artifact left on disk must pass the gate closure as a fresh evaluation would run it.
	// That shows the fix that unblocked the pane's read also satisfies the mechanical check.
	// The result is not an artifact of the run loop's own bookkeeping.
	verdict, gerr := gate()
	if gerr != nil {
		t.Fatalf("gate() error = %v; want nil", gerr)
	}
	if !verdict.Passed {
		t.Errorf("gate() Passed = false (Findings: %q); want true -- the on-disk artifact must pass the gate after the re-prompt fix", verdict.Findings)
	}

	// No agent pane is left behind: a Done run's own strand is reaped by finalize's cleanup step,
	// and no respawn or stray attempt in this test spawned a second one.
	status, err := reedEngine.Status()
	if err != nil {
		t.Fatalf("reed status: %v", err)
	}
	for _, s := range status.Strands {
		if s.Live {
			t.Errorf("live strand %q (%s) survived the gated run; want none -- a leftover pane here is a second agent nothing owns", s.Name, s.GUID)
		}
	}
}
