// open.go implements Runner.Open: it opens a named interactive provider session with no prompt in a window of its own, without a run directory, run record or output file.

package shuttleengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/shell"
)

// openMarkerDirName is the directory under the anchor's ephemeral lyx directory that holds the exit markers of opened sessions.
// It sits beside, never inside, the run-directory root, so no run sweep or step stop reads it.
const openMarkerDirName = "shuttle-open"

// OpenResult identifies the strand and session Runner.Open opened.
type OpenResult struct {
	StrandGUID string
	StrandName string
	SessionID  string
	// Engine is the resolved name of the engine the session opened under; Open writes no run record, so this is where it is recorded.
	Engine string
}

// openMarkerPath returns the exit marker an opened session touches when its provider ends, under anchorPath's shuttle-open directory and named by token.
func openMarkerPath(anchorPath, token string) string {
	return filepath.Join(anchorPath, lyxdirs.DotLyxDirName, openMarkerDirName, token+".exited")
}

// Open opens a named interactive session with no prompt in its own window and returns without waiting.
// The strand takes its name from spec.Role, numbered by reed when the role is held.
// Its pane line runs the provider, then writes the exit marker, asks reed to remove the strand and exits the shell, so ending the session closes its window;
// when the self-removal fails the strand stays as a dead pane until `lyx reed remove`, and a resume drops it through the marker instead of relaunching it.
// It writes no run directory, no run record and no output file.
// spec.Engine names the engine, empty for the set's default;
// an unregistered name, a covered pause geometry and an engine without SessionOpener each refuse the open before anything is created.
// Bound: one interactive session per call, in the caller's own worktree under its settings, never a turn; nothing caps the count.
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
			return OpenResult{}, pauserecord.CoveredError(coverage.Entry, r.pause.Slug, "re-run lyx shuttle open")
		}
	}
	opener, ok := engine.(SessionOpener)
	if !ok {
		return OpenResult{}, fmt.Errorf("shuttle: open: the engine %q cannot open a session with no prompt", engineName)
	}
	launch, err := opener.OpenLaunch(spec, r.cfg)
	if err != nil {
		return OpenResult{}, err
	}

	token, err := newRunID()
	if err != nil {
		return OpenResult{}, fmt.Errorf("shuttle: open: mint the exit marker token: %w", err)
	}
	marker := openMarkerPath(r.anchorPath, token)
	if err := os.MkdirAll(filepath.Dir(marker), 0o755); err != nil {
		return OpenResult{}, fmt.Errorf("shuttle: open: create the exit marker directory: %w", err)
	}

	sh := shell.ForGOOS()
	selfRemove := sh.Invoke("lyx") + " reed remove --name " + sh.EnvRef(agentname.StrandNameEnv) + " --detach"
	strand, err := r.reed.AddStrand(reedengine.AddSpec{
		Role:      spec.Role,
		Segment:   string(spec.Segment),
		Provider:  engineName,
		Cmd:       sh.Chain(launch.Cmd, sh.Touch(marker), selfRemove, sh.Exit()),
		ResumeCmd: sh.Chain(launch.ResumeCmd, sh.Touch(marker), selfRemove, sh.Exit()),
		SessionID: launch.SessionID,
		Display:   render.Display{Anchor: render.AnchorOwnWindow},
		DoneWhen:  []string{marker},
	})
	if err != nil {
		return OpenResult{}, fmt.Errorf("shuttle: open: add the strand: %w", err)
	}

	logger.Info("shuttle: opened session", "strandGUID", strand.GUID, "strandName", strand.Name, "role", spec.Role, "sessionID", launch.SessionID, "marker", marker, "engine", engineName)
	return OpenResult{StrandGUID: strand.GUID, StrandName: strand.Name, SessionID: launch.SessionID, Engine: engineName}, nil
}
