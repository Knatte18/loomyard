// panebin.go owns the whole pane-binary seam: composing the shell prelude that resolves `lyx` to the
// binary that spawned every strand pane, and the one function (composePaneLaunchLine) that joins it
// onto a strand's launch command. The composition lives in this one file, reached from the one
// chokepoint (launchStrandLocked in spawn.go), so the property "every strand pane resolves lyx to its
// spawning binary" holds by construction rather than by every strand-realizing call site remembering
// to apply it.

package reedengine

import (
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shell"
)

// lyxBinEnvKey is the env key the prelude exports. It carries the absolute path of the binary
// itself, never its directory, so a script or prompt can invoke it explicitly without re-appending a
// platform-specific "lyx" / "lyx.exe" name.
const lyxBinEnvKey = "LYX_BIN"

// executablePath is a package-local testability seam over os.Executable, modelled on
// tools/sandbox/resolve.go's own devBinPath seam. Under `go test` the live value is the test
// binary's path, so hermetic tests inject a path here instead of asserting against the real one —
// re-exec'ing os.Executable() under go test is itself barred by CONSTRAINTS.md's Live-Substrate
// Spawn Observability clause.
var executablePath = os.Executable

// paneBinPrelude returns the pure, injectable pane-binary composition for exe on sh's dialect: sh's
// own PrependPathEntry of exe's parent directory, then sh's own ExportEnv of lyxBinEnvKey to exe,
// joined by sh.Chain. The prepend is unconditional — no guard against the directory already being on
// the pane's PATH — because every strand pane is a fresh pane with a fresh shell, so nothing
// accumulates across relaunches or resumes, and the one nesting case (a pane whose command itself
// spawns another strand pane) produces a duplicate PATH entry that resolves identically.
func paneBinPrelude(sh shell.Shell, exe string) string {
	return sh.Chain(sh.PrependPathEntry(filepath.Dir(exe)), sh.ExportEnv(lyxBinEnvKey, exe))
}

// composePaneLaunchLine returns the send-keys payload launchStrandLocked sends: the pane-binary
// prelude followed by launchCmd, on sh's dialect. It reads the running process's own path via
// executablePath; on error it logs a named logger.Warn and returns launchCmd unchanged, so the pane
// launches with no prelude rather than failing the strand launch (executable-error-warns-and-degrades
// Shared Decision). Because Chain drops empty parts, an empty launchCmd yields the prelude alone with
// no trailing separator and no empty command fragment.
func composePaneLaunchLine(sh shell.Shell, launchCmd, strandGUID string) string {
	exe, err := executablePath()
	if err != nil {
		logger.Warn("reed: could not resolve this binary, launching strand pane with no lyx-bin prelude", "strand", strandGUID, "err", err)
		return launchCmd
	}
	return sh.Chain(paneBinPrelude(sh, exe), launchCmd)
}
