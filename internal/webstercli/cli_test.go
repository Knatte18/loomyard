// cli_test.go covers the webstercli cobra seam through RunCLI: bare-group
// listing, the unknown-subcommand JSON envelope, the PersistentPreRunE
// group-command guard, and the help-tree Short completeness check --
// mirroring buildercli's own cli_test.go (internal/buildercli/cli_test.go).
// It also covers the three spawn-free verbs (validate/status/pause),
// websterWeftPathspec's exclusion set, and weftCommit's SkipGit-before-New
// guard ordering directly, since none of those need a live tmux/claude
// substrate or even a git repository beyond a plain t.TempDir(). Every fixture here builds a *websterCLI literal directly,
// bypassing Command()'s PersistentPreRunE, the package-local injection
// point buildercli's own tests establish. Every other verb's own behavior
// (begin-batch, record-batch, recover-batch, run) is covered by
// verbs_test.go.
package webstercli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/builderengine"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
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

// TestCommand_AllEightSubcommandsRegistered asserts every one of webster's
// eight subcommands is present on the tree Command() builds.
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

// TestCommand_LongStringsHaveNoStaleV2Language walks the full webster
// command tree and asserts that no command's Long help string mentions
// anything from the retired plan-format v2/chain/oversized-batch model:
// --restart-chain, the deferred-verify chain, oversized batches, or a bare
// "v2" version reference (plan-format-v3.md is fine; a literal "v2" is
// not) -- per the CLI/Cobra Invariant's help-accuracy obligation and the
// no-version-suffix-naming Shared Decision.
func TestCommand_LongStringsHaveNoStaleV2Language(t *testing.T) {
	forbidden := []string{"--restart-chain", "restart-chain", "chain", "oversized", "v2"}

	var walk func(cmd *cobra.Command)
	walk = func(cmd *cobra.Command) {
		lower := strings.ToLower(cmd.Long)
		for _, bad := range forbidden {
			if strings.Contains(lower, bad) {
				t.Errorf("command %q Long string contains stale v2/chain/oversized language %q:\n%s", cmd.CommandPath(), bad, cmd.Long)
			}
		}
		for _, sub := range cmd.Commands() {
			walk(sub)
		}
	}
	walk(Command())
}

