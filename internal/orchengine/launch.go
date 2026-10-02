// launch.go holds the two judgments `lyx orch start` makes: which branch to take, and which prompt to launch the session with.
// Both are pure so the verb is assembly over them.

package orchengine

import "fmt"

// StartAction is the branch `start` takes.
type StartAction int

const (
	// StartAttachOnly: strand and watcher are both live; only hand the terminal over.
	StartAttachOnly StartAction = iota
	// StartSpawnWatcher: strand is live but no watcher holds it; spawn one.
	StartSpawnWatcher
	// StartRelaunch: strand is dead or absent; launch a fresh session and a watcher for it.
	StartRelaunch
)

// Start-prompt sources, reported on start's envelope and log line.
const (
	SourceFlag        = "flag"
	SourceLastHandoff = "last-handoff"
	SourceFresh       = "fresh"
	SourceAdopt       = "adopt"
)

// DecideStart maps the strand and watcher liveness pair onto a StartAction.
// A dead or absent strand relaunches whatever the watcher's state, since a relaunched session needs a watcher bound to it anyway.
func DecideStart(strandLive, watcherLive bool) StartAction {
	switch {
	case !strandLive:
		return StartRelaunch
	case watcherLive:
		return StartAttachOnly
	default:
		return StartSpawnWatcher
	}
}

// ChooseStartPrompt picks the launch prompt: the resume stencil pointed at handoffFlag, else at s.LastHandoff when it exists, else the start stencil.
// A handoffFlag naming a missing file is an error, since a silent fallback would resume from the wrong context.
// Only LastHandoff is consulted, never PendingHandoff, so a partial handoff an aborted cycle left is never chosen.
func ChooseStartPrompt(stencilsDir, handoffFlag string, s State, exists func(string) bool) (prompt, source string, err error) {
	if handoffFlag != "" {
		if !exists(handoffFlag) {
			return "", "", fmt.Errorf("orch: --handoff file %q does not exist", handoffFlag)
		}
		prompt, err = RenderResumePrompt(stencilsDir, handoffFlag)
		return prompt, SourceFlag, err
	}
	if s.LastHandoff != "" && exists(s.LastHandoff) {
		prompt, err = RenderResumePrompt(stencilsDir, s.LastHandoff)
		return prompt, SourceLastHandoff, err
	}
	prompt, err = RenderStartPrompt(stencilsDir)
	return prompt, SourceFresh, err
}
