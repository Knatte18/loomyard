// spawn_test.go holds Tier-1 tests: untagged, with no git spawn and no process fork.
// They cover SpawnDetachedPush's skip-env and empty-path gating.
// The real fork path is not exercised here: forking from the test binary would re-exec the test binary itself.

package fabricengine

import (
	"errors"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestSpawnDetachedPush_SkipEnvAndEmptyPaths covers the cases where SpawnDetachedPush returns nil without forking.
// The cases are FABRIC_SKIP_GIT set, FABRIC_SKIP_PUSH set, and both paths empty with neither env var set.
// With both paths set and no skip env, it returns ErrSpawnUnderTest instead of forking the test binary.
func TestSpawnDetachedPush_SkipEnvAndEmptyPaths(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name    string
		envKey  string
		req     fabriclayout.PushSpawn
		wantErr error
	}{
		{name: "SkipGit", envKey: "FABRIC_SKIP_GIT", req: fabriclayout.PushSpawn{WarpPath: "/warp", WeftPath: "/weft"}},
		{name: "SkipPush", envKey: "FABRIC_SKIP_PUSH", req: fabriclayout.PushSpawn{WarpPath: "/warp", WeftPath: "/weft"}},
		{name: "BothPathsEmpty", req: fabriclayout.PushSpawn{}},
		{name: "UnderTest", req: fabriclayout.PushSpawn{WarpPath: "/warp", WeftPath: "/weft"}, wantErr: ErrSpawnUnderTest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envKey != "" {
				t.Setenv(tt.envKey, "1")
			}
			if err := SpawnDetachedPush(tt.req); !errors.Is(err, tt.wantErr) {
				t.Errorf("SpawnDetachedPush(%+v) = %v; want %v", tt.req, err, tt.wantErr)
			}
		})
	}
}

// TestPushSpawnCommand covers the pusher command line, working directory and owner of a pair and of the prime.
func TestPushSpawnCommand(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	hub := t.TempDir()
	code := filepath.Join(hub, "task")
	records := filepath.Join(hub, "task-weft")
	prime := filepath.Join(hub, "prime")
	primeRecords := filepath.Join(hub, "prime-weft")
	board := filepath.Join(hub, fabriclayout.BoltDirName)

	tests := []struct {
		name     string
		req      fabriclayout.PushSpawn
		wantArgs []string
	}{
		{
			"APairNamesItsCodeWorktreeAndSlug",
			fabriclayout.PushSpawn{WarpPath: code, WeftPath: records, WorkDir: board},
			[]string{"loom", "pusher", "--worktree", code, "--slug", "task"},
		},
		{
			"ThePrimeNamesItsCodeWorktreeAndNoSlug",
			fabriclayout.PushSpawn{WeftPath: primeRecords, WorkDir: board},
			[]string{"loom", "pusher", "--worktree", prime},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			args, dir, owner := pushSpawnCommand(tt.req)
			if !slices.Equal(args, tt.wantArgs) {
				t.Errorf("pushSpawnCommand(%+v) args = %q; want %q", tt.req, args, tt.wantArgs)
			}
			if dir != board {
				t.Errorf("pushSpawnCommand(%+v) dir = %q; want %q", tt.req, dir, board)
			}
			if owner != runproc.HubOwner {
				t.Errorf("pushSpawnCommand(%+v) owner = %q; want %q", tt.req, owner, runproc.HubOwner)
			}
		})
	}
}
