// cli_unit_test.go holds the boardcli CLI tests that never reach layout resolution.
// They spawn no git repo and seed no board config, so they stay in the untagged Tier 1 loop.
// runCLI lives here because the untagged build must expose it to the integration-tagged cli_test.go.

package boardcli_test

import (
	"bytes"
	"testing"

	"github.com/Knatte18/loomyard/internal/boardcli"
)

// runCLI invokes boardcli.RunCLI in-process and returns the exit code plus the JSON the command wrote.
// The caller must have called seedCwd, or otherwise set up the cwd and the git repo, before calling runCLI.
// BOARD_SKIP_GIT must be set by the caller.
func runCLI(t *testing.T, args ...string) (exitCode int, stdout string) {
	t.Helper()

	var buf bytes.Buffer
	code := boardcli.RunCLI(&buf, args)
	return code, buf.String()
}
