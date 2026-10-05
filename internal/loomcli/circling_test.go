package loomcli

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/loomrecipe"
	"github.com/Knatte18/loomyard/internal/loomshed"
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
	records   []string
}

func (f *circlingFake) deps() circlingDeps {
	return circlingDeps{
		readStatus:    func() (shedengine.Status, bool, error) { return f.status, f.found, f.statusErr },
		bouncerSubdir: loomrecipe.BouncerRunSubdir,
		record: func(subdir string, d shedadapters.CirclingDecision) (int, shedadapters.EscalationCause, error) {
			if f.recordErr != nil {
				return 0, "", f.recordErr
			}
			f.records = append(f.records, subdir+":"+string(d))
			return f.round, f.cause, nil
		},
	}
}

func awaitingAt(row string) shedengine.Status {
	return shedengine.Status{State: shedengine.StateAwaiting, CurrentProducer: row}
}

func TestCirclingVerb_Records(t *testing.T) {
	for _, d := range []shedadapters.CirclingDecision{shedadapters.CirclingAccept, shedadapters.CirclingContinue} {
		t.Run(string(d), func(t *testing.T) {
			f := &circlingFake{status: awaitingAt(loomshed.NamePlanBouncer), found: true, round: 3, cause: shedadapters.EscalationBudget}
			var out bytes.Buffer
			if code := circlingVerb(&out, "task-a", f.deps(), d); code != 0 {
				t.Fatalf("exit = %d, out %s; want 0", code, out.String())
			}
			data := envelope.RequireOK(t, out.String()).Raw
			if data["slug"] != "task-a" || data["decision"] != string(d) || data["round"] != float64(3) || data["cause"] != "budget" || data["resume"] != "lyx loom start" {
				t.Fatalf("envelope = %v; want slug, decision, round 3, cause budget and the resume hint", data)
			}
			if want := "plan:" + string(d); len(f.records) != 1 || f.records[0] != want {
				t.Fatalf("records = %v; want [%s]", f.records, want)
			}
		})
	}
}

func TestCirclingVerb_Refusals(t *testing.T) {
	tests := []struct {
		name string
		fake circlingFake
		want string
	}{
		{"no status file", circlingFake{}, "no status file"},
		{"running run", circlingFake{status: shedengine.Status{State: shedengine.StateRunning, CurrentProducer: loomshed.NamePlanBouncer}, found: true}, "not awaiting"},
		{"awaiting at PR-Gate", circlingFake{status: awaitingAt(loomshed.NamePRGate), found: true}, "not a review segment's Bouncer row"},
		{"latest round not escalated", circlingFake{status: awaitingAt(loomshed.NameWebsterBouncer), found: true, recordErr: shedadapters.ErrNotEscalated}, "is not escalated"},
		{"malformed escalation record", circlingFake{status: awaitingAt(loomshed.NamePlanBouncer), found: true, recordErr: fmt.Errorf("%w: round-2-escalation.md: bad cause", shedadapters.ErrEscalationMalformed)}, "fix or delete the named escalation file"},
		{"second decision",circlingFake{status: awaitingAt(loomshed.NameDiscussionBouncer), found: true, recordErr: shedadapters.ErrCirclingDecided}, "already recorded"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := tt.fake
			var out bytes.Buffer
			if code := circlingVerb(&out, "task-a", f.deps(), shedadapters.CirclingAccept); code != 1 {
				t.Fatalf("exit = %d; want 1", code)
			}
			msg := envelope.RequireErr(t, out.String(), tt.want).Error
			if !strings.Contains(msg, "way forward:") || !strings.HasPrefix(msg, "loom: circling accept: ") {
				t.Fatalf("message = %q; want a circling accept refusal containing %q and a way forward", msg, tt.want)
			}
			if len(f.records) != 0 {
				t.Fatalf("records = %v; want none", f.records)
			}
		})
	}
}

func TestCirclingVerb_StatusReadFailure(t *testing.T) {
	f := &circlingFake{statusErr: errors.New("boom")}
	var out bytes.Buffer
	if code := circlingVerb(&out, "task-a", f.deps(), shedadapters.CirclingContinue); code != 1 {
		t.Fatalf("exit = %d; want 1", code)
	}
	envelope.RequireErr(t, out.String(), "boom")
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
