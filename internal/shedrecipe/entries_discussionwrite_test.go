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
