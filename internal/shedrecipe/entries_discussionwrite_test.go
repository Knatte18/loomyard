// entries_discussionwrite_test.go covers discussionWriteEntry: its construction-time validation over Config and the three Env seams it reads,
// and the Calls proving the told table is evaluated once, carries the row's gate, and that the parent-review round is opened exactly on a fresh spawn.

package shedrecipe

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/parentreview"
	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
)

// liveSeatLead is a non-nil seatengine.Handle the fake seat runner hands back as a live lead; the producer never calls it.
var liveSeatLead seatengine.Handle = (*shuttleengine.Run)(nil)

// withSeatTable fills env.DiscussionTable with a one-lead table writing output and returns the count of times the source was evaluated.
func withSeatTable(env *Env, output string) *int {
	evaluations := 0
	env.DiscussionTable = func() (seatengine.Table, error) {
		evaluations++
		return seatengine.Table{
			RolePrefix: "discussion",
			Segment:    segmentcolor.Discussion,
			Seats:      []seatengine.Seat{{Stencil: "loom-template-discussion-chair", Outputs: []string{output}}},
		}, nil
	}
	return &evaluations
}

// leadDoneResult is a done lead writing output whose result carries a passing GateOutcome, so the fake seat runner never evaluates the row's gate:
// the discussion gate would fail on newTestEnv's absent support log and the parent-review gate would open a round, either of which would make a case assert the fake rather than the entry.
func leadDoneResult(output string) seatengine.Result {
	return seatengine.Result{
		Chair:        shuttleengine.Result{Outcome: shuttleengine.OutcomeDone, Gate: &shuttleengine.GateOutcome{Passed: true, Attempts: 1}},
		ChairOutputs: []string{output},
	}
}

// TestDiscussionWriteEntry_ConstructionFailures covers the three seams discussionWriteEntry requires and the Config rejection every entry in this package performs first.
func TestDiscussionWriteEntry_ConstructionFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mutate  func(*Env)
		cfg     Config
		wantErr []string
	}{
		{"NilDiscussionTable", func(env *Env) { env.DiscussionTable = nil }, Config{}, []string{"DiscussionWrite", "DiscussionTable"}},
		{"NilCommitDiscussion", func(env *Env) { env.CommitDiscussion = nil }, Config{}, []string{"DiscussionWrite", "CommitDiscussion"}},
		{"NilSeats", func(env *Env) { env.Seats = nil }, Config{}, []string{"DiscussionWrite", "Seats"}},
		{"UnknownConfigKey", func(*Env) {}, Config{"bogus_key": "x"}, []string{"bogus_key"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env := newTestEnv(t)
			tt.mutate(&env)
			_, err := discussionWriteEntry(RowIdentity{Name: "Row"}, tt.cfg, env)
			for _, want := range tt.wantErr {
				assertErrContains(t, err, want)
			}
		})
	}
}

// TestDiscussionWriteEntry_HappyPath asserts a fully-filled Env constructs a non-nil producer with a nil error.
func TestDiscussionWriteEntry_HappyPath(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	producer, err := discussionWriteEntry(RowIdentity{Name: "Row"}, Config{}, env)
	if err != nil {
		t.Fatalf("discussionWriteEntry() error = %v; want nil", err)
	}
	if producer == nil {
		t.Fatalf("discussionWriteEntry() = nil producer; want non-nil")
	}
}

// TestDiscussionWriteEntry_CallDone drives the producer's Call once against a done lead and asserts the told table was evaluated once and reached the seat runner with the row's gate, the returned OutputPointer.Path is the lead's first output, and the commit closure fired exactly once.
func TestDiscussionWriteEntry_CallDone(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t)
	output := filepath.Join(env.WorktreeRoot, "decision-record.md")
	evaluations := withSeatTable(&env, output)
	commits := 0
	env.CommitDiscussion = func() error {
		commits++
		return nil
	}
	fake := &shedfake.SeatRunner{Results: []seatengine.Result{leadDoneResult(output)}}
	env.Seats = fake

	producer, err := discussionWriteEntry(RowIdentity{Name: "Row"}, gatesCfg("discussion", 2), env)
	if err != nil {
		t.Fatalf("discussionWriteEntry() error = %v; want nil", err)
	}
	pointer := shedfake.RequireOutcome(t, producer, shedengine.Done)

	if *evaluations != 1 {
		t.Errorf("table source evaluated %d times; want 1", *evaluations)
	}
	if len(fake.GotTables) != 1 {
		t.Fatalf("seat runner saw %d tables; want 1", len(fake.GotTables))
	}
	gate := fake.GotTables[0].Gate
	if len(gate) != 1 || gate[0].Name != "discussion" || gate[0].Attempts != 2 {
		t.Errorf("table gate = %+v; want one \"discussion\" entry with 2 attempts", gate)
	}
	if pointer.Path != output {
		t.Errorf("Call() OutputPointer.Path = %q; want the lead's first output %q", pointer.Path, output)
	}
	if commits != 1 {
		t.Errorf("commit closure invoked %d times; want exactly 1", commits)
	}
}

