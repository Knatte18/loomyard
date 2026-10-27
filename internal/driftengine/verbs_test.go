//go:build tier0

// verbs_test.go — tier 0 tests for the drift verbs' rules over a fake run status and seeded scope records.

package driftengine

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedadapters"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// statusAt returns a VerbStatus at row in state, with an error text and the facts the verbs read beside the status.
func statusAt(row string, state shedengine.State, errText string, agentSession bool) VerbStatus {
	return VerbStatus{
		Status:       shedengine.Status{CurrentProducer: row, State: state, Error: errText},
		AgentSession: agentSession,
	}
}

// seedScope writes rec as the scope record under a fresh anchor and returns its paths.
func seedScope(t *testing.T, rec *ScopeRecord) Paths {
	t.Helper()
	p := NewPaths(t.TempDir())
	if rec == nil {
		return p
	}
	err := UpdateScope(p, func(ScopeRecord, bool) (ScopeRecord, error) { return *rec, nil })
	if err != nil {
		t.Fatalf("seed the scope record: %v", err)
	}
	return p
}

// mustReadScope reads the scope record and its presence.
func mustReadScope(t *testing.T, p Paths) (ScopeRecord, bool) {
	t.Helper()
	rec, found, err := ReadScope(p)
	if err != nil {
		t.Fatalf("ReadScope() = %v", err)
	}
	return rec, found
}

// requireRefusal checks that err names every fragment of want and that the scope record still reads as it did before.
func requireRefusal(t *testing.T, err error, want []string, p Paths, before *ScopeRecord) {
	t.Helper()
	if err == nil {
		t.Fatalf("verb = nil; want a refusal naming %q", want)
	}
	for _, fragment := range want {
		if !strings.Contains(err.Error(), fragment) {
			t.Errorf("refusal %q does not name %q", err.Error(), fragment)
		}
	}
	got, found := mustReadScope(t, p)
	if before == nil {
		if found {
			t.Errorf("a refused verb wrote the scope record %+v", got)
		}
		return
	}
	if !found || !reflect.DeepEqual(got, *before) {
		t.Errorf("a refused verb changed the scope record to %+v; want %+v", got, *before)
	}
}

// TestRecordScope covers the admitted scopes and every refusal, each refusal naming its way forward and leaving the record as it was.
func TestRecordScope(t *testing.T) {
	t.Parallel()

	recorded := &ScopeRecord{Scope: ScopeFull, RecordedAt: "2026-01-01T00:00:00Z"}
	tests := []struct {
		name     string
		row      string
		before   *ScopeRecord
		scope    string
		skip     []string
		wantErr  []string
		wantSkip []string
	}{
		{name: "off", row: "Drift-Baseline", scope: ScopeOff},
		{name: "full", row: "Drift-Baseline", scope: ScopeFull},
		{name: "partial with skips", row: "Drift-Baseline", scope: ScopePartial, skip: []string{"a", "b"}, wantSkip: []string{"a", "b"}},
		{name: "wrong row", row: "Drift-Await", scope: ScopeFull, wantErr: []string{"Drift-Baseline", "lym loom status"}},
		{name: "already recorded", row: "Drift-Baseline", before: recorded, scope: ScopeOff, wantErr: []string{"already recorded as full", "write baseline.md alone and end your turn"}},
		{name: "skip with full", row: "Drift-Baseline", scope: ScopeFull, skip: []string{"a"}, wantErr: []string{"--skip", "partial"}},
		{name: "partial without skip", row: "Drift-Baseline", scope: ScopePartial, wantErr: []string{"partial", "--skip <test>"}},
		{name: "unknown scope", row: "Drift-Baseline", scope: "some", wantErr: []string{`"some" is not a scope`, "--scope off, partial or full"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := seedScope(t, tt.before)
			err := RecordScope(p, statusAt(tt.row, shedengine.StateRunning, "", false), tt.scope, tt.skip)
			if tt.wantErr != nil {
				requireRefusal(t, err, tt.wantErr, p, tt.before)
				return
			}
			if err != nil {
				t.Fatalf("RecordScope() = %v; want nil", err)
			}
			got, found := mustReadScope(t, p)
			if !found || got.Scope != tt.scope || !slices.Equal(got.Skip, tt.wantSkip) || got.RecordedAt == "" {
				t.Errorf("record = %+v, %v; want scope %s, skip %v and recorded_at set", got, found, tt.scope, tt.wantSkip)
			}
		})
	}
}

