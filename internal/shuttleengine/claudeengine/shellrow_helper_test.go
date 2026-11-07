// shellrow_helper_test.go holds the shell guard row lookup the tier0 settings tests and the tier1 hook command tests share.
// It carries no build tag, so it compiles into every tag set.

package claudeengine

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/shellguard"
)

// sampleRefuseRaw is the non-empty refuse list the deny tests render the shell guard rows with.
var sampleRefuseRaw = []string{"go test", "dotnet test"}

// shellRow returns the row named name from shellguard.Rows(bashForegroundPhrase, sampleRefuseRaw).
// It fails the test when no row has that name.
func shellRow(t *testing.T, name string) shellguard.Row {
	t.Helper()
	for _, row := range shellguard.Rows(bashForegroundPhrase, sampleRefuseRaw) {
		if row.Name == name {
			return row
		}
	}
	t.Fatalf("shellguard.Rows has no row named %q", name)
	return shellguard.Row{}
}
