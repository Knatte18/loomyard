// tokenvocab_test.go is the hermetic unit test suite for tokenvocab: each registry token's Resolve,
// Build's aggregate output, Render's happy path and its propagated unfilled-marker error, and a
// demonstration of the "one registry entry per token" extension rule.
// Every case builds a Ctx struct literal directly with distinct literal values for RepoName and
// HubPath — never lyxcwd.Resolve — so this suite stays untagged and spawn-free (Test Tier Purity).
// It is a same-package test so the token-by-token cases can inspect the unexported registry
// directly, rather than only observing it through Build's aggregated map.

package tokenvocab

import (
	"strings"
	"testing"
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

// TestTokenResolve covers each registry token's Resolve function, asserting it reads its own
// matching Ctx field and not some other field of the same struct.
func TestTokenResolve(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		tokenName string
		ctx       Ctx
		want      string
	}{
		{
			name:      "repo reads Ctx.RepoName",
			tokenName: "repo",
			ctx:       Ctx{RepoName: "loomyard", HubPath: "unrelated-hub-value"},
			want:      "loomyard",
		},
		{
			name:      "hub reads Ctx.HubPath",
			tokenName: "hub",
			ctx:       Ctx{RepoName: "unrelated-repo-value", HubPath: "/hub/loomyard-LYXHUB"},
			want:      "/hub/loomyard-LYXHUB",
		},
		{
			name:      "worktree reads Ctx.WorktreeName",
			tokenName: "worktree",
			ctx:       Ctx{RepoName: "unrelated-repo-value", HubPath: "unrelated-hub-value", WorktreeName: "reed-header-selvage"},
			want:      "reed-header-selvage",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			token := tokenByName(t, tt.tokenName)
			got := token.Resolve(tt.ctx)
			if got != tt.want {
				t.Errorf("token %q Resolve() = %q; want %q", tt.tokenName, got, tt.want)
			}
		})
	}
}

// TestBuild_ReturnsAllThreeKeys verifies Build resolves the full registry into a flat map keyed by
// token name, with all three current tokens (repo, hub, worktree) present and correctly valued.
func TestBuild_ReturnsAllThreeKeys(t *testing.T) {
	t.Parallel()

	ctx := Ctx{RepoName: "loomyard", HubPath: "/hub/loomyard-LYXHUB", WorktreeName: "reed-header-selvage"}
	got := Build(ctx)

	want := map[string]string{"repo": "loomyard", "hub": "/hub/loomyard-LYXHUB", "worktree": "reed-header-selvage"}
	if len(got) != len(want) {
		t.Fatalf("Build() returned %d keys; want %d: %+v", len(got), len(want), got)
	}
	for name, wantValue := range want {
		if got[name] != wantValue {
			t.Errorf("Build()[%q] = %q; want %q", name, got[name], wantValue)
		}
	}
}

// TestRender covers Render's happy path and its propagated error.
// A two-marker template is filled with the resolved vocabulary, byte-for-byte.
// A template referencing a token the registry does not define (e.g. the deferred "slug" token) surfaces stencil.Fill's unfilled-top-level-marker error unchanged, rather than swallowing or rewording it.
func TestRender(t *testing.T) {
	t.Parallel()

	ctx := Ctx{RepoName: "loomyard", HubPath: "/hub/loomyard-LYXHUB"}
	tests := []struct {
		name        string
		template    string
		want        string
		wantErrWith string
	}{
		{"fills template verbatim", "{{.hub}}/{{.repo}}", "/hub/loomyard-LYXHUB/loomyard", ""},
		{"propagates unknown token error", "{{.slug}}", "", "unfilled top-level marker(s): slug"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := Render([]byte(tt.template), ctx)
			if tt.wantErrWith != "" {
				if err == nil {
					t.Fatalf("Render(%q) got nil error; want an error containing %q", tt.template, tt.wantErrWith)
				}
				if !strings.Contains(err.Error(), tt.wantErrWith) {
					t.Errorf("Render(%q) error = %q; want substring %q", tt.template, err.Error(), tt.wantErrWith)
				}
				return
			}
			if err != nil {
				t.Fatalf("Render(%q) unexpected error: %v", tt.template, err)
			}
			if string(got) != tt.want {
				t.Errorf("Render(%q) = %q; want %q", tt.template, string(got), tt.want)
			}
		})
	}
}

// TestRegistry_AddingATokenIsOneEntry documents the "one registry entry per token" extension rule
// from doc.go.
// It builds a throwaway registry-shaped slice — copied from the real registry, never mutating it —
// appends a single hypothetical {Name, Resolve} entry, and resolves it the same way Build resolves
// the production registry, showing that one entry is sufficient to introduce a brand-new token.
func TestRegistry_AddingATokenIsOneEntry(t *testing.T) {
	t.Parallel()

	hypothetical := append(append([]Token{}, registry...), Token{
		Name:    "slug",
		Resolve: func(c Ctx) string { return "example-slug" },
	})

	ctx := Ctx{RepoName: "loomyard", HubPath: "/hub/loomyard-LYXHUB", WorktreeName: "reed-header-selvage"}
	got := make(map[string]string, len(hypothetical))
	for _, token := range hypothetical {
		got[token.Name] = token.Resolve(ctx)
	}

	want := map[string]string{"repo": "loomyard", "hub": "/hub/loomyard-LYXHUB", "worktree": "reed-header-selvage", "slug": "example-slug"}
	if len(got) != len(want) {
		t.Fatalf("hypothetical registry resolved to %d keys; want %d: %+v", len(got), len(want), got)
	}
	for name, wantValue := range want {
		if got[name] != wantValue {
			t.Errorf("hypothetical registry[%q] = %q; want %q", name, got[name], wantValue)
		}
	}
}
