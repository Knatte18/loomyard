// open.go implements Runner.Open: it opens a named interactive provider session with no prompt in a window of its own.
// It writes no run directory, run record or output file.

package shuttleengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/shell"
)

// openMarkerDirName is the directory under the anchor's ephemeral lym directory that holds the exit markers of opened sessions.
// It sits beside, never inside, the run-directory root, so no run sweep or step stop reads it.
const openMarkerDirName = "shuttle-open"

// OpenResult identifies the strand and session of an interactive session opened with no prompt.
type OpenResult struct {
	StrandGUID string
	StrandName string
	SessionID  string
	// Engine is the resolved name of the engine the session opened under.
	// No run record is written, so this field is where the engine is recorded.
	Engine string
}

// openMarkerPath returns the exit marker an opened session touches when its provider ends.
// The marker lies under anchorPath's shuttle-open directory and is named by token.
func openMarkerPath(anchorPath, token string) string {
	return filepath.Join(anchorPath, lymdirs.DotLymDirName, openMarkerDirName, token+".exited")
}

// openSessionDir returns the directory an opened session's engine may write its launch files into.
// It sits beside the exit marker and is named by the same token.
func openSessionDir(anchorPath, token string) string {
	return filepath.Join(anchorPath, lymdirs.DotLymDirName, openMarkerDirName, token)
}

// Open opens a named interactive session with no prompt in its own window and returns without waiting.
// The strand takes its name from spec.Role, numbered by reed when the role is held.
// It carries the launch's CreelHooks mark.
// Its pane line runs the provider, writes the exit marker and asks reed to remove the strand, then exits the shell.
// Ending the session therefore closes its window.
// When the self-removal fails, the strand stays as a dead pane until `lym reed remove`;
// a resume drops it through the marker instead of relaunching it.
// It writes no run directory, run record or output file;
// the engine may write its launch files into the session's own directory beside the marker.
// spec.Engine names the engine; empty selects the set's default.
// An open refuses before creating anything if the engine name is unregistered or the pause geometry is covered.
// It also refuses an engine without SessionOpener, and a non-empty spec.Skills for an engine without SkillLoader.
// Each call opens one interactive session in the caller's own worktree under its settings.
// The session takes no turn but the load of spec.Skills, and nothing caps how many calls open.
func (r *Runner) Open(spec OpenSpec) (OpenResult, error) {
	if r.toldErr != nil {
		return OpenResult{}, r.toldErr
	}
	if spec.Role == "" {
		return OpenResult{}, errors.New("shuttle: open: the role must not be empty")
	}
	engine, engineName, err := r.engineFor(RunState{Engine: spec.Engine})
	if err != nil {
		return OpenResult{}, err
	}
	if r.pause.RecordPath != "" {
		coverage, err := pauserecord.ReadCoverage(r.pause.RecordPath, r.pause.Slug, r.clock.Now())
		if err != nil {
			logger.Warn("shuttle: opening despite an unreadable pause record", "path", r.pause.RecordPath, "cause", err)
		} else if coverage.Covered {
			return OpenResult{}, pauserecord.CoveredError(coverage.Entry, r.pause.Slug, "re-run lym shuttle open")
		}
	}
	opener, ok := engine.(SessionOpener)
	if !ok {
		return OpenResult{}, fmt.Errorf("shuttle: open: the engine %q cannot open a session with no prompt", engineName)
	}
	if len(spec.Skills) > 0 {
		if _, err := r.skillLoader(engine); err != nil {
			return OpenResult{}, fmt.Errorf("shuttle: open: the spec names skills %v: %w", spec.Skills, err)
		}
	}
	token, err := newRunID()
	if err != nil {
		return OpenResult{}, fmt.Errorf("shuttle: open: mint the exit marker token: %w", err)
	}
	sessionDir := openSessionDir(r.anchorPath, token)
	launch, err := opener.OpenLaunch(sessionDir, spec, r.cfg)
	if err != nil {
		return OpenResult{}, err
	}

	marker := openMarkerPath(r.anchorPath, token)
	if err := os.MkdirAll(filepath.Dir(marker), 0o755); err != nil {
		return OpenResult{}, fmt.Errorf("shuttle: open: create the exit marker directory: %w", err)
	}

	sh := shell.ForGOOS()
	selfRemove := sh.Invoke("lym") + " reed remove --name " + sh.EnvRef(agentname.StrandNameEnv) + " --detach"
	strand, err := r.reed.AddStrand(reedengine.AddSpec{
		Role:       spec.Role,
		Segment:    string(spec.Segment),
		Provider:   engineName,
		CreelHooks: launch.CreelHooks,
		Cmd:        sh.Chain(launch.Cmd, sh.Touch(marker), selfRemove, sh.Exit()),
		ResumeCmd:  sh.Chain(launch.ResumeCmd, sh.Touch(marker), selfRemove, sh.Exit()),
		SessionID:  launch.SessionID,
		Display:    render.Display{Anchor: render.AnchorOwnWindow},
		DoneWhen:   []string{marker},
	})
	if err != nil {
		_ = os.RemoveAll(sessionDir)
		return OpenResult{}, fmt.Errorf("shuttle: open: add the strand: %w", err)
	}

	logger.Info("shuttle: opened session", "strandGUID", strand.GUID, "strandName", strand.Name, "role", spec.Role, "sessionID", launch.SessionID, "marker", marker, "engine", engineName, "permissionMode", spec.PermissionMode, "skills", spec.Skills, "creelHooks", launch.CreelHooks)
	return OpenResult{StrandGUID: strand.GUID, StrandName: strand.Name, SessionID: launch.SessionID, Engine: engineName}, nil
}
