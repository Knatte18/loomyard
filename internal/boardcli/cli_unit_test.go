// cli_unit_test.go holds the boardcli CLI tests that never reach layout resolution — no git repo or
// board config is spawned or seeded — so they stay in the untagged Tier 1 loop.
// runCLI lives here (not in cli_test.go) because the untagged build must expose it to the
// integration-tagged cli_test.go in the same package.

package boardcli_test

import (
	"bytes"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardcli"
)

// runCLI invokes boardcli.RunCLI in-process and returns the exit code plus the JSON
// written to out. Caller must have called seedCwd (or otherwise set up cwd and the
// git repo) before calling runCLI. BOARD_SKIP_GIT must be set by the caller.
func runCLI(t *testing.T, args ...string) (exitCode int, stdout string) {
	t.Helper()

	var buf bytes.Buffer
	code := boardcli.RunCLI(&buf, args)
	return code, buf.String()
}
