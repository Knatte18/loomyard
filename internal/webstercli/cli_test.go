// cli_test.go covers the webstercli cobra seam through RunCLI: bare-group
// listing, the unknown-subcommand JSON envelope, the PersistentPreRunE
// group-command guard, and the help-tree Short completeness check --
// mirroring buildercli's own cli_test.go (internal/buildercli/cli_test.go).
// It also covers the three spawn-free verbs (validate/status/pause) and
// websterWeftPathspec's exclusion set directly, since none of those need a
// live tmux/claude substrate or even a git repository beyond a plain
// t.TempDir(). Every fixture here builds a *websterCLI literal directly,
// bypassing Command()'s PersistentPreRunE, the package-local injection
// point buildercli's own tests establish. Every other verb's own behavior
// (begin-batch, record-batch, recover-batch, run) is covered by
// verbs_test.go.
package webstercli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/builderengine"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/hubgeometry"
	"github.com/Knatte18/loomyard/internal/websterengine"
	"github.com/spf13/cobra"
)

// TestRunCLI_NoArgs verifies that "lyx webster" with no subcommand exits 0
// and lists whatever subcommands are currently registered -- no git repo
// is needed, since the PersistentPreRunE guard skips layout/config/engine
// resolution for the group command itself.
func TestRunCLI_NoArgs(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	exitCode := RunCLI(&out, nil)

	if exitCode != 0 {
		t.Errorf("RunCLI(nil) = %d; want 0", exitCode)
	}
}

// TestRunCLI_UnknownSubcommand verifies that an unknown subcommand exits 1
// and emits a JSON error envelope with ok=false, without needing a git repo
// (the PersistentPreRunE guard for cmd.Name() == "webster" fires before
// layout resolution).
func TestRunCLI_UnknownSubcommand(t *testing.T) {
	t.Chdir(t.TempDir())

	var out bytes.Buffer
	exitCode := RunCLI(&out, []string{"bogus"})

	if exitCode != 1 {
		t.Errorf("RunCLI(bogus) = %d; want 1", exitCode)
	}

	got := out.String()
	if !strings.Contains(got, `"ok":false`) {
		t.Errorf("RunCLI(bogus) output missing ok:false envelope; got: %q", got)
	}
	if !strings.Contains(got, "unknown") {
		t.Errorf("RunCLI(bogus) output missing \"unknown\"; got: %q", got)
	}
}

// TestRunCLI_GroupGuard_OutsideGitRepo asserts the PersistentPreRunE guard:
// bare "lyx webster" works outside a git repository, mirroring buildercli's
// guard rationale (neither the bare listing nor the unknown-subcommand path
// should require layout/config resolution).
func TestRunCLI_GroupGuard_OutsideGitRepo(t *testing.T) {
	t.Chdir(t.TempDir())

	var out bytes.Buffer
	exitCode := RunCLI(&out, nil)

	if exitCode != 0 {
		t.Errorf("RunCLI(nil) outside a git repo = %d; want 0", exitCode)
	}
}

// TestCommand_EveryCommandHasShort walks the full webster command tree and
// asserts that every command -- the parent group and every subcommand --
// carries a non-empty Short, per the CLI/Cobra Invariant.
func TestCommand_EveryCommandHasShort(t *testing.T) {
	var walk func(cmd *cobra.Command)
	walk = func(cmd *cobra.Command) {
		if cmd.Short == "" {
			t.Errorf("command %q has empty Short", cmd.CommandPath())
		}
		for _, sub := range cmd.Commands() {
			walk(sub)
		}
	}
	walk(Command())
}

// TestCommand_AllSevenSubcommandsRegistered asserts every one of webster's
// seven subcommands is present on the tree Command() builds.
func TestCommand_AllEightSubcommandsRegistered(t *testing.T) {
	want := []string{"validate", "run", "status", "pause", "begin-batch", "await-batch", "record-batch", "recover-batch"}
	got := map[string]bool{}
	for _, sub := range Command().Commands() {
		got[sub.Name()] = true
	}
	for _, name := range want {
		if !got[name] {
			t.Errorf("Command() is missing subcommand %q", name)
		}
	}
}

