// tokenvocab_test.go is the hermetic unit test suite for tokenvocab: each
// registry token's Resolve, Build's aggregate output, Render's happy path and its
// propagated unfilled-marker error, and a demonstration of the "one registry entry
// per token" extension rule. Every case builds a hubgeometry.Layout struct literal
// directly — never hubgeometry.Resolve — so this suite stays untagged and
// spawn-free (Test Tier Purity). It is a same-package test so the token-by-token
// cases can inspect the unexported registry directly, rather than only observing
// it through Build's aggregated map.

package tokenvocab

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/hubgeometry"
)

// tokenByName returns the registry entry named name, failing the test if no such
// entry exists — a small helper so the per-token Resolve cases below read the
// production registry rather than duplicating its literal values.
func tokenByName(t *testing.T, name string) Token {
	t.Helper()
	for _, token := range registry {
		if token.Name == name {
			return token
		}
	}
	t.Fatalf("registry has no token named %q", name)
	return Token{}
}

// TestTokenResolve covers each registry token's Resolve function, asserting it
// reads its own matching Layout field and not some other field of the same
// struct.
func TestTokenResolve(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		tokenName string
		layout    *hubgeometry.Layout
		want      string
	}{
		{
			name:      "repo reads Layout.Repo",
			tokenName: "repo",
			layout:    &hubgeometry.Layout{Repo: "loomyard", Hub: "unrelated-hub-value"},
			want:      "loomyard",
		},
		{
			name:      "hub reads Layout.Hub",
			tokenName: "hub",
			layout:    &hubgeometry.Layout{Repo: "unrelated-repo-value", Hub: "/hub/loomyard-HUB"},
			want:      "/hub/loomyard-HUB",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			token := tokenByName(t, tt.tokenName)
			got := token.Resolve(Ctx{Layout: tt.layout})
			if got != tt.want {
				t.Errorf("token %q Resolve() = %q; want %q", tt.tokenName, got, tt.want)
			}
		})
	}
}

// TestTokenResolve_RepoFromEmptyPrimeFallback verifies the repo token reflects
// Layout.Repo exactly even when Repo was populated via deriveRepo's empty-Prime
// fallback (hubgeometry's fallback branch, added in batch 1) rather than from a
// resolved Prime worktree — the token has no opinion about how Repo was derived,
// only that it reads the field verbatim.
func TestTokenResolve_RepoFromEmptyPrimeFallback(t *testing.T) {
	t.Parallel()

	// A Layout as hubgeometry.Resolve would produce when Prime resolves empty:
	// deriveRepo falls back to filepath.Base(WorktreeRoot), so Repo lands here
	// with no Prime involved at all.
	layout := &hubgeometry.Layout{Repo: "feature-branch", Prime: ""}

	got := tokenByName(t, "repo").Resolve(Ctx{Layout: layout})
	if got != "feature-branch" {
		t.Errorf("repo token Resolve() = %q; want %q", got, "feature-branch")
	}
}

// TestBuild_ReturnsBothKeys verifies Build resolves the full registry into a flat
// map keyed by token name, with both current tokens (repo, hub) present and
// correctly valued.
func TestBuild_ReturnsBothKeys(t *testing.T) {
	t.Parallel()

	layout := &hubgeometry.Layout{Repo: "loomyard", Hub: "/hub/loomyard-HUB"}
	got := Build(Ctx{Layout: layout})

	want := map[string]string{"repo": "loomyard", "hub": "/hub/loomyard-HUB"}
	if len(got) != len(want) {
		t.Fatalf("Build() returned %d keys; want %d: %+v", len(got), len(want), got)
	}
	for name, wantValue := range want {
		if got[name] != wantValue {
			t.Errorf("Build()[%q] = %q; want %q", name, got[name], wantValue)
		}
	}
}

// TestRender_FillsTemplateVerbatim verifies Render fills a two-marker template
// with the resolved vocabulary, byte-for-byte.
func TestRender_FillsTemplateVerbatim(t *testing.T) {
	t.Parallel()

	layout := &hubgeometry.Layout{Repo: "loomyard", Hub: "/hub/loomyard-HUB"}
	template := []byte("{{.hub}}/{{.repo}}")

	got, err := Render(template, Ctx{Layout: layout})
	if err != nil {
		t.Fatalf("Render() unexpected error: %v", err)
	}

	want := "/hub/loomyard-HUB/loomyard"
	if string(got) != want {
		t.Errorf("Render() = %q; want %q", string(got), want)
	}
}

// TestRender_PropagatesUnknownTokenError verifies Render surfaces stencil.Fill's
// unfilled-top-level-marker error unchanged for a template referencing a token
// the registry does not define (e.g. the deferred "slug" token), rather than
// swallowing or rewording it.
func TestRender_PropagatesUnknownTokenError(t *testing.T) {
	t.Parallel()

	layout := &hubgeometry.Layout{Repo: "loomyard", Hub: "/hub/loomyard-HUB"}
	template := []byte("{{.slug}}")

	_, err := Render(template, Ctx{Layout: layout})
	if err == nil {
		t.Fatal("Render() got nil error for an unknown top-level token; want an error")
	}
	wantSubstr := "unfilled top-level marker(s): slug"
	if !strings.Contains(err.Error(), wantSubstr) {
		t.Errorf("Render() error = %q; want substring %q", err.Error(), wantSubstr)
	}
}

// TestRegistry_AddingATokenIsOneEntry documents the "one registry entry per
// token" extension rule from doc.go. It builds a throwaway registry-shaped slice
// — copied from the real registry, never mutating it — appends a single
// hypothetical {Name, Resolve} entry, and resolves it the same way Build resolves
// the production registry, showing that one entry is sufficient to introduce a
// brand-new token.
func TestRegistry_AddingATokenIsOneEntry(t *testing.T) {
	t.Parallel()

	hypothetical := append(append([]Token{}, registry...), Token{
		Name:    "slug",
		Resolve: func(c Ctx) string { return "example-slug" },
	})

	layout := &hubgeometry.Layout{Repo: "loomyard", Hub: "/hub/loomyard-HUB"}
	got := make(map[string]string, len(hypothetical))
	for _, token := range hypothetical {
		got[token.Name] = token.Resolve(Ctx{Layout: layout})
	}

	want := map[string]string{"repo": "loomyard", "hub": "/hub/loomyard-HUB", "slug": "example-slug"}
	if len(got) != len(want) {
		t.Fatalf("hypothetical registry resolved to %d keys; want %d: %+v", len(got), len(want), got)
	}
	for name, wantValue := range want {
		if got[name] != wantValue {
			t.Errorf("hypothetical registry[%q] = %q; want %q", name, got[name], wantValue)
		}
	}
}
