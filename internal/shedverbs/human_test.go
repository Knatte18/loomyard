// human_test.go covers the human status renderer's exact output, and the status verb's choice
// between it and the JSON envelope: terminal vs. non-terminal writer, --json, --watch --json, and
// an absent status file on a terminal.

package shedverbs

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func humanRouting() shedengine.Routing {
	return shedengine.Routing{
		Entry:      "Intro",
		MaxBounces: 3,
		Producers: []shedengine.ProducerDef{
			{Name: "Intro", OnDone: "Plan"},
			{Name: "Plan", OnDone: "Ship", OnStuck: "Burl", Segment: "Plan"},
			{Name: "Burl", OnStuck: "Plan", Segment: "Plan"},
			{Name: "Ship"},
		},
	}
}

// blockRouting is humanRouting's shape around a block: the Motif block row, which skips to Ship, and two rows inlined from it.
func blockRouting() shedengine.Routing {
	return shedengine.Routing{
		Entry:      "Intro",
		MaxBounces: 3,
		Producers: []shedengine.ProducerDef{
			{Name: "Intro", OnDone: "Motif"},
			{Name: "Motif", OnDone: "Discussion-Write", SkipTo: "Ship"},
			{Name: "Discussion-Write", OnDone: "Discussion-Fix", Block: "Motif"},
			{Name: "Discussion-Fix", OnDone: "Ship", Block: "Motif"},
			{Name: "Ship"},
		},
	}
}

func TestRenderStatusHuman(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	stuckHistory := []shedengine.HistoryEntry{{Producer: "Plan", Outcome: shedengine.Stuck}}
	type renderArgs struct {
		label, runID, waiting string
		routing               shedengine.Routing
	}
	tests := []struct {
		name string
		st   shedengine.Status
		// args overrides the default loom/self rendering over humanRouting.
		args *renderArgs
		want string
	}{
		{
			name: "mid-segment",
			st: shedengine.Status{
				CurrentProducer: "Burl",
				State:           shedengine.StateRunning,
				History:         stuckHistory,
				Activity:        shedengine.Activity{Now: "Burl running", Last: "Plan → bounced to Burl"},
			},
			want: "loom self | running\n" +
				"step 2/3 Plan | bounce 1/3\n" +
				"now  Burl running\n" +
				"last Plan → bounced to Burl\n" +
				"next Ship\n",
		},
		{
			name: "blocked",
			st: shedengine.Status{
				CurrentProducer: "Intro",
				State:           shedengine.StateBlocked,
				Activity:        shedengine.Activity{Now: "Intro blocked", Wait: "needs a human"},
			},
			want: "loom self | blocked\n" +
				"step 1/3 Intro\n" +
				"now  Intro blocked\n" +
				"wait needs a human\n" +
				"next Plan, Ship\n",
		},
		{
			name: "awaiting",
			st: shedengine.Status{
				CurrentProducer: "Ship",
				State:           shedengine.StateAwaiting,
				Activity:        shedengine.Activity{Now: "Ship awaiting", Wait: "PR review"},
			},
			want: "loom self | awaiting\n" +
				"step 3/3 Ship\n" +
				"now  Ship awaiting\n" +
				"wait PR review\n",
		},
		{
			name: "done",
			st: shedengine.Status{
				CurrentProducer: "Ship",
				State:           shedengine.StateDone,
				Activity:        shedengine.Activity{Now: "done", Last: "Ship → done", Wait: "ignored while done"},
			},
			want: "loom self | done\n" +
				"step 3/3 Ship\n" +
				"now  done\n" +
				"last Ship → done\n",
		},
		{
			name: "paused with a reason",
			st: shedengine.Status{
				CurrentProducer: "Plan",
				State:           shedengine.StatePaused,
				PauseReason:     "pause requested by operator",
				Activity:        shedengine.Activity{Now: "Plan paused", Last: "Intro → done"},
			},
			want: "loom self | paused\n" +
				"step 2/3 Plan | bounce 0/3\n" +
				"now  Plan paused\n" +
				"last Intro → done\n" +
				"why  pause requested by operator\n" +
				"next Ship\n",
		},
		{
			name: "paused without a reason",
			st: shedengine.Status{
				CurrentProducer: "Plan",
				State:           shedengine.StatePaused,
				Activity:        shedengine.Activity{Now: "Plan paused"},
			},
			want: "loom self | paused\n" +
				"step 2/3 Plan | bounce 0/3\n" +
				"now  Plan paused\n" +
				"next Ship\n",
		},
		{
			name: "no routing omits progress",
			st:   shedengine.Status{State: shedengine.StateRunning, Activity: shedengine.Activity{Now: "working"}},
			args: &renderArgs{label: "batten"},
			want: "batten | running\nnow  working\n",
		},
		{
			name: "a latest Done of a block row reads entered and the current inlined row is named in its block",
			st: shedengine.Status{
				CurrentProducer: "Discussion-Write",
				State:           shedengine.StateRunning,
				History:         []shedengine.HistoryEntry{{Producer: "Motif", Outcome: shedengine.Done}},
				Activity:        shedengine.Activity{Now: "Discussion-Write", Last: "Motif → done"},
			},
			args: &renderArgs{label: "loom", runID: "self", routing: blockRouting()},
			want: "loom self | running\n" +
				"step 3/5 Discussion-Write\n" +
				"now  Discussion-Write in Motif\n" +
				"last Motif → entered\n" +
				"next Discussion-Fix, Ship\n",
		},
		{
			name: "a latest entry written inside a block names its row in the block",
			st: shedengine.Status{
				CurrentProducer: "Discussion-Fix",
				State:           shedengine.StateRunning,
				History: []shedengine.HistoryEntry{
					{Producer: "Motif", Outcome: shedengine.Done},
					{Producer: "Discussion-Write", Outcome: shedengine.Done, Block: "Motif"},
				},
				Activity: shedengine.Activity{Now: "Discussion-Fix", Last: "Discussion-Write → done"},
			},
			args: &renderArgs{label: "loom", runID: "self", routing: blockRouting()},
			want: "loom self | running\n" +
				"step 4/5 Discussion-Fix\n" +
				"now  Discussion-Fix in Motif\n" +
				"last Discussion-Write in Motif → done\n" +
				"next Ship\n",
		},
		{
			name: "an idle gap after a stuck step prints an idle line and a short one prints none",
			st: shedengine.Status{
				CurrentProducer: "Ship",
				State:           shedengine.StateRunning,
				History: []shedengine.HistoryEntry{
					{Producer: "Intro", Outcome: shedengine.Done, At: "2026-11-11T08:00:00Z"},
					{Producer: "Plan", Outcome: shedengine.Stuck, StartedAt: "2026-11-11T08:01:00Z", At: "2026-11-11T08:10:00Z"},
					{Producer: "Plan", Outcome: shedengine.Done, StartedAt: "2026-11-11T11:10:00Z", At: "2026-11-11T11:20:00Z"},
				},
				Activity: shedengine.Activity{Now: "Ship", Last: "Plan → done"},
			},
			want: "loom self | running\n" +
				"step 3/3 Ship\n" +
				"now  Ship\n" +
				"last Plan → done\n" +
				"idle 3h0m0s before Plan, after Plan stuck\n",
		},
		{
			name: "waiting note replaces state",
			st:   shedengine.Status{State: shedengine.StateRunning, Activity: shedengine.Activity{Now: "working"}},
			args: &renderArgs{label: "loom", runID: "self", waiting: "the parent's review of round 2"},
			want: "loom self | waiting the parent's review of round 2\nnow  working\n",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			args := tc.args
			if args == nil {
				args = &renderArgs{label: "loom", runID: "self", routing: humanRouting()}
			}
			got := RenderStatusHuman(args.label, args.runID, tc.st, args.routing, args.waiting)
			if got != tc.want {
				t.Errorf("RenderStatusHuman =\n%q\nwant\n%q", got, tc.want)
			}
		})
	}
}

