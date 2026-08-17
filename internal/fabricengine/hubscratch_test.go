// hubscratch_test.go covers fabricengine.HubScratchDir and reed's own log-dir idempotency against it.
// It is an external test package because it imports both fabricengine and reedengine, which an
// in-package (package fabricengine) file cannot do without closing the fabricengine[test] ->
// reedengine -> fabricengine cycle — clone_test.go's own header comment documents the same
// constraint from the in-package side.
// It adds no TestMain: the package shares the single one testmain_test.go declares for the whole
// fabricengine test binary.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/reedengine"
)

// TestHubScratchDir_IsBoardAnchored asserts that fabricengine.HubScratchDir(hub) equals
// filepath.Join(fabricengine.BoardDir(hub), lyxdirs.DotLyxDirName), and that it is a sibling of
// fabricengine.StencilsDir(hub)'s `_lyx` component rather than nested under it — the ephemeral
// `.lyx` tree and the durable `_lyx` tree sit side by side inside `_board`, never one inside the
// other.
func TestHubScratchDir_IsBoardAnchored(t *testing.T) {
	hub := filepath.Join(string(filepath.Separator), "synthetic", "repo-HUB")

	got := fabricengine.HubScratchDir(hub)
	want := filepath.Join(fabricengine.BoardDir(hub), lyxdirs.DotLyxDirName)
	if got != want {
		t.Errorf("HubScratchDir(%q) = %q; want %q", hub, got, want)
	}

	stencils := fabricengine.StencilsDir(hub)
	if filepath.Dir(got) != filepath.Dir(filepath.Dir(stencils)) {
		t.Errorf("HubScratchDir(%q) = %q; want a sibling of StencilsDir(%q) = %q's _lyx component, not nested under it", hub, got, hub, stencils)
	}
}

// TestHubScratchDir_IgnoresAnchorRel asserts that HubScratchDir's value is byte-identical for a
// subpath-anchored hub: HubScratchDir takes a bare hub string and must never pick up AnchorRel,
// because the board's `_lyx`/`.lyx` trees are flat.
func TestHubScratchDir_IgnoresAnchorRel(t *testing.T) {
	hub := filepath.Join(string(filepath.Separator), "synthetic", "repo-HUB")

	unanchored := fabricengine.HubScratchDir(hub)
	l := &lyxcwd.Location{HubPath: hub, AnchorRel: "backend"}
	subpathAnchored := fabricengine.HubScratchDir(l.HubPath)

	if unanchored != subpathAnchored {
		t.Errorf("HubScratchDir(%q) = %q; want byte-identical result %q regardless of AnchorRel", hub, subpathAnchored, unanchored)
	}
}

// TestHubLogsDir_IsHubScratchDirLogsSubdir asserts that fabricengine.HubLogsDir(hub) equals
// filepath.Join(fabricengine.HubScratchDir(hub), "logs") for a synthetic hub path — the derivation
// HubLogsDir's own doc comment states.
func TestHubLogsDir_IsHubScratchDirLogsSubdir(t *testing.T) {
	hub := filepath.Join(string(filepath.Separator), "synthetic", "repo-HUB")

	got := fabricengine.HubLogsDir(hub)
	want := filepath.Join(fabricengine.HubScratchDir(hub), "logs")
	if got != want {
		t.Errorf("HubLogsDir(%q) = %q; want %q", hub, got, want)
	}
}

// TestReedHubLogsDir_MkdirAllIdempotentAgainstFabricCreatedDotLyx asserts the second half of the
// hub-scratch-move batch's coverage: reedengine's own idempotent os.MkdirAll(HubLogsDir(...))
// boot-path call still succeeds, twice in a row, when fabricengine.HubScratchDir(hubPath) already
// exists as a real directory — the shape CloneHub now produces at <hub>/_board/.lyx.
// Moved verbatim from clone_test.go except that the pre-created directory is
// fabricengine.HubScratchDir(hubPath) instead of the old bare <hub>/.lyx join.
func TestReedHubLogsDir_MkdirAllIdempotentAgainstFabricCreatedDotLyx(t *testing.T) {
	hubPath := t.TempDir()
	if err := os.MkdirAll(fabricengine.HubScratchDir(hubPath), 0o755); err != nil {
		t.Fatalf("mkdir HubScratchDir(%s): %v", hubPath, err)
	}

	l := &lyxcwd.Location{HubPath: hubPath}
	logsDir := reedengine.HubLogsDir(l)
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		t.Fatalf("first MkdirAll(%s) = %v; want nil", logsDir, err)
	}
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		t.Fatalf("second MkdirAll(%s) = %v; want nil (idempotent)", logsDir, err)
	}
}
