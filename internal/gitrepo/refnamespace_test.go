// refnamespace_test.go covers RefNamespace's name mapping offline: it spawns no git.

package gitrepo_test

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestRefNamespace(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	namespaced := gitrepo.RefNamespace{Heads: "refs/lym/heads/", Tags: "refs/lym/tags/"}
	tests := []struct {
		name          string
		ns            gitrepo.RefNamespace
		wantBranch    string
		wantTag       string
		localRef      string
		wantLocalRef  string
		wantLocalOkay bool
	}{
		{"IdentityHead", gitrepo.RefNamespace{}, "refs/heads/feat", "refs/tags/feat", "refs/heads/feat", "refs/heads/feat", true},
		{"IdentityOutside", gitrepo.RefNamespace{}, "refs/heads/feat", "refs/tags/feat", "refs/lym/heads/feat", "", false},
		{"NamespacedHead", namespaced, "refs/lym/heads/feat", "refs/lym/tags/feat", "refs/lym/heads/feat", "refs/heads/feat", true},
		{"NamespacedTag", namespaced, "refs/lym/heads/feat", "refs/lym/tags/feat", "refs/lym/tags/archive/s/abc", "refs/tags/archive/s/abc", true},
		{"NamespacedOutside", namespaced, "refs/lym/heads/feat", "refs/lym/tags/feat", "refs/heads/feat", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.ns.Branch("feat"); got != tt.wantBranch {
				t.Errorf("Branch(feat) = %q; want %q", got, tt.wantBranch)
			}
			if got := tt.ns.Tag("feat"); got != tt.wantTag {
				t.Errorf("Tag(feat) = %q; want %q", got, tt.wantTag)
			}
			got, ok := tt.ns.LocalRef(tt.localRef)
			if got != tt.wantLocalRef || ok != tt.wantLocalOkay {
				t.Errorf("LocalRef(%q) = (%q, %v); want (%q, %v)", tt.localRef, got, ok, tt.wantLocalRef, tt.wantLocalOkay)
			}
		})
	}
}
