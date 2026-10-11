// entries_discussionwrite_test.go covers discussionWriteEntry: its construction-time validation
// over Config and the three injected Env seams it reads, and one full Call proving the injected
// SpecSource, the shuttle, and the commit closure are all reached and the outcome mapping is
// preserved untouched.

package shedrecipe

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/parentreview"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
	"github.com/Knatte18/loomyard/internal/testkit/shedfake"
)

// TestDiscussionWriteEntry_ConstructionFailures covers the three seams discussionWriteEntry
// requires and the Config rejection every entry in this package performs first. A bare zero-value
// assignment on a copy of newTestEnv(t)'s Env follows entries_simple_test.go's zeroEnvField
// precedent for the structurally identical WebsterRun func field: DiscussionSpec and
// CommitDiscussion are both concrete named func types, so env.DiscussionSpec = nil already boxes as
// a typed-nil when it reaches requireSeam's any parameter, making the plain-nil branch unreachable
// and a second "typed-nil versus untyped-nil" variant unnecessary.
func TestDiscussionWriteEntry_ConstructionFailures(t *testing.T) {
	t.Run("NilDiscussionSpec", func(t *testing.T) {
		env := newTestEnv(t)
		env.DiscussionSpec = nil
		_, err := discussionWriteEntry("Row", Config{}, env)
		if err == nil {
			t.Fatalf("discussionWriteEntry() error = nil; want non-nil when Env.DiscussionSpec is nil")
		}
		if !strings.Contains(err.Error(), "DiscussionWrite") || !strings.Contains(err.Error(), "DiscussionSpec") {
			t.Errorf("discussionWriteEntry() error = %v; want it to name entry %q and field %q", err, "DiscussionWrite", "DiscussionSpec")
		}
	})

	t.Run("NilPrepareDiscussionSpawn", func(t *testing.T) {
		env := newTestEnv(t)
		env.PrepareDiscussionSpawn = nil
		_, err := discussionWriteEntry("Row", Config{}, env)
		if err == nil {
			t.Fatalf("discussionWriteEntry() error = nil; want non-nil when Env.PrepareDiscussionSpawn is nil")
		}
		if !strings.Contains(err.Error(), "DiscussionWrite") || !strings.Contains(err.Error(), "PrepareDiscussionSpawn") {
			t.Errorf("discussionWriteEntry() error = %v; want it to name entry %q and field %q", err, "DiscussionWrite", "PrepareDiscussionSpawn")
		}
	})

	t.Run("NilCommitDiscussion", func(t *testing.T) {
		env := newTestEnv(t)
		env.CommitDiscussion = nil
		_, err := discussionWriteEntry("Row", Config{}, env)
		if err == nil {
			t.Fatalf("discussionWriteEntry() error = nil; want non-nil when Env.CommitDiscussion is nil")
		}
		if !strings.Contains(err.Error(), "DiscussionWrite") || !strings.Contains(err.Error(), "CommitDiscussion") {
			t.Errorf("discussionWriteEntry() error = %v; want it to name entry %q and field %q", err, "DiscussionWrite", "CommitDiscussion")
		}
	})

	t.Run("NilShuttle", func(t *testing.T) {
		env := newTestEnv(t)
		env.Shuttle = nil
		_, err := discussionWriteEntry("Row", Config{}, env)
		if err == nil {
			t.Fatalf("discussionWriteEntry() error = nil; want non-nil when Env.Shuttle is nil")
		}
		if !strings.Contains(err.Error(), "DiscussionWrite") || !strings.Contains(err.Error(), "Shuttle") {
			t.Errorf("discussionWriteEntry() error = %v; want it to name entry %q and field %q", err, "DiscussionWrite", "Shuttle")
		}
	})
}

// TestDiscussionWriteEntry_HappyPath asserts a fully-filled Env constructs a non-nil producer with
// a nil error.
func TestDiscussionWriteEntry_HappyPath(t *testing.T) {
	env := newTestEnv(t)
	producer, err := discussionWriteEntry("Row", Config{}, env)
	if err != nil {
		t.Fatalf("discussionWriteEntry() error = %v; want nil", err)
	}
	if producer == nil {
		t.Fatalf("discussionWriteEntry() = nil producer; want non-nil")
	}
}