// TestAcceptWithdraw covers Accept's callers and rulings, Withdraw, and CheckVerifyRow's admissions.
func TestAcceptWithdraw(t *testing.T) {
	t.Parallel()

	decisionHalt := shedadapters.DecisionHaltPrefix + "bouncer Drift-Verify: round 2"
	scoped := func(scope string) *ScopeRecord { return &ScopeRecord{Scope: scope} }
	withdrawn := func(scope string) *ScopeRecord { return &ScopeRecord{Scope: scope, Withdrawn: []string{"t1"}} }

	t.Run("Accept", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name       string
			st         VerbStatus
			before     *ScopeRecord
			wantErr    []string
			wantBy     string
			wantRuling string
			wantNoWd   bool
		}{
			{name: "agent under partial", st: statusAt("Drift-Verify", shedengine.StateRunning, "", true), before: scoped(ScopePartial), wantBy: AcceptedByAgent},
			{name: "agent under full is unruled", st: statusAt("Drift-Verify", shedengine.StateRunning, "", true), before: scoped(ScopeFull), wantBy: AcceptedByAgent, wantRuling: RulingUnruled},
			{name: "caller at a halt under partial", st: statusAt("Drift-Verify", shedengine.StateBlocked, "stuck", false), before: scoped(ScopePartial), wantBy: AcceptedByCaller},
			{name: "caller at a decision halt under full is ruled", st: statusAt("Drift-Verify", shedengine.StateBlocked, decisionHalt, false), before: scoped(ScopeFull), wantBy: AcceptedByCaller, wantRuling: RulingRuled},
			{name: "caller at a decision halt takes a test off withdrawn", st: statusAt("Drift-Verify", shedengine.StateBlocked, decisionHalt, false), before: withdrawn(ScopePartial), wantBy: AcceptedByCaller, wantNoWd: true},
			{name: "agent refused a withdrawn test", st: statusAt("Drift-Verify", shedengine.StateRunning, "", true), before: withdrawn(ScopePartial), wantErr: []string{"the run's parent withdrew t1", "only the parent re-accepts it at a decision halt"}},
			{name: "caller outside a decision halt refused a withdrawn test", st: statusAt("Drift-Verify", shedengine.StateBlocked, "stuck", false), before: withdrawn(ScopePartial), wantErr: []string{"parent's ruling", "decision halt"}},
			{name: "caller outside a decision halt refused under full", st: statusAt("Drift-Verify", shedengine.StateBlocked, "stuck", false), before: scoped(ScopeFull), wantErr: []string{"parent's ruling", "decision halt"}},
			{name: "running with the mark closed", st: statusAt("Drift-Verify", shedengine.StateRunning, "", false), before: scoped(ScopePartial), wantErr: []string{"wait for the row's halt", "lym loom status", "parent's ruling"}},
			{name: "failed status", st: statusAt("Drift-Verify", shedengine.StateFailed, "boom", false), before: scoped(ScopePartial), wantErr: []string{"lym loom resume"}},
			{name: "wrong row", st: statusAt("Drift-Await", shedengine.StateBlocked, "", false), before: scoped(ScopePartial), wantErr: []string{"Drift-Verify", "lym loom status"}},
			{name: "no scope record", st: statusAt("Drift-Verify", shedengine.StateRunning, "", true), wantErr: []string{"no scope is recorded"}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				p := seedScope(t, tt.before)
				err := Accept(p, tt.st, "t1", "intended")
				if tt.wantErr != nil {
					requireRefusal(t, err, tt.wantErr, p, tt.before)
					return
				}
				if err != nil {
					t.Fatalf("Accept() = %v; want nil", err)
				}
				got, _ := mustReadScope(t, p)
				if len(got.Accepted) != 1 {
					t.Fatalf("accepted = %+v; want one entry", got.Accepted)
				}
				entry := got.Accepted[0]
				if entry.Test != "t1" || entry.Reason != "intended" || entry.By != tt.wantBy || entry.Ruling != tt.wantRuling || entry.At == "" {
					t.Errorf("acceptance = %+v; want t1, reason intended, by %s, ruling %q and a time", entry, tt.wantBy, tt.wantRuling)
				}
				if tt.wantNoWd && len(got.Withdrawn) != 0 {
					t.Errorf("withdrawn = %v; want t1 taken off it", got.Withdrawn)
				}
			})
		}
	})

	t.Run("Withdraw", func(t *testing.T) {
		t.Parallel()
		running := statusAt("Drift-Verify", shedengine.StateRunning, "", true)

		t.Run("an accepted test is compared again and refused to the agent", func(t *testing.T) {
			t.Parallel()
			before := &ScopeRecord{Scope: ScopePartial, Skip: []string{"s"}, Accepted: []Acceptance{{Test: "t1"}, {Test: "t2"}}}
			p := seedScope(t, before)
			if err := Withdraw(p, running, "t1"); err != nil {
				t.Fatalf("Withdraw() = %v; want nil", err)
			}
			got, _ := mustReadScope(t, p)
			if want := []string{"s", "t2"}; !slices.Equal(SkipList(got), want) {
				t.Errorf("SkipList() = %v; want %v, so t1 is compared again", SkipList(got), want)
			}
			if !slices.Equal(got.Withdrawn, []string{"t1"}) {
				t.Errorf("withdrawn = %v; want [t1]", got.Withdrawn)
			}
			err := Accept(p, running, "t1", "again")
			if err == nil || !strings.Contains(err.Error(), "the run's parent withdrew t1") {
				t.Errorf("Accept() of a withdrawn test by the agent = %v; want the parent's-ruling refusal", err)
			}
		})

		refusals := []struct {
			name    string
			st      VerbStatus
			wantErr []string
		}{
			{"a test not accepted", running, []string{"t9 is not accepted", "lym drift status"}},
			{"wrong row", statusAt("Webster", shedengine.StateRunning, "", false), []string{"Drift-Verify", "lym loom status"}},
		}
		for _, tt := range refusals {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				before := &ScopeRecord{Scope: ScopePartial, Accepted: []Acceptance{{Test: "t1"}}}
				p := seedScope(t, before)
				requireRefusal(t, Withdraw(p, tt.st, "t9"), tt.wantErr, p, before)
			})
		}
	})

	t.Run("CheckVerifyRow", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name    string
			st      VerbStatus
			wantErr bool
		}{
			{"running at Drift-Verify", statusAt("Drift-Verify", shedengine.StateRunning, "", true), false},
			{"blocked at Drift-Verify", statusAt("Drift-Verify", shedengine.StateBlocked, "", false), false},
			{"Drift-Await", statusAt("Drift-Await", shedengine.StateBlocked, "", false), true},
			{"Webster", statusAt("Webster", shedengine.StateRunning, "", false), true},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				err := CheckVerifyRow(tt.st, "compare")
				if !tt.wantErr {
					if err != nil {
						t.Errorf("CheckVerifyRow() = %v; want nil", err)
					}
					return
				}
				if err == nil || !strings.Contains(err.Error(), "Drift-Verify") || !strings.Contains(err.Error(), "drift compare") {
					t.Errorf("CheckVerifyRow() = %v; want a refusal naming drift compare and Drift-Verify", err)
				}
			})
		}
	})
}

