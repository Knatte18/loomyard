// wire_test.go proves no seam wire (wire.go) builds is evaluated at wiring time: building the
// wiring for a slug whose worktree does not exist must succeed, and none of the four seams that
// resolve the managed task worktree's own Location -- the status-path resolver, the spawn
// directory, and both teardown halves -- may be invoked here, since each of the three that actually
// resolves that Location reaches the resolver (lyxcwd.ResolveWorktree), which spawns git; this
// suite stays untagged and Tier 1, so it proves laziness structurally rather than by invoking a
// seam and observing it run.

package battencli

import (
	"context"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Knatte18/loomyard/internal/battenshed"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// TestWire_SucceedsForNonexistentTaskWorktree asserts that wire returns no error even though the
// managed task worktree named by slug does not exist anywhere on disk -- the mechanical proof that
// wire itself resolves nothing about that worktree.
func TestWire_SucceedsForNonexistentTaskWorktree(t *testing.T) {
	c := &battenCLI{}
	location := &lyxcwd.Location{
		RepoName:     "example",
		HubPath:      t.TempDir(),
		WorktreeName: "hub-repo",
		AnchorRel:    ".",
	}

	if err := c.wire(location, "a-slug-with-no-worktree-anywhere"); err != nil {
		t.Fatalf("wire() error = %v; want nil", err)
	}
}

// TestWire_LazySeams covers all four lazy seams individually: the status-path resolver
// (Env.InnerRun.ResolveStatus), the spawn directory (Env.InnerRun.Spawn), and both teardown halves
// (Env.Teardown.Shutdown, Env.Teardown.Remove) are each present as an injected closure after wire
// returns -- not already-evaluated values -- for a slug whose worktree does not exist. Covering all
// four separately, rather than just the first, is deliberate: eager evaluation is exactly the
// failure laziness exists to avoid, and a test covering only one seam would let the other three
// regress silently.
func TestWire_LazySeams(t *testing.T) {
	c := &battenCLI{}
	location := &lyxcwd.Location{
		RepoName:     "example",
		HubPath:      t.TempDir(),
		WorktreeName: "hub-repo",
		AnchorRel:    ".",
	}

	if err := c.wire(location, "a-slug-with-no-worktree-anywhere"); err != nil {
		t.Fatalf("wire() error = %v; want nil", err)
	}

	tests := []struct {
		name    string
		present bool
	}{
		{"StatusPathResolver", c.env.InnerRun.ResolveStatus != nil},
		{"SpawnDirectory", c.env.InnerRun.Spawn != nil},
		{"TeardownShutdown", c.env.Teardown.Shutdown != nil},
		{"TeardownRemove", c.env.Teardown.Remove != nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.present {
				t.Errorf("wire() left this seam nil; want an injected closure present but uncalled")
			}
		})
	}
}

// TestWire_SeedChildClosuresFilled asserts all five Env.SeedChild closures are non-nil after wire
// returns, for a slug whose worktree does not exist -- the same laziness proof
// TestWire_LazySeams gives the four pre-existing seams, extended to this batch's fifth seam group.
func TestWire_SeedChildClosuresFilled(t *testing.T) {
	c := &battenCLI{}
	location := &lyxcwd.Location{
		RepoName:     "example",
		HubPath:      t.TempDir(),
		WorktreeName: "hub-repo",
		AnchorRel:    ".",
	}

	if err := c.wire(location, "a-slug-with-no-worktree-anywhere"); err != nil {
		t.Fatalf("wire() error = %v; want nil", err)
	}

	tests := []struct {
		name    string
		present bool
	}{
		{"ReadBoardType", c.env.SeedChild.ReadBoardType != nil},
		{"ChildDriver", c.env.SeedChild.ChildDriver != nil},
		{"WriteSeed", c.env.SeedChild.WriteSeed != nil},
		{"CommitSeed", c.env.SeedChild.CommitSeed != nil},
		{"PushSeed", c.env.SeedChild.PushSeed != nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.present {
				t.Errorf("wire() left this seam nil; want an injected closure present but uncalled")
			}
		})
	}
}

