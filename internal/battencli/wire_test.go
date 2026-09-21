// wire_test.go proves no seam wire (wire.go) builds is evaluated at wiring time: building the
// wiring for a slug whose worktree does not exist must succeed, and none of the four seams that
// resolve the managed task worktree's own Location -- the status-path resolver, the spawn
// directory, and both teardown halves -- may be invoked here, since each of the three that actually
// resolves that Location reaches the resolver (lyxcwd.ResolveWorktree), which spawns git; this
// suite stays untagged and Tier 1, so it proves laziness structurally rather than by invoking a
// seam and observing it run.

package battencli

import (
	"errors"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
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

// TestChildSpawnError proves the composition that turns a child bootstrap's exit status into a
// diagnosis: a non-zero exit carries the child's own output into the error text, an exit with no
// output is passed through unchanged, a nil run error stays nil, and an over-long output is
// truncated with an explicit marker rather than silently.
//
// The regression this pins: before it, every child-bootstrap failure reached the operator and the
// persisted status.error as the identical bare "exit status 1", because the Spawn seam discarded
// the child's stdout and stderr.
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