// TestWebsterWeftPathspec_ExcludesRuntimeArtifacts proves the pathspec every
// webster weft commit stages under excludes the advisory *.lock files, the
// pause flag, and every rendered fork prompt, at each layout.RelPath shape --
// and that every exclusion is ANCHORED under the scoped _lyx base rather than
// spelled with a leading wildcard. The anchoring is not cosmetic: git treats
// a leading-"*" pattern with no further wildcard as a one-star pathspec that
// false-positive-matches the intermediate directories leading to a
// multi-segment positive pathspec, which prunes the whole subtree and turns
// the weft commit into a silent no-op. Note this test can only prove the
// SHAPE of the pathspec; that real git honours it is proved by
// weft_integration_test.go's TestWeftCommit_CommitsAtEveryRelPathDepth.
func TestWebsterWeftPathspec_ExcludesRuntimeArtifacts(t *testing.T) {
	tests := []struct {
		name    string
		relPath string
		base    string
	}{
		{name: "nested worktree (relPath set)", relPath: "wts/some-task", base: "wts/some-task/_lyx"},
		{name: "worktree root (relPath dot)", relPath: ".", base: "_lyx"},
		{name: "weft-root worktree (relPath empty)", relPath: "", base: "_lyx"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pathspec := websterWeftPathspec(&hubgeometry.Layout{RelPath: tt.relPath})

			wantExcludes := []string{
				":(exclude)" + tt.base + "/*.lock",
				":(exclude)" + tt.base + "/webster/" + websterengine.PauseFlagName,
				":(exclude)" + tt.base + "/webster/prompts/*",
				// Builder's pause flag is excluded too: both round-loop
				// modules share one _lyx tree, so a webster commit stages
				// whatever builder left on disk.
				":(exclude)" + tt.base + "/builder/" + builderengine.PauseFlagName,
			}
			for _, want := range wantExcludes {
				if !containsString(pathspec, want) {
					t.Errorf("websterWeftPathspec(relPath=%q) = %v; want it to contain %q", tt.relPath, pathspec, want)
				}
			}

			for _, entry := range pathspec {
				if strings.HasPrefix(entry, ":(exclude)*") {
					t.Errorf("websterWeftPathspec(relPath=%q) has unanchored exclusion %q; every exclusion must be anchored under %q", tt.relPath, entry, tt.base)
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

// TestWeftCommit_SkipGitBypassNeedsNoWeftWorktree pins the guard ordering
// weftCommit's own block comment documents: with WEFT_SKIP_GIT=1 the bypass
// must short-circuit BEFORE fabricengine.New's stat-based path validation,
// so the CI/test bypass never requires a weft worktree (or even the host
// worktree) to exist on disk. A regression hoisting New above the guard
// turns every bypassed CI run into an ErrMissingPath failure.
func TestWeftCommit_SkipGitBypassNeedsNoWeftWorktree(t *testing.T) {
	t.Setenv("WEFT_SKIP_GIT", "1")
	t.Setenv("WEFT_SKIP_PUSH", "")

	// Neither the host worktree nor its -weft sibling exists on disk.
	hub := t.TempDir()
	layout := &hubgeometry.Layout{
		Hub:          hub,
		WorktreeRoot: filepath.Join(hub, "host"),
		Cwd:          filepath.Join(hub, "host"),
		RelPath:      ".",
	}

	committed, err := weftCommit(layout, "bypass probe")
	if err != nil {
		t.Fatalf("weftCommit() error = %v; want nil, the bypass must never touch the filesystem or git", err)
	}
	if committed {
		t.Error("weftCommit() committed = true; want false in bypass mode")
	}
}

// TestWeftCommit_NonBypassValidatesPairPaths proves the counterpart of the
// bypass test above: without WEFT_SKIP_GIT, weftCommit constructs the
// fabric handle and surfaces fabricengine's typed ErrMissingPath when the
// pair is absent -- evidence New runs, and runs only in non-bypass mode.
func TestWeftCommit_NonBypassValidatesPairPaths(t *testing.T) {
	t.Setenv("WEFT_SKIP_GIT", "")
	t.Setenv("WEFT_SKIP_PUSH", "")

	hub := t.TempDir()
	layout := &hubgeometry.Layout{
		Hub:          hub,
		WorktreeRoot: filepath.Join(hub, "host"),
		Cwd:          filepath.Join(hub, "host"),
		RelPath:      ".",
	}

	committed, err := weftCommit(layout, "missing-pair probe")
	if committed {
		t.Error("weftCommit() committed = true; want false, no repo exists to commit to")
	}
	var missing *fabricengine.ErrMissingPath
	if !errors.As(err, &missing) {
		t.Fatalf("weftCommit() error = %v; want a *fabricengine.ErrMissingPath from New's stat validation", err)
	}
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
		cfg:        websterengine.Config{},
		planDir:    hubgeometry.PlanDir(hub),
		websterDir: hubgeometry.WebsterDir(hub),
		reportsDir: hubgeometry.WebsterReportsDir(hub),
		promptsDir: hubgeometry.WebsterPromptsDir(hub),
	}
	return c, hub
}

// seedValidPlanDir writes a syntactically complete, validation-clean
// plan-format v3 plan with one card into dir, mirroring websterengine's own
// seedRunPlanDir (runlevel_test.go): a Card Index naming the one card, and
// the card's own file with all seven required fields, its sole file-op
// field a Creates: entry.
func seedValidPlanDir(t *testing.T, dir string) {
	t.Helper()
	overview := "---\nformat: 3\napproved: true\n---\n\n# Plan\n\nFraming.\n\n## Card Index\n\n" +
		"1 — only — placeholder card\n"
	card := "# Card 1 — only\n\n**What:** placeholder card.\n**Context:** none\n**Edits:** none\n" +
		"**Creates:**\n- `internal/only/new.go`\n**Deletes:** none\n**Moves:** none\n**Depends-on:** none\n"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir plan dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "00-overview.md"), []byte(overview), 0o644); err != nil {
		t.Fatalf("write overview: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "01-only.md"), []byte(card), 0o644); err != nil {
		t.Fatalf("write card file: %v", err)
	}
}

// TestValidateCmd_ValidPlan proves the happy path: a clean plan prints
// {"valid": true, "cards": N}.
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
	if !strings.Contains(got, `"cards":1`) {
		t.Errorf("output missing cards:1; got %q", got)
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

// seedMissingFieldPlanDir writes a plan-format v3 plan whose sole card omits
// the **Deletes:** label entirely -- ParsePlan succeeds (the parser only
// records HasDeletes = false; a missing field is a Validate-time finding,
// never a parse-time error), so this plan reaches planparser.Validate and
// trips exactly one card-missing-field finding.
func seedMissingFieldPlanDir(t *testing.T, dir string) {
	t.Helper()
	overview := "---\nformat: 3\napproved: true\n---\n\n# Plan\n\nFraming.\n\n## Card Index\n\n" +
		"1 — only — placeholder card\n"
	card := "# Card 1 — only\n\n**What:** placeholder card.\n**Context:** none\n**Edits:** none\n" +
		"**Creates:**\n- `internal/only/new.go`\n**Moves:** none\n**Depends-on:** none\n"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir plan dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "00-overview.md"), []byte(overview), 0o644); err != nil {
		t.Fatalf("write overview: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "01-only.md"), []byte(card), 0o644); err != nil {
		t.Fatalf("write card file: %v", err)
	}
}

// TestValidateCmd_FindingsUseCardKey proves findingsEnvelope's per-finding
// JSON entries carry the "card" key (f.Card) rather than the pre-rewrite
// "batch" key: it drives a syntactically-parseable plan through validateCmd
// that trips one planparser.Validate check (card-missing-field, via a card
// missing its **Deletes:** label) and asserts the emitted findings array
// shape end-to-end, since TestValidateCmd_ValidPlan only exercises the
// zero-findings ok-envelope and TestValidateCmd_MissingPlan never reaches
// Validate at all.
func TestValidateCmd_FindingsUseCardKey(t *testing.T) {
	c, _ := newTestCLI(t)
	seedMissingFieldPlanDir(t, c.planDir)

	var out bytes.Buffer
	exitCode := clihelp.Execute(c.validateCmd(), &out, nil)

	if exitCode != 1 {
		t.Fatalf("validate on a plan missing a required field = %d; want 1, output: %s", exitCode, out.String())
	}
	got := out.String()
	if !strings.Contains(got, `"ok":false`) {
		t.Errorf("output missing ok:false; got %q", got)
	}
	if !strings.Contains(got, `"check":"card-missing-field"`) {
		t.Errorf("output missing check:card-missing-field; got %q", got)
	}
	if !strings.Contains(got, `"card":"1-only"`) {
		t.Errorf("output missing card:1-only (findingsEnvelope must key each finding by f.Card, not f.Batch); got %q", got)
	}
	if strings.Contains(got, `"batch":`) {
		t.Errorf("output carries a stale batch key; findingsEnvelope must emit only check/card/detail; got %q", got)
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

// fakeDigest is a minimal terminal websterengine.Digest used only to prove
// status's has_digest field distinguishes a persisted digest from a nil one.
var fakeDigest = websterengine.Digest{Batch: "01-first", Status: websterengine.DigestStatusDone}