func forceTerminal(t *testing.T, isTerminal bool) {
	t.Helper()
	orig := writerIsTerminal
	writerIsTerminal = func(io.Writer) bool { return isTerminal }
	t.Cleanup(func() { writerIsTerminal = orig })
}

func humanSpec(t *testing.T) *Spec {
	t.Helper()
	paths := newTestPaths(t)
	seedStatus(t, paths, "Intro")
	return &Spec{
		StatusPath:     paths.StatusPath,
		StatusLockPath: paths.StatusLockPath,
		StatusLabel:    "loom",
		RunID:          "self",
		Routing:        humanRouting(),
	}
}

func runStatus(spec *Spec, args ...string) (string, int) {
	var buf bytes.Buffer
	code := clihelp.Execute(statusCmd(statusTexts(), spec), &buf, args)
	return buf.String(), code
}

func TestStatusCmd_TerminalPrintsHumanView(t *testing.T) {
	tiergate.Require(t, 0)
	forceTerminal(t, true)
	out, code := runStatus(humanSpec(t))
	if code != 0 {
		t.Fatalf("exit code = %d; want 0; output %q", code, out)
	}
	if !strings.HasPrefix(out, "loom self | running\nstep 1/3 Intro\n") {
		t.Errorf("terminal output is not the human view: %q", out)
	}
}

func TestStatusCmd_NonTerminalPrintsEnvelope(t *testing.T) {
	tiergate.Require(t, 0)
	forceTerminal(t, false)
	out, code := runStatus(humanSpec(t))
	if code != 0 || !strings.HasPrefix(out, "{") || !strings.Contains(out, `"run_id":"self"`) {
		t.Errorf("non-terminal output = %q (exit %d); want the JSON envelope", out, code)
	}
}

func TestStatusCmd_JSONFlagPrintsEnvelopeOnTerminal(t *testing.T) {
	tiergate.Require(t, 0)
	forceTerminal(t, true)
	out, code := runStatus(humanSpec(t), "--json")
	if code != 0 || !strings.HasPrefix(out, "{") {
		t.Errorf("--json output = %q (exit %d); want the JSON envelope", out, code)
	}
}

func TestStatusCmd_WatchJSONRefusedBeforeReading(t *testing.T) {
	tiergate.Require(t, 0)
	forceTerminal(t, true)
	// No status file is seeded and the paths are unset: a refusal that read anything would fail
	// differently, and a --watch that got through would block forever.
	out, code := runStatus(&Spec{StatusLabel: "loom"}, "--watch", "--json")
	if code != 1 || !strings.Contains(out, `"ok":false`) || !strings.Contains(out, "--watch") {
		t.Errorf("--watch --json output = %q (exit %d); want a refusal on the error envelope", out, code)
	}
}

func TestStatusCmd_AbsentFileOnTerminalStillReportsEnvelope(t *testing.T) {
	tiergate.Require(t, 0)
	forceTerminal(t, true)
	paths := newTestPaths(t)
	spec := &Spec{StatusPath: paths.StatusPath, StatusLockPath: paths.StatusLockPath, EnsureStatusLockDir: true}
	out, code := runStatus(spec)
	if code != 0 || !strings.Contains(out, `"found":false`) {
		t.Errorf("absent-file output on a terminal = %q (exit %d); want the found:false envelope", out, code)
	}
}
