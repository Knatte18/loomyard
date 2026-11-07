// spec.go defines Spec, the caller-supplied description of one shuttle run.
// Its validate method enforces the file contract, under which a run's output files are its return value.
// It also fills in the defaults a caller may omit: the timeout and the display anchor.

package shuttleengine

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/agentconfig"
	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
)

// Spec describes one shuttle run: its prompt, its output files, and its display and lifecycle settings.
// Callers construct it as a plain value.
// Its only method, validate, normalizes and checks it in place.
type Spec struct {
	// Prompt is the task text handed to the provider as the launch argument.
	// shuttle never templates prompt content; the caller composes it.
	// shuttle only transports the text.
	Prompt string
	// OutputFiles names the files the agent is instructed to write.
	// The run is not done until every entry exists.
	// A run's output file is its return value.
	// Entries must not already exist when the run starts.
	// A stale file would satisfy the contract on the first turn end.
	// It would silently classify an unfinished run as done.
	// Entries may be absolute or relative to the worktree root.
	// Validation resolves relative entries.
	// It rewrites this slice in place with the absolute paths.
	OutputFiles []string
	// Unawaited marks a session nobody waits on, with no output file: its run record reads running until a stop ends it.
	// Only Runner.StartSession accepts such a spec, and validation refuses one that also names output files.
	Unawaited bool
	// ModelChoice is the engine and the model, effort and version the run uses.
	ModelChoice
	// ForkSubagents, when true, authorizes this run to spawn in-session fork subagents.
	// The engine must realize the authorization or refuse the run, through whatever its provider needs.
	// ForkSubagents is engine vocabulary, and validation does not inspect it.
	// The run records it at spawn.
	// The record, not a later Call's Spec, decides the fork audit.
	// It also decides a gate entry's Forked closure.
	ForkSubagents bool
	// DenyPublish, when true, makes the engine install a shell guard for this one run and announce it.
	// The guard refuses git push and gh calls beyond reads, comments, and title and body edits.
	DenyPublish bool
	// PermissionMode, when non-empty, selects the run's permission mode.
	// Empty defers to the engine's default, its bypass mode.
	// The values are provider vocabulary; only the engine knows which ones its provider realizes.
	// A value the engine cannot realize is a hard error from that engine.
	// Spec validation does not check the field.
	PermissionMode string
	// ResumeSessionID, when non-empty, names an existing session the engine launches instead of minting one.
	// The run takes that session over.
	// The engine validates the id's shape; validation does not inspect the field.
	// Whether the session can be resumed at all is decided through the optional SessionResumer capability.
	ResumeSessionID string
	// Interactive encodes !Autonomous.
	// The Go zero value, false, means autonomous.
	// Autonomous runs add the AskUserQuestion PreToolUse deny.
	// Interactive runs do not.
	// The Agent tool deny and the python deny are included in both modes.
	// The python deny is toggleable via the shuttle config's deny_python key.
	// The Agent tool and AskUserQuestion denies are toggleable via the claude engine's own config.
	// Whether the launch carries --dangerously-skip-permissions follows the resolved PermissionMode.
	// An empty PermissionMode skips in both modes.
	Interactive bool
	// QuietHold skips the held-turn parent notice alone.
	// The hold itself, the wait mark, the deadline and attach behaviour are unchanged.
	QuietHold bool
	// APIErrorRetries is how many times the run re-sends a turn the provider's API ended on an error.
	// Re-sends apply only while an output file is missing.
	// Each re-send waits a back-off that doubles with each API-error turn end of the run.
	// The API-error turn end past the last re-send ends the run as OutcomeAPIError, whose Result names the error.
	// Zero re-sends nothing, so such a turn end waits out the run's deadline.
	// A turn end marked as rejecting the session's model ends the run as OutcomeAPIError at once.
	APIErrorRetries int
	// HoldDeferral, when set, reports whether outstanding work elsewhere defers this run's held-turn parent notice.
	// A held turn end is noticed once the settle interval has passed and the predicate reports false.
	// Past the maximum deferral it is noticed whatever the predicate reports.
	// A new event or turn start drops the pending notice.
	// The predicate is called from the run's wait loop, so it must not block.
	HoldDeferral func() bool
	// Segment is the loom segment the spawning module names for its role.
	// It is forwarded to reed, which colors the strand's bar button and border from the segment's palette.
	// After startup, shuttle types the provider's color command for that color.
	// Validation does not inspect it.
	Segment segmentcolor.Segment
	// ColorByCaller, when true, makes shuttle type no color command, because the caller types the color itself.
	// Validation does not inspect it.
	ColorByCaller bool
	// Role is the role segment of the strand's name; it may be empty.
	// Round is not part of the name; it names the run's directory.
	Role  string
	Round string
	// Parent is the parent strand's GUID, or "" for a root strand.
	Parent string
	// NameOverride is forwarded verbatim into the reed add spec.
	// Runner.Start builds that spec.
	// It is never interpreted.
	// It is an explicit role segment or full name.
	// An empty value leaves reed naming the strand from Role.
	NameOverride string
	// Display carries the reed placement, focus and shrink settings for this run's strand.
	// A strand lands in a window of its own unless the spec names an anchor.
	// An empty Anchor defaults to render.AnchorOwnWindow.
	Display render.Display
	// Timeout is the wall-clock deadline of a run.
	// After it, an in-progress run is classified as timed out.
	// Zero defers to cfg.RunTimeoutMin minutes.
	// cfg.RunTimeoutMin has no unlimited value.
	// A configured 0 makes every run's deadline equal its start time,
	// so the run is classified OutcomeTimeout on the first poll tick.
	// A negative value is rejected at validation.
	// Such a value can only be a mistake, a sign typo or broken duration arithmetic.
	// Letting it through would launch a run that times out on the first tick.
	// It would also leave a live stray agent pane and a kept run directory behind.
	// The caller never meant to start a run with that deadline.
	// Validation therefore refuses the value before any run starts.
	Timeout time.Duration
	// KeepPane, when true, leaves the strand and its pane alive after a "done" outcome.
	// Without it, the default removes the strand and cleans up the run directory.
	KeepPane bool
	// AwaitedShellPrefixes names the background shells the run waits on, the way it waits on a fork.
	// A shell whose label starts with one of them stays out of the background-shell wait marker.
	// Such a shell also keeps a gated waiting turn end waiting, with no nudge and no expiry.
	// It likewise keeps an ungated wait from expiring.
	AwaitedShellPrefixes []string
	// ConfigRole, when non-empty, names the agents.yaml role the session runs as.
	// Start puts the role's skill list ahead of Skills, dropping repeats, and refuses a role the config holds no list for.
	// An engine reads the role's efforts for the launch and for the subagents the session spawns.
	// Empty leaves Skills as the whole list.
	ConfigRole agentconfig.Role
	// Skills names the provider-neutral skills shuttle loads into the fresh session, after ConfigRole's list.
	// They all load in one turn, before the prompt is delivered.
	// The engine realizes the list through its skill-loader capability.
	// An engine without that capability refuses a non-empty list at start.
	Skills []string
	// SkillLoadTimeout bounds each load turn's wait before skipping its skills; zero takes the engine's default.
	// A turn still open at the timeout is awaited until it ends or the run's deadline fails the start;
	// a turn the provider's API fails is run again however long that takes, so neither skips a skill at this bound.
	SkillLoadTimeout time.Duration
	// ContextAfterCompaction, when non-empty, is a shell command.
	// The provider adds its standard output to the session's context right after every compaction of this run.
	// The engine runs it verbatim inside the provider's hook shell, on no other session event.
	ContextAfterCompaction string
	// Requires lists what the run requires of its engine, from the closed set of Requirement constants.
	// Start refuses an unknown entry and an engine that does not meet every entry.
	// Attach checks the engine the run's record names in the same way.
	// A ForkSubagents spec requires RequirementForks without listing it, and duplicates are harmless.
	Requires []Requirement
}

