//go:build !windows

// sandboxclear_integration_test.go drives the real spawn of `lym harden sandbox clear` that a teardown makes.
// It runs over stand-in binaries that exit as the verb would.

package pairteardown

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestClearWorktreeSandbox(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	tests := []struct {
		name string
		// binary is the stand-in for lym: it runs with `harden sandbox clear` as its arguments, in the task worktree.
		binary      string
		withRoot    bool
		wantCleared bool
		wantErr     []string
	}{
		{name: "WorktreeWithNoRootStartsNothing", binary: "false"},
		{name: "RootClearedWhenTheVerbExitsZero", binary: "true", withRoot: true, wantCleared: true},
		{name: "FailingVerbReportsItsExitAndItsOutput", binary: "ls", withRoot: true, wantErr: []string{"exit status", "harden"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			prime := &lymcwd.Location{HubPath: t.TempDir()}
			const slug = "pair"
			worktree := fabricengine.WorktreePath(prime, slug)
			if err := os.MkdirAll(worktree, 0o755); err != nil {
				t.Fatal(err)
			}
			if tt.withRoot {
				if err := os.MkdirAll(filepath.Join(worktree, lymdirs.DotLymDirName, lymdirs.HardenSandboxDirName), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			teardown := &Teardown{prime: prime, lymBinary: func() (string, error) { return exec.LookPath(tt.binary) }}

			cleared, err := teardown.clearWorktreeSandbox(slug)

			if len(tt.wantErr) == 0 && err != nil {
				t.Fatalf("clearWorktreeSandbox() error = %v; want nil", err)
			}
			for _, want := range tt.wantErr {
				if err == nil || !strings.Contains(err.Error(), want) {
					t.Errorf("clearWorktreeSandbox() error = %v; want it to hold %q", err, want)
				}
			}
			if cleared != tt.wantCleared {
				t.Errorf("clearWorktreeSandbox() cleared = %v; want %v", cleared, tt.wantCleared)
			}
		})
	}
}
