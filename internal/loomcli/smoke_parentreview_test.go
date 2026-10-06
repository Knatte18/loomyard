//go:build tmux

// smoke_parentreview_test.go is the live-substrate guard for the parent-review notice: the one hop no untagged or integration test reaches,
// because the wait loop's send must type the delivery prompt into a real pane and the pane must read it back.
// It follows smoke_gate_test.go: a real hub, a real reed session, a real tmux pane and a real shuttleengine.Runner, with a stub Engine whose Prepare launches a shell script rather than a provider.
//
// The resolver names the prime's orch as the run's parent, and the stub writer stands in for the live Discussion-Write agent.
// A real agent would answer the delivery prompt by calling SendMessage to the parent;
// the stub cannot, so it records the text it received in a notice file, which is what the test waits on.
// The notice must name the brief path, which is all a real parent needs to review and submit.
//
// Its compile gate (`go vet -tags tmux ./...`) is the only automatic guard; an operator runs this test by hand against a real substrate.

package loomcli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/parentreview"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/stencilkit"
)

// parentNoticeEngine is gateRepromptReadEngine with a Prepare that writes a valid discussion, blocks reading its next turn, and records that turn in noticePath.
// Every other Engine method, including the ComposeSend that types the delivery prompt into the pane, is inherited.
type parentNoticeEngine struct {
	gateRepromptReadEngine
	// noticePath receives the line the pane read, standing in for the SendMessage the real writer makes.
	noticePath string
}

// Prepare writes the launch script: a valid discussion and support log first, a turn-end event, then a blocking read of the delivery prompt.
func (e *parentNoticeEngine) Prepare(runDir string, spec shuttleengine.Spec, _ shuttleengine.Config) (shuttleengine.Launch, error) {
	if len(spec.OutputFiles) != 2 {
		return shuttleengine.Launch{}, fmt.Errorf("parentNoticeEngine: want exactly 2 output files (decision record, support log), got %d", len(spec.OutputFiles))
	}
	decisionRecordPath, supportLogPath := spec.OutputFiles[0], spec.OutputFiles[1]
	eventsPath := filepath.Join(runDir, "events.jsonl")

	validDecisionRecord := "" +
		"# Decision Record\n\n" +
		"## Goal\nSmoke the parent-review notice.\n\n" +
		"## Scope\nOne gated Discussion-Write row.\n\n" +
		"## Decisions\nSend the notice through a real pane.\n\n" +
		"## Constraints\nNone beyond the smoke harness.\n\n" +
		"## Auto-mode assumptions\nNone.\n\n" +
		"## Open risks\nNone.\n\n" +
		"## Acceptance criteria\nThe pane reads the delivery prompt.\n"

	var script strings.Builder
	fmt.Fprintf(&script, "#!/bin/sh\n")
	fmt.Fprintf(&script, "sleep %d\n", e.quietSeconds)
	fmt.Fprintf(&script, "cat > '%s' <<'DECISIONRECORD_VALID'\n%sDECISIONRECORD_VALID\n", decisionRecordPath, validDecisionRecord)
	fmt.Fprintf(&script, "printf 'support log\\n' > '%s'\n", supportLogPath)
	fmt.Fprintf(&script, "printf 'turn-end\\n' >> '%s'\n", eventsPath)
	// Block for the next turn, exactly where a real agent's pane blocks between turns; the delivery prompt lands here.
	fmt.Fprintf(&script, "read _notice_line\n")
	fmt.Fprintf(&script, "printf '%%s\\n' \"$_notice_line\" > '%s'\n", e.noticePath)
	fmt.Fprintf(&script, "printf 'turn-end\\n' >> '%s'\n", eventsPath)
	fmt.Fprintf(&script, "sleep 600\n")

	scriptPath := filepath.Join(runDir, "smoke-parentreview-launch.sh")
	if err := os.WriteFile(scriptPath, []byte(script.String()), 0o755); err != nil {
		return shuttleengine.Launch{}, fmt.Errorf("write smoke parent-review launch script: %w", err)
	}
	return shuttleengine.Launch{Cmd: "sh " + scriptPath, SessionID: "smoke-parent-review"}, nil
}

