// launch.go holds the two judgments `orchcli.BeamUp` makes: which branch to take, and which prompt to launch a fresh session with.
// Both are pure so the start is assembly over them.

package orchengine

// StartAction is the branch the bring-up of `lym beam start` takes.
type StartAction int

const (
	// StartAlreadyRunning: strand and watcher are both live;
	// start changes nothing.
	StartAlreadyRunning StartAction = iota
	// StartSpawnWatcher: strand is live but no watcher holds it; spawn one.
	StartSpawnWatcher
	// StartRelaunch: strand is dead or absent; launch a session and a watcher for it.
	StartRelaunch
)

// Start-prompt sources, reported on a start's report and log line.
const (
	SourceLastHandoff = "last-handoff"
	SourceFresh       = "fresh"
	// SourceResume names a launch that resumed the session State.SessionID records.
	SourceResume = "resume"
)

// DecideStart maps the strand and watcher liveness pair onto a StartAction.
// A dead or absent strand relaunches whatever the watcher's state, since a relaunched session needs a watcher bound to it anyway.
func DecideStart(strandLive, watcherLive bool) StartAction {
	switch {
	case !strandLive:
		return StartRelaunch
	case watcherLive:
		return StartAlreadyRunning
	default:
		return StartSpawnWatcher
	}
}

// ChooseStartPrompt picks the fresh-launch prompt: the resume stencil pointed at s.LastHandoff when it exists, else the start stencil.
// Only LastHandoff is consulted, never PendingHandoff, so a partial handoff an aborted cycle left is never chosen.
// Every prompt is a one-line pointer at rolePath, the role file the caller has rendered.
func ChooseStartPrompt(stencilsDir, rolePath string, s State, exists func(string) bool) (prompt, source string, err error) {
	if s.LastHandoff != "" && exists(s.LastHandoff) {
		prompt, err = RenderResumePrompt(stencilsDir, rolePath, s.LastHandoff)
		return prompt, SourceLastHandoff, err
	}
	prompt, err = RenderStartPrompt(stencilsDir, rolePath)
	return prompt, SourceFresh, err
}
