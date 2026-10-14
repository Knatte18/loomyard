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
	"testing"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
)

// TestSpawnDetachedPush_SkipEnvAndEmptyPaths covers every case where SpawnDetachedPush must return
// nil without forking a child: FABRIC_SKIP_GIT set, FABRIC_SKIP_PUSH set, and both paths empty (with
// neither env var set).
func TestSpawnDetachedPush_SkipEnvAndEmptyPaths(t *testing.T) {
	tests := []struct {
		name     string
		envKey   string
		warpPath string
		weftPath string
	}{
		{name: "SkipGit", envKey: "FABRIC_SKIP_GIT", warpPath: "/warp", weftPath: "/weft"},
		{name: "SkipPush", envKey: "FABRIC_SKIP_PUSH", warpPath: "/warp", weftPath: "/weft"},
		{name: "BothPathsEmpty", warpPath: "", weftPath: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envKey != "" {
				t.Setenv(tt.envKey, "1")
			}
			if err := SpawnDetachedPush(tt.warpPath, tt.weftPath); err != nil {
				t.Errorf("SpawnDetachedPush(%q, %q) = %v; want nil", tt.warpPath, tt.weftPath, err)
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

// TestSpawnPairPush_HandsTheDetachedPushBothSidesOfThePair asserts SpawnPairPush resolves the code worktree and its paired records worktree from the location alone, and returns the spawn's error.
// Serial: the test assigns the package-level spawnDetachedPushFn.
func TestSpawnPairPush_HandsTheDetachedPushBothSidesOfThePair(t *testing.T) {
	l := &lyxcwd.Location{HubPath: filepath.Join(t.TempDir(), "hub-LYXHUB"), WorktreeName: "task", AnchorRel: "."}
	spawnErr := errors.New("spawn failed")

	original := spawnDetachedPushFn
	t.Cleanup(func() { spawnDetachedPushFn = original })
	var gotWarpPath, gotWeftPath string
	spawnDetachedPushFn = func(warpPath, weftPath string) error {
		gotWarpPath, gotWeftPath = warpPath, weftPath
		return spawnErr
	}

	if err := SpawnPairPush(l); !errors.Is(err, spawnErr) {
		t.Errorf("SpawnPairPush() = %v; want %v", err, spawnErr)
	}
	if gotWarpPath != l.WorktreePath() {
		t.Errorf("warp path = %q; want %q", gotWarpPath, l.WorktreePath())
	}
	if want := RecordsWorktree(l); gotWeftPath != want {
		t.Errorf("weft path = %q; want %q", gotWeftPath, want)
	}
}