// TestDiscussionWriteEntry_GateConfig covers the "gates" Config key discussionWriteEntry resolves through resolveGateSpec:
// a "discussion" entry resolves to the discussion validator, an unrecognised name fails loud naming both legal values, and a row carrying no key resolves to the empty shuttleengine.GateSpec -- an ungated producer.
func TestDiscussionWriteEntry_GateConfig(t *testing.T) {
	t.Parallel()
	t.Run("GateDiscussionResolvesToDiscussionValidator", func(t *testing.T) {
		t.Parallel()
		env := newTestEnv(t)
		gateSpec, err := resolveGateSpec("DiscussionWrite", gatesCfg("discussion", 3), env)
		if err != nil {
			t.Fatalf("resolveGateSpec() error = %v; want nil", err)
		}
		if len(gateSpec) != 1 || gateSpec[0].Gate == nil {
			t.Fatalf("resolveGateSpec() = %+v; want one entry carrying the discussion closure", gateSpec)
		}

		// Drive the constructed gate rather than comparing func values, which Go cannot compare.
		// newTestEnv's SupportLogPath is a joined path nobody creates, so the discussion validator
		// reports it missing -- a finding text no plan-gate failure could ever produce, which is
		// what proves the discussion closure and not the plan one was resolved.
		result, err := gateSpec[0].Gate()
		if err != nil {
			t.Fatalf("gateSpec[0].Gate() error = %v; want nil", err)
		}
		if result.Passed {
			t.Fatal("gateSpec[0].Gate() Passed = true; want false for a missing support log")
		}
		if !strings.Contains(result.Findings, "support log does not exist") {
			t.Errorf("gateSpec[0].Gate() Findings = %q; want it to name the missing support log, proving the discussion validator ran", result.Findings)
		}
	})

	t.Run("UnrecognisedGateValueFails", func(t *testing.T) {
		t.Parallel()
		env := newTestEnv(t)
		_, err := discussionWriteEntry(RowIdentity{Name: "Row"}, gatesCfg("bogus", 3), env)
		if err == nil {
			t.Fatal("discussionWriteEntry() error = nil; want non-nil for an unrecognised gate value")
		}
		assertErrContains(t, err, "name")
		assertErrContains(t, err, "discussion")
		assertErrContains(t, err, "plan")
	})

	t.Run("NeitherKeyBuildsUngatedProducer", func(t *testing.T) {
		t.Parallel()
		env := newTestEnv(t)
		gateSpec, err := resolveGateSpec("DiscussionWrite", Config{}, env)
		if err != nil {
			t.Fatalf("resolveGateSpec() error = %v; want nil", err)
		}
		if len(gateSpec) != 0 {
			t.Errorf("resolveGateSpec() = %+v; want an empty list for a row carrying neither key", gateSpec)
		}
	})
}

// hasRound reports whether the parent-review store holds a round.
func hasRound(t *testing.T, env Env) bool {
	t.Helper()
	_, ok, err := env.ParentReview.Store.Latest()
	if err != nil {
		t.Fatalf("Latest() error = %v", err)
	}
	return ok
}

