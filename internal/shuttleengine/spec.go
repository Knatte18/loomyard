// spec.go defines Spec, the caller-supplied description of one shuttle run, and its validate
// method: the single place that enforces the file contract (a run's output files ARE its return
// value) and fills in the defaults a caller is allowed to omit (timeout, display anchor).

package shuttleengine

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Knatte18/loomyard/internal/reedengine/render"
	"github.com/Knatte18/loomyard/internal/segmentcolor"
)

// Spec describes one shuttle run: the prompt handed to the provider as the launch argument, the
// output files that constitute the run's return value, and the display/lifecycle knobs the run loop
// and reed need.
// Spec is a plain value the caller (review, loom) constructs;
// it carries no methods beyond validate, which normalizes and checks it in place.
type Spec struct {
	// Prompt is the task text handed to the provider as the launch
	// argument. shuttle never templates prompt content — the caller
	// composes it (dumb transport, like reed).
	Prompt string
	// OutputFiles names the files the agent is instructed to write. The
	// run is not "done" until every entry exists — the file contract: a
	// run's output file IS its return value. Entries must NOT already
	// exist when the run starts (validate rejects a pre-existing entry):
	// a stale file would satisfy the contract on the very first turn end,
	// silently classifying an unfinished run as done. Entries
	// may be absolute or relative to the worktree root; validate resolves
	// relative entries and rewrites this slice in place with the resolved
	// absolute paths.
	OutputFiles []string
	// Model, when non-empty, selects a specific provider model; empty
	// defers to the engine/provider default.
	Model string
	// Effort, when non-empty, selects a reasoning-effort override; empty
	// defers to the engine/provider default. Effort values are provider
	// vocabulary — validate does NOT inspect this field at all (neither
	// defaulting nor rejecting it); the engine is the sole validator, since
	// only it knows which values its provider realizes. A non-empty value
	// the engine cannot realize is a hard error from the engine (see
	// claudeengine's validateEffort), not from Spec.validate.
	Effort string
	// Version, when non-empty, selects a version pin the provider engine
	// realizes by translating (Model, Version) into a provider-specific
	// model id; empty means no pin. Version values are provider
	// vocabulary — validate does NOT inspect this field at all (neither
	// defaulting nor rejecting it); the engine is the sole validator, since
	// only it knows which values its provider realizes. A (Model, Version)
	// pair the engine cannot realize is a hard error from the engine (see
	// claudeengine's resolveModelID), not from Spec.validate.
	Version string
	// ForkSubagents, when true, authorizes this run to spawn in-session fork
	// subagents: the engine must realize the authorization (an env flag, a
	// hook shape — whatever its provider needs) or hard-error if it cannot.
	// ForkSubagents is engine vocabulary, exactly like Effort/Version above —
	// validate does not inspect this field at all.
	ForkSubagents bool
	// AllowAgentTool, when true, lets this one run use the Agent tool for every subagent type:
	// the engine installs no Agent deny and announces none, whatever the shuttle config's claude_deny_agent_tool says.
	// It is engine vocabulary exactly like ForkSubagents — validate does not inspect this field at all.
	// No config key or CLI flag reaches it;
	// only a caller that sets it gets the allowance.
	AllowAgentTool bool
	// PermissionMode, when non-empty, selects the run's permission mode;
	// empty defers to the run mode's default.
	// PermissionMode values are provider vocabulary, exactly like Effort — validate does NOT inspect this field at all;
	// the engine is the sole validator, since only it knows which values its provider realizes.
	// A value the engine cannot realize is a hard error from the engine (see claudeengine's validatePermissionMode), not from Spec.validate.
	PermissionMode string
	// ResumeSessionID, when non-empty, names an existing session the engine launches instead of minting a new one;
	// the run takes that session over.
	// It is engine vocabulary exactly like Effort — validate does NOT inspect this field;
	// the engine validates the id's shape and, through the optional SessionResumer capability, whether the session can be resumed at all.
	ResumeSessionID string
	// Interactive encodes !Autonomous: the Go zero value (false) means
	// autonomous, the default.
	// Autonomous runs add the AskUserQuestion PreToolUse deny;
	// interactive runs do not.
	// Whether the launch carries --dangerously-skip-permissions follows the resolved PermissionMode,
	// whose empty default skips in an autonomous run and prompts in an interactive one.
	// The Agent tool deny is included
	// in both modes (each deny still individually toggleable via the
	// shuttle config's claude_deny_agent_tool / claude_deny_ask_user_question
	// keys).
	Interactive bool
	// Segment is the loom segment the spawning module names for its role.
	// It is forwarded to reed, which resolves the segment's palette color for the strand's bar button and border,
	// and shuttle types the provider's color command for that color after startup.
	// validate does not inspect it.
	Segment segmentcolor.Segment
	// ColorByCaller, when true, makes shuttle type no color command, because the caller types the color itself.
	// validate does not inspect it.
	ColorByCaller bool
	// Role is the role segment of the strand's name; it may be empty.
	// Round is not part of the name; it names the run's directory.
	Role  string
	Round string
	// Parent is the parent strand's GUID, or "" for a root strand.
	Parent string
	// NameOverride is forwarded verbatim into the reedengine.AddSpec that
	// Runner.Start builds and never interpreted — the same contract
	// SessionID's own doc comment states above.
	// It is an explicit role segment or full name;
	// an empty value leaves reed naming the strand from Role (see reedengine.strandNameLocked).
	NameOverride string
	// Display carries the reed placement/focus/shrink settings for this
	// run's strand.
	Display render.Display
	// Timeout is the wall-clock deadline after which an in-progress run is
	// classified as timed out. Zero defers to cfg.RunTimeoutMin minutes —
	// note that this means cfg.RunTimeoutMin itself has no "unlimited"
	// value: a configured RunTimeoutMin of 0 makes every run's deadline equal
	// to its start time, so it is classified OutcomeTimeout on the very
	// first poll tick, not "no timeout". A NEGATIVE value (a sign typo, or a
	// caller's broken duration arithmetic) is rejected at validate: unlike
	// the documented 0 footgun it can only ever be a mistake, and letting it
	// through would launch a full run only to classify it OutcomeTimeout on
	// the first tick — leaving a live stray agent pane and a kept run dir
	// behind a run the caller never meant to start with that deadline
	// (proven live).
	Timeout time.Duration
	// KeepPane, when true, leaves the strand and its pane alive after a
	// "done" outcome instead of the default RemoveStrand + run-dir cleanup.
	KeepPane bool
	// AwaitedShellPrefixes names the background shells the run waits on like a fork:
	// a shell whose label starts with one of them is bounded only by Timeout, never by background_shell_wait_min.
	// It is caller data, not provider knowledge, and validate does not inspect it.
	AwaitedShellPrefixes []string
	// Skills names the provider-neutral skills shuttle loads into the fresh session, all in one turn, before it delivers the prompt.
	// The engine realizes the list through its SkillLoader capability,
	// and a non-empty list on an engine without one is refused at start.
	// validate does not inspect it.
	Skills []string
	// SkillLoadTimeout bounds how long shuttle waits for one load turn to end, the first and the retry each, before skipping its skills.
	// Zero defers to the engine's DefaultSkillLoadTimeout; validate does not inspect it.
	SkillLoadTimeout time.Duration
}

