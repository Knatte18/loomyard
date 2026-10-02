// entries_discussionwrite_test.go covers discussionWriteEntry: its construction-time validation
// over Config and the three injected Env seams it reads, and one full Call proving the injected
// SpecSource, the shuttle, and the commit closure are all reached and the outcome mapping is
// preserved untouched.

package shedrecipe

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shuttleengine"
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

	t.Run("UnrecognisedConfigKey", func(t *testing.T) {
		env := newTestEnv(t)
		_, err := discussionWriteEntry("Row", Config{"bogus_key": "x"}, env)
		if err == nil {
			t.Fatalf("discussionWriteEntry() error = nil; want non-nil for an unrecognised config key")
		}
		if !strings.Contains(err.Error(), "bogus_key") {
			t.Errorf("discussionWriteEntry() error = %v; want it to name the offending key %q", err, "bogus_key")
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
// fakeShuttle reporting Done, and asserts the injected SpecSource was evaluated, the returned
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

	fake := env.Shuttle.(*fakeShuttle)
	fake.result = shuttleengine.Result{Outcome: shuttleengine.OutcomeDone}

	outcome, pointer, err := producer.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Done {
		t.Errorf("Call() outcome = %v; want %v", outcome, shedengine.Done)
	}
	if len(fake.specs) != 1 {
		t.Fatalf("fake.specs has %d entries; want 1 -- the injected SpecSource must have been evaluated", len(fake.specs))
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

// TestDiscussionWriteEntry_CallAsking asserts an OutcomeAsking shuttle result maps to
// shedengine.Stuck and leaves the commit closure uninvoked -- the outcome mapping the decorator
// must preserve untouched.
func TestDiscussionWriteEntry_CallAsking(t *testing.T) {
	env := newTestEnv(t)

	commitCalls := 0
	env.CommitDiscussion = func() error {
		commitCalls++
		return nil
	}

	producer, err := discussionWriteEntry("Row", Config{}, env)
	if err != nil {
		t.Fatalf("discussionWriteEntry() error = %v; want nil", err)
	}

	fake := env.Shuttle.(*fakeShuttle)
	fake.result = shuttleengine.Result{Outcome: shuttleengine.OutcomeAsking}

	outcome, _, err := producer.Call(context.Background())
	if err != nil {
		t.Fatalf("Call() error = %v; want nil", err)
	}
	if outcome != shedengine.Stuck {
		t.Errorf("Call() outcome = %v; want %v", outcome, shedengine.Stuck)
	}
	if commitCalls != 0 {
		t.Errorf("commit closure invoked %d times; want 0 for an Asking outcome", commitCalls)
	}
}
