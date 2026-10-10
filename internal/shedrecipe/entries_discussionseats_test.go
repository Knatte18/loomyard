// entries_discussionseats_test.go covers discussionSeatsEntry: its construction-time validation over Config and the three Env seams it reads,
// and the Calls proving the told table is evaluated once, carries the row's gate, and that the parent-review round is opened exactly on a fresh spawn.

package shedrecipe

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/seatengine"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
)

// liveSeatChair is a non-nil seatengine.Handle the fake seat runner hands back as a live chair; the producer never calls it.
var liveSeatChair seatengine.Handle = (*shuttleengine.Run)(nil)

// withSeatTable fills env.DiscussionTable with a one-chair table writing output and returns the count of times the source was evaluated.
func withSeatTable(env *Env, output string) *int {
	evaluations := 0
	env.DiscussionTable = func() (seatengine.Table, error) {
		evaluations++
		return seatengine.Table{
			RolePrefix: "discussion",
			Segment:    segmentcolor.Discussion,
			Seats:      []seatengine.Seat{{Name: seatengine.RoleChair, Stencil: "loom-template-discussion-chair", Outputs: []string{output}}},
		}, nil
	}
	return &evaluations
}

// chairDoneResult is a done chair writing output whose result carries a passing GateOutcome, so the fake seat runner never evaluates the row's gate:
// the discussion gate would fail on newTestEnv's absent support log and the parent-review gate would open a round, either of which would make a case assert the fake rather than the entry.
func chairDoneResult(output string) seatengine.Result {
	return seatengine.Result{
		Chair:        shuttleengine.Result{Outcome: shuttleengine.OutcomeDone, Gate: &shuttleengine.GateOutcome{Passed: true, Attempts: 1}},
		ChairOutputs: []string{output},
	}
}

func TestDiscussionSeatsEntry_ConstructionFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mutate  func(*Env)
		cfg     Config
		wantErr []string
	}{
		{"NilDiscussionTable", func(env *Env) { env.DiscussionTable = nil }, Config{}, []string{"DiscussionSeats", "DiscussionTable"}},
		{"NilCommitDiscussion", func(env *Env) { env.CommitDiscussion = nil }, Config{}, []string{"DiscussionSeats", "CommitDiscussion"}},
		{"NilSeats", func(env *Env) { env.Seats = nil }, Config{}, []string{"DiscussionSeats", "Seats"}},
		{"UnknownConfigKey", func(*Env) {}, Config{"bogus_key": "x"}, []string{"bogus_key"}},
		{"UnrecognisedGateName", func(*Env) {}, gatesCfg("bogus", 3), []string{"DiscussionSeats", "bogus"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env := newTestEnv(t)
			tt.mutate(&env)
			_, err := discussionSeatsEntry("Row", tt.cfg, env)
			for _, want := range tt.wantErr {
				assertErrContains(t, err, want)
			}
		})
	}
}

func TestDiscussionSeatsEntry_Call(t *testing.T) {
	t.Parallel()

	t.Run("DoneChairCarriesTheRowGate", func(t *testing.T) {
		t.Parallel()
		env := newTestEnv(t)
		output := filepath.Join(env.WorktreeRoot, "decision-record.md")
		evaluations := withSeatTable(&env, output)
		commits := 0
		env.CommitDiscussion = func() error {
			commits++
			return nil
		}
		fake := &shedfake.SeatRunner{Results: []seatengine.Result{chairDoneResult(output)}}
		env.Seats = fake

		producer, err := discussionSeatsEntry("Row", gatesCfg("discussion", 2), env)
		if err != nil {
			t.Fatalf("discussionSeatsEntry() error = %v; want nil", err)
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
			t.Errorf("Call() OutputPointer.Path = %q; want the chair's first output %q", pointer.Path, output)
		}
		if commits != 1 {
			t.Errorf("commit closure invoked %d times; want exactly 1", commits)
		}
	})

	t.Run("ParentReviewRound", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name      string
			attempts  int
			live      bool
			wantRound bool
		}{
			{"FreshSpawnOpensRound", 3, false, true},
			{"LiveChairOpensNoRound", 3, true, false},
			{"DisabledEntryOpensNoRound", 0, false, false},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				env := parentReviewEnv(t)
				output := filepath.Join(env.WorktreeRoot, "decision-record.md")
				withSeatTable(&env, output)
				fake := &shedfake.SeatRunner{Results: []seatengine.Result{chairDoneResult(output)}}
				if tt.live {
					fake.ProbeFn = func(seatengine.Table) (seatengine.LiveTable, error) {
						return seatengine.LiveTable{Chair: liveSeatChair}, nil
					}
					fake.ResumeFn = func(seatengine.Table, seatengine.LiveTable) (seatengine.Result, error) {
						return chairDoneResult(output), nil
					}
				}
				env.Seats = fake

				producer, err := discussionSeatsEntry("Row", parentReviewCfg(tt.attempts), env)
				if err != nil {
					t.Fatalf("discussionSeatsEntry() error = %v; want nil", err)
				}
				shedfake.RequireOutcome(t, producer, shedengine.Done)

				if got := hasRound(t, env); got != tt.wantRound {
					t.Errorf("round opened = %v; want %v", got, tt.wantRound)
				}
			})
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

		producer, err := discussionSeatsEntry("Row", parentReviewCfg(3), env)
		if err != nil {
			t.Fatalf("discussionSeatsEntry() error = %v; want nil", err)
		}
		if _, _, err := producer.Call(context.Background()); err == nil {
			t.Fatal("Call() error = nil; want the failed preparation")
		}

		if fake.Calls != 0 {
			t.Errorf("seat runner ran %d times; want no seat started", fake.Calls)
		}
		if got, err := os.ReadFile(output); err != nil || string(got) != "previous run" {
			t.Errorf("chair output = %q, %v; want it left in place, not archived", got, err)
		}
	})
}
