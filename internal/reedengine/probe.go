// probe.go implements the capability probe run once at server-ensure
// (lifecycle.go): a decidable, pure core (probeCapability) that checks a
// multiplexer's `-V` version against this GOOS's pinned floor
// (minMultiplexerVersion, version.go) and its `list-commands` output
// against the engine's required subcommand set, plus a thin Engine method
// (probeCapabilityLocked) that binds the pure core to a real exec.Command
// invocation of the configured tmux binary.

package reedengine

import (
	"fmt"
	"os/exec"
	"strings"
)

// CapabilityError reports the multiplexer binary does not meet minimum
// surface requirements (version floor or missing subcommands).
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

// probeCapabilityLocked runs the capability probe against the configured
// multiplexer binary directly (bypassing TmuxCmd's -L socket prefix).
func (e *Engine) probeCapabilityLocked() error {
	run := func(args ...string) (string, error) {
		out, err := exec.Command(e.cfg.Tmux, args...).Output()
		return string(out), err
	}
	return probeCapability(run)
}
