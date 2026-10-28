// paths.go declares the orch module's directory name and the paths every orch file lives at, joined once from a worktree's anchor.

package orchroute

import (
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/orchengine"
)

// DirName is the orch module's own subdirectory name, beneath the anchor's .lym directory and beneath _lym, where the rules files live.
const DirName = "orch"

// RulesRel returns the rules file's path relative to the board dir and to the anchor.
func RulesRel() string {
	return filepath.Join(lymdirs.LymDirName, DirName, "rules.md")
}

// Paths returns every path orch operates on, rooted at location's anchor.
func Paths(location *lymcwd.Location) orchengine.Paths {
	dotLym := filepath.Join(location.AnchorPath(), lymdirs.DotLymDirName)
	dir := filepath.Join(dotLym, DirName)
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
		HubRulesPath:     filepath.Join(fabricengine.BoardDir(location.HubPath), RulesRel()),
		OwnRulesPath:     filepath.Join(location.AnchorPath(), RulesRel()),
		PrimeRoot:        location.WorktreePath(),

		RetiredNoticeLogPath: filepath.Join(dotLym, "notices.log"),
	}
}