// validate normalizes s in place and reports an error if it is not
// runnable. Prompt must be non-empty; OutputFiles must name at least one
// file (an empty list would make "done" undetectable, since the file
// contract is the only return channel a shuttle run has). Each OutputFiles
// entry is resolved to an absolute path — already-absolute entries are kept
// verbatim, relative entries are joined onto worktreeRoot and
// filepath.Clean-ed — and the resolved paths are written back into
// s.OutputFiles so every later reader sees only absolute paths.
// A resolved entry that already exists on disk is rejected:
// outcome classification tests bare existence, so a stale file would classify the run done on its very first turn end —
// a misconfigured spec must fail loudly here, never become silent success
// (proven live: a run that stopped to ask a question, against a pre-existing output file, returned "done" with the question discarded).
// A negative Timeout is rejected (see the Timeout field's doc comment: it would launch
// a run whose deadline is already in the past, leaving stray live state
// behind an instant OutcomeTimeout); a zero Timeout is replaced with
// cfg.RunTimeoutMin minutes, and an empty Display.Anchor defaults to
// render.AnchorBelowParent.
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

	// Reject entries that already exist: "done" is bare file existence, so
	// a stale artifact would satisfy the contract before the agent writes
	// anything, silently swallowing an unfinished run as success.
	for _, f := range s.OutputFiles {
		if _, err := os.Stat(f); err == nil {
			return fmt.Errorf("shuttle: spec.OutputFiles entry %q already exists — a pre-existing file would satisfy the file contract immediately; remove it or name a fresh path", f)
		}
	}

	// A negative deadline can only be a caller mistake (0 is the documented
	// "defer to config" value; there is no "unlimited" spelling): fail here,
	// before any run dir or strand exists, rather than launching a run whose
	// first poll tick classifies OutcomeTimeout and keeps its live pane and
	// run dir around as stray state.
	if s.Timeout < 0 {
		return fmt.Errorf("shuttle: spec.Timeout must not be negative (got %s); use 0 for the config default run_timeout_min", s.Timeout)
	}
	if s.Timeout == 0 {
		s.Timeout = time.Duration(cfg.RunTimeoutMin) * time.Minute
	}

	if s.Display.Anchor == "" {
		s.Display.Anchor = render.AnchorBelowParent
	}

	return nil
}
