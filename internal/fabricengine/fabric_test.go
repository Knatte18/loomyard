// fabric_test.go — unit tests for the Fabric handle, sync options, and
// ScopedPathspec. No git spawn: New only stat-checks paths.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
)

// TestNew_MissingWarpPath asserts that a missing warp directory errors,
// naming the warp path specifically.
func TestNew_MissingWarpPath(t *testing.T) {
	tmp := t.TempDir()
	weftPath := filepath.Join(tmp, "weft")
	if err := os.Mkdir(weftPath, 0755); err != nil {
		t.Fatalf("mkdir weft: %v", err)
	}
	missingWarpPath := filepath.Join(tmp, "warp")

	_, err := fabricengine.New(missingWarpPath, weftPath)
	if err == nil {
		t.Fatalf("New(%q, %q) error = nil; want error naming %q", missingWarpPath, weftPath, missingWarpPath)
	}
	if !strings.Contains(err.Error(), missingWarpPath) {
		t.Errorf("New() error = %v; want error naming %q", err, missingWarpPath)
	}
}

// TestNew_MissingWeftPath asserts that a missing weft directory errors,
// naming the weft path specifically.
func TestNew_MissingWeftPath(t *testing.T) {
	tmp := t.TempDir()
	warpPath := filepath.Join(tmp, "warp")
	if err := os.Mkdir(warpPath, 0755); err != nil {
		t.Fatalf("mkdir warp: %v", err)
	}
	missingWeftPath := filepath.Join(tmp, "weft")

	_, err := fabricengine.New(warpPath, missingWeftPath)
	if err == nil {
		t.Fatalf("New(%q, %q) error = nil; want error naming %q", warpPath, missingWeftPath, missingWeftPath)
	}
	if !strings.Contains(err.Error(), missingWeftPath) {
		t.Errorf("New() error = %v; want error naming %q", err, missingWeftPath)
	}
}

// TestNew_HappyPath asserts that New yields a non-nil Warp and Weft when both
// paths exist as directories.
func TestNew_HappyPath(t *testing.T) {
	tmp := t.TempDir()
	warpPath := filepath.Join(tmp, "warp")
	weftPath := filepath.Join(tmp, "weft")
	if err := os.Mkdir(warpPath, 0755); err != nil {
		t.Fatalf("mkdir warp: %v", err)
	}
	if err := os.Mkdir(weftPath, 0755); err != nil {
		t.Fatalf("mkdir weft: %v", err)
	}

	f, err := fabricengine.New(warpPath, weftPath)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if f.Warp == nil {
		t.Error("New().Warp = nil; want non-nil")
	}
	if f.Weft == nil {
		t.Error("New().Weft = nil; want non-nil")
	}
}

// TestEnvSyncOptions covers the WEFT_SKIP_GIT / WEFT_SKIP_PUSH mapping:
// unset, "1", and other values.
func TestEnvSyncOptions(t *testing.T) {
	tests := []struct {
		name         string
		skipGitVal   string
		setSkipGit   bool
		skipPushVal  string
		setSkipPush  bool
		wantSkipGit  bool
		wantSkipPush bool
	}{
		{name: "unset", wantSkipGit: false, wantSkipPush: false},
		{name: "both_one", setSkipGit: true, skipGitVal: "1", setSkipPush: true, skipPushVal: "1", wantSkipGit: true, wantSkipPush: true},
		{name: "other_values", setSkipGit: true, skipGitVal: "true", setSkipPush: true, skipPushVal: "yes", wantSkipGit: false, wantSkipPush: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setSkipGit {
				t.Setenv("WEFT_SKIP_GIT", tt.skipGitVal)
			} else {
				t.Setenv("WEFT_SKIP_GIT", "")
			}
			if tt.setSkipPush {
				t.Setenv("WEFT_SKIP_PUSH", tt.skipPushVal)
			} else {
				t.Setenv("WEFT_SKIP_PUSH", "")
			}

			got := fabricengine.EnvSyncOptions()
			if got.SkipGit != tt.wantSkipGit {
				t.Errorf("EnvSyncOptions().SkipGit = %v; want %v", got.SkipGit, tt.wantSkipGit)
			}
			if got.SkipPush != tt.wantSkipPush {
				t.Errorf("EnvSyncOptions().SkipPush = %v; want %v", got.SkipPush, tt.wantSkipPush)
			}
		})
	}
}

// TestScopedPathspec mirrors weftengine's ScopedPathspec cases: root relPath
// (no-op join) and a nested relPath (prefixed join).
func TestScopedPathspec(t *testing.T) {
	tests := []struct {
		name    string
		relPath string
		dirs    []string
		want    []string
	}{
		{"root", ".", []string{"_lyx"}, []string{"_lyx"}},
		{"nested", "sub", []string{"_lyx"}, []string{filepath.Join("sub", "_lyx")}},
		{"nested_multiple_dirs", "sub", []string{"_lyx", "_raddle"}, []string{filepath.Join("sub", "_lyx"), filepath.Join("sub", "_raddle")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fabricengine.ScopedPathspec(tt.relPath, tt.dirs)
			if len(got) != len(tt.want) {
				t.Fatalf("ScopedPathspec(%q, %v) = %v; want %v", tt.relPath, tt.dirs, got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("ScopedPathspec(%q, %v)[%d] = %q; want %q", tt.relPath, tt.dirs, i, got[i], tt.want[i])
				}
			}
		})
	}
}
