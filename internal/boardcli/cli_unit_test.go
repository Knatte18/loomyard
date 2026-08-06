// cli_unit_test.go holds the boardcli CLI tests that never reach layout
// resolution — no git repo or board config is spawned or seeded — so they
// stay in the untagged Tier 1 loop. runCLI lives here (not in cli_test.go)
// because the untagged build must expose it to the integration-tagged
// cli_test.go in the same package.

package boardcli_test

import (
	"bytes"
	"encoding/json"
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

// TestCLINoArg asserts that invoking board with no subcommand exits 0 and
// lists available subcommands in the output. Under cobra, the no-arg parent
// command prints usage/help and exits cleanly — no config resolution is attempted.
func TestCLINoArg(t *testing.T) {
	t.Setenv("BOARD_SKIP_GIT", "1")

	// No-arg board does not require a seeded cwd because PersistentPreRunE is
	// not invoked when no subcommand is given — cobra handles the listing path.
	cwd := t.TempDir()
	t.Chdir(cwd)

	exitCode, stdout := runCLI(t)

	if exitCode != 0 {
		t.Errorf("RunCLI() exit = %d; want 0\nstdout: %s", exitCode, stdout)
	}

	// cobra's usage output lists subcommand names; verify at least one known
	// subcommand name appears so we know a real listing was printed.
	if !strings.Contains(stdout, "upsert") {
		t.Errorf("no-arg output does not list subcommands; stdout: %s", stdout)
	}
}

// TestCLIUnknownSubcommand asserts that an unknown subcommand exits 1 and emits
// a JSON error envelope with ok=false. GroupRunE handles the unknown-subcommand
// path, so the output is always a machine-parseable JSON envelope.
func TestCLIUnknownSubcommand(t *testing.T) {
	t.Setenv("BOARD_SKIP_GIT", "1")

	// GroupRunE fires before PersistentPreRunE reaches layout resolution, so a
	// plain temp dir (no board config, no git repo) is sufficient.
	cwd := t.TempDir()
	t.Chdir(cwd)

	exitCode, stdout := runCLI(t, "no-such-subcommand")

	if exitCode != 1 {
		t.Errorf("RunCLI(unknown) exit = %d; want 1\nstdout: %s", exitCode, stdout)
	}

	// GroupRunE wraps the error in a JSON envelope; parse and assert ok=false.
	var env map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &env); err != nil {
		t.Fatalf("RunCLI(unknown) output is not valid JSON: %v; stdout: %q", err, stdout)
	}
	if ok, _ := env["ok"].(bool); ok {
		t.Errorf("RunCLI(unknown) ok = true; want false")
	}
	// The error text contains "unknown" (GroupRunE produces "unknown subcommand").
	if errMsg, _ := env["error"].(string); !strings.Contains(errMsg, "unknown") {
		t.Errorf("RunCLI(unknown) error = %q; want \"unknown\" substring", errMsg)
	}
}
