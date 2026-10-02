// panebin.go owns the whole pane-binary seam: composing the shell prelude that resolves `lyx` to the
// binary that spawned every strand pane, and the one function (composePaneLaunchLine) that joins it
// onto a strand's launch command. The composition lives in this one file, reached from the one
// chokepoint (launchStrandLocked in spawn.go), so the property "every strand pane resolves lyx to its
// spawning binary" holds by construction rather than by every strand-realizing call site remembering
// to apply it.
// The same composition exports LYX_STRAND_NAME (the strand's full name) and LYX_PARENT (the worktree's parent, when told) beside LYX_BIN.
// The file also owns the per-strand launch script the composed line is written to,
// so the pane types a short source statement instead of the full line.

package reedengine

import (
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/agentname"
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

// nameExports returns the statements exporting the strand's full name and, when parent is non-empty, its worktree's parent, on sh's dialect.
// An empty name exports nothing, so a strand with no formed name never sees an empty LYX_STRAND_NAME.
func nameExports(sh shell.Shell, name, parent string) string {
	var parts []string
	if name != "" {
		parts = append(parts, sh.ExportEnv(agentname.StrandNameEnv, name))
	}
	if parent != "" {
		parts = append(parts, sh.ExportEnv(agentname.ParentEnv, parent))
	}
	return sh.Chain(parts...)
}

// composePaneLaunchLine returns the launch script's content: the pane-binary prelude, the name exports, then launchCmd, on sh's dialect.
// launchStrandLocked writes it through stageLaunchScript and types only the source statement.
// It reads the running process's own path via executablePath;
// on error it logs a named logger.Warn and drops the prelude, so the pane launches with no prelude rather than failing the strand launch (executable-error-warns-and-degrades Shared Decision).
// The name exports do not depend on that lookup and stay either way.
// Because Chain drops empty parts, an empty launchCmd yields the statements alone with no trailing separator and no empty command fragment.
func composePaneLaunchLine(sh shell.Shell, launchCmd, strandGUID, name, parent string) string {
	prelude := ""
	exe, err := executablePath()
	if err != nil {
		logger.Warn("reed: could not resolve this binary, launching strand pane with no lyx-bin prelude", "strand", strandGUID, "err", err)
	} else {
		prelude = paneBinPrelude(sh, exe)
	}
	return sh.Chain(prelude, nameExports(sh, name, parent), launchCmd)
}

// launchScriptReedSegment and launchScriptLaunchSegment are reed's own relative subpath under the state dir for per-strand launch scripts.
const (
	launchScriptReedSegment   = "reed"
	launchScriptLaunchSegment = "launch"
)

// launchScriptDir returns the directory holding per-strand launch scripts under stateDir, the value Engine.stateDir returns.
func launchScriptDir(stateDir string) string {
	return filepath.Join(stateDir, launchScriptReedSegment, launchScriptLaunchSegment)
}

// launchScriptPath returns the launch script path for strandGUID on sh's dialect.
func launchScriptPath(sh shell.Shell, stateDir, strandGUID string) string {
	return filepath.Join(launchScriptDir(stateDir), strandGUID+sh.ScriptExt())
}

// stageLaunchScript writes composedLine plus a trailing newline to the strand's launch script and returns sh's source statement for it, the send-keys payload.
// On any write error it logs a named logger.Warn and returns composedLine unchanged,
// so a cosmetic feature never fails a strand launch.
func stageLaunchScript(sh shell.Shell, stateDir, strandGUID, composedLine string) string {
	path := launchScriptPath(sh, stateDir, strandGUID)
	if err := writeLaunchScript(path, composedLine+"\n"); err != nil {
		logger.Warn("reed: could not write launch script, sending the full launch line", "strand", strandGUID, "path", path, "err", err)
		return composedLine
	}
	return sh.Source(path)
}

// removeLaunchScripts deletes the launch script of every GUID in guids.
// Deletion is best-effort: a missing file is silent,
// and any other error is a logger.Warn, never returned, so a cosmetic file never fails the removal that triggered it.
func removeLaunchScripts(sh shell.Shell, stateDir string, guids []string) {
	for _, guid := range guids {
		path := launchScriptPath(sh, stateDir, guid)
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			logger.Warn("reed: could not delete launch script", "strand", guid, "path", path, "err", err)
		}
	}
}

// writeLaunchScript atomically writes content to path with mode 0o644.
// The file is sourced, never executed, so it carries no exec bit.
// fsx.AtomicWriteBytes is not reused because its temp file keeps mode 0o600.
func writeLaunchScript(path, content string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	renamed := false
	defer func() {
		if !renamed {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.WriteString(content); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	renamed = true
	return nil
}