// TestWebsterWeftPathspec_ExcludesRuntimeArtifacts proves the pathspec every
// webster weft commit stages under excludes the advisory *.lock files, the
// pause flag, and every rendered fork prompt, regardless of whether
// layout.RelPath prefixes the _lyx path.
func TestWebsterWeftPathspec_ExcludesRuntimeArtifacts(t *testing.T) {
	tests := []struct {
		name    string
		relPath string
	}{
		{name: "nested worktree (relPath set)", relPath: "wts/some-task"},
		{name: "weft-root worktree (relPath empty)", relPath: ""},
	}

	wantExcludes := []string{
		":(exclude)*.lock",
		":(exclude)*/webster/pause",
		":(exclude)*/webster/prompts/*",
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pathspec := websterWeftPathspec(&hubgeometry.Layout{RelPath: tt.relPath})

			for _, want := range wantExcludes {
				if !containsString(pathspec, want) {
					t.Errorf("websterWeftPathspec(relPath=%q) = %v; want it to contain %q", tt.relPath, pathspec, want)
				}
			}
		})
	}
}

// containsString reports whether haystack contains needle.
func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// newTestCLI builds a minimal *websterCLI wired only with the fields
// validate/status/pause need (layout, cfg, and the four hubgeometry dirs),
// bypassing Command()'s PersistentPreRunE -- the package-local injection
// pattern every verb's own test uses. hub is a plain t.TempDir(), never a
// real git repo: none of these three verbs ever call gitquery or spawn.
func newTestCLI(t *testing.T) (*websterCLI, string) {
	t.Helper()
	hub := t.TempDir()
	c := &websterCLI{
		layout:     &hubgeometry.Layout{WorktreeRoot: hub, Cwd: hub, RelPath: "."},
		cfg:        websterengine.Config{BatchContextCapTokens: 1_000_000, BatchCardCap: 50},
		planDir:    hubgeometry.PlanDir(hub),
		websterDir: hubgeometry.WebsterDir(hub),
		reportsDir: hubgeometry.WebsterReportsDir(hub),
		promptsDir: hubgeometry.WebsterPromptsDir(hub),
	}
	return c, hub
}

// seedValidPlanDir writes a syntactically complete, validation-clean v2
// plan with one batch into dir, mirroring websterengine's own
// seedRunPlanDir (runlevel_test.go): one card whose sole file-op field is a
// Creates: entry covered by the batch's own Scope, and a verify: command.
func seedValidPlanDir(t *testing.T, dir string) {
	t.Helper()
	overview := "---\nformat: 2\napproved: true\n---\n\n# Plan\n\nFraming.\n\n## Batch Index\n\n- 01 — only (1 card) — placeholder batch\n"
	batch := "# Batch\n\n## Scope\n\n- internal/only\n\n## Cards\n\n### Card 01.1 — placeholder\n\n" +
		"**What:** placeholder card.\n**Context:** none\n**Edits:** none\n" +
		"**Creates:**\n- `internal/only/new.go`\n**Deletes:** none\n**Moves:** none\n\n" +
		"## verify:\n\ngo build ./...\n"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir plan dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "00-overview.md"), []byte(overview), 0o644); err != nil {
		t.Fatalf("write overview: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "01-only.md"), []byte(batch), 0o644); err != nil {
		t.Fatalf("write batch file: %v", err)
	}
}

// TestValidateCmd_ValidPlan proves the happy path: a clean plan prints
// {"valid": true, "batches": N}.
func TestValidateCmd_ValidPlan(t *testing.T) {
	c, _ := newTestCLI(t)
	seedValidPlanDir(t, c.planDir)

	var out bytes.Buffer
	exitCode := clihelp.Execute(c.validateCmd(), &out, nil)

	if exitCode != 0 {
		t.Fatalf("validate on a clean plan = %d; want 0, output: %s", exitCode, out.String())
	}
	got := out.String()
	if !strings.Contains(got, `"valid":true`) {
		t.Errorf("output missing valid:true; got %q", got)
	}
	if !strings.Contains(got, `"batches":1`) {
		t.Errorf("output missing batches:1; got %q", got)
	}
}

