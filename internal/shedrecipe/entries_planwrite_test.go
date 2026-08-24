// entries_planwrite_test.go covers planWriteEntry: its construction-time validation over Config
// and the four injected/told Env fields it reads, and one full Call proving the injected
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

// TestPlanWriteEntry_ConstructionFailures covers the three seams planWriteEntry requires, the
// AnchorPath root it validates, and the Config rejection every entry in this package performs
// first. A bare zero-value assignment on a copy of newTestEnv(t)'s Env follows
// entries_discussionwrite_test.go's precedent: PlanSpec and CommitPlan are both concrete named func
// types, so env.PlanSpec = nil already boxes as a typed-nil when it reaches requireSeam's any
// parameter, making the plain-nil branch unreachable and a second "typed-nil versus untyped-nil"
// variant unnecessary.
func TestPlanWriteEntry_ConstructionFailures(t *testing.T) {
	t.Run("NilPlanSpec", func(t *testing.T) {
		env := newTestEnv(t)
		env.PlanSpec = nil
		_, err := planWriteEntry("Row", Config{}, env)
		if err == nil {
			t.Fatalf("planWriteEntry() error = nil; want non-nil when Env.PlanSpec is nil")
		}
		if !strings.Contains(err.Error(), "PlanWrite") || !strings.Contains(err.Error(), "PlanSpec") {
			t.Errorf("planWriteEntry() error = %v; want it to name entry %q and field %q", err, "PlanWrite", "PlanSpec")
		}
	})

	t.Run("NilCommitPlan", func(t *testing.T) {
		env := newTestEnv(t)
		env.CommitPlan = nil
		_, err := planWriteEntry("Row", Config{}, env)
		if err == nil {
			t.Fatalf("planWriteEntry() error = nil; want non-nil when Env.CommitPlan is nil")
		}
		if !strings.Contains(err.Error(), "PlanWrite") || !strings.Contains(err.Error(), "CommitPlan") {
			t.Errorf("planWriteEntry() error = %v; want it to name entry %q and field %q", err, "PlanWrite", "CommitPlan")
		}
	})

	t.Run("NilShuttle", func(t *testing.T) {
		env := newTestEnv(t)
		env.Shuttle = nil
		_, err := planWriteEntry("Row", Config{}, env)
		if err == nil {
			t.Fatalf("planWriteEntry() error = nil; want non-nil when Env.Shuttle is nil")
		}
		if !strings.Contains(err.Error(), "PlanWrite") || !strings.Contains(err.Error(), "Shuttle") {
			t.Errorf("planWriteEntry() error = %v; want it to name entry %q and field %q", err, "PlanWrite", "Shuttle")
		}
	})

	t.Run("EmptyAnchorPath", func(t *testing.T) {
		env := newTestEnv(t)
		env.AnchorPath = ""
		_, err := planWriteEntry("Row", Config{}, env)
		if err == nil {
			t.Fatalf("planWriteEntry() error = nil; want non-nil when Env.AnchorPath is empty")
		}
		if !strings.Contains(err.Error(), "PlanWrite") || !strings.Contains(err.Error(), "AnchorPath") {
			t.Errorf("planWriteEntry() error = %v; want it to name entry %q and field %q", err, "PlanWrite", "AnchorPath")
		}
	})

	t.Run("UnrecognisedConfigKey", func(t *testing.T) {
		env := newTestEnv(t)
		_, err := planWriteEntry("Row", Config{"bogus_key": "x"}, env)
		if err == nil {
			t.Fatalf("planWriteEntry() error = nil; want non-nil for an unrecognised config key")
		}
		if !strings.Contains(err.Error(), "bogus_key") {
			t.Errorf("planWriteEntry() error = %v; want it to name the offending key %q", err, "bogus_key")
		}
	})
}

// TestPlanWriteEntry_HappyPath asserts a fully-filled Env constructs a non-nil producer with a nil
// error.
func TestPlanWriteEntry_HappyPath(t *testing.T) {
	env := newTestEnv(t)
	producer, err := planWriteEntry("Row", Config{}, env)
	if err != nil {
		t.Fatalf("planWriteEntry() error = %v; want nil", err)
	}
	if producer == nil {
		t.Fatalf("planWriteEntry() = nil producer; want non-nil")
	}
}

// TestPlanWriteEntry_CallDone drives the happy-path producer's Call once against a fakeShuttle
// reporting Done, and asserts the injected SpecSource was evaluated, the returned
// OutputPointer.Path equals the Spec's first OutputFiles entry, and the injected commit closure
// fired exactly once. env.AnchorPath from newTestEnv is a real directory that contains no plan
// directory, so the decorator's rotation is a legitimate absent-directory no-op here.
func TestPlanWriteEntry_CallDone(t *testing.T) {
	env := newTestEnv(t)

	var gotSpec shuttleengine.Spec
	env.PlanSpec = func() (shuttleengine.Spec, error) {
		gotSpec = shuttleengine.Spec{
			Prompt:      "plan prompt",
			OutputFiles: []string{filepath.Join(env.AnchorPath, "00-overview.md")},
			Interactive: false,
		}
		return gotSpec, nil
	}
	commitCalls := 0
	env.CommitPlan = func() error {
		commitCalls++
		return nil
	}

	producer, err := planWriteEntry("Row", Config{}, env)
	if err != nil {
		t.Fatalf("planWriteEntry() error = %v; want nil", err)
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

// TestPlanWriteEntry_CallAsking asserts an OutcomeAsking shuttle result maps to shedengine.Stuck
// and leaves the commit closure uninvoked -- the outcome mapping the decorator must preserve
// untouched. env.AnchorPath from newTestEnv is a real directory that contains no plan directory,
// so the decorator's rotation is a legitimate absent-directory no-op here.
func TestPlanWriteEntry_CallAsking(t *testing.T) {
	env := newTestEnv(t)

	commitCalls := 0
	env.CommitPlan = func() error {
		commitCalls++
		return nil
	}

	producer, err := planWriteEntry("Row", Config{}, env)
	if err != nil {
		t.Fatalf("planWriteEntry() error = %v; want nil", err)
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
