// seedlocalbares_integration_test.go pins the bares SeedLocalBares and SeedLocalOrigin build,
// and that CloneHub clones a hub from each.

package fabricengine_test

import (
	"os"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestSeedLocalBares(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	warp, weft, err := fabricengine.SeedLocalBares(t.TempDir())
	if err != nil {
		t.Fatalf("SeedLocalBares() error = %v", err)
	}
	for _, bare := range []string{warp, weft} {
		if info, err := os.Stat(bare); err != nil || !info.IsDir() {
			t.Fatalf("bare %s is not a directory: %v", bare, err)
		}
	}

	if got := gitOutput(t, warp, "rev-parse", "--is-bare-repository"); got != "true" {
		t.Errorf("warp is bare = %q; want true", got)
	}
	if got := gitOutput(t, warp, "symbolic-ref", "HEAD"); got != "refs/heads/main" {
		t.Errorf("warp HEAD = %q; want refs/heads/main", got)
	}
	if got := gitOutput(t, warp, "ls-tree", "--name-only", "main"); got != "README" {
		t.Errorf("warp main holds %q; want only README", got)
	}
	if got := gitOutput(t, warp, "rev-list", "--count", "main"); got != "1" {
		t.Errorf("warp main commit count = %q; want 1", got)
	}
	if got := gitOutput(t, weft, "for-each-ref"); got != "" {
		t.Errorf("weft refs = %q; want none", got)
	}

	if _, err := fabricengine.CloneHub(t.TempDir(), fabriclayout.CloneOptions{WeftURL: weft, WarpURL: warp, Shortname: "tst"}); err != nil {
		t.Errorf("CloneHub over the seeded bares error = %v", err)
	}
}

func TestSeedLocalOrigin(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	origin, err := fabricengine.SeedLocalOrigin(t.TempDir())
	if err != nil {
		t.Fatalf("SeedLocalOrigin() error = %v", err)
	}
	if got := gitOutput(t, origin, "rev-parse", "--is-bare-repository"); got != "true" {
		t.Errorf("origin is bare = %q; want true", got)
	}
	if got := gitOutput(t, origin, "ls-tree", "--name-only", "main"); got != "README" {
		t.Errorf("origin main holds %q; want only README", got)
	}

	if _, err := fabricengine.CloneHub(t.TempDir(), fabriclayout.CloneOptions{WeftURL: origin, Shortname: "tst", SingleRepo: true}); err != nil {
		t.Errorf("CloneHub of a one-repository hub over the seeded origin error = %v", err)
	}
}
