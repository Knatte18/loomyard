// cli_test.go covers quarrycli's cobra seam: every verb's Short being non-empty, Command()'s exact
// child set, and the bare-group listing's git-free guard. Per-verb answer coverage lives in
// verbs_test.go.

package quarrycli

import (
	"bytes"
	"strings"
	"testing"
)

// TestCommand_ChildrenHaveNonEmptyShort asserts every direct child command of Command() carries a
// non-empty Short, the CLI/Cobra Invariant's per-command requirement.
func TestCommand_ChildrenHaveNonEmptyShort(t *testing.T) {
	cmd := Command()
	for _, child := range cmd.Commands() {
		if child.Short == "" {
			t.Errorf("child command %q has an empty Short", child.Name())
		}
	}
}

// TestCommand_ExactChildSet asserts Command()'s child set is exactly the four verbs: toc, glyphs,
// resolve, and expand -- no more, no fewer.
func TestCommand_ExactChildSet(t *testing.T) {
	cmd := Command()

	want := map[string]bool{"toc": true, "glyphs": true, "resolve": true, "expand": true}
	got := map[string]bool{}
	for _, child := range cmd.Commands() {
		got[child.Name()] = true
	}

	for name := range want {
		if !got[name] {
			t.Errorf("Command() is missing child %q", name)
		}
	}
	for name := range got {
		if !want[name] {
			t.Errorf("Command() has unexpected child %q", name)
		}
	}
}

// TestRunCLI_NoArgs verifies that "lyx quarry" with no subcommand lists all four registered verbs
// and exits 0 without requiring a git repository, since the parent's PersistentPreRunE skips
// resolution when cmd.Name() is "quarry".
func TestRunCLI_NoArgs(t *testing.T) {
	t.Chdir(t.TempDir())

	var out bytes.Buffer
	exitCode := RunCLI(&out, nil)

	if exitCode != 0 {
		t.Errorf("RunCLI(nil) = %d; want 0", exitCode)
	}

	got := out.String()
	for _, sub := range []string{"toc", "glyphs", "resolve", "expand"} {
		if !strings.Contains(got, sub) {
			t.Errorf("RunCLI(nil) no-arg listing missing subcommand %q; got:\n%s", sub, got)
		}
	}
}

// TestRunCLI_UnknownSubcommand verifies that an unknown subcommand exits 1 and emits a JSON error
// envelope, requiring no git repository either.
func TestRunCLI_UnknownSubcommand(t *testing.T) {
	t.Chdir(t.TempDir())

	var out bytes.Buffer
	exitCode := RunCLI(&out, []string{"bogus"})

	if exitCode != 1 {
		t.Errorf("RunCLI(bogus) = %d; want 1", exitCode)
	}
	if !strings.Contains(out.String(), "unknown") {
		t.Errorf("RunCLI(bogus) output missing \"unknown\"; got: %q", out.String())
	}
}
