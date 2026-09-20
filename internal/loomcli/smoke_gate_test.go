//go:build smoke

// smoke_gate_test.go is the live-substrate regression guard for the gate re-prompt loop
// (internal/shuttleengine's Wait, batch 1): the one mechanism no untagged test can exercise, because
// every hermetic test fake in internal/shedrecipe and internal/shedadapters evaluates a gate closure
// exactly once rather than running the real send-and-poll loop (see the "every test fake evaluates
// the gate once" Shared Decision) -- the loop's own coverage belongs against the real wait loop,
// which is here.
//
// It drives the REAL substrate -- a real hub, a real reed session, a real tmux pane, a real
// shuttleengine.Runner writing a real run.json, and a real loomshed.NewDiscussionGate closure --
// and spawns ZERO provider subprocesses, following smoke_attachprobe_test.go's own substitution: a
// stub Engine whose Prepare launches a plain shell script rather than a provider. That script plays
// the one part a real substrate is needed for: it writes a deliberately-invalid first artifact,
// blocks reading its next turn from the pane (exactly where a real agent would block reading a
// Send), and only writes a valid artifact once that turn arrives -- proving the gate's re-prompt
// text was genuinely typed into a real pane and genuinely read back out of it, which no fake Send
// can stand in for.
//
// It lives in this package rather than in internal/shuttleengine because it needs the same hub
// fixture, tmux skip, reed engine probe, and hermetic git TestMain smoke_attachprobe_test.go already
// built here.
//
// This file's compile gate is its only automatic guard: its build tag excludes it from every
// untagged run and from the repo-wide done gate, and the task's own chained tagged invocation
// (`go test -tags smoke -run '^$' ./internal/loomcli/...`) type-checks it without ever executing it.
// An operator runs this test itself, by hand, against a real substrate.

package loomcli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
)

// gateRepromptReadEngine is a shuttleengine.Engine that launches a plain shell script standing in
// for a re-prompted agent, and that -- unlike shellLaunchEngine's own no-op -- gives ComposeSend
// real behaviour: it types text into the pane and submits it, exactly what the script's own `read`
// needs to unblock.
//
// Every other method matches shellLaunchEngine's own zero-behaviour answers, because the path this
// suite exercises (Start's run.json persistence, reed's liveness answer, Wait's events-plus-
// output-files poll, and now Send's real delivery) never reaches any of them.
type gateRepromptReadEngine struct {
	// quietSeconds is how long the script waits before writing its first, deliberately-invalid
	// artifact, so the run is genuinely in flight rather than already complete when Wait's first
	// tick runs.
	quietSeconds int
}