// TestValidateCmd_MissingPlan proves a plan directory that does not parse
// at all surfaces a loud error envelope, never a panic or a false valid:true.
func TestValidateCmd_MissingPlan(t *testing.T) {
	c, _ := newTestCLI(t)
	// Deliberately never seed c.planDir: ParsePlan must fail loud.

	var out bytes.Buffer
	exitCode := clihelp.Execute(c.validateCmd(), &out, nil)

	if exitCode != 1 {
		t.Fatalf("validate on a missing plan = %d; want 1, output: %s", exitCode, out.String())
	}
	if !strings.Contains(out.String(), `"ok":false`) {
		t.Errorf("output missing ok:false; got %q", out.String())
	}
}

// TestStatusCmd_NotInitialized proves a run that never started prints
// {"initialized": false}.
func TestStatusCmd_NotInitialized(t *testing.T) {
	c, _ := newTestCLI(t)

	var out bytes.Buffer
	exitCode := clihelp.Execute(c.statusCmd(), &out, nil)

	if exitCode != 0 {
		t.Fatalf("status with no state.json = %d; want 0, output: %s", exitCode, out.String())
	}
	if !strings.Contains(out.String(), `"initialized":false`) {
		t.Errorf("output missing initialized:false; got %q", out.String())
	}
}

// TestStatusCmd_WithBatches proves status renders every persisted batch's
// kind, status, terminal, and digest-presence fields, plus the run-level
// identity fields, from a plain on-disk state.json -- no git, no spawn.
func TestStatusCmd_WithBatches(t *testing.T) {
	c, _ := newTestCLI(t)

	st := &websterengine.State{
		RunGUID:         "guid-1",
		PlanFingerprint: "fp-1",
		Batches: map[int]*websterengine.BatchState{
			1: {Slug: "first", Kind: "fork", Status: "done", Terminal: true, Digest: &fakeDigest},
			2: {Slug: "second", Kind: "recovery", Status: "", Terminal: false},
		},
	}
	if err := websterengine.SaveState(c.websterDir, st); err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}

	var out bytes.Buffer
	exitCode := clihelp.Execute(c.statusCmd(), &out, nil)

	if exitCode != 0 {
		t.Fatalf("status = %d; want 0, output: %s", exitCode, out.String())
	}
	got := out.String()
	for _, want := range []string{
		`"run_guid":"guid-1"`, `"plan_fingerprint":"fp-1"`,
		`"kind":"fork"`, `"kind":"recovery"`,
		`"has_digest":true`, `"has_digest":false`,
		`"terminal":true`, `"terminal":false`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("status output missing %q; got %q", want, got)
		}
	}
}

// TestPauseCmd_RequestsPauseIdempotent proves pause writes the flag file
// and reports {"paused": true}, and that a second call is a no-op success,
// never an error.
func TestPauseCmd_RequestsPauseIdempotent(t *testing.T) {
	c, _ := newTestCLI(t)

	for i := 0; i < 2; i++ {
		var out bytes.Buffer
		exitCode := clihelp.Execute(c.pauseCmd(), &out, nil)
		if exitCode != 0 {
			t.Fatalf("pause call %d = %d; want 0, output: %s", i+1, exitCode, out.String())
		}
		if !strings.Contains(out.String(), `"paused":true`) {
			t.Errorf("pause call %d output missing paused:true; got %q", i+1, out.String())
		}
	}
}

// fakeDigest is a minimal terminal builderengine.Digest used only to prove
// status's has_digest field distinguishes a persisted digest from a nil one.
var fakeDigest = builderengine.Digest{Batch: "01-first", Status: builderengine.DigestStatusDone}
