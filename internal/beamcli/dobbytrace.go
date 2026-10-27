// dobbytrace.go leaves a trace of each exit `lym beam dobby` takes before dobbyengine.Run: a line in the dobby's log and, after a re-exec, a notice.
// It also holds the rotation of the dobby's raw output file.

package beamcli

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/proc"
)

// selfexecs reports whether this process can re-exec onto a replaced binary, which it does off Windows and outside a test binary.
func selfexecs() bool {
	return runtime.GOOS != "windows" && !testing.Testing()
}

// appendDobbyLine appends one stamped line to the dobby log at path without rotating it, since the caller holds no dobby lock.
// A failed append is logged at Debug: the directory of a worktree that was never a beam does not exist, and no append creates it.
func appendDobbyLine(path, text string) {
	if err := dobbyengine.AppendLog(path, time.Now().UTC().Format(time.RFC3339)+" "+text, false); err != nil {
		logger.Debug("beamcli: a dobby log line was not written", "path", path, "error", err)
	}
}

// readsAsReexec reports whether the state record at statePath names this process as the dobby that last recorded itself.
func readsAsReexec(statePath string) bool {
	ticks, _ := proc.StartTime(os.Getpid())
	reexec, err := dobbyengine.ReadsAsReexec(statePath, os.Getpid(), ticks)
	if err != nil {
		logger.Debug("beamcli: the dobby's state record was not read to tell a re-exec", "path", statePath, "error", err)
		return false
	}
	return reexec
}

// postDobbyNotice posts line to the nearest live orch of from, signed as the dobby scoped to slug; a failed or unrouted post is logged.
func postDobbyNotice(notify func(from *lymcwd.Location, slug, program, line string) (bool, error), from *lymcwd.Location, slug, line string) {
	posted, err := notify(from, slug, dobbyProgram, line)
	if err != nil {
		logger.Warn("beamcli: a dobby notice was not posted", "worktree", from.WorktreeName, "error", err)
		return
	}
	if !posted {
		logger.Debug("beamcli: no orch strand is recorded to receive the dobby notice", "worktree", from.WorktreeName)
	}
}

// traceRefusedStart records that the dobby of location was refused, in that worktree's own `dobby.log` when it has a `.lym/beam` directory.
// A refusal of a process that is a re-exec of the dobby there also posts a notice, since the refusal's own way forward would only repeat it.
func traceRefusedStart(location *lymcwd.Location, refusal error, notify func(from *lymcwd.Location, slug, program, line string) (bool, error)) {
	beamDir := hubgeom.DobbyBeamDir(location)
	appendDobbyLine(filepath.Join(beamDir, dobbyengine.LogFileName), "start refused: "+refusal.Error())
	if !readsAsReexec(filepath.Join(beamDir, dobbyengine.StateFileName)) {
		return
	}
	slug := location.WorktreeName
	if primeName, err := fabricengine.PrimeName(location); err == nil && primeName == slug {
		slug = ""
	}
	postDobbyNotice(notify, location, slug, fmt.Sprintf("%s: the dobby could not start after a re-exec: %v; way forward: fix the cause, then run lym beam start", location.WorktreeName, refusal))
}

// traceFailedWiring records that wiring the dobby of geom over the hub failed, in its `dobby.log`.
// A failure in a process that is a re-exec of that dobby also posts a notice; when the worktree no longer resolves, the line is the only trace.
func traceFailedWiring(geom dobbyengine.Geometry, wiring error, notify func(from *lymcwd.Location, slug, program, line string) (bool, error)) {
	appendDobbyLine(geom.LogPath, "start failed: wire the dobby over the hub: "+wiring.Error())
	if !readsAsReexec(geom.StatePath) {
		return
	}
	worktree, err := lymcwd.ResolveWorktree(geom.WorktreeRoot)
	if err != nil {
		logger.Debug("beamcli: the worktree of a failed dobby wiring does not resolve; no notice is posted", "root", geom.WorktreeRoot, "error", err)
		return
	}
	postDobbyNotice(notify, worktree, geom.Slug, fmt.Sprintf("%s: the dobby's wiring failed after a re-exec: %v; way forward: fix the cause, then run lym beam start", geom.WorktreeName, wiring))
}

// rotateOutput renames the dobby's output file to its previous file when it is past its bound, and points this process's output at the fresh file.
// Only the holder of the dobby lock calls it, so a process that loses the lock renames nothing.
func rotateOutput(path string) {
	rotated, err := dobbyengine.RotatePastBound(path)
	if err != nil {
		logger.Warn("beamcli: the dobby output file was not rotated", "path", path, "error", err)
		return
	}
	if !rotated {
		return
	}
	if err := reopenStdio(path); err != nil {
		logger.Warn("beamcli: the dobby's output was not moved onto the fresh output file", "path", path, "error", err)
	}
}
