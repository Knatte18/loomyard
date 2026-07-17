// cli_test.go drives RunCLI through its seam: the bare/--help subcommand listing,
// every command's Short, and the ErrNoLanguage error-envelope path. It is
// deliberately untagged, offline, and spawn-free: it never shells out to a
// subprocess, never touches git, and never copies a fixture tree, so it never
// launches a language server or requires a git repo. A real "refs" query against a
// live language server belongs to the //go:build integration tier
// (internal/codeintelengine's own integration test) and batch 4's measurement, not
// here.

package codeintelcli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// TestRunCLI_NoArgsListsRefsSubcommand verifies that "lyx codeintel" with no
// subcommand lists the "refs" subcommand and exits 0 — matching every other
// module group's bare-invocation behavior (clihelp.GroupRunE).
func TestRunCLI_NoArgsListsRefsSubcommand(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	exitCode := RunCLI(&out, []string{})

	if exitCode != 0 {
		t.Errorf("RunCLI() = %d; want 0 for no-arg listing", exitCode)
	}
	if got := out.String(); !strings.Contains(got, "refs") {
		t.Errorf("RunCLI() no-arg output missing subcommand %q; got: %q", "refs", got)
	}
}

// TestRunCLI_Help verifies that "lyx codeintel --help" also lists the "refs"
// subcommand and exits 0, mirroring the bare-invocation assertion above for the
// explicit --help path.
func TestRunCLI_Help(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	exitCode := RunCLI(&out, []string{"--help"})

	if exitCode != 0 {
		t.Errorf("RunCLI(--help) = %d; want 0", exitCode)
	}
	if got := out.String(); !strings.Contains(got, "refs") {
		t.Errorf("RunCLI(--help) output missing subcommand %q; got: %q", "refs", got)
	}
}

// TestCommand_EveryCommandHasShort walks the full command tree returned by
// Command() and asserts every node (the "codeintel" group and each subcommand)
// carries a non-empty Short — the same structural self-documentation contract
// cmd/lyx/drift_test.go enforces repo-wide, checked locally here so this module's
// own test suite catches a missing Short before the root-level guard would.
func TestCommand_EveryCommandHasShort(t *testing.T) {
	t.Parallel()

	violations := collectMissingShorts(Command())
	for _, v := range violations {
		t.Errorf("command %q has no Short description", v)
	}
}

// collectMissingShorts performs a depth-first walk of the command tree rooted at
// cmd and returns the command path of every node whose Short is empty.
func collectMissingShorts(cmd *cobra.Command) []string {
	var violations []string
	if cmd.Short == "" {
		violations = append(violations, cmd.CommandPath())
	}
	for _, child := range cmd.Commands() {
		violations = append(violations, collectMissingShorts(child)...)
	}
	return violations
}

// TestRunCLI_Refs_NoLanguageError verifies that "refs <symbol> --target-dir
// <empty dir>" fails through the ErrNoLanguage path: an empty temp dir has no
// registry markers, so DetectLanguage fails before References ever launches a
// language server. This exercises the engine-error-to-output.Err mapping without
// any subprocess spawn.
func TestRunCLI_Refs_NoLanguageError(t *testing.T) {
	// Chdir into a fresh, non-git temp dir so hubgeometry.Resolve degrades to
	// codeintelengine.BuiltinRegistry() deterministically, independent of
	// whatever git repo or servers.yaml the test happens to run inside.
	t.Chdir(t.TempDir())

	emptyTargetDir := t.TempDir()

	var out bytes.Buffer
	exitCode := RunCLI(&out, []string{"refs", "MySymbol", "--target-dir", emptyTargetDir})

	if exitCode == 0 {
		t.Fatalf("RunCLI(refs MySymbol --target-dir <empty>) = 0; want non-zero exit for ErrNoLanguage")
	}

	// Assert the JSON envelope shape: exactly one object on one line, ok=false,
	// and a populated, non-empty error field.
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("RunCLI output has %d lines; want exactly 1. output:\n%s", len(lines), out.String())
	}

	var env map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &env); err != nil {
		t.Fatalf("RunCLI output is not valid JSON: %v; got: %q", err, lines[0])
	}

	if ok, _ := env["ok"].(bool); ok {
		t.Errorf("RunCLI(refs MySymbol --target-dir <empty>) ok = true; want false")
	}

	errMsg, _ := env["error"].(string)
	if errMsg == "" {
		t.Errorf("RunCLI(refs MySymbol --target-dir <empty>) error field empty or missing; got envelope: %v", env)
	}
	if !strings.Contains(errMsg, "no language detected") {
		t.Errorf("RunCLI(refs MySymbol --target-dir <empty>) error = %q; want it to mention ErrNoLanguage's \"no language detected\"", errMsg)
	}
}

// TestRunCLI_Refs_RequiresExactlyOneArg verifies that Args: cobra.ExactArgs(1)
// rejects both a bare "refs" call and a 2-arg call through the same JSON error
// envelope, without touching detection or the registry at all.
func TestRunCLI_Refs_RequiresExactlyOneArg(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
	}{
		{"bare", []string{"refs"}},
		{"two_args", []string{"refs", "one", "two"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			exitCode := RunCLI(&out, tt.args)

			if exitCode == 0 {
				t.Fatalf("RunCLI(%v) = 0; want non-zero exit for arg-count violation", tt.args)
			}

			var env map[string]any
			if err := json.Unmarshal([]byte(strings.TrimSpace(out.String())), &env); err != nil {
				t.Fatalf("RunCLI(%v) output is not valid JSON: %v; got: %q", tt.args, err, out.String())
			}
			if ok, _ := env["ok"].(bool); ok {
				t.Errorf("RunCLI(%v) ok = true; want false", tt.args)
			}
		})
	}
}