// TestDiscussionWriteEntry_ParentReviewBeginRound asserts a fresh spawn opens a round, a live lead's resume does not, a disabled entry never does, a reject round is kept across a fresh spawn, and a failed preparation starts no seat and archives nothing.
func TestDiscussionWriteEntry_ParentReviewBeginRound(t *testing.T) {
	t.Parallel()

	t.Run("RoundOpening", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name      string
			attempts  int
			live      bool
			wantRound bool
		}{
			{"FreshSpawnOpensRound", 3, false, true},
			{"LiveLeadOpensNoRound", 3, true, false},
			{"DisabledEntryOpensNoRound", 0, false, false},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				env := parentReviewEnv(t)
				output := filepath.Join(env.WorktreeRoot, "decision-record.md")
				withSeatTable(&env, output)
				fake := &shedfake.SeatRunner{Results: []seatengine.Result{leadDoneResult(output)}}
				if tt.live {
					fake.ProbeFn = func(seatengine.Table) (seatengine.LiveTable, error) {
						return seatengine.LiveTable{Chair: liveSeatLead}, nil
					}
					fake.ResumeFn = func(seatengine.Table, seatengine.LiveTable) (seatengine.Result, error) {
						return leadDoneResult(output), nil
					}
				}
				env.Seats = fake

				producer, err := discussionWriteEntry(RowIdentity{Name: "Row"}, parentReviewCfg(tt.attempts), env)
				if err != nil {
					t.Fatalf("discussionWriteEntry() error = %v; want nil", err)
				}
				shedfake.RequireOutcome(t, producer, shedengine.Done)

				if got := hasRound(t, env); got != tt.wantRound {
					t.Errorf("round opened = %v; want %v", got, tt.wantRound)
				}
			})
		}
	})

	t.Run("FreshSpawnKeepsRejectRound", func(t *testing.T) {
		t.Parallel()
		env := parentReviewEnv(t)
		output := filepath.Join(env.WorktreeRoot, "decision-record.md")
		withSeatTable(&env, output)
		env.Seats = &shedfake.SeatRunner{Results: []seatengine.Result{leadDoneResult(output)}}
		spec, err := resolveGateSpec("Row", parentReviewCfg(3), env)
		if err != nil {
			t.Fatalf("resolveGateSpec() error = %v", err)
		}
		if _, err := spec[0].Gate(); err != nil {
			t.Fatalf("gate() error = %v", err)
		}
		round, ok, err := env.ParentReview.Store.Latest()
		if err != nil || !ok {
			t.Fatalf("Latest() = %v, %v", ok, err)
		}
		review := filepath.Join(t.TempDir(), "review.md")
		if err := os.WriteFile(review, []byte("fix this"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := env.ParentReview.Store.RecordVerdict(parentreview.VerdictReject, review); err != nil {
			t.Fatalf("RecordVerdict() error = %v", err)
		}
		producer, err := discussionWriteEntry(RowIdentity{Name: "Row"}, parentReviewCfg(3), env)
		if err != nil {
			t.Fatalf("discussionWriteEntry() error = %v", err)
		}
		_, _, _ = producer.Call(context.Background())
		latest, ok, err := env.ParentReview.Store.Latest()
		if err != nil || !ok {
			t.Fatalf("Latest() = %v, %v", ok, err)
		}
		if latest.Number != round.Number {
			t.Errorf("latest round = %d after a fresh spawn; want the reject round %d kept", latest.Number, round.Number)
		}
	})

	t.Run("PreparationFailureStartsNoSeatAndArchivesNothing", func(t *testing.T) {
		t.Parallel()
		env := parentReviewEnv(t)
		output := filepath.Join(env.WorktreeRoot, "decision-record.md")
		if err := os.WriteFile(output, []byte("previous run"), 0o644); err != nil {
			t.Fatalf("write output: %v", err)
		}
		withSeatTable(&env, output)
		// A store root under a regular file cannot be created, so opening the round fails.
		blocker := filepath.Join(t.TempDir(), "blocker")
		if err := os.WriteFile(blocker, nil, 0o644); err != nil {
			t.Fatalf("write blocker: %v", err)
		}
		env.ParentReview.Store.Root = filepath.Join(blocker, "reviews")
		fake := &shedfake.SeatRunner{}
		env.Seats = fake

		producer, err := discussionWriteEntry(RowIdentity{Name: "Row"}, parentReviewCfg(3), env)
		if err != nil {
			t.Fatalf("discussionWriteEntry() error = %v; want nil", err)
		}
		if _, _, err := producer.Call(context.Background()); err == nil {
			t.Fatal("Call() error = nil; want the failed preparation")
		}

		if fake.Calls != 0 {
			t.Errorf("seat runner ran %d times; want no seat started", fake.Calls)
		}
		if got, err := os.ReadFile(output); err != nil || string(got) != "previous run" {
			t.Errorf("lead output = %q, %v; want it left in place, not archived", got, err)
		}
	})
}
