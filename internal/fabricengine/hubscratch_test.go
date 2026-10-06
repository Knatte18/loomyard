// hubscratch_test.go covers fabricengine.HubScratchDir and fabricengine.HubLogsDir's own
// idempotency against it.
// It is an external test package because clone_test.go's own header comment documents the same
// import-cycle constraint from the in-package side; keeping this file as package fabricengine_test
// rather than folding it back into an in-package file is deliberate and out of scope to change.
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
)

// TestHubScratchAndLogsDir covers the hub scratch geometry:
// HubScratchDir(hub) equals filepath.Join(BoardDir(hub), lyxdirs.DotLyxDirName) and is a sibling of
// StencilsDir(hub)'s `_lyx` component rather than nested under it — the ephemeral `.lyx` tree and
// the durable `_lyx` tree sit side by side inside `_board`, never one inside the other;
// HubScratchDir's value is byte-identical for a subpath-anchored hub, because it takes a bare hub
// string and must never pick up AnchorRel, the board's `_lyx`/`.lyx` trees being flat;
// and HubLogsDir(hub) equals filepath.Join(HubScratchDir(hub), "logs") — the derivation HubLogsDir's
// own doc comment states.
//
//testtiming:keep HubScratchDir being board-anchored, a sibling of the stencils _lyx tree, ignoring AnchorRel, and HubLogsDir deriving from it; coverage of its blocks by other tests does not show an assertion of this
func TestHubScratchAndLogsDir(t *testing.T) {
	t.Parallel()

	hub := filepath.Join(string(filepath.Separator), "synthetic", "repo-LYXHUB")

	got := fabricengine.HubScratchDir(hub)
	want := filepath.Join(fabricengine.BoardDir(hub), lyxdirs.DotLyxDirName)
	if got != want {
		t.Errorf("HubScratchDir(%q) = %q; want %q", hub, got, want)
	}

	stencils := fabricengine.StencilsDir(hub)
	if filepath.Dir(got) != filepath.Dir(filepath.Dir(stencils)) {
		t.Errorf("HubScratchDir(%q) = %q; want a sibling of StencilsDir(%q) = %q's _lyx component, not nested under it", hub, got, hub, stencils)
	}

	l := &lyxcwd.Location{HubPath: hub, AnchorRel: "backend"}
	if subpathAnchored := fabricengine.HubScratchDir(l.HubPath); subpathAnchored != got {
		t.Errorf("HubScratchDir(%q) = %q; want byte-identical result %q regardless of AnchorRel", hub, subpathAnchored, got)
	}

	gotLogs := fabricengine.HubLogsDir(hub)
	wantLogs := filepath.Join(got, "logs")
	if gotLogs != wantLogs {
		t.Errorf("HubLogsDir(%q) = %q; want %q", hub, gotLogs, wantLogs)
	}
}

// TestHubLogsDir_MkdirAllIdempotentAgainstFabricCreatedDotLyx asserts the second half of the
// hub-scratch-move batch's coverage: reed's boot path's idempotent
// os.MkdirAll(fabricengine.HubLogsDir(...)) call still succeeds, twice in a row, when
// fabricengine.HubScratchDir(hubPath) already exists as a real directory — the shape CloneHub now
// produces at <hub>/_board/.lyx.
// Moved verbatim from clone_test.go except that the pre-created directory is
// fabricengine.HubScratchDir(hubPath) instead of the old bare <hub>/.lyx join.
//
//testtiming:keep the logs directory's MkdirAll succeeding twice over an existing scratch directory; coverage of its blocks by other tests does not show an assertion of this
func TestHubLogsDir_MkdirAllIdempotentAgainstFabricCreatedDotLyx(t *testing.T) {
	hubPath := t.TempDir()
	if err := os.MkdirAll(fabricengine.HubScratchDir(hubPath), 0o755); err != nil {
		t.Fatalf("mkdir HubScratchDir(%s): %v", hubPath, err)
	}

	logsDir := fabricengine.HubLogsDir(hubPath)
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		t.Fatalf("first MkdirAll(%s) = %v; want nil", logsDir, err)
	}
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		t.Fatalf("second MkdirAll(%s) = %v; want nil (idempotent)", logsDir, err)
	}
}
