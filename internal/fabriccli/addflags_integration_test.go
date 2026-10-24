//go:build tier1

// addflags_integration_test.go drives the fabric CLI's `add --parent` and `add --adopt-local` over one real hub, with the refusals of `--local` combined with either.
// Package fabriccli_test, sharing the single TestMain in testmain_test.go.

package fabriccli_test

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/testkit/envelope"
)

// TestFabricAdd_ParentAndAdoptLocal adds a beam, a member forked from it and a pair over an existing local branch, then the two refused `--local` combinations.
// Steps run serially in this order over one hub.
func TestFabricAdd_ParentAndAdoptLocal(t *testing.T) {
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	prime := h.PrimeWorktree()
	const beam = "beam"

	if code, output := runFabric(t, prime, "add", beam); code != 0 {
		t.Fatalf("add %s exit = %d; output: %s", beam, code, output)
	}
	gitkit.CommitFile(t, h.PairCodeWorktree(beam), "beam-only.txt", "beam\n", "commit on the beam")
	beamHead := strings.TrimSpace(gitOutputCLI(t, h.PairCodeWorktree(beam), "rev-parse", "HEAD"))

	t.Run("ParentForksFromThePair", func(t *testing.T) {
		const slug = "member"
		code, output := runFabric(t, prime, "add", slug, "--parent", beam)
		if code != 0 {
			t.Fatalf("add %s --parent %s exit = %d; output: %s", slug, beam, code, output)
		}
		envelope.RequireOK(t, output)
		if got := strings.TrimSpace(gitOutputCLI(t, h.PairCodeWorktree(slug), "rev-parse", "HEAD")); got != beamHead {
			t.Errorf("member HEAD = %s; want the beam's HEAD %s", got, beamHead)
		}
		origin, found, err := fabricengine.ReadOriginFor(h.Location, slug)
		if err != nil || !found || origin.ParentWorktree != beam {
			t.Errorf("ReadOriginFor(%q) = (%+v, %v, %v); want a record naming %q as the parent worktree", slug, origin, found, err, beam)
		}
	})

	t.Run("AdoptLocalAdoptsTheBranch", func(t *testing.T) {
		const slug = "adopted"
		defaultBranch := strings.TrimSpace(gitOutputCLI(t, prime, "rev-parse", "--abbrev-ref", "HEAD"))
		gitkit.MustRun(t, prime, "git", "switch", "-c", slug)
		gitkit.CommitFile(t, prime, "adopted.txt", "adopted\n", "commit on the local branch")
		commit := strings.TrimSpace(gitOutputCLI(t, prime, "rev-parse", "HEAD"))
		gitkit.MustRun(t, prime, "git", "switch", defaultBranch)

		if code, output := runFabric(t, prime, "add", slug); code == 0 {
			t.Fatalf("add %s over an existing local branch exit = 0; want a refusal\noutput: %s", slug, output)
		}
		code, output := runFabric(t, prime, "add", slug, "--adopt-local")
		if code != 0 {
			t.Fatalf("add %s --adopt-local exit = %d; output: %s", slug, code, output)
		}
		envelope.RequireOK(t, output)
		if got := strings.TrimSpace(gitOutputCLI(t, h.PairCodeWorktree(slug), "rev-parse", "HEAD")); got != commit {
			t.Errorf("adopted pair HEAD = %s; want the branch's commit %s", got, commit)
		}
	})

	t.Run("LocalRefusesBothFlags", func(t *testing.T) {
		tests := []struct {
			name string
			flag []string
			want string
		}{
			{"Parent", []string{"--parent", beam}, "--parent"},
			{"AdoptLocal", []string{"--adopt-local"}, "--adopt-local"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				slug := "local-" + strings.ToLower(tt.name)
				code, output := runFabric(t, prime, append([]string{"add", slug, "--local"}, tt.flag...)...)
				if code == 0 {
					t.Fatalf("add --local %v exit = 0; want a refusal\noutput: %s", tt.flag, output)
				}
				envelope.RequireErr(t, output, tt.want)
				if out := strings.TrimSpace(gitOutputCLI(t, prime, "branch", "--list", slug)); out != "" {
					t.Errorf("branch %s exists after the refusal: %q; want none", slug, out)
				}
			})
		}
	})
}
