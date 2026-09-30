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

func TestRenderStatusHuman(t *testing.T) {
	stuckHistory := []shedengine.HistoryEntry{{Producer: "Plan", Outcome: shedengine.Stuck}}
	tests := []struct {
		name string
		st   shedengine.Status
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
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := RenderStatusHuman("loom", "self", tc.st, humanRouting())
			if got != tc.want {
				t.Errorf("RenderStatusHuman =\n%q\nwant\n%q", got, tc.want)
			}
		})
	}
}

func TestRenderStatusHuman_NoRoutingOmitsProgress(t *testing.T) {
	st := shedengine.Status{State: shedengine.StateRunning, Activity: shedengine.Activity{Now: "working"}}
	got := RenderStatusHuman("batten", "", st, shedengine.Routing{})
	want := "batten | running\nnow  working\n"
	if got != want {
		t.Errorf("RenderStatusHuman = %q; want %q", got, want)
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
	forceTerminal(t, false)
	out, code := runStatus(humanSpec(t))
	if code != 0 || !strings.HasPrefix(out, "{") || !strings.Contains(out, `"run_id":"self"`) {
		t.Errorf("non-terminal output = %q (exit %d); want the JSON envelope", out, code)
	}
}

func TestStatusCmd_JSONFlagPrintsEnvelopeOnTerminal(t *testing.T) {
	forceTerminal(t, true)
	out, code := runStatus(humanSpec(t), "--json")
	if code != 0 || !strings.HasPrefix(out, "{") {
		t.Errorf("--json output = %q (exit %d); want the JSON envelope", out, code)
	}
}

func TestStatusCmd_WatchJSONRefusedBeforeReading(t *testing.T) {
	forceTerminal(t, true)
	// No status file is seeded and the paths are unset: a refusal that read anything would fail
	// differently, and a --watch that got through would block forever.
	out, code := runStatus(&Spec{StatusLabel: "loom"}, "--watch", "--json")
	if code != 1 || !strings.Contains(out, `"ok":false`) || !strings.Contains(out, "--watch") {
		t.Errorf("--watch --json output = %q (exit %d); want a refusal on the error envelope", out, code)
	}
}

func TestStatusCmd_AbsentFileOnTerminalStillReportsEnvelope(t *testing.T) {
	forceTerminal(t, true)
	paths := newTestPaths(t)
	spec := &Spec{StatusPath: paths.StatusPath, StatusLockPath: paths.StatusLockPath, EnsureStatusLockDir: true}
	out, code := runStatus(spec)
	if code != 0 || !strings.Contains(out, `"found":false`) {
		t.Errorf("absent-file output on a terminal = %q (exit %d); want the found:false envelope", out, code)
	}
}