// Prepare writes a script that: waits quietSeconds, writes an invalid decision record plus an
// existing support log, appends a turn-end event, blocks on a `read` of its next turn exactly as a
// real agent's pane blocks between turns, then -- once that turn arrives -- overwrites the decision
// record with a valid one and appends a second turn-end event before sitting idle.
//
// The `read` line is what makes this a genuine live-substrate proof rather than a restatement of the
// gate contract: the shell's own tty driver is what echoes the re-prompt text into the pane's visible
// viewport, which is the same evidence sendVerified (run.go) demands of a real agent's pane, and the
// script does not react until that evidence-producing delivery actually lands.
func (e gateRepromptReadEngine) Prepare(runDir string, spec shuttleengine.Spec, _ shuttleengine.Config) (shuttleengine.Launch, error) {
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

// ParseEvents maps any non-empty events.jsonl content onto a single turn-end event, matching
// shellLaunchEngine's own reading of this stub's events file.
func (e gateRepromptReadEngine) ParseEvents(data []byte) ([]shuttleengine.Event, error) {
	if len(data) == 0 {
		return nil, nil
	}
	return []shuttleengine.Event{{Kind: shuttleengine.EventStop, Raw: data}}, nil
}

func (e gateRepromptReadEngine) Startup(_ string) shuttleengine.StartupState {
	return shuttleengine.StartupReady
}

func (e gateRepromptReadEngine) InterruptSequence() []shuttleengine.PaneInput          { return nil }
func (e gateRepromptReadEngine) TrustDismissSequence(string) []shuttleengine.PaneInput { return nil }

// ComposeSend types text into the pane and submits it with an Enter, which is what unblocks the
// script's own `read` -- the one method this stub gives real behaviour, unlike every other method
// here and unlike shellLaunchEngine's own no-op ComposeSend, which this suite's send-verification
// path never needs to reach.
func (e gateRepromptReadEngine) ComposeSend(text string) []shuttleengine.PaneInput {
	return []shuttleengine.PaneInput{{Text: text, Submit: true}}
}

func (e gateRepromptReadEngine) ModelSwitchSequence(_ string) []shuttleengine.PaneInput {
	return nil
}

func (e gateRepromptReadEngine) AuditForks(_, _ string) (shuttleengine.ForkAudit, error) {
	return shuttleengine.ForkAudit{}, nil
}

func (e gateRepromptReadEngine) AuditForksIncremental(_, _ string, _ map[string]bool) (shuttleengine.ForkAudit, error) {
	return shuttleengine.ForkAudit{}, nil
}

// TestSmokeGate_RepromptsThroughARealPaneAndFixesTheArtifact proves the one thing no untagged test
// can: a real agent, re-prompted through a real pane after a deliberately-invalid first artifact,
// fixes it and the row reports Done.
//
// It runs loomshed.NewDiscussionGate -- the same closure a gated Discussion-Write row builds through
// resolveGateSpec -- over a real shuttleengine.Runner's RunGated, with gateRepromptReadEngine
// standing in for the provider exactly as shellLaunchEngine does in smoke_attachprobe_test.go, and
// asserts the run reaches a passed-gate shuttleengine.OutcomeDone -- the shape a gated
// SingleLLMProducer.Call (internal/shedadapters/singlellm.go's mapOutcome) maps onto shedengine.Done
// -- that the gate's reported Attempts is at least 1, and that the artifact on disk passes the same
// gate closure afterwards.
func TestSmokeGate_RepromptsThroughARealPaneAndFixesTheArtifact(t *testing.T) {
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

	shuttleCfg, err := shuttleengine.LoadConfig(loc.AnchorPath(), "shuttle")
	if err != nil {
		t.Fatalf("load shuttle config: %v", err)
	}
	reedGeom := hubgeom.ReedGeometry(loc)
	runner := shuttleengine.NewRunner(reedEngine, gateRepromptReadEngine{quietSeconds: 3}, reedGeom.AnchorPath, reedGeom.WorktreeRoot, shuttleCfg)

	spec := shuttleengine.Spec{
		Prompt:      "smoke: stand in for a re-prompted discussion writer",
		OutputFiles: []string{decisionRecordPath, supportLogPath},
		Role:        "discussion",
		Round:       "1",
		Timeout:     3 * time.Minute,
	}

	// The same closure resolveGateSpec builds for a "gate: discussion" row (see
	// internal/shedrecipe/entries_gate.go), driven here over a real Runner instead of a fake one.
	gate := loomshed.NewDiscussionGate(decisionRecordPath, supportLogPath)

	// RunGated blocks until Wait reaches a terminal outcome, bounded by spec.Timeout above -- no
	// separate test-level deadline is needed on top of it.
	result, err := runner.RunGated(spec, shuttleengine.GateSpec{Gate: gate, Attempts: 3})
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

	// The artifact left on disk must itself pass the same gate closure a fresh evaluation would run
	// -- proving the fix that unblocked the pane's read is the same fix that satisfies the mechanical
	// check, not an artifact of the run loop's own bookkeeping.
	verdict, gerr := gate()
	if gerr != nil {
		t.Fatalf("gate() error = %v; want nil", gerr)
	}
	if !verdict.Passed {
		t.Errorf("gate() Passed = false (Findings: %q); want true -- the on-disk artifact must pass the gate after the re-prompt fix", verdict.Findings)
	}

	// No agent pane left behind: a Done run's own strand is reaped by finalize's cleanup step, and no
	// respawn or stray attempt in this test ever spawned a second one.
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
