// recordslayout_refnames_integration_test.go covers OriginRefNamesAt against real repositories of each layout.

package fabricengine

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestOriginRefNamesAt_NamesTheRefsTheStoresOriginHolds(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	for _, tt := range []struct {
		name       string
		namespaced bool
		wantBranch string
		wantTag    string
	}{
		{"OneRepositoryStoreAnswersItsNamespace", true, "refs/lym/heads/feat", "refs/lym/tags/archive/s/abc"},
		{"OtherStoreAnswersTheDefaultRefs", false, "refs/heads/feat", "refs/tags/archive/s/abc"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			bare := t.TempDir()
			gitkit.Git(t, bare, "init", "--bare", "-b", "main")
			dir := filepath.Join(t.TempDir(), "store")
			newLayoutStore(t, dir, bare, tt.namespaced)

			branchRef, tagRef, err := OriginRefNamesAt(dir, "feat", "archive/s/abc")

			if err != nil || branchRef != tt.wantBranch || tagRef != tt.wantTag {
				t.Errorf("OriginRefNamesAt() = (%q, %q, %v); want (%q, %q, nil)", branchRef, tagRef, err, tt.wantBranch, tt.wantTag)
			}
		})
	}
}
