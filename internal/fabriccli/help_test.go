// help_test.go pins the flag surface of the built fabric command tree.
// It inspects cobra commands only, spawning nothing, per the Test Tier Purity Invariant.

package fabriccli_test

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/fabriccli"
)

func TestCommand_CommitDeclaresNoMessageFlag(t *testing.T) {
	t.Parallel()

	commit, _, err := fabriccli.Command().Find([]string{"commit"})
	if err != nil {
		t.Fatalf("Find([commit]) error: %v", err)
	}
	if commit.Flags().Lookup("message") != nil {
		t.Errorf("commit declares a --message flag; the commit message is fixed")
	}
}

func TestCommand_PullAndMergeInDeclareShort(t *testing.T) {
	t.Parallel()

	for _, verb := range []string{"pull", "merge-in"} {
		cmd, _, err := fabriccli.Command().Find([]string{verb})
		if err != nil {
			t.Fatalf("Find([%s]) error: %v", verb, err)
		}
		if cmd.Short == "" {
			t.Errorf("%s.Short is empty; want a non-empty summary", verb)
		}
	}
}
