// hubscratch_test.go covers fabricengine.HubScratchDir and the idempotency of fabricengine.HubLogsDir against it.
// It is an external test package to avoid an import cycle that an in-package file would hit.
// It declares no TestMain: it shares the single TestMain of testmain_test.go.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestHubScratchAndLogsDir covers the hub scratch geometry.
// HubScratchDir(hub) equals filepath.Join(BoardDir(hub), lymdirs.DotLymDirName).
// It is a sibling of StencilsDir(hub)'s `_lym` component, not nested under it.
// The ephemeral `.lym` tree and the durable `_lym` tree sit side by side inside the Bolt checkout.
// Neither lies inside the other.
// HubScratchDir never picks up AnchorRel, because the board's `_lym` and `.lym` trees are flat.
// Its value is byte-identical for a subpath-anchored hub.
// HubLogsDir(hub) equals filepath.Join(HubScratchDir(hub), "logs").
// LogArchiveDir(hub) equals filepath.Join(HubScratchDir(hub), "log-archive").
// It is the archive root beside the logs directory.
//
//testtiming:keep HubScratchDir being board-anchored, a sibling of the stencils _lym tree, ignoring AnchorRel, and HubLogsDir and LogArchiveDir deriving from it; coverage of its blocks by other tests does not show an assertion of this
func TestHubScratchAndLogsDir(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	hub := filepath.Join(string(filepath.Separator), "synthetic", "repo-LYMHUB")

	got := fabricengine.HubScratchDir(hub)
	want := filepath.Join(fabricengine.BoardDir(hub), lymdirs.DotLymDirName)
	if got != want {
		t.Errorf("HubScratchDir(%q) = %q; want %q", hub, got, want)
	}

	stencils := fabricengine.StencilsDir(hub)
	if filepath.Dir(got) != filepath.Dir(filepath.Dir(stencils)) {
		t.Errorf("HubScratchDir(%q) = %q; want a sibling of StencilsDir(%q) = %q's _lym component, not nested under it", hub, got, hub, stencils)
	}

	l := &lymcwd.Location{HubPath: hub, AnchorRel: "backend"}
	if subpathAnchored := fabricengine.HubScratchDir(l.HubPath); subpathAnchored != got {
		t.Errorf("HubScratchDir(%q) = %q; want byte-identical result %q regardless of AnchorRel", hub, subpathAnchored, got)
	}

	gotLogs := fabricengine.HubLogsDir(hub)
	wantLogs := filepath.Join(got, "logs")
	if gotLogs != wantLogs {
		t.Errorf("HubLogsDir(%q) = %q; want %q", hub, gotLogs, wantLogs)
	}

	gotArchive := fabricengine.LogArchiveDir(hub)
	wantArchive := filepath.Join(got, "log-archive")
	if gotArchive != wantArchive {
		t.Errorf("LogArchiveDir(%q) = %q; want %q", hub, gotArchive, wantArchive)
	}

	gotAdopt := fabricengine.AdoptScratchDir(hub)
	wantAdopt := filepath.Join(got, lymdirs.AdoptDirName)
	if gotAdopt != wantAdopt {
		t.Errorf("AdoptScratchDir(%q) = %q; want %q", hub, gotAdopt, wantAdopt)
	}
}

// TestHubLogsDir_MkdirAllIdempotentAgainstFabricCreatedDotLym checks the boot path's idempotent MkdirAll call.
// It must still succeed twice in a row when fabricengine.HubScratchDir(hubPath) exists as a real directory.
// CloneHub produces that shape at the Bolt checkout's .lym, and the boot path makes the same call there.
//
//testtiming:keep the logs directory's MkdirAll succeeding twice over an existing scratch directory; coverage of its blocks by other tests does not show an assertion of this
func TestHubLogsDir_MkdirAllIdempotentAgainstFabricCreatedDotLym(t *testing.T) {
	tiergate.Require(t, 0)
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
