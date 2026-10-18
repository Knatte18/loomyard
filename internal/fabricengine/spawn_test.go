// spawn_test.go — Tier-1 (untagged, no git spawn, no process fork) tests for SpawnDetachedPush's
// skip-env/both-paths-empty gating and PushWarpAt's SkipGit/SkipPush gating.
// The real fork path is intentionally not exercised here — forking from the test binary would
// re-exec the test binary itself;
// the real both-sides push is covered by the batch's integration test
// (internal/fabriccli/pushbypass_integration_test.go).

package fabricengine

import (
	"errors"
	"path/filepath"
	"slices"
	"testing"
)

// TestSpawnDetachedPush_SkipEnvAndEmptyPaths covers every case where SpawnDetachedPush must return
// nil without forking a child: FABRIC_SKIP_GIT set, FABRIC_SKIP_PUSH set, and both paths empty (with
// neither env var set).
// With both paths set and no skip env it returns ErrSpawnUnderTest instead of forking the test binary.
func TestSpawnDetachedPush_SkipEnvAndEmptyPaths(t *testing.T) {
	tests := []struct {
		name    string
		envKey  string
		req     PushSpawn
		wantErr error
	}{
		{name: "SkipGit", envKey: "FABRIC_SKIP_GIT", req: PushSpawn{WarpPath: "/warp", WeftPath: "/weft"}},
		{name: "SkipPush", envKey: "FABRIC_SKIP_PUSH", req: PushSpawn{WarpPath: "/warp", WeftPath: "/weft"}},
		{name: "BothPathsEmpty", req: PushSpawn{}},
		{name: "UnderTest", req: PushSpawn{WarpPath: "/warp", WeftPath: "/weft"}, wantErr: ErrSpawnUnderTest},
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

// TestPushWarpAt_Gating covers PushWarpAt's SkipGit/SkipPush early-out, which returns nil without
// ever constructing a real gitrepo.Repo push — no git spawn, since PushCoalesced is never reached.
func TestPushWarpAt_Gating(t *testing.T) {
	tests := []struct {
		name string
		opts SyncOptions
	}{
		{name: "SkipGit", opts: SyncOptions{SkipGit: true}},
		{name: "SkipPush", opts: SyncOptions{SkipPush: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := PushWarpAt("/nonexistent-warp-path", tt.opts); err != nil {
				t.Errorf("PushWarpAt(%+v) = %v; want nil", tt.opts, err)
			}
		})
	}
}

// TestPushSpawnCommand covers the child's arguments and working directory: a flag is present only when its field is set, except the pause flag, which is always present.
func TestPushSpawnCommand(t *testing.T) {
	t.Parallel()

	warp := filepath.Join(t.TempDir(), "warp")
	weft := filepath.Join(t.TempDir(), "weft")
	board := filepath.Join(t.TempDir(), "_board")

	tests := []struct {
		name     string
		req      PushSpawn
		wantArgs []string
	}{
		{
			"EveryFieldSet",
			PushSpawn{WarpPath: warp, WeftPath: weft, PauseSlug: "task", TeardownSlug: "task", WorkDir: board},
			[]string{"fabric", "--warp-path", warp, "--weft-path", weft, "--teardown-slug", "task", "--pause-slug=task", "push"},
		},
		{
			"EmptyWarpPathOmitsItsFlagOnly",
			PushSpawn{WeftPath: weft, PauseSlug: "task", TeardownSlug: "task", WorkDir: board},
			[]string{"fabric", "--weft-path", weft, "--teardown-slug", "task", "--pause-slug=task", "push"},
		},
		{
			"EmptyTeardownSlugOmitsItsFlagOnly",
			PushSpawn{WeftPath: weft, PauseSlug: "gate", WorkDir: board},
			[]string{"fabric", "--weft-path", weft, "--pause-slug=gate", "push"},
		},
		{
			"EmptyPauseSlugIsStillWritten",
			PushSpawn{WeftPath: weft, WorkDir: board},
			[]string{"fabric", "--weft-path", weft, "--pause-slug=", "push"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			args, dir := pushSpawnCommand(tt.req)
			if !slices.Equal(args, tt.wantArgs) {
				t.Errorf("pushSpawnCommand(%+v) args = %q; want %q", tt.req, args, tt.wantArgs)
			}
			if dir != board {
				t.Errorf("pushSpawnCommand(%+v) dir = %q; want %q", tt.req, dir, board)
			}
		})
	}
}
