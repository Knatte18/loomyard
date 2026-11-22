// revival_test.go covers the revival filter that drops remote control from a stored orch command line.

package orchengine_test

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/orchengine"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestStripRemoteControlArgument pins which text of a stored launch line the strip removes.
func TestStripRemoteControlArgument(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"PosixQuotedMiddle", `'claude' --resume 's' --remote-control 'neve::hub:orch' --model 'opus'`, `'claude' --resume 's' --model 'opus'`},
		{"PosixQuotedLast", `'claude' --resume 's' --remote-control 'neve::hub:orch'`, `'claude' --resume 's'`},
		{"NameWithEscapedQuote", `'claude' --remote-control 'a'\''b' --model 'opus'`, `'claude' --model 'opus'`},
		{"PwshDoubledQuote", `& 'claude' --remote-control 'a''b' --model 'opus'`, `& 'claude' --model 'opus'`},
		{"BareName", `claude --remote-control name --model opus`, `claude --model opus`},
		{"NoArgument", `'claude' --resume 's' --model 'opus'`, `'claude' --resume 's' --model 'opus'`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := orchengine.StripRemoteControlArgument(tt.input); got != tt.want {
				t.Errorf("StripRemoteControlArgument(%q) = %q; want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestRevivalFilter pins that a revived line keeps remote control only while orch.yaml sets remote_control on.
// An orch.yaml that does not load cannot ask for it, so the argument is dropped.
func TestRevivalFilter(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()
	const line = `'claude' --resume 's' --remote-control 'neve::hub:orch' --model 'opus'`
	const stripped = `'claude' --resume 's' --model 'opus'`
	tests := []struct {
		name string
		yaml string
		want string
	}{
		{"NoFileTakesTemplateOff", "", stripped},
		{"ExplicitOff", "remote_control: false\n", stripped},
		{"ExplicitOn", "remote_control: true\nremote_control_name: \"{machine}::{hub}:{role}\"\n", line},
		{"OnWithoutNameDoesNotLoad", "remote_control: true\nremote_control_name: \"\"\n", stripped},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			if tt.yaml != "" {
				seedLymConfig(t, dir, "orch", tt.yaml)
			}
			if got := orchengine.RevivalFilter(configengine.Dirs{Anchor: dir})(line); got != tt.want {
				t.Errorf("RevivalFilter(%q) = %q; want %q", tt.yaml, got, tt.want)
			}
		})
	}
}