// TestDeferWaive covers the halts Defer and Waive admit and every refusal, each leaving the record as it was.
func TestDeferWaive(t *testing.T) {
	t.Parallel()

	scoped := &ScopeRecord{Scope: ScopeFull}

	t.Run("Defer", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name    string
			st      VerbStatus
			wantErr []string
		}{
			{name: "blocked at Drift-Await", st: statusAt("Drift-Await", shedengine.StateBlocked, "", false)},
			{name: "blocked at a Drift-Baseline capture halt", st: statusAt("Drift-Baseline", shedengine.StateBlocked, CaptureHaltPrefix+"exit 1", false)},
			{name: "blocked at another Drift-Baseline halt", st: statusAt("Drift-Baseline", shedengine.StateBlocked, "something else", false), wantErr: []string{"lym loom status", "capture halt"}},
			{name: "running at Drift-Await", st: statusAt("Drift-Await", shedengine.StateRunning, "", false), wantErr: []string{"lym loom status"}},
			{name: "wrong row", st: statusAt("Drift-Verify", shedengine.StateBlocked, "", false), wantErr: []string{"Drift-Await", "Drift-Baseline"}},
			{name: "capture lock held", st: VerbStatus{Status: shedengine.Status{CurrentProducer: "Drift-Await", State: shedengine.StateBlocked}, CaptureHeld: true}, wantErr: []string{"lym loom resume", "running capture"}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				p := seedScope(t, scoped)
				err := Defer(p, tt.st)
				if tt.wantErr != nil {
					requireRefusal(t, err, tt.wantErr, p, scoped)
					return
				}
				if err != nil {
					t.Fatalf("Defer() = %v; want nil", err)
				}
				got, _ := mustReadScope(t, p)
				if !got.Deferred || got.AwaitedAt == "" {
					t.Errorf("record = %+v; want deferred with awaited_at set", got)
				}
			})
		}
	})

	t.Run("Waive", func(t *testing.T) {
		t.Parallel()
		blocked := statusAt("Drift-Verify", shedengine.StateBlocked, "no counted capture", false)
		tests := []struct {
			name    string
			st      VerbStatus
			capture *CaptureRecord
			wantErr []string
		}{
			{name: "no capture", st: blocked},
			{name: "a failed capture", st: blocked, capture: &CaptureRecord{State: CaptureFailed}},
			{name: "a passed capture not awaited", st: blocked, capture: &CaptureRecord{State: CapturePassed}},
			{name: "a counted capture", st: blocked, capture: &CaptureRecord{State: CapturePassed}, wantErr: []string{"counted capture", "lym loom resume"}},
			{name: "running", st: statusAt("Drift-Verify", shedengine.StateRunning, "", true), wantErr: []string{"blocked Drift-Verify", "lym loom status"}},
			{name: "wrong row", st: statusAt("Drift-Await", shedengine.StateBlocked, "", false), wantErr: []string{"blocked Drift-Verify"}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				before := *scoped
				if tt.name == "a counted capture" {
					before.AwaitedAt = "2026-01-01T00:00:00Z"
				}
				p := seedScope(t, &before)
				if tt.capture != nil {
					if err := WriteCapture(p, *tt.capture); err != nil {
						t.Fatalf("seed the capture record: %v", err)
					}
				}
				err := Waive(p, tt.st, "no baseline possible")
				if tt.wantErr != nil {
					requireRefusal(t, err, tt.wantErr, p, &before)
					return
				}
				if err != nil {
					t.Fatalf("Waive() = %v; want nil", err)
				}
				got, _ := mustReadScope(t, p)
				if got.WaivedReason != "no baseline possible" || got.WaivedAt == "" {
					t.Errorf("record = %+v; want the waiver and its time", got)
				}
			})
		}
	})
}
