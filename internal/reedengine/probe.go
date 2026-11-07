// probe.go checks the multiplexer binary's surface once, at server-ensure.
// probeCapability is the pure core: it checks the `-V` version against the minimum floor,
// and it checks the `list-commands` output against the required subcommands.
// probeCapabilityLocked binds that core to this engine's `TmuxCmd`, re-pointed off Windows at a private socket.

package reedengine

import (
	"fmt"
	"math/rand/v2"
	"os"
	"runtime"
	"strings"

	"github.com/Knatte18/loomyard/internal/logger"
)

// CapabilityError reports that the multiplexer binary lacks the minimum surface reed needs:
// a version below the floor, or a missing required subcommand.
type CapabilityError struct {
	Reason string
}

// Error implements error and returns Reason.
func (e *CapabilityError) Error() string {
	return e.Reason
}

// requiredSubcommands lists the tmux subcommands the engine's lifecycle, overlay, and pane-management code issues.
// A multiplexer whose list-commands output omits any of them is unusable.
// The probe fails loud here, once at server-ensure, rather than deep inside an unrelated engine operation later.
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
// A multiplexer lacking one still boots, since a required subcommand that psmux lacks would take every reed verb down;
// the probe logs a warning for each missing one.
var optionalSubcommands = []string{
	"bind-key",
	"run-shell",
	"if-shell",
	"switch-client",
}

// probeCapability checks the version floor and the required subcommands, and warns about missing optional ones.
// run is the multiplexer invocation, injected so that tests can supply their own.
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

// parseCommandNames returns the set of command names in list-commands output.
// Each name is the first token of its line, since descriptions and aliases follow it.
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

// probeCapabilityLocked runs the capability probe against the configured multiplexer binary.
// It goes through this engine's `TmuxCmd`, and off Windows that is a copy bound to a private `-L` socket.
//
// The socket is load-bearing: `-V` is answered client-side and creates nothing.
// `list-commands` is answered by a server.
// On a socket with none, tmux starts a transient one that exits as soon as it has answered.
// A shared socket makes that transient server a race:
// a concurrent reed invocation whose client connects while it exits fails with `server exited unexpectedly`.
// Off Windows the probe therefore addresses a private `-L` key that no client of the hub's socket reaches.
// It also reads `-f /dev/null`,
// so an operator config that creates a session cannot keep the transient server alive.
// Once the server is gone, the probe removes the private key's socket file;
// a removal failure is logged, not returned.
// Windows keeps the engine's own `TmuxCmd`, since psmux has no /dev/null.
// Going through `TmuxCmd` also means a failure carries tmux's own stderr instead of a bare exit status.
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

// newProbeSocketKey returns a fresh `-L` key for one capability probe: `lym-p` and seven random hex digits.
// The key is 12 bytes, no longer than any hub socket key reed forms,
// so the probe's socket path fits wherever the hub's does.
// Two concurrent probes drawing the same key share one transient server, which only costs the isolation.
func newProbeSocketKey() string {
	return fmt.Sprintf("lym-p%07x", rand.Uint32()>>4)
}