// TestDiscussionWriteEntry_CallDone drives the happy-path producer's Call once against a
// shedfake.Shuttle reporting Done, and asserts the injected SpecSource was evaluated, the returned
// OutputPointer.Path equals the Spec's first OutputFiles entry, and the injected commit closure
// fired exactly once.
func TestDiscussionWriteEntry_CallDone(t *testing.T) {
	env := newTestEnv(t)

	var gotSpec shuttleengine.Spec
	env.DiscussionSpec = func() (shuttleengine.Spec, error) {
		gotSpec = shuttleengine.Spec{
			Prompt:      "discussion prompt",
			OutputFiles: []string{filepath.Join(env.WorktreeRoot, "decision-record.md")},
			Interactive: false,
		}
		return gotSpec, nil
	}
	commitCalls := 0
	env.CommitDiscussion = func() error {
		commitCalls++
		return nil
	}

	producer, err := discussionWriteEntry("Row", Config{}, env)
	if err != nil {
		t.Fatalf("discussionWriteEntry() error = %v; want nil", err)
	}

	fake := env.Shuttle.(*shedfake.Shuttle)
	fake.Result = shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}

	pointer := shedfake.RequireOutcome(t, producer, shedengine.Done)
	if len(fake.Specs) != 1 {
		t.Fatalf("fake.Specs has %d entries; want 1 -- the injected SpecSource must have been evaluated", len(fake.Specs))
	}
	if pointer.Path != gotSpec.OutputFiles[0] {
		t.Errorf("Call() OutputPointer.Path = %q; want %q", pointer.Path, gotSpec.OutputFiles[0])
	}
	if commitCalls != 1 {
		t.Errorf("commit closure invoked %d times; want exactly 1", commitCalls)
	}
}