// TestSmokeParentReview_NoticeReachesTheWriterPaneAndApproveLetsTheRunThrough proves the delivery prompt, naming the brief path, is typed into a real pane after the discussion passes,
// and that an approve submitted through the real verb then ends the wait with the run Done.
func TestSmokeParentReview_NoticeReachesTheWriterPaneAndApproveLetsTheRunThrough(t *testing.T) {
	tmuxBinaryPath(t)
	_, loc, worktree, _ := newWiredPairFixture(t)
	registerBootstrapTeardown(t, loc, worktree)

	// The fixture's pair is created from the prime, so the resolver names the prime's orch as the parent.
	const parent = hubforge.TestShortname + ":orch"

	// stencilstore.Read hard-errors on a missing file, so the stencils the closure renders are seeded into the hub's stencils directory.
	stencilsDir := fabricengine.StencilsDir(loc.HubPath)
	stencilkit.SeedInto(t, stencilsDir)

	reedEngine := probeReedEngine(t, loc)
	if _, err := reedEngine.Up(); err != nil {
		t.Fatalf("reed up: %v", err)
	}

	decisionRecordPath := loomengine.DiscussionDecisionRecord(loc)
	supportLogPath := loomengine.DiscussionSupportLog(loc)
	if err := os.MkdirAll(filepath.Dir(decisionRecordPath), 0o755); err != nil {
		t.Fatal(err)
	}
	noticePath := filepath.Join(t.TempDir(), "notice.txt")

	shuttleCfg, err := shuttleengine.LoadConfig(loc.AnchorPath(), "shuttle")
	if err != nil {
		t.Fatalf("load shuttle config: %v", err)
	}
	reedGeom, err := hubgeom.ReedGeometry(loc)
	if err != nil {
		t.Fatalf("reed geometry: %v", err)
	}
	runner := shuttleengine.NewRunner(reedEngine, &parentNoticeEngine{gateRepromptReadEngine: gateRepromptReadEngine{quietSeconds: 3}, noticePath: noticePath}, reedGeom.AnchorPath, reedGeom.WorktreeRoot, shuttleCfg)

	// The same two closures resolveGateSpec builds for the row's "gates" list.
	reedCfg, err := reedengine.LoadConfig(loc.AnchorPath(), "reed")
	if err != nil {
		t.Fatalf("load reed config: %v", err)
	}
	prCfg, err := newParentReviewConfig(loc, reedCfg, loomengine.Config{ParentReviewWaitMin: 5}, stencilsDir)
	if err != nil {
		t.Fatalf("newParentReviewConfig: %v", err)
	}
	// No orch session runs in this hub, and the stub writer stands in for the parent's reviewer, so the reviewer counts as live.
	prCfg.ReviewerLive = nil
	reviewGate, reviewFinal := parentreview.NewGate(prCfg)
	gates := shuttleengine.GateSpec{
		{Name: "discussion", Gate: loomshed.NewDiscussionGate(decisionRecordPath, supportLogPath), Attempts: 3},
		{Name: "parent-review", Gate: reviewGate, Final: reviewFinal, Attempts: 1, PassOnCap: true},
	}
	spec := shuttleengine.Spec{
		Prompt:      "smoke: stand in for a discussion writer",
		OutputFiles: []string{decisionRecordPath, supportLogPath},
		Role:        "discussion",
		Round:       "1",
		Timeout:     3 * time.Minute,
	}

	type runResult struct {
		result shuttleengine.Result
		err    error
	}
	done := make(chan runResult, 1)
	go func() {
		result, err := runner.RunGated(spec, gates)
		done <- runResult{result, err}
	}()

	// The notice names the request the gate opened, typed into the real pane the writer's script reads.
	var notice string
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		if raw, err := os.ReadFile(noticePath); err == nil && len(raw) > 0 {
			notice = strings.TrimSpace(string(raw))
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if notice == "" {
		t.Fatalf("no notice reached the writer's pane within 2m; want the delivery prompt")
	}
	round := reviewStoreFor(loc)
	latest, ok, err := round.Latest()
	if err != nil || !ok {
		t.Fatalf("Latest = (%v, %v); want the opened round", ok, err)
	}
	if !strings.Contains(notice, latest.BriefPath()) || !strings.Contains(notice, parent) {
		t.Errorf("notice = %q; want it to name the brief path %q and the parent %q", notice, latest.BriefPath(), parent)
	}

	// The parent approves through the real verb, and the held wait ends with the run Done.
	var approveOut bytes.Buffer
	if code := RunCLIIn(loc.AnchorPath(), &approveOut, []string{"review", "approve"}); code != 0 {
		t.Fatalf("review approve = %d %q; want 0", code, approveOut.String())
	}
	select {
	case r := <-done:
		if r.err != nil {
			t.Fatalf("RunGated error = %v; want nil", r.err)
		}
		if r.result.Outcome != shuttleengine.OutcomeDone {
			t.Errorf("RunGated Outcome = %q; want %q once the parent approved", r.result.Outcome, shuttleengine.OutcomeDone)
		}
	case <-time.After(2 * time.Minute):
		t.Fatal("RunGated did not return within 2m of the approve; want the held wait to end")
	}
}
