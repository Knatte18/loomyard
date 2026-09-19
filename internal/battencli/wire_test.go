// wire_test.go proves no seam wire (wire.go) builds is evaluated at wiring time: building the
// wiring for a slug whose worktree does not exist must succeed, and none of the four seams that
// resolve the managed task worktree's own Location -- the status-path resolver, the spawn
// directory, and both teardown halves -- may be invoked here, since each of the three that actually
// resolves that Location reaches the resolver (lyxcwd.ResolveWorktree), which spawns git; this
// suite stays untagged and Tier 1, so it proves laziness structurally rather than by invoking a
// seam and observing it run.

package battencli

import (
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
