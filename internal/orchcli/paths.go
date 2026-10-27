// paths.go declares the orch module's one scratch accessor: every file orch writes lives under the prime's `<anchor>/.lym/orch/`, joined here once and handed to orchengine as told paths (Durable-vs-Ephemeral State, Lymdirs Single-Declarer and Told-Geometry Invariants).
// The two rules files are durable `_lym/orch/` files: the hub-wide one under the board dir and the own one under the anchor.
// PrimeRoot is the worktree root a relative note path resolves against.

package orchcli

import (
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/orchengine"
)

// orchDirName is the module's own subdirectory name, beneath the anchor's .lym directory and beneath _lym, where the rules files live.
const orchDirName = "orch"

// PrimePaths returns the orch paths for the prime at location, so another module's CLI wiring reaches the orch state without re-deriving it.
func PrimePaths(location *lymcwd.Location) orchengine.Paths {
	return orchPaths(location)
}

// rulesRel returns the rules file's path relative to the board dir and to the anchor.
func rulesRel() string {
	return filepath.Join(lymdirs.LymDirName, orchDirName, "rules.md")
}

// orchPaths returns every path orch operates on, rooted at location's anchor.
func orchPaths(location *lymcwd.Location) orchengine.Paths {
	dotLym := filepath.Join(location.AnchorPath(), lymdirs.DotLymDirName)
	dir := filepath.Join(dotLym, orchDirName)
	return orchengine.Paths{
		Dir:              dir,
		StatePath:        filepath.Join(dir, "state.json"),
		StateLockPath:    filepath.Join(dir, "state.json.lock"),
		WatchLockPath:    filepath.Join(dir, "watch.lock"),
		StartLockPath:    filepath.Join(dir, "start.lock"),
		CycleRequestPath: filepath.Join(dir, "cycle-request"),
		ResumeMarkPath:   filepath.Join(dir, "resume-mark"),
		HandoffsDir:      filepath.Join(dir, "handoffs"),
		NoticesDir:       filepath.Join(dir, "notices"),
		WatchLogPath:     filepath.Join(dir, "watch.log"),
		RolePath:         filepath.Join(dir, "role.md"),
		NoteTemplatePath: filepath.Join(dir, "note-template.md"),
		HubRulesPath:     filepath.Join(fabricengine.BoardDir(location.HubPath), rulesRel()),
		OwnRulesPath:     filepath.Join(location.AnchorPath(), rulesRel()),
		PrimeRoot:        location.WorktreePath(),

		RetiredNoticeLogPath: filepath.Join(dotLym, "notices.log"),
	}
}
