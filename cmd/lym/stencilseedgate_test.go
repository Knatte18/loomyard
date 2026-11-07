// stencilseedgate_test.go pins skipStencilSeed's predicate against synthetic *cobra.Command values:
// it checks that the reed and creel commands carry the annotation the predicate reads.
// It does not pin the order of skipStencilSeed and stencilSeedTarget:
// seedStencils returns under testing.Testing() before either runs.
// Building the command tree only constructs cobra values and runs no hook:
// nothing spawns a process, so the Test Tier Purity Invariant holds.

package main

import (
	"testing"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/creelcli"
	"github.com/Knatte18/loomyard/internal/reedcli"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// reedSubcommand returns the direct subcommand named name of the reed command, and fails t when there is none.
func reedSubcommand(t *testing.T, name string) *cobra.Command {
	t.Helper()
	for _, sub := range reedcli.Command().Commands() {
		if sub.Name() == name {
			return sub
		}
	}
	t.Fatalf("reedcli.Command() has no %q subcommand", name)
	return nil
}

// TestSkipStencilSeed_HonoursTheAnnotation drives skipStencilSeed against synthetic *cobra.Command values,
// and against the real reed commands it lists and every creel command.
// Those commands must carry the annotation, or the gate is worthless for them.
func TestSkipStencilSeed_HonoursTheAnnotation(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	tests := []struct {
		name string
		cmd  *cobra.Command
		want bool
	}{
		{
			name: "carries the enabled annotation",
			cmd: &cobra.Command{
				Annotations: map[string]string{clihelp.SkipStencilSeedAnnotation: clihelp.AnnotationEnabled},
			},
			want: true,
		},
		{
			name: "no annotations map at all",
			cmd:  &cobra.Command{},
			want: false,
		},
		{
			name: "an unrelated annotation key",
			cmd: &cobra.Command{
				Annotations: map[string]string{"some.other.key": clihelp.AnnotationEnabled},
			},
			want: false,
		},
		{
			name: "the key present with value \"false\"",
			cmd: &cobra.Command{
				Annotations: map[string]string{clihelp.SkipStencilSeedAnnotation: "false"},
			},
			want: false,
		},
		{
			name: "a nil command",
			cmd:  nil,
			want: false,
		},
		{
			name: "reed switch carries the annotation",
			cmd:  reedSubcommand(t, "switch"),
			want: true,
		},
		{
			name: "reed follow carries the annotation",
			cmd:  reedSubcommand(t, "follow"),
			want: true,
		},
		{
			name: "reed button carries the annotation",
			cmd:  reedSubcommand(t, "button"),
			want: true,
		},
	}
	creel := creelcli.Command()
	for _, cmd := range append([]*cobra.Command{creel}, creel.Commands()...) {
		tests = append(tests, struct {
			name string
			cmd  *cobra.Command
			want bool
		}{name: cmd.CommandPath() + " carries the annotation", cmd: cmd, want: true})
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := skipStencilSeed(tt.cmd); got != tt.want {
				t.Errorf("skipStencilSeed(%+v) = %v; want %v", tt.cmd, got, tt.want)
			}
		})
	}
}
