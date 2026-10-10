// paths.go declares the orch module's one scratch accessor: every file orch writes lives under the prime's `<anchor>/.lyx/orch/`, joined here once and handed to orchengine as told paths (Durable-vs-Ephemeral State, Lyxdirs Single-Declarer and Told-Geometry Invariants).

package orchcli

import (
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/orchengine"
)

// orchDirName is the module's own subdirectory beneath the anchor's .lyx directory.
const orchDirName = "orch"

// PrimePaths returns the orch paths for the prime at location, so another module's CLI wiring reaches the notice queue and the orch state without re-deriving either.
func PrimePaths(location *lyxcwd.Location) orchengine.Paths {
	return orchPaths(location)
}

// orchPaths returns every path orch operates on, rooted at location's anchor.
func orchPaths(location *lyxcwd.Location) orchengine.Paths {
	dir := filepath.Join(location.AnchorPath(), lyxdirs.DotLyxDirName, orchDirName)
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
	}
}
