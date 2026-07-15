// probe.go implements the capability probe run once at server-ensure
// (lifecycle.go): a decidable, pure core (probeCapability) that checks a
// multiplexer's `-V` version against this GOOS's pinned floor
// (minMultiplexerVersion, version.go) and its `list-commands` output
// against the engine's required subcommand set, plus a thin Engine method
// (probeCapabilityLocked) that binds the pure core to a real exec.Command
// invocation of the configured tmux binary.

package muxengine

import (
	"fmt"
	"os/exec"
	"strings"
)

// CapabilityError reports that the configured multiplexer binary does not
// meet this engine's minimum surface requirements — either its reported
// version is below the pinned floor (minMultiplexerVersion) or its
// list-commands output is missing one of requiredSubcommands. It is
// returned as *CapabilityError so callers can distinguish a capability
// failure from a plain exec/parse error via errors.As, and it propagates
// unwrapped through Engine.Up() onto the existing output.Err JSON envelope
// (the typed-errors-through-existing-envelope Shared Decision).
type CapabilityError struct {
	Reason string
}

// Error implements error, returning the human-readable capability failure.
func (e *CapabilityError) Error() string {
	return e.Reason
}

// requiredSubcommands names every tmux subcommand the engine's
// lifecycle, overlay, and pane-management code depends on (has-session,
// new-session, and the pane-lifecycle/query verbs overlay.go and
// lifecycle.go issue). The capability probe treats a multiplexer binary
// whose own list-commands output omits any of these as unusable — better
// to fail loud once at server-ensure than deep inside an unrelated engine
// operation later.
var requiredSubcommands = []string{
	"new-session",
	"has-session",
	"split-window",
	"select-layout",
	"select-pane",
	"send-keys",
	"capture-pane",
	"list-panes",
	"list-sessions",
	"display-message",
	"set-option",
	"kill-pane",
	"kill-session",
	"kill-server",
}

// probeCapability runs the version and command-surface checks against the
// multiplexer binary reachable through run, returning a *CapabilityError
// describing the first failure found (version floor, then a missing
// subcommand) or nil when both checks pass. run is injected so this
// pure-logic core is host-testable with a fake — probeCapabilityLocked
// binds it to a real exec.Command invocation for production use.
func probeCapability(run func(args ...string) (string, error)) error {
	versionOut, err := run("-V")
	if err != nil {
		return fmt.Errorf("run -V: %w", err)
	}
	got, err := parseMultiplexerVersion(versionOut)
	if err != nil {
		return fmt.Errorf("parse -V output: %w", err)
	}
	floor := minMultiplexerVersion()
	if !versionAtLeast(got, floor) {
		return &CapabilityError{
			Reason: fmt.Sprintf("multiplexer version %v is below the required minimum %v", got, floor),
		}
	}

	listOut, err := run("list-commands")
	if err != nil {
		return fmt.Errorf("run list-commands: %w", err)
	}
	available := parseCommandNames(listOut)
	for _, want := range requiredSubcommands {
		if !available[want] {
			return &CapabilityError{
				Reason: fmt.Sprintf("multiplexer is missing required subcommand %q", want),
			}
		}
	}
	return nil
}

// parseCommandNames extracts the first whitespace-delimited token from each
// line of list-commands output into a set. tmux appends aliases/descriptions
// after the command name on the same line (e.g. "kill-server               -
// Kill the tmux server"), so only the leading token is a stable command name
// across list-commands formatting; blank lines and header text are harmless
// extras in the returned set since callers only ever look up known names.
func parseCommandNames(out string) map[string]bool {
	names := make(map[string]bool)
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		names[fields[0]] = true
	}
	return names
}

// probeCapabilityLocked runs the capability probe against this engine's
// configured multiplexer binary (e.cfg.Tmux), invoking it directly rather
// than through the overlay's -L <socket> prefix: -V and list-commands are
// socket-free queries that must succeed even before any server exists, so
// routing them through TmuxCmd's socket-bound run/output would be both
// unnecessary and, before the first boot, meaningless (no socket to name
// yet). It assumes the op lock is already held, matching every other
// *Locked helper in this package.
func (e *Engine) probeCapabilityLocked() error {
	run := func(args ...string) (string, error) {
		out, err := exec.Command(e.cfg.Tmux, args...).Output()
		return string(out), err
	}
	return probeCapability(run)
}
