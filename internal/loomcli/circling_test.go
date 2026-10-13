package loomcli

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/loomrecipe"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
)

// circlingFake is a circlingDeps whose status and record outcome the test sets, counting the records made.
type circlingFake struct {
	status    shedengine.Status
	found     bool
	statusErr error
	round     int
	cause     shedadapters.EscalationCause
	recordErr error
	// subdirErr, when set, makes the Bouncer lookup fail.
	subdirErr error
	// latest and latestErr are the Bouncer run directory's latest round; hardCap and raise are the row's resolved hard cap and the way forward that raises it, a zero hardCap meaning no cap.
	latest     int
	latestErr  error
	hardCap    int
	raise      string
	hardCapErr error
	records    []string
}

func (f *circlingFake) deps() circlingDeps {
	bouncerSubdir := loomrecipe.BouncerRunSubdir
	if f.subdirErr != nil {
		bouncerSubdir = func(string) (string, bool, error) { return "", false, f.subdirErr }
	}
	return circlingDeps{
		readStatus:    func() (shedengine.Status, bool, error) { return f.status, f.found, f.statusErr },
		bouncerSubdir: bouncerSubdir,
		record: func(subdir string, d shedadapters.CirclingDecision) (int, shedadapters.EscalationCause, error) {
			if f.recordErr != nil {
				return 0, "", f.recordErr
			}
			f.records = append(f.records, subdir+":"+string(d))
			return f.round, f.cause, nil
		},
		latestRound: func(string) (int, error) { return f.latest, f.latestErr },
		hardCap:     func(string) (int, string, error) { return f.hardCap, f.raise, f.hardCapErr },
	}
}

func awaitingAt(row string) shedengine.Status {
	return shedengine.Status{State: shedengine.StateAwaiting, CurrentProducer: row}
}

func TestCirclingVerb_Records(t *testing.T) {
	escalated := func(round int, cause shedadapters.EscalationCause) circlingFake {
		return circlingFake{status: awaitingAt(loomshed.NamePlanBouncer), found: true, round: round, cause: cause}
	}
	belowCap := escalated(3, shedadapters.EscalationCircling)
	belowCap.latest, belowCap.hardCap = 3, 4
	acceptAtCap := escalated(4, shedadapters.EscalationHardCap)
	acceptAtCap.latest, acceptAtCap.hardCap = 4, 4
	tests := []struct {
		name      string
		decision  shedadapters.CirclingDecision
		fake      circlingFake
		wantRound float64
		wantCause string
	}{
		{"accept", shedadapters.CirclingAccept, escalated(3, shedadapters.EscalationBudget), 3, "budget"},
		{"continue", shedadapters.CirclingContinue, escalated(3, shedadapters.EscalationBudget), 3, "budget"},
		{"continue below the hard cap on a circling escalation", shedadapters.CirclingContinue, belowCap, 3, "circling"},
		{"accept at the hard cap", shedadapters.CirclingAccept, acceptAtCap, 4, "hard-cap"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := tt.fake
			var out bytes.Buffer
			if code := circlingVerb(&out, "task-a", f.deps(), tt.decision); code != 0 {
				t.Fatalf("exit = %d, out %s; want 0", code, out.String())
			}
			data := envelope.RequireOK(t, out.String()).Raw
			if data["slug"] != "task-a" || data["decision"] != string(tt.decision) || data["round"] != tt.wantRound || data["cause"] != tt.wantCause || data["resume"] != "lyx loom resume" {
				t.Fatalf("envelope = %v; want slug, decision, round %v, cause %s and the resume hint", data, tt.wantRound, tt.wantCause)
			}
			if want := "plan:" + string(tt.decision); len(f.records) != 1 || f.records[0] != want {
				t.Fatalf("records = %v; want [%s]", f.records, want)
			}
		})
	}
}

