// carddependents_integration_test.go covers a --card scope's module whose dependent packages cannot be listed.
// It spawns `go list` in a temp directory, so it is Tier 1.

package gatecli

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/testkit/plankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

func TestCardDependents_UnlistableModuleKeepsTheCardsPackagesAndSaysSo(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	module := planparser.ModulePackages{Dir: ".", Packages: []string{"./lib"}}
	root := t.TempDir()
	plankit.WriteTree(t, root, map[string]string{"lib/lib.go": "package lib\n"})
	var stderr bytes.Buffer

	if got, want := cardDependents(context.Background(), root, module, &stderr), []string{"./lib"}; !slices.Equal(got, want) {
		t.Errorf("cardDependents() = %q; want %q", got, want)
	}
	want := "gate test: the card scope holds the cards' packages alone: cannot list the packages that depend on them in .: "
	if got := stderr.String(); !strings.HasPrefix(got, want) {
		t.Errorf("cardDependents() stderr = %q; want it to open with %q", got, want)
	}
}
