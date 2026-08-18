// probe_test.go drives probeCapability's pure decidable core (probe.go) with a fake run closure,
// covering the healthy path plus each of its two failure modes (version below the pinned floor, a
// missing required subcommand) without ever shelling out to a real multiplexer binary.

package reedengine

import (
	"errors"
	"strings"
	"testing"
)

// fakeVersionOutput reports a version comfortably above both
// minTmuxVersion and minTmuxVersion, in both binaries' own -V shapes, so
// the version check passes regardless of which GOOS branch
// minMultiplexerVersion selects on the host running this test.
const fakeVersionOutput = "psmux 99.0.0 / tmux 99.0\n"

// fakeFullCommandsOutput renders every requiredSubcommands entry as one
// list-commands line (name plus filler description text, mirroring real
// psmux/tmux output), so parseCommandNames sees a complete command set.
func fakeFullCommandsOutput() string {
	var b strings.Builder
	for _, name := range requiredSubcommands {
		b.WriteString(name)
		b.WriteString("               - description\n")
	}
	return b.String()
}

func TestProbeCapability(t *testing.T) {
	tests := []struct {
		name       string
		run        func(args ...string) (string, error)
		wantErr    bool
		wantCapErr bool
	}{
		{
			name: "healthy version and full command set",
			run: func(args ...string) (string, error) {
				if args[0] == "-V" {
					return fakeVersionOutput, nil
				}
				return fakeFullCommandsOutput(), nil
			},
			wantErr:    false,
			wantCapErr: false,
		},
		{
			name: "version below pin",
			run: func(args ...string) (string, error) {
				if args[0] == "-V" {
					return "psmux 0.0.1 / tmux 0.0\n", nil
				}
				return fakeFullCommandsOutput(), nil
			},
			wantErr:    true,
			wantCapErr: true,
		},
		{
			name: "missing required subcommand",
			run: func(args ...string) (string, error) {
				if args[0] == "-V" {
					return fakeVersionOutput, nil
				}
				// Emit every required subcommand except kill-server, so
				// the missing-subcommand branch is the only failure hit.
				var b strings.Builder
				for _, name := range requiredSubcommands {
					if name == "kill-server" {
						continue
					}
					b.WriteString(name)
					b.WriteString("               - description\n")
				}
				return b.String(), nil
			},
			wantErr:    true,
			wantCapErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := probeCapability(tt.run)
			if (err != nil) != tt.wantErr {
				t.Fatalf("probeCapability() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantCapErr {
				var capErr *CapabilityError
				if !errors.As(err, &capErr) {
					t.Errorf("probeCapability() error = %v, want *CapabilityError", err)
				}
			}
		})
	}
}

// TestProbeCapabilityLocked_GoesThroughTheSocketScopedTmuxCmd is the regression guard for the R2
// review's R2-F6: the probe used to shell out to the binary directly, bypassing TmuxCmd's -L prefix,
// which made tmux answer `list-commands` from a server on the operator's GLOBAL DEFAULT socket — a
// live-substrate spawn outside reed's per-hub server model, and a global that every concurrent reed
// invocation on the box then raced (observed: `run list-commands: exit status 1` aborting `up` twice
// in nine runs of a 3x concurrent smoke sweep).
//
// TmuxCmd's execHook is the seam that makes this decidable hermetically: a hook is consulted only by
// TmuxCmd.run/output, so it fires if and only if the probe goes through TmuxCmd. newTestEngine's
// cfg.Tmux deliberately names a path that does not exist, so the bypassing implementation cannot
// pass this test by accident — it would exec that missing binary and fail.
func TestProbeCapabilityLocked_GoesThroughTheSocketScopedTmuxCmd(t *testing.T) {
	e := newTestEngine(t)

	var sawArgs [][]string
	e.tmux.execHook = func(capture bool, args ...string) (string, error) {
		sawArgs = append(sawArgs, args)
		if len(args) > 0 && args[0] == "-V" {
			return fakeVersionOutput, nil
		}
		return fakeFullCommandsOutput(), nil
	}

	if err := e.probeCapabilityLocked(); err != nil {
		t.Fatalf("probeCapabilityLocked() = %v; want nil — a non-nil error here means the probe shelled out to the (deliberately nonexistent) configured binary instead of going through TmuxCmd", err)
	}

	if len(sawArgs) != 2 {
		t.Fatalf("TmuxCmd saw %d probe call(s) (%v); want exactly 2 (-V and list-commands) — every probe call must carry reed's own -L socket", len(sawArgs), sawArgs)
	}
	if sawArgs[0][0] != "-V" {
		t.Errorf("first probe call = %v; want -V", sawArgs[0])
	}
	if sawArgs[1][0] != "list-commands" {
		t.Errorf("second probe call = %v; want list-commands", sawArgs[1])
	}
}
