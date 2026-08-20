// probe.go implements the capability probe run once at server-ensure (lifecycle.go): a decidable,
// pure core (probeCapability) that checks a multiplexer's `-V` version against this GOOS's pinned
// floor (minMultiplexerVersion, version.go) and its `list-commands` output against the engine's
// required subcommand set, plus a thin Engine method (probeCapabilityLocked) that binds the pure
// core to this engine's own socket-scoped TmuxCmd.

package reedengine

import (
	"fmt"
	"strings"
)

// CapabilityError reports the multiplexer binary does not meet minimum surface requirements
// (version floor or missing subcommands).
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

// probeCapability checks version floor and required subcommands.
// run is injected for testability; probeCapabilityLocked binds it to real exec.
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

// parseCommandNames extracts command names from list-commands output.
// Returns a set of leading tokens (descriptions/aliases come after).
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

// probeCapabilityLocked runs the capability probe against the configured multiplexer binary through
// this engine's own TmuxCmd, so both probe calls carry reed's -L socket.
//
// The -L is load-bearing, not incidental tidiness. An earlier version shelled out to the binary
// directly, which meant tmux fell back to the operator's GLOBAL DEFAULT socket: `-V` is answered
// client-side and creates nothing, but `list-commands` is answered by a SERVER, so tmux started one
// on the operator's own default socket every time `lyx reed up` or `resume` probed — a live-substrate
// spawn outside the "exactly one named tmux server per hub" model, on a socket reed neither owns nor
// tears down. Sharing that global made the probe race every other concurrent reed invocation on the
// box, and `up` then aborted with `run list-commands: exit status 1` for a reason having nothing to
// do with the hub it was booting (reproduced twice in nine runs of a 3x concurrent smoke sweep).
//
// Through TmuxCmd instead: `-V` still creates nothing, and `list-commands` is answered by this hub's
// own server when it is already up — spawning nothing at all — or otherwise by a transient server on
// reed's OWN socket that exits as soon as it has answered, leaving behind only the socket file the
// boot about to follow would create anyway.
// Routing through TmuxCmd additionally puts the probe on wrapTmuxError, so a failure now carries
// tmux's own stderr instead of a bare exit status; every other tmux call in this package already had
// that and the probe was the one site that had opted out.
func (e *Engine) probeCapabilityLocked() error {
	return probeCapability(e.tmux.output)
}