// TestDiscussionWriteEntry_GateConfig covers the "gates" Config key discussionWriteEntry resolves through resolveGateSpec:
// a "discussion" entry resolves to the discussion validator, an unrecognised name fails loud naming both legal values, and a row carrying no key resolves to the empty shuttleengine.GateSpec -- an ungated producer.
func TestDiscussionWriteEntry_GateConfig(t *testing.T) {
	t.Run("GateDiscussionResolvesToDiscussionValidator", func(t *testing.T) {
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
		env := newTestEnv(t)
		_, err := discussionWriteEntry("Row", gatesCfg("bogus", 3), env)
		if err == nil {
			t.Fatal("discussionWriteEntry() error = nil; want non-nil for an unrecognised gate value")
		}
		assertErrContains(t, err, "name")
		assertErrContains(t, err, "discussion")
		assertErrContains(t, err, "plan")
	})

	t.Run("NeitherKeyBuildsUngatedProducer", func(t *testing.T) {
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

// TestDiscussionWriteEntry_ParentReviewBeginRound asserts a fresh spawn opens a round, an attach does not, and a disabled entry never does.
func TestDiscussionWriteEntry_ParentReviewBeginRound(t *testing.T) {
	t.Run("FreshSpawnBeginsRound", func(t *testing.T) {
		env := parentReviewEnv(t)
		p, err := discussionWriteEntry("Row", parentReviewCfg(3), env)
		if err != nil {
			t.Fatalf("discussionWriteEntry() error = %v", err)
		}
		env.Shuttle.(*shedfake.Shuttle).Result = shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}
		if _, _, err := p.Call(context.Background()); err != nil {
			t.Fatalf("Call() error = %v", err)
		}
		if !hasRound(t, env) {
			t.Error("no round after a fresh spawn; want BeginRound to have run")
		}
	})
	t.Run("FreshSpawnKeepsRejectRound", func(t *testing.T) {
		env := parentReviewEnv(t)
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
		p, err := discussionWriteEntry("Row", parentReviewCfg(3), env)
		if err != nil {
			t.Fatalf("discussionWriteEntry() error = %v", err)
		}
		env.Shuttle.(*shedfake.Shuttle).Result = shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}
		_, _, _ = p.Call(context.Background())
		latest, ok, err := env.ParentReview.Store.Latest()
		if err != nil || !ok {
			t.Fatalf("Latest() = %v, %v", ok, err)
		}
		if latest.Number != round.Number {
			t.Errorf("latest round = %d after a fresh spawn; want the reject round %d kept", latest.Number, round.Number)
		}
	})
	t.Run("AttachDoesNotBeginRound", func(t *testing.T) {
		env := parentReviewEnv(t)
		attached := env.Shuttle.(*shedfake.Shuttle)
		attached.AttachFound = true
		// shedfake.Shuttle evaluates the gate on an attached Done run, and the parent-review gate opens a round; a Died run takes the attach branch without evaluating it.
		attached.AttachResult = shuttleengine.Result{Outcome: shuttleengine.OutcomeDied}
		p, err := discussionWriteEntry("Row", parentReviewCfg(3), env)
		if err != nil {
			t.Fatalf("discussionWriteEntry() error = %v", err)
		}
		_, _, _ = p.Call(context.Background())
		if hasRound(t, env) {
			t.Error("round exists after an attach; want BeginRound not to run")
		}
	})
	t.Run("PrepareDiscussionSpawnRunsOnFreshSpawnOnly", func(t *testing.T) {
		tests := []struct {
			name      string
			attach    bool
			attempts  int
			wantCalls int
		}{
			{"fresh spawn with a parent-review gate", false, 3, 1},
			{"fresh spawn without a parent-review gate", false, 0, 1},
			{"attach with a parent-review gate", true, 3, 0},
			{"attach without a parent-review gate", true, 0, 0},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				env := parentReviewEnv(t)
				calls := 0
				env.PrepareDiscussionSpawn = func() error {
					calls++
					return nil
				}
				fake := env.Shuttle.(*shedfake.Shuttle)
				if tt.attach {
					fake.AttachFound = true
					fake.AttachResult = shuttleengine.Result{Outcome: shuttleengine.OutcomeDied}
				} else {
					fake.Result = shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}
				}
				p, err := discussionWriteEntry("Row", parentReviewCfg(tt.attempts), env)
				if err != nil {
					t.Fatalf("discussionWriteEntry() error = %v", err)
				}
				_, _, _ = p.Call(context.Background())
				if calls != tt.wantCalls {
					t.Errorf("PrepareDiscussionSpawn ran %d times; want %d", calls, tt.wantCalls)
				}
			})
		}
	})
	t.Run("PrepareDiscussionSpawnErrorStopsTheSpawn", func(t *testing.T) {
		env := parentReviewEnv(t)
		env.PrepareDiscussionSpawn = func() error { return errors.New("marker locked") }
		fake := env.Shuttle.(*shedfake.Shuttle)
		fake.Result = shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}
		p, err := discussionWriteEntry("Row", parentReviewCfg(3), env)
		if err != nil {
			t.Fatalf("discussionWriteEntry() error = %v", err)
		}
		_, _, err = p.Call(context.Background())
		if err == nil || !strings.Contains(err.Error(), "marker locked") {
			t.Errorf("Call() error = %v; want it to carry the closure's error", err)
		}
		if len(fake.Specs) != 0 || hasRound(t, env) {
			t.Errorf("spawned %d specs and round = %v after a failed preparation; want neither", len(fake.Specs), hasRound(t, env))
		}
	})
	t.Run("DisabledEntryDoesNotBeginRound", func(t *testing.T) {
		env := parentReviewEnv(t)
		p, err := discussionWriteEntry("Row", parentReviewCfg(0), env)
		if err != nil {
			t.Fatalf("discussionWriteEntry() error = %v", err)
		}
		env.Shuttle.(*shedfake.Shuttle).Result = shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}
		if _, _, err := p.Call(context.Background()); err != nil {
			t.Fatalf("Call() error = %v", err)
		}
		if hasRound(t, env) {
			t.Error("round exists with the entry off; want no BeginRound")
		}
	})
}
