// cli_test.go covers Command's bare-listing, unknown-subcommand, default-flag, unknown-recipe and
// unsupported-verb-refusal behaviour. It stays untagged tier 1 for a reason narrower than "never
// calls RunCLIIn": it does call RunCLIIn(t.TempDir(), …), because RunCLI delegates to
// RunCLIIn("", …) and therefore reads the process cwd -- the package source directory, which is
// inside this repo's worktree -- so an injected non-git cwd is reachable only through the
// cwd-carrying seam. What keeps every case here tier 1 is that each one short-circuits before Arm
// and therefore before lyxcwd.Resolve, which is the one call in this whole subtree that spawns git:
// the bare listing exits at the cmd.Name() == "shed" guard, the unknown-recipe case at lookup, and
// the unsupported-verb case at the Verbs check.

package shedcli

import (
	"bytes"
	"strings"
	"testing"
)

// TestCommand_RecipeFlagDefaultsToLoom asserts --recipe's DefValue is "loom", read directly off the
// built flag rather than by driving an invocation -- resolving what a default recipe of "lifecycle"
// versus "loom" actually does would require reaching Arm, which spawns git and does not belong in
// an untagged file.
func TestCommand_RecipeFlagDefaultsToLoom(t *testing.T) {
	cmd := Command()
	flag := cmd.PersistentFlags().Lookup("recipe")
	if flag == nil {
		t.Fatal("shed command has no --recipe flag")
	}
	if flag.DefValue != "loom" {
		t.Errorf("--recipe DefValue = %q; want %q", flag.DefValue, "loom")
	}
}

// TestRunCLIIn_BareListingNeedsNoGitRepository asserts a bare "lyx shed" lists the four
// subcommands and succeeds against a cwd that is not a git worktree at all.
func TestRunCLIIn_BareListingNeedsNoGitRepository(t *testing.T) {
	var out bytes.Buffer
	exitCode := RunCLIIn(t.TempDir(), &out, nil)

	if exitCode != 0 {
		t.Fatalf("RunCLIIn(nil) exit code = %d; want 0; output: %s", exitCode, out.String())
	}
	for _, sub := range []string{"run", "step", "status", "pause"} {
		if !strings.Contains(out.String(), sub) {
			t.Errorf("bare shed listing missing subcommand %q; got:\n%s", sub, out.String())
		}
	}
}

// TestRunCLIIn_UnknownSubcommandEmitsJSONEnvelope asserts "lyx shed bogus" emits a JSON error
// envelope rather than cobra's own plain-text help.
func TestRunCLIIn_UnknownSubcommandEmitsJSONEnvelope(t *testing.T) {
	var out bytes.Buffer
	exitCode := RunCLIIn(t.TempDir(), &out, []string{"bogus"})

	if exitCode != 1 {
		t.Fatalf("RunCLIIn([bogus]) exit code = %d; want 1; output: %s", exitCode, out.String())
	}
	if !strings.Contains(out.String(), `"ok":false`) {
		t.Errorf("RunCLIIn([bogus]) output missing ok:false envelope; got: %q", out.String())
	}
}

// TestRunCLIIn_UnknownRecipeEmitsUnknownRecipeEnvelope asserts an unresolvable --recipe value
// refuses on the unknown-recipe envelope rather than an argument-count error, since the recipe
// lookup happens ahead of Args validation's own reach into the table.
func TestRunCLIIn_UnknownRecipeEmitsUnknownRecipeEnvelope(t *testing.T) {
	var out bytes.Buffer
	exitCode := RunCLIIn(t.TempDir(), &out, []string{"status", "--recipe", "bogus-recipe"})

	if exitCode != 1 {
		t.Fatalf("RunCLIIn(unknown recipe) exit code = %d; want 1; output: %s", exitCode, out.String())
	}
	if !strings.Contains(out.String(), "unknown recipe") {
		t.Errorf("RunCLIIn(unknown recipe) output = %q; want it to name the unknown-recipe refusal", out.String())
	}
	if !strings.Contains(out.String(), "bogus-recipe") {
		t.Errorf("RunCLIIn(unknown recipe) output = %q; want it to name the unresolved recipe", out.String())
	}
}

// TestRunCLIIn_UnsupportedVerbRefusesBeforeResolvingCwd asserts "lyx shed step --recipe lifecycle
// <slug>" is refused on the envelope naming the verb, the recipe, and lifecycle's three supported
// verbs -- and that the refusal happens without ever resolving cwd, proven by passing a directory
// that is not a git worktree at all and still succeeding at the refusal rather than failing on a
// "not a git repository" error.
func TestRunCLIIn_UnsupportedVerbRefusesBeforeResolvingCwd(t *testing.T) {
	var out bytes.Buffer
	exitCode := RunCLIIn(t.TempDir(), &out, []string{"step", "--recipe", "lifecycle", "some-slug"})

	if exitCode != 1 {
		t.Fatalf("RunCLIIn(step --recipe lifecycle) exit code = %d; want 1; output: %s", exitCode, out.String())
	}
	got := out.String()
	for _, want := range []string{"step", "lifecycle", "run", "status", "pause"} {
		if !strings.Contains(got, want) {
			t.Errorf("unsupported-verb refusal = %q; want it to name %q", got, want)
		}
	}
	if strings.Contains(got, "not a git repository") {
		t.Errorf("unsupported-verb refusal = %q; must refuse before ever resolving cwd", got)
	}
}
