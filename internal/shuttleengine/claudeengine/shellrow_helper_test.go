// shellrow_helper_test.go holds the shell guard row lookup the tier0 settings tests and the tier1 hook command tests share.
// It carries no build tag, so it compiles into every tag set.

package claudeengine

import (
	"testing"

	"github.com/Knatte18/loomyard/internal/shellguard"
)

// sampleRefuseRaw is the non-empty refuse list the deny tests render the shell guard rows with.
var sampleRefuseRaw = []string{"go test", "dotnet test"}

// shellRow returns the shellguard.Rows(bashForegroundPhrase, sampleRefuseRaw) row named name, failing the test when none has it.
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
