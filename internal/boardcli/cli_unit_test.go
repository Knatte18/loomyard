// cli_unit_test.go holds the boardcli CLI tests that never reach layout resolution — no git repo or
// board config is spawned or seeded — so they stay in the untagged Tier 1 loop.
// runCLI lives here (not in cli_test.go) because the untagged build must expose it to the
// integration-tagged cli_test.go in the same package.

package boardcli_test

import (
	"bytes"
	"strings"
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

// TestCLIAliasesHiddenWithShort asserts that notes and promote-note stay out of the help listing while every alias command, including each notes child, still carries a non-empty Short.
func TestCLIAliasesHiddenWithShort(t *testing.T) {
	t.Setenv("BOARD_SKIP_GIT", "1")
	t.Chdir(t.TempDir())

	_, stdout := runCLI(t, "--help")
	for _, alias := range []string{"notes", "promote-note"} {
		for _, line := range strings.Split(stdout, "\n") {
			if fields := strings.Fields(line); len(fields) > 0 && fields[0] == alias {
				t.Errorf("--help lists hidden alias %q: %q", alias, line)
			}
		}
	}

	root := boardcli.Command()
	for _, name := range []string{"notes", "promote-note"} {
		c, _, err := root.Find([]string{name})
		if err != nil || c.Name() != name {
			t.Fatalf("alias %q not found: %v", name, err)
		}
		if !c.Hidden {
			t.Errorf("%q is not hidden", name)
		}
		if c.Short == "" {
			t.Errorf("%q has an empty Short", name)
		}
		for _, child := range c.Commands() {
			if child.Short == "" {
				t.Errorf("%s %s has an empty Short", name, child.Name())
			}
		}
	}
}
