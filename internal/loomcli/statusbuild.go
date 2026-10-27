package loomcli

import (
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
)

// statusSidecarFile is the ephemeral sidecar's filename, joined onto loomengine.LoomScratchDir so the
// file lives under .lym and never in tracked content.
const statusSidecarFile = "status-strand.json"

// statusSidecarPath is the sidecar's path under the worktree's loom scratch directory.
func statusSidecarPath(l *lymcwd.Location) string {
	return filepath.Join(loomengine.LoomScratchDir(l), statusSidecarFile)
}
