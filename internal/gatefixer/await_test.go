// await_test.go covers Await's classification order, what each outcome stops, reads and removes, and the errors it returns before it stops anything.

package gatefixer

import (
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// appendTurnEnd makes the fixture's engine read a turn end from rec's events file and writes a line there for it to read.
func appendTurnEnd(t *testing.T, fx *fixture, rec *Record) {
	t.Helper()
	fx.engine.Events = []shuttleengine.Event{{Kind: shuttleengine.EventStop}}
	f, err := os.OpenFile(rec.EventsPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open events file: %v", err)
	}
	defer f.Close()
	if _, err := f.WriteString("{\"type\":\"stop\"}\n"); err != nil {
		t.Fatalf("append turn end: %v", err)
	}
}

// TestAwait pins the order done, asking, timeout and died are decided in, and that every terminal result stops the strand once.
func TestAwait(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const timeout = time.Hour
	pastTimeout := func(fx *fixture) { fx.clock.now = time.Unix(0, 0).Add(2 * timeout) }
	goneStrand := func(fx *fixture) { fx.reed.Strands = nil }

	tests := []struct {
		name       string
		arrange    func(t *testing.T, fx *fixture, rec *Record)
		wait       time.Duration
		wantStatus string
		wantReason string
		wantReply  string
	}{
		{
			name: "ReplyFileIsDoneOverEveryDeadSignal",
			arrange: func(t *testing.T, fx *fixture, rec *Record) {
				if err := os.WriteFile(fx.replyPath, []byte("fixed a"), 0o644); err != nil {
					t.Fatalf("write reply: %v", err)
				}
				appendTurnEnd(t, fx, rec)
				pastTimeout(fx)
				goneStrand(fx)
			},
			wantStatus: StatusDone,
			wantReply:  "fixed a",
		},
		{
			name:       "TurnEndWithoutReplyIsAsking",
			arrange:    func(t *testing.T, fx *fixture, rec *Record) { appendTurnEnd(t, fx, rec) },
			wantStatus: StatusDead,
			wantReason: DeadReasonAsking,
		},
		{
			name: "AskingIsDecidedBeforeTimeoutAndDied",
			arrange: func(t *testing.T, fx *fixture, rec *Record) {
				appendTurnEnd(t, fx, rec)
				pastTimeout(fx)
				goneStrand(fx)
			},
			wantStatus: StatusDead,
			wantReason: DeadReasonAsking,
		},
		{
			name: "TimeoutIsDecidedBeforeDied",
			arrange: func(_ *testing.T, fx *fixture, _ *Record) {
				pastTimeout(fx)
				goneStrand(fx)
			},
			wantStatus: StatusDead,
			wantReason: DeadReasonTimeout,
		},
		{
			name:       "GoneStrandIsDied",
			arrange:    func(_ *testing.T, fx *fixture, _ *Record) { goneStrand(fx) },
			wantStatus: StatusDead,
			wantReason: DeadReasonDied,
		},
		{
			name:       "LiveFixerOutlastingTheTimeoutIsDeadOnIt",
			arrange:    func(*testing.T, *fixture, *Record) {},
			wait:       timeout + pollTick,
			wantStatus: StatusDead,
			wantReason: DeadReasonTimeout,
		},
		{
			name:       "ShorterWaitReturnsRunning",
			arrange:    func(*testing.T, *fixture, *Record) {},
			wait:       2 * time.Second,
			wantStatus: StatusRunning,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fx := newFixture(t)
			rec := fx.spawn(t)
			tt.arrange(t, fx, rec)

			result, err := Await(fx.deps, rec, timeout, tt.wait)
			if err != nil {
				t.Fatalf("Await() error = %v; want nil", err)
			}
			if result.Status != tt.wantStatus || result.DeadReason != tt.wantReason || result.Reply != tt.wantReply || len(result.Warnings) != 0 {
				t.Errorf("Await() = %+v; want status %q, reason %q, reply %q and no warnings", result, tt.wantStatus, tt.wantReason, tt.wantReply)
			}
			wantStops := []string{rec.StrandGUID}
			if tt.wantStatus == StatusRunning {
				wantStops = nil
			}
			if !slices.Equal(fx.stopper.guids, wantStops) {
				t.Errorf("stopped = %v; want %v", fx.stopper.guids, wantStops)
			}
			_, statErr := os.Stat(rec.ShuttleRunDir)
			if removed := os.IsNotExist(statErr); removed != (tt.wantStatus == StatusDone) {
				t.Errorf("run dir removed = %v; want removed only on done", removed)
			}
		})
	}

	t.Run("FailedStopIsAWarning", func(t *testing.T) {
		t.Parallel()
		fx := newFixture(t)
		rec := fx.spawn(t)
		if err := os.WriteFile(fx.replyPath, []byte("fixed"), 0o644); err != nil {
			t.Fatalf("write reply: %v", err)
		}
		fx.stopper.err = os.ErrPermission

		result, err := Await(fx.deps, rec, timeout, 0)
		if err != nil || result.Status != StatusDone || result.Reply != "fixed" {
			t.Fatalf("Await() = %+v, %v; want done with the reply", result, err)
		}
		if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "stop strand "+rec.StrandGUID) {
			t.Errorf("warnings = %v; want the one failed stop", result.Warnings)
		}
	})

	errorRows := []struct {
		name    string
		mutate  func(rec *Record)
		wantErr string
	}{
		{"EngineTheDepsDoNotHoldIsAnError", func(rec *Record) { rec.Engine = "missing-engine" }, "missing-engine"},
		{"UnparsableSpawnedAtIsAnError", func(rec *Record) { rec.SpawnedAt = "yesterday" }, "yesterday"},
	}
	for _, tt := range errorRows {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fx := newFixture(t)
			rec := *fx.spawn(t)
			tt.mutate(&rec)

			result, err := Await(fx.deps, &rec, timeout, 0)
			if err == nil || result != nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Await() = %+v, %v; want an error containing %q", result, err, tt.wantErr)
			}
			if len(fx.stopper.guids) != 0 {
				t.Errorf("stopped = %v; want nothing stopped", fx.stopper.guids)
			}
		})
	}
}