// validate normalizes s in place and reports an error if it is not runnable.
// Prompt must be non-empty.
// OutputFiles must name at least one file, since the file contract is the only return channel a run has.
// Without one, "done" would be undetectable.
// An Unawaited spec is the exception: it has no output file, so it is refused when it names one.
// Each OutputFiles entry is resolved to an absolute path: an absolute entry is kept as it is,
// and a relative one is joined onto worktreeRoot and cleaned.
// The resolved paths are written back into s.OutputFiles.
// A resolved entry that already exists on disk is rejected, since outcome classification tests bare existence.
// A stale file would classify the run as done at its first turn end.
// A misconfigured spec must fail loudly here, never become silent success.
// A negative Timeout is rejected, because it would launch a run whose deadline has already passed.
// A zero Timeout is replaced with cfg.RunTimeoutMin minutes.
// An empty Display.Anchor defaults to render.AnchorOwnWindow,
// so a run's strand opens in its own window unless the spec names an anchor.
// A hidden anchor is refused: reed registers a hidden strand without starting its command or giving it a pane,
// so its provider would never start.
func (s *Spec) validate(worktreeRoot string, cfg Config) error {
	if s.Prompt == "" {
		return fmt.Errorf("shuttle: spec.Prompt must not be empty")
	}
	if s.Unawaited && len(s.OutputFiles) > 0 {
		return fmt.Errorf("shuttle: spec.OutputFiles must be empty for an unawaited session, which has no output file")
	}
	if !s.Unawaited && len(s.OutputFiles) == 0 {
		return fmt.Errorf("shuttle: spec.OutputFiles must name at least one file — a run's output file IS its return value")
	}

	// Resolve every output file to an absolute path up front,
	// so the run loop's later existence polls never reason about worktree context.
	resolved := make([]string, len(s.OutputFiles))
	for i, f := range s.OutputFiles {
		if filepath.IsAbs(f) {
			resolved[i] = f
			continue
		}
		resolved[i] = filepath.Clean(filepath.Join(worktreeRoot, f))
	}
	s.OutputFiles = resolved

	// Reject entries that already exist: "done" is bare file existence.
	// A stale artifact would satisfy the contract before the agent writes anything.
	// It would silently swallow an unfinished run as success.
	for _, f := range s.OutputFiles {
		if _, err := os.Stat(f); err == nil {
			return fmt.Errorf("shuttle: spec.OutputFiles entry %q already exists — a pre-existing file would satisfy the file contract immediately; remove it or name a fresh path", f)
		}
	}

	// A negative deadline can only be a caller mistake: 0 is the documented "defer to config" value,
	// and there is no "unlimited" spelling.
	// Fail here, before any run dir or strand exists.
	// Otherwise the run would launch and classify OutcomeTimeout on its first poll tick.
	// It would then keep its live pane and run dir as stray state.
	if s.Timeout < 0 {
		return fmt.Errorf("shuttle: spec.Timeout must not be negative (got %s); use 0 for the config default run_timeout_min", s.Timeout)
	}
	if s.Timeout == 0 {
		s.Timeout = time.Duration(cfg.RunTimeoutMin) * time.Minute
	}

	if s.Display.Anchor == "" {
		s.Display.Anchor = render.AnchorOwnWindow
	}
	if s.Display.Anchor == render.AnchorHidden {
		return fmt.Errorf("shuttle: spec.Display.Anchor %q cannot run: reed registers a hidden strand without starting its command or giving it a pane, so the provider never starts; use %q, with KeepViewer set to leave the viewer where it is", render.AnchorHidden, render.AnchorOwnWindow)
	}

	return nil
}
