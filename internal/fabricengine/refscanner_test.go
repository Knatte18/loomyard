// refscanner_test.go covers RefScanner.Matches against the three cases websterengine's
// audit_test.go exercises today for its own weftReferencePattern (a command containing the sibling
// worktree path, a command containing a -weft sibling name, and a lyx fabric/weft/warp invocation),
// plus a clean command that must not match. The *lyxcwd.Location is built synthetically, the way
// websterengine/audit_test.go's fakeLayout helper does, so no git fixture is needed.

package fabricengine_test

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// fakeLayout returns a lyxcwd.Location that resolves fabricengine.WeftWorktree() without spawning
// git, the same shape websterengine/audit_test.go's own fakeLayout builds.
func fakeLayout() *lyxcwd.Location {
	return &lyxcwd.Location{HubPath: "/hub", WorktreeName: filepath.Base("/hub/master-builder")}
}

// TestRefScanner_Matches covers the behavioural contract that must not regress when the regex moves
// packages: the sibling worktree path, a -weft sibling name, a lyx fabric/weft/warp invocation, and
// a clean command that must not match.
func TestRefScanner_Matches(t *testing.T) {
	layout := fakeLayout()
	scanner := fabricengine.NewRefScanner(layout)
	weftWorktree := fabricengine.WeftWorktree(layout)

	tests := []struct {
		name string
		cmd  string
		want bool
	}{
		{"command containing the sibling worktree path", "git -C " + weftWorktree + " add -A", true},
		{"command containing a -weft sibling name", "cd /hub/master-builder-weft && git status", true},
		{"lyx fabric invocation", "lyx fabric sync", true},
		{"lyx weft invocation", "lyx weft sync", true},
		{"lyx warp invocation", "lyx warp checkout feature", true},
		{"clean command does not match", "git commit -am wip", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := scanner.Matches(tt.cmd); got != tt.want {
				t.Errorf("Matches(%q) = %v; want %v", tt.cmd, got, tt.want)
			}
		})
	}
}