// TestWire_ChildDriverDelegatesToChildDriverOf asserts the wired SeedChild.ChildDriver closure
// reads exactly the value childDriverOf(seed) (arm.go) would compute over the same seed, for both
// an explicit child_driver param and an absent one -- the property F2 (crucible round
// sonnet-xhigh-r3) exists to guarantee structurally: refuseAdoptedSeed's own comparison and the
// value Seed-Child actually writes now share one defaulting implementation, so they cannot drift.
func TestWire_ChildDriverDelegatesToChildDriverOf(t *testing.T) {
	tests := []struct {
		name   string
		params map[string]string
		want   string
	}{
		{"ExplicitLLM", map[string]string{"child_driver": shedrun.DriverLLM}, shedrun.DriverLLM},
		{"AbsentParamDefaultsToGo", nil, shedrun.DriverGo},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &battenCLI{}
			location := &lyxcwd.Location{
				RepoName:     "example",
				HubPath:      t.TempDir(),
				WorktreeName: "hub-repo",
				AnchorRel:    ".",
			}
			if err := c.wire(location, "some-slug"); err != nil {
				t.Fatalf("wire() error = %v; want nil", err)
			}
			seed := shedrun.Seed{Recipe: shedrun.RecipeBatten, Driver: shedrun.DriverGo, Params: tt.params}
			if err := shedrun.WriteSeed(location, "some-slug", seed); err != nil {
				t.Fatalf("shedrun.WriteSeed = %v; want nil", err)
			}

			got, err := c.env.SeedChild.ChildDriver()
			if err != nil {
				t.Fatalf("ChildDriver() error = %v; want nil", err)
			}
			if got != tt.want {
				t.Errorf("ChildDriver() = %q; want %q", got, tt.want)
			}
			if want := childDriverOf(seed); got != want {
				t.Errorf("ChildDriver() = %q; want it to equal childDriverOf(seed) = %q", got, want)
			}
		})
	}
}

// TestWire_WriteSeedRefusesANonLoomChildBeforeTouchingTheWorktree pins the order inside the wired
// WriteSeed seam: a registered recipe the task worktree cannot bootstrap is refused with
// battenshed.ErrUnsupportedChildRecipe before the seam resolves the task worktree at all -- the
// location here has no worktree, so a loom recipe reaches the absent-pair refusal instead, which
// is an os.Stat, never the resolver, so this file stays Tier 1.
func TestWire_WriteSeedRefusesANonLoomChildBeforeTouchingTheWorktree(t *testing.T) {
	c := &battenCLI{}
	location := &lyxcwd.Location{
		RepoName:     "example",
		HubPath:      t.TempDir(),
		WorktreeName: "hub-repo",
		AnchorRel:    ".",
	}
	if err := c.wire(location, "a-slug-with-no-worktree-anywhere"); err != nil {
		t.Fatalf("wire() error = %v; want nil", err)
	}

	err := c.env.SeedChild.WriteSeed(context.Background(), shedrun.RecipeBatten, shedrun.DriverGo)
	if !errors.Is(err, battenshed.ErrUnsupportedChildRecipe) {
		t.Fatalf("WriteSeed(batten) error = %v; want it to wrap ErrUnsupportedChildRecipe", err)
	}
	if !strings.Contains(err.Error(), shedrun.RecipeLoom) {
		t.Errorf("WriteSeed(batten) error = %q; want it to name the one recipe a child may run", err)
	}

	err = c.env.SeedChild.WriteSeed(context.Background(), shedrun.RecipeLoom, shedrun.DriverGo)
	if errors.Is(err, battenshed.ErrUnsupportedChildRecipe) || errors.Is(err, battenshed.ErrUnknownRecipe) {
		t.Fatalf("WriteSeed(loom) error = %v; want a loom child admitted past the recipe checks", err)
	}
	if err == nil || !strings.Contains(err.Error(), "not present") {
		t.Errorf("WriteSeed(loom) error = %v; want the absent-pair refusal, proving the recipe check ran first", err)
	}
}

// TestWire_CommitStatusFilled asserts c.shedPaths.CommitStatus is non-nil after wire returns, now
// that the status file is durable, fabric-synced state (see paths.go) rather than the per-machine
// state nil used to document.
func TestWire_CommitStatusFilled(t *testing.T) {
	c := &battenCLI{}
	location := &lyxcwd.Location{
		RepoName:     "example",
		HubPath:      t.TempDir(),
		WorktreeName: "hub-repo",
		AnchorRel:    ".",
	}

	if err := c.wire(location, "a-slug-with-no-worktree-anywhere"); err != nil {
		t.Fatalf("wire() error = %v; want nil", err)
	}

	if c.shedPaths.CommitStatus == nil {
		t.Error("c.shedPaths.CommitStatus = nil; want a non-nil seam")
	}
}

