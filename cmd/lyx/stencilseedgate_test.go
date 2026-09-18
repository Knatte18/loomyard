// stencilseedgate_test.go pins two things about the stencil-seed skip gate this batch adds:
// skipStencilSeed's predicate against synthetic *cobra.Command values, and that "lyx reed statusline"
// actually carries the annotation the predicate reads. It deliberately does NOT pin any ordering
// between skipStencilSeed and stencilSeedTarget, and does NOT assert "no `git rev-parse` was
// spawned": seedStencils returns under testing.Testing() before either step runs, so an in-process
// test can never observe their relative order, and "no git rev-parse was spawned" is the unfalsifiable
// shape the discussion driving this task rejected. Building the command tree here only constructs
// cobra values and runs no hook, so nothing spawns a process and the Test Tier Purity Invariant holds.

package main

import (
	"testing"

	"github.com/spf13/cobra"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/reedcli"
)

// TestSkipStencilSeed_HonoursTheAnnotation drives skipStencilSeed directly against synthetic
// *cobra.Command values.
func TestSkipStencilSeed_HonoursTheAnnotation(t *testing.T) {
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := skipStencilSeed(tt.cmd); got != tt.want {
				t.Errorf("skipStencilSeed(%+v) = %v; want %v", tt.cmd, got, tt.want)
			}
		})
	}
}

// TestReedStatuslineCarriesTheStencilSeedSkipAnnotation walks reedcli.Command()'s subcommands for
// "statusline" and asserts it carries clihelp.SkipStencilSeedAnnotation set to clihelp.AnnotationEnabled.
func TestReedStatuslineCarriesTheStencilSeedSkipAnnotation(t *testing.T) {
	var statusline *cobra.Command
	for _, sub := range reedcli.Command().Commands() {
		if sub.Name() == "statusline" {
			statusline = sub
			break
		}
	}
	if statusline == nil {
		t.Fatal("reedcli.Command() has no \"statusline\" subcommand")
	}
	if got := statusline.Annotations[clihelp.SkipStencilSeedAnnotation]; got != clihelp.AnnotationEnabled {
		t.Errorf("reed statusline Annotations[%q] = %q; want %q -- the annotation was silently dropped, making the gate worthless",
			clihelp.SkipStencilSeedAnnotation, got, clihelp.AnnotationEnabled)
	}
}
