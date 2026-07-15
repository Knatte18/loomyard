// probe_test.go drives probeCapability's pure decidable core (probe.go)
// with a fake run closure, covering the healthy path plus each of its two
// failure modes (version below the pinned floor, a missing required
// subcommand) without ever shelling out to a real multiplexer binary.

package muxengine

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