// TestChildSpawnError asserts a child bootstrap's exit status is turned into a diagnosis: a
// non-zero exit carries the child's own output, a silent child passes the run error through, a nil
// run error stays nil, and over-long output is truncated with an explicit marker.
func TestChildSpawnError(t *testing.T) {
	runErr := errors.New("exit status 1")
	longOutput := strings.Repeat("x", maxChildOutputInError+50)

	tests := []struct {
		name        string
		runErr      error
		childOutput string
		wantNil     bool
		wantSubstr  []string
	}{
		{
			name:        "nil_run_error_stays_nil",
			runErr:      nil,
			childOutput: `{"ok":true}`,
			wantNil:     true,
		},
		{
			name:        "output_is_folded_into_the_error",
			runErr:      runErr,
			childOutput: `{"error":"shedrun: refusing to overwrite with disagreeing seed","ok":false}`,
			wantSubstr:  []string{"exit status 1", "disagreeing seed"},
		},
		{
			name:        "silent_child_passes_the_run_error_through",
			runErr:      runErr,
			childOutput: "   \n  ",
			wantSubstr:  []string{"exit status 1"},
		},
		{
			name:        "over_long_output_is_truncated_visibly",
			runErr:      runErr,
			childOutput: longOutput,
			wantSubstr:  []string{"exit status 1", "... (truncated)"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := childSpawnError(tt.runErr, tt.childOutput)
			if tt.wantNil {
				if got != nil {
					t.Fatalf("childSpawnError(nil, %q) = %v; want nil", tt.childOutput, got)
				}
				return
			}
			if got == nil {
				t.Fatalf("childSpawnError(%v, ...) = nil; want an error", tt.runErr)
			}
			if !errors.Is(got, tt.runErr) {
				t.Errorf("childSpawnError(...) does not unwrap to the run error; want errors.Is to hold")
			}
			for _, want := range tt.wantSubstr {
				if !strings.Contains(got.Error(), want) {
					t.Errorf("childSpawnError(...) = %q; want it to contain %q", got.Error(), want)
				}
			}
			if len(got.Error()) > maxChildOutputInError+200 {
				t.Errorf("childSpawnError(...) produced %d bytes; want the output capped near maxChildOutputInError", len(got.Error()))
			}
		})
	}
}

// TestChildSpawnError_TruncationStaysValidUTF8 pins the truncation boundary against a multi-byte
// rune straddling it: a naive byte slice at maxChildOutputInError can land mid-rune, embedding an
// invalid UTF-8 tail into the returned error. The fixture places a 3-byte rune ("€") exactly across
// that boundary.
func TestChildSpawnError_TruncationStaysValidUTF8(t *testing.T) {
	runErr := errors.New("exit status 1")
	childOutput := strings.Repeat("x", maxChildOutputInError-1) + "€ trailing text after the cut point"

	got := childSpawnError(runErr, childOutput)
	if got == nil {
		t.Fatal("childSpawnError(...) = nil; want an error")
	}
	if !utf8.ValidString(got.Error()) {
		t.Errorf("childSpawnError(...) = %q; want valid UTF-8, the truncation boundary split a multi-byte rune", got.Error())
	}
}

// TestTaskWorktreeLocation_AbsentPairIsNamed asserts an unmaterialized task worktree is reported on
// its own terms -- naming the run and the expected path, and pointing at a real remedy rather than a
// fabric command that would actually mutate prime itself -- rather than as the resolver's generic
// "not a git repository" failure.
func TestTaskWorktreeLocation_AbsentPairIsNamed(t *testing.T) {
	prime := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "warp", AnchorRel: "."}

	_, err := taskWorktreeLocation(prime, "never-created")
	if err == nil {
		t.Fatal("taskWorktreeLocation(prime, \"never-created\") = nil error; want a named refusal")
	}
	for _, want := range []string{"never-created", "is not present at", "resolve this by hand"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("taskWorktreeLocation(...) = %q; want it to contain %q", err.Error(), want)
		}
	}
	if strings.Contains(err.Error(), "lyx fabric checkout") {
		t.Errorf("taskWorktreeLocation(...) = %q; want it to never suggest \"lyx fabric checkout\" -- run from prime, that command mutates prime's own branch rather than restoring the missing task worktree", err.Error())
	}
	if strings.Contains(err.Error(), "not a git repository") {
		t.Errorf("taskWorktreeLocation(...) = %q; want the absent-pair case reported on its own terms, not as a resolver failure", err.Error())
	}
}

// TestTaskWorktreePresent_AbsentIsAnAnswerNotAnError asserts the create row's idempotency probe
// answers false with no error for a task worktree that was never created, so a genuinely absent one
// still reaches fabric's own create rather than short-circuiting the row.
//
// The present case needs a real git worktree and so lives at the integration tier
// (TestBattenIntegration_CreateRow_IsIdempotentAgainstAnAlreadyPresentWorktree); this suite stays
// untagged and never spawns git.
func TestTaskWorktreePresent_AbsentIsAnAnswerNotAnError(t *testing.T) {
	prime := &lyxcwd.Location{HubPath: t.TempDir(), WorktreeName: "warp", AnchorRel: "."}

	present, err := taskWorktreePresent(prime, "never-created")
	if err != nil {
		t.Fatalf("taskWorktreePresent(prime, \"never-created\") error = %v; want nil", err)
	}
	if present {
		t.Error("taskWorktreePresent(prime, \"never-created\") = true; want false")
	}
}
