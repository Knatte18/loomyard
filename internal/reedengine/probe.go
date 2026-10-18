// probe.go implements the capability probe run once at server-ensure (lifecycle.go): a decidable,
// pure core (probeCapability) that checks a multiplexer's `-V` version against this GOOS's pinned
// floor (minMultiplexerVersion, version.go) and its `list-commands` output against the engine's
// required subcommand set, plus a thin Engine method (probeCapabilityLocked) that binds the pure
// core to this engine's TmuxCmd, re-pointed off Windows at a private socket.

package reedengine

import (
	"fmt"
	"math/rand/v2"
	"os"
	"runtime"
	"strings"

	"github.com/Knatte18/loomyard/internal/logger"
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
	"new-window",
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

// optionalSubcommands names the tmux subcommands the key bindings depend on.
// A multiplexer lacking one still boots, since a required verb psmux lacks would take every reed verb down;
// the probe logs one warning naming each missing verb.
var optionalSubcommands = []string{
	"bind-key",
	"run-shell",
	"if-shell",
	"switch-client",
}

// probeCapability checks version floor and required subcommands, and warns about missing optional ones.
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
	for _, optional := range optionalSubcommands {
		if !available[optional] {
			logger.Warn("reed: multiplexer lacks an optional subcommand, the key bindings that need it are not pinned", "subcommand", optional)
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

// probeCapabilityLocked runs the capability probe against the configured multiplexer binary through a copy of this engine's own TmuxCmd, so both probe calls carry an explicit -L socket.
//
// The socket is load-bearing: `-V` is answered client-side and creates nothing,
// but `list-commands` is answered by a SERVER, and on a socket with none tmux starts a transient one that exits as soon as it has answered.
// A shared socket makes that transient server a race: a concurrent reed invocation whose client connects while it exits fails with `server exited unexpectedly`.
// Off Windows the probe therefore addresses a private `-L` key of its own, which no client of the hub's socket can reach.
// It also reads `-f /dev/null`, so an operator config that creates a session cannot keep the transient server alive.
// The probe removes the private key's socket file once the server is gone, logging a failure without failing the probe.
// Windows keeps the engine's own TmuxCmd, since psmux has no /dev/null.
// Routing through TmuxCmd additionally puts the probe on wrapTmuxError, so a failure carries tmux's own stderr instead of a bare exit status.
func (e *Engine) probeCapabilityLocked() error {
	if runtime.GOOS == "windows" {
		return probeCapability(e.tmux.output)
	}
	probe := e.tmux
	probe.configFile = os.DevNull
	probe.socket = newProbeSocketKey()
	defer func() {
		if err := removeSocketFileOnceGone(socketDirFromEnv(), probe.socket, socketGoneWait); err != nil {
			logger.Warn("reed: could not remove the capability probe's socket file", "socket", probe.socket, "err", err)
		}
	}()
	return probeCapability(probe.output)
}

// newProbeSocketKey returns a fresh `-L` key for one capability probe: `lyx-p` and seven random hex digits.
// Its 12 bytes are no more than any hub socket key reed forms, so the probe's socket path fits wherever the hub's passed the path-length check.
// Two concurrent probes drawing the same key share one transient server, which only costs the isolation.
func newProbeSocketKey() string {
	return fmt.Sprintf("lyx-p%07x", rand.Uint32()>>4)
}
