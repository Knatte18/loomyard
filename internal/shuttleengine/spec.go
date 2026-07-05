// spec.go defines Spec, the caller-supplied description of one shuttle run,
// and its validate method: the single place that enforces the file
// contract (a run's output files ARE its return value) and fills in the
// defaults a caller is allowed to omit (timeout, display anchor).

package shuttleengine

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/muxengine/render"
)

// Spec describes one shuttle run: the prompt handed to the provider as the
// launch argument, the output files that constitute the run's return value,
// and the display/lifecycle knobs the run loop and mux need. Spec is a
// plain value the caller (review, loom) constructs; it carries no methods
// beyond validate, which normalizes and checks it in place.
type Spec struct {
	// Prompt is the task text handed to the provider as the launch
	// argument. shuttle never templates prompt content — the caller
	// composes it (dumb transport, like mux).
	Prompt string
	// OutputFiles names the files the agent is instructed to write. The
	// run is not "done" until every entry exists — the file contract: a
	// run's output file IS its return value. Entries may be absolute or
	// relative to the worktree root; validate resolves relative entries
	// and rewrites this slice in place with the resolved absolute paths.
	OutputFiles []string
	// Model, when non-empty, selects a specific provider model; empty
	// defers to the engine/provider default.
	Model string
	// Interactive encodes !Autonomous: the Go zero value (false) means
	// autonomous, the default. Autonomous runs add
	// --dangerously-skip-permissions and the AskUserQuestion PreToolUse
	// deny; interactive runs add neither. The Agent tool deny is included
	// in both modes (each deny still individually toggleable via the
	// shuttle config's claude_deny_agent_tool / claude_deny_ask_user_question
	// keys).
	Interactive bool
	// Role and Round feed the strand display name template
	// (<ROLE>:<ROUND>:<SHORT_GUID>); both may be empty.
	Role  string
	Round string
	// Parent is the parent strand's GUID, or "" for a root strand.
	Parent string
	// Display carries the mux placement/focus/shrink settings for this
	// run's strand.
	Display render.Display
	// Timeout is the wall-clock deadline after which an in-progress run is
	// classified as timed out. Zero defers to cfg.RunTimeoutMin minutes —
	// note that this means cfg.RunTimeoutMin itself has no "unlimited"
	// value: a configured RunTimeoutMin of 0 makes every run's deadline equal
	// to its start time, so it is classified OutcomeTimeout on the very
	// first poll tick, not "no timeout".
	Timeout time.Duration
	// KeepPane, when true, leaves the strand and its pane alive after a
	// "done" outcome instead of the default RemoveStrand + run-dir cleanup.
	KeepPane bool
}

// validate normalizes s in place and reports an error if it is not
// runnable. Prompt must be non-empty; OutputFiles must name at least one
// file (an empty list would make "done" undetectable, since the file
// contract is the only return channel a shuttle run has). Each OutputFiles
// entry is resolved to an absolute path — already-absolute entries are kept
// verbatim, relative entries are joined onto worktreeRoot and
// filepath.Clean-ed — and the resolved paths are written back into
// s.OutputFiles so every later reader sees only absolute paths. A zero
// Timeout is replaced with cfg.RunTimeoutMin minutes, and an empty
// Display.Anchor defaults to render.AnchorBelowParent.
func (s *Spec) validate(worktreeRoot string, cfg Config) error {
	if s.Prompt == "" {
		return fmt.Errorf("shuttle: spec.Prompt must not be empty")
	}
	if len(s.OutputFiles) == 0 {
		return fmt.Errorf("shuttle: spec.OutputFiles must name at least one file — a run's output file IS its return value")
	}

	// Resolve every output file to an absolute path up front so the run
	// loop's later existence polls never have to reason about worktree
	// context again.
	resolved := make([]string, len(s.OutputFiles))
	for i, f := range s.OutputFiles {
		if filepath.IsAbs(f) {
			resolved[i] = f
			continue
		}
		resolved[i] = filepath.Clean(filepath.Join(worktreeRoot, f))
	}
	s.OutputFiles = resolved

	if s.Timeout == 0 {
		s.Timeout = time.Duration(cfg.RunTimeoutMin) * time.Minute
	}

	if s.Display.Anchor == "" {
		s.Display.Anchor = render.AnchorBelowParent
	}

	return nil
}
