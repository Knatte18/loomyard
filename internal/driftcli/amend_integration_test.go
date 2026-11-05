//go:build !windows

// amend_integration_test.go drives the built `lym drift amend` in a hubforge hub pair: what it refuses before it reads the run, from the operator's unnamed shell.

package driftcli_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
	"github.com/Knatte18/loomyard/internal/testkit/lymbin"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// TestAmend_RefusesWhatItCannotRead covers a slug that names no worktree and an --entries file that cannot be read, each refused naming what it cannot read.
func TestAmend_RefusesWhatItCannotRead(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	bin := lymbin.Build(t)
	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: ".", Pairs: []string{pairSlug}})
	pair := h.PairCodeWorktree(pairSlug)
	missing := filepath.Join(t.TempDir(), "no-such-entries.yaml")
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{"AnUnknownSlug", []string{"amend", "no-such-slug", "--discard"}, []string{"no-such-slug", "lym board list"}},
		{"AMissingEntriesFile", []string{"amend", pairSlug, "--entries", missing}, []string{missing, "readable"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// An empty strand name is the operator's own unnamed shell, whatever session runs this test.
			out, _, err := lymbin.RunWithEnv(bin, pair, []string{agentname.StrandNameEnv + "="}, verbWait, append([]string{"drift"}, tt.args...)...)
			if err != nil {
				t.Fatalf("lym drift %v: %v", tt.args, err)
			}
			env := envelope.Decode(t, out)
			if env.OK {
				t.Fatalf("lym drift %v = ok; want a refusal naming %q", tt.args, tt.want)
			}
			for _, fragment := range tt.want {
				if !strings.Contains(env.Error, fragment) {
					t.Errorf("lym drift %v refused with %q; want it to name %q", tt.args, env.Error, fragment)
				}
			}
		})
	}
}