func TestCirclingVerb_Refusals(t *testing.T) {
	const loomRaise = `run "lyx config loom --set review_hard_max_bounces=<n>" in the task worktree`
	const localRaise = `run "lyx config loom --layer local --set review_hard_max_bounces=<n>" in the task worktree, and correct the task's run_settings with "lyx board upsert" too, so a later Seed-Child writes the same value`
	atPlanBouncer := awaitingAt(loomshed.NamePlanBouncer)
	tests := []struct {
		name string
		// decision defaults to accept.
		decision shedadapters.CirclingDecision
		fake     circlingFake
		want     []string
	}{
		{"no status file", "", circlingFake{}, []string{"no status file"}},
		{"status read failure", "", circlingFake{statusErr: errors.New("boom")}, []string{"re-run the verb"}},
		{"bouncer lookup failure", "", circlingFake{status: atPlanBouncer, found: true, subdirErr: errors.New("recipe broken")}, []string{"rebuild or reinstall lyx"}},
		{"record I/O failure", "", circlingFake{status: atPlanBouncer, found: true, recordErr: errors.New("write circling decision: disk full")}, []string{"re-run the verb"}},
		{"running run", "", circlingFake{status: shedengine.Status{State: shedengine.StateRunning, CurrentProducer: loomshed.NamePlanBouncer}, found: true}, []string{"not awaiting"}},
		{"awaiting at PR-Gate", "", circlingFake{status: awaitingAt(loomshed.NamePRGate), found: true}, []string{"not a review segment's Bouncer row"}},
		{"latest round not escalated", "", circlingFake{status: awaitingAt(loomshed.NameWebsterBouncer), found: true, recordErr: shedadapters.ErrNotEscalated}, []string{"is not escalated"}},
		{"malformed escalation record", "", circlingFake{status: atPlanBouncer, found: true, recordErr: fmt.Errorf("%w: round-2-escalation.md: bad cause", shedadapters.ErrEscalationMalformed)}, []string{"fix or delete the named escalation file"}},
		{"second decision", "", circlingFake{status: awaitingAt(loomshed.NameDiscussionBouncer), found: true, recordErr: shedadapters.ErrCirclingDecided}, []string{`already recorded for the escalated round at Discussion-Bouncer; way forward: the decision for this round is recorded and stays; run "lyx loom resume"`}},
		{
			"continue at the hard cap", shedadapters.CirclingContinue,
			circlingFake{status: atPlanBouncer, found: true, latest: 4, hardCap: 4, raise: loomRaise},
			[]string{"at or past its perch's hard cap 4", "lyx loom circling accept task-a", loomRaise, "lyx loom circling continue task-a"},
		},
		{
			"continue whose hard cap comes from run_settings", shedadapters.CirclingContinue,
			circlingFake{status: atPlanBouncer, found: true, latest: 5, hardCap: 4, raise: localRaise},
			[]string{"at or past its perch's hard cap 4", localRaise},
		},
		{
			"hard cap unresolved", shedadapters.CirclingContinue,
			circlingFake{status: atPlanBouncer, found: true, hardCapErr: errors.New("loom config key \"review_hard_max_bounces\": must be at least 1")},
			[]string{"fix the file or key the message names", "lyx loom circling continue task-a", "in place of any"},
		},
		{
			"latest round unreadable", shedadapters.CirclingContinue,
			circlingFake{status: atPlanBouncer, found: true, latestErr: errors.New("read the run directory: boom")},
			[]string{"re-run the verb"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := tt.fake
			decision := tt.decision
			if decision == "" {
				decision = shedadapters.CirclingAccept
			}
			var out bytes.Buffer
			if code := circlingVerb(&out, "task-a", f.deps(), decision); code != 1 {
				t.Fatalf("exit = %d; want 1", code)
			}
			msg := envelope.RequireErr(t, out.String(), tt.want[0]).Error
			if !strings.Contains(msg, "way forward:") || !strings.HasPrefix(msg, "loom: circling "+string(decision)+": ") {
				t.Fatalf("message = %q; want a circling %s refusal containing %q and a way forward", msg, decision, tt.want[0])
			}
			for _, want := range tt.want[1:] {
				if !strings.Contains(msg, want) {
					t.Errorf("message = %q; want it to contain %q", msg, want)
				}
			}
			if len(f.records) != 0 {
				t.Fatalf("records = %v; want none", f.records)
			}
		})
	}
}

