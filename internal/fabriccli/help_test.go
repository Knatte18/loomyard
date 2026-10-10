// help_test.go pins the declared surface of the built fabric command tree: every verb's summary and the commit verb's flag set.
// It inspects cobra commands only, spawning nothing, per the Test Tier Purity Invariant.

package fabriccli_test

import (
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabriccli"
)

// TestCommand_VerbDeclarations asserts every verb declares a non-empty Short and the commit verb declares no --message flag.
//
//testtiming:keep pins that every verb carries a summary and that commit's message stays fixed, assertions the arity test that covers its blocks does not make
func TestCommand_VerbDeclarations(t *testing.T) {
	t.Parallel()

	t.Run("EveryVerbDeclaresShort", func(t *testing.T) {
		t.Parallel()

		for _, sub := range fabriccli.Command().Commands() {
			if name := sub.Name(); name == "help" || name == "completion" {
				continue
			}
			if sub.Short == "" {
				t.Errorf("%s.Short is empty; want a non-empty summary", sub.Name())
			}
		}
	})

	t.Run("AddSaysItPushesToOrigin", func(t *testing.T) {
		t.Parallel()

		add, _, err := fabriccli.Command().Find([]string{"add"})
		if err != nil {
			t.Fatalf("Find([add]) error: %v", err)
		}
		if !strings.Contains(add.Long, "pushes the new code branch and the records branches to origin") {
			t.Errorf("add.Long = %q; want it to say the verb pushes to origin", add.Long)
		}
	})

	t.Run("CommitDeclaresNoMessageFlag", func(t *testing.T) {
		t.Parallel()

		commit, _, err := fabriccli.Command().Find([]string{"commit"})
		if err != nil {
			t.Fatalf("Find([commit]) error: %v", err)
		}
		if commit.Flags().Lookup("message") != nil {
			t.Errorf("commit declares a --message flag; the commit message is fixed")
		}
	})
}