// TestPerchHardCap pins the hard cap and the raising route perchHardCap resolves per Bouncer row, each from the layer the value comes from.
func TestPerchHardCap(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		row       string
		seed      func(t *testing.T, loc *lyxcwd.Location)
		wantHard  int
		wantRaise []string
	}{
		{
			name: "a worktree plan cap",
			row:  loomshed.NamePlanBouncer,
			seed: func(t *testing.T, loc *lyxcwd.Location) {
				seedLoomConfigWithKeys(t, loc.AnchorPath(), map[string]string{"plan_hard_max_bounces": "6"})
			},
			wantHard:  6,
			wantRaise: []string{"lyx config loom --set plan_hard_max_bounces=<n>"},
		},
		{
			name: "a local run-wide cap",
			row:  loomshed.NameDiscussionBouncer,
			seed: func(t *testing.T, loc *lyxcwd.Location) {
				localPath := configengine.LocalFile(loc.AnchorPath(), "loom")
				if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil {
					t.Fatalf("MkdirAll(%q) = %v; want nil", filepath.Dir(localPath), err)
				}
				if err := os.WriteFile(localPath, []byte("review_hard_max_bounces: 5\n"), 0o644); err != nil {
					t.Fatalf("WriteFile(%q) = %v; want nil", localPath, err)
				}
			},
			wantHard:  5,
			wantRaise: []string{"lyx config loom --layer local --set review_hard_max_bounces=<n>", "lyx board upsert"},
		},
		{
			name: "the darn template cap",
			row:  loomshed.NameDarnBouncer,
			seed: func(t *testing.T, loc *lyxcwd.Location) {
				seedDarnHubConfig(t, loc.HubPath, `"go test ./..."`, "3")
			},
			wantHard:  4,
			wantRaise: []string{"lyx config darn --set review_hard_max_bounces=<n>"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			loc := hubLocation(t, "pair", ".")
			tt.seed(t, loc)

			hard, raise, err := perchHardCap(loc, tt.row)
			if err != nil || hard != tt.wantHard {
				t.Fatalf("perchHardCap(%q) = (%d, _, %v); want (%d, _, nil)", tt.row, hard, err, tt.wantHard)
			}
			for _, want := range tt.wantRaise {
				if !strings.Contains(raise, want) {
					t.Errorf("perchHardCap(%q) raise = %q; want it to contain %q", tt.row, raise, want)
				}
			}
		})
	}
}

func TestCirclingStatusReader(t *testing.T) {
	t.Run("no status file", func(t *testing.T) {
		dir := t.TempDir()
		_, found, err := circlingStatusReader(filepath.Join(dir, "status.json"), filepath.Join(dir, "scratch", "status.json.lock"))()
		if err != nil || found {
			t.Fatalf("found = %v, err = %v; want not found and no error", found, err)
		}
	})
	t.Run("status file without a scratch directory", func(t *testing.T) {
		dir := t.TempDir()
		statusPath := filepath.Join(dir, "status.json")
		if err := os.WriteFile(statusPath, []byte(`{"current_producer": "Plan-Bouncer", "state": "awaiting"}`), 0o644); err != nil {
			t.Fatal(err)
		}
		st, found, err := circlingStatusReader(statusPath, filepath.Join(dir, "scratch", "status.json.lock"))()
		if err != nil || !found || st.State != shedengine.StateAwaiting {
			t.Fatalf("status = %+v, found = %v, err = %v; want the awaiting status read", st, found, err)
		}
	})
}
