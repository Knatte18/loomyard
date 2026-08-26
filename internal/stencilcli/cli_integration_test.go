//go:build integration

// cli_integration_test.go drives the stencil module through RunCLIIn against a real fabric hub
// built by internal/hubforge, exercising list, validate, diff (both modes), sync, and the promote
// round trip end to end. It is Tier 2: it builds a real hub and spawns git, so it needs
// gitkit.HermeticGitEnv() (see testmain_test.go) and the integration build tag.

package stencilcli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/contracts/stencils"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/stencil"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// runCLI drives RunCLIIn against cwd with args, and parses the printed envelope as JSON when the
// output is non-empty. It fails the test outright on a malformed envelope, since every command
// under test always emits one.
func runCLI(t *testing.T, cwd string, args ...string) (env map[string]any, code int, raw string) {
	t.Helper()

	var out bytes.Buffer
	code = RunCLIIn(cwd, &out, args)
	raw = strings.TrimSpace(out.String())
	if raw == "" {
		return nil, code, raw
	}
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatalf("RunCLIIn(%v) output not valid JSON: %v; output: %s", args, err, raw)
	}
	return env, code, raw
}

// mustGitIn runs a git subcommand in dir, failing the test on a non-zero exit.
func mustGitIn(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s (in %s): %v\n%s", strings.Join(args, " "), dir, err, out)
	}
}

// fakeRegistry implements stencilstore.Registry over the real stencils.Registry(), with a handful
// of names overridden. It exists solely to simulate "the next deploy landed a new shipped default"
// for internal/stencilstore.Reconcile calls made directly by a test -- the CLI itself is bound to
// the real compiled stencils.Registry() and offers no way to inject a fake default in-process.
type fakeRegistry struct {
	names     []string
	overrides map[string][]byte
}

func (f fakeRegistry) Names() []string { return f.names }

func (f fakeRegistry) Default(name string) ([]byte, bool) {
	if override, ok := f.overrides[name]; ok {
		return override, true
	}
	return stencils.Registry().Default(name)
}

// diffAllEntryDiffers returns the "differs" field of env["stencils"]'s entry for name, failing the
// test if env carries no such entry.
func diffAllEntryDiffers(t *testing.T, env map[string]any, name string) bool {
	t.Helper()

	list, ok := env["stencils"].([]any)
	if !ok {
		t.Fatalf("diff --all response missing a stencils field; got %v", env)
	}
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok || m["name"] != name {
			continue
		}
		differs, _ := m["differs"].(bool)
		return differs
	}
	t.Fatalf("diff --all response has no entry for %q; got %v", name, list)
	return false
}

// TestStencilCLI_ListAndValidate seeds a hub, asserts list names every registered stencil in the
// untouched state, then breaks two board copies directly -- one gains a marker unknown to its
// shipped default, one loses a marker its shipped default declares -- and asserts validate reports
// the first as an error, the second as a warning, and exits non-zero overall.
func TestStencilCLI_ListAndValidate(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	worktree := h.PrimeWorktree()

	if _, code, raw := runCLI(t, worktree, "sync"); code != 0 {
		t.Fatalf("stencil sync = %d; want 0. output: %s", code, raw)
	}

	env, code, raw := runCLI(t, worktree, "list")
	if code != 0 {
		t.Fatalf("stencil list = %d; want 0. output: %s", code, raw)
	}
	list, ok := env["stencils"].([]any)
	if !ok {
		t.Fatalf("stencils field missing or wrong type; got %v", env)
	}

	registryNames := stencils.Registry().Names()
	if len(list) != len(registryNames) {
		t.Fatalf("stencil list returned %d entries; want %d (%v)", len(list), len(registryNames), registryNames)
	}

	seen := make(map[string]string, len(list))
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("list entry not an object: %v", item)
		}
		name, _ := m["name"].(string)
		state, _ := m["state"].(string)
		seen[name] = state
	}
	for _, name := range registryNames {
		state, ok := seen[name]
		if !ok {
			t.Errorf("stencil list missing entry for %q", name)
			continue
		}
		if state != "untouched" {
			t.Errorf("stencil %q state = %q; want %q immediately after a fresh sync", name, state, "untouched")
		}
	}

	stencilsDir := fabricengine.StencilsDir(h.Path)
	errName := registryNames[0]
	warnName := registryNames[1]

	errPath := stencilstore.Path(stencilsDir, errName)
	errContent, err := os.ReadFile(errPath)
	if err != nil {
		t.Fatalf("read %s: %v", errPath, err)
	}
	broken := append(append([]byte{}, errContent...), []byte("\n{{.ZZZ_UNKNOWN_MARKER}}\n")...)
	if err := os.WriteFile(errPath, broken, 0o644); err != nil {
		t.Fatalf("write %s: %v", errPath, err)
	}

	warnPath := stencilstore.Path(stencilsDir, warnName)
	warnContent, err := os.ReadFile(warnPath)
	if err != nil {
		t.Fatalf("read %s: %v", warnPath, err)
	}
	markers, err := stencil.TopLevelMarkers(warnContent)
	if err != nil || len(markers) == 0 {
		t.Fatalf("TopLevelMarkers(%s) = %v, %v; want at least one marker", warnName, markers, err)
	}
	droppedMarker := markers[0]
	droppedToken := "{{." + droppedMarker + "}}"
	// Every occurrence has to go: TopLevelMarkers reports a deduplicated set, so a stencil that
	// declares the same marker in two places still declares it after one occurrence is removed,
	// and validate would rightly report nothing.
	newWarnContent := strings.ReplaceAll(string(warnContent), droppedToken, "")
	if newWarnContent == string(warnContent) {
		t.Fatalf("marker token %q not found in %s's on-disk content", droppedToken, warnName)
	}
	remaining, err := stencil.TopLevelMarkers([]byte(newWarnContent))
	if err != nil {
		t.Fatalf("TopLevelMarkers(%s) after dropping %q: %v", warnName, droppedMarker, err)
	}
	if slices.Contains(remaining, droppedMarker) {
		t.Fatalf("marker %q still declared by %s after dropping every %q token; markers = %v",
			droppedMarker, warnName, droppedToken, remaining)
	}
	if err := os.WriteFile(warnPath, []byte(newWarnContent), 0o644); err != nil {
		t.Fatalf("write %s: %v", warnPath, err)
	}

	env, code, raw = runCLI(t, worktree, "validate")
	if code == 0 {
		t.Fatalf("stencil validate = %d; want non-zero with an error-severity finding present. output: %s", code, raw)
	}
	findings, ok := env["findings"].([]any)
	if !ok {
		t.Fatalf("findings field missing or wrong type; got %v", env)
	}

	var sawError, sawWarning bool
	for _, f := range findings {
		m, ok := f.(map[string]any)
		if !ok {
			continue
		}
		switch {
		case m["name"] == errName && m["severity"] == stencilstore.SeverityError && m["marker"] == "ZZZ_UNKNOWN_MARKER":
			sawError = true
		case m["name"] == warnName && m["severity"] == stencilstore.SeverityWarning && m["marker"] == droppedMarker:
			sawWarning = true
		}
	}
	if !sawError {
		t.Errorf("validate findings missing the expected error finding for %q; got %v", errName, findings)
	}
	if !sawWarning {
		t.Errorf("validate findings missing the expected warning finding for %q; got %v", warnName, findings)
	}
}

// TestStencilCLI_SyncIsIdempotent asserts a second consecutive sync, with nothing changed in
// between, writes nothing, creates no commit, and returns an empty mutations array.
func TestStencilCLI_SyncIsIdempotent(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	worktree := h.PrimeWorktree()

	if _, code, raw := runCLI(t, worktree, "sync"); code != 0 {
		t.Fatalf("first stencil sync = %d; want 0. output: %s", code, raw)
	}

	env, code, raw := runCLI(t, worktree, "sync")
	if code != 0 {
		t.Fatalf("second stencil sync = %d; want 0. output: %s", code, raw)
	}
	mutations, ok := env["mutations"].([]any)
	if !ok {
		t.Fatalf("mutations field missing or wrong type; got %v", env)
	}
	if len(mutations) != 0 {
		t.Errorf("second consecutive sync mutations = %v; want an empty array", mutations)
	}
	if committed, _ := env["committed"].(bool); committed {
		t.Errorf("second consecutive sync committed = true; want false since there was nothing to write")
	}
}

// TestStencilCLI_DiffOne covers both `diff <name>` outcomes: a recoverable base (a fabricated
// historical board revision committed directly to board history, then diffed against today's real
// shipped default) and an unrecoverable one (an on-disk stamp matching no board history revision at
// all), asserting the latter reports found:false with an explicit reason and a non-empty fallback
// diff rather than a silently empty one.
func TestStencilCLI_DiffOne(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	worktree := h.PrimeWorktree()
	boardDir := h.BoardDir()
	stencilsDir := fabricengine.StencilsDir(h.Path)

	if _, code, raw := runCLI(t, worktree, "sync"); code != 0 {
		t.Fatalf("stencil sync = %d; want 0. output: %s", code, raw)
	}

	names := stencils.Registry().Names()

	recoverName := names[2]
	recoverPath := stencilstore.Path(stencilsDir, recoverName)
	fabricated := []byte("<!-- fabricated historical default for a diff test -->\n\nFABRICATED BODY V1\n")
	fabHash := stencilstore.BodyHash(fabricated)
	fabStamped := stencilstore.ApplyStamp(fabricated, fabHash)
	if err := os.WriteFile(recoverPath, fabStamped, 0o644); err != nil {
		t.Fatalf("write %s: %v", recoverPath, err)
	}
	mustGitIn(t, boardDir, "add", "-A")
	mustGitIn(t, boardDir, "commit", "-m", "test: fabricate a historical stencil revision")

	env, code, raw := runCLI(t, worktree, "diff", recoverName)
	if code != 0 {
		t.Fatalf("stencil diff %s = %d; want 0. output: %s", recoverName, code, raw)
	}
	if found, _ := env["found"].(bool); !found {
		t.Fatalf("stencil diff %s found = %v; want true. output: %s", recoverName, env["found"], raw)
	}
	if diffText, _ := env["diff"].(string); diffText == "" {
		t.Errorf("stencil diff %s produced an empty diff against a seeded-then-changed default; want non-empty", recoverName)
	}

	unrecoverName := names[3]
	unrecoverPath := stencilstore.Path(stencilsDir, unrecoverName)
	onDisk, err := os.ReadFile(unrecoverPath)
	if err != nil {
		t.Fatalf("read %s: %v", unrecoverPath, err)
	}
	bogusStamp := strings.Repeat("a", 64)
	if err := os.WriteFile(unrecoverPath, stencilstore.ApplyStamp(onDisk, bogusStamp), 0o644); err != nil {
		t.Fatalf("write %s: %v", unrecoverPath, err)
	}

	env, code, raw = runCLI(t, worktree, "diff", unrecoverName)
	if code != 0 {
		t.Fatalf("stencil diff %s = %d; want 0 (found:false is not itself an error). output: %s", unrecoverName, code, raw)
	}
	if found, _ := env["found"].(bool); found {
		t.Fatalf("stencil diff %s found = true; want false for an unrecoverable base", unrecoverName)
	}
	if reason, _ := env["reason"].(string); reason == "" {
		t.Errorf("stencil diff %s reports found:false with no reason; want an explicit reason", unrecoverName)
	}
	if diffText, _ := env["diff"].(string); diffText == "" {
		t.Errorf("stencil diff %s rendered an empty diff for an unrecoverable base; want the shipped-default-vs-on-disk fallback diff", unrecoverName)
	}
}

// TestStencilCLI_PromoteRoundTripAndDiffAll closes the loop: it edits the board copy, confirms
// diff --all --exit-code fails on the unported edit, runs promote, confirms the source tree received
// exactly the edit with the stamp line stripped and that diff --all --exit-code now succeeds, then
// simulates the next deploy landing the promoted content as the new shipped default and confirms the
// reconciliation row restamps the board copy back to the untouched state.
func TestStencilCLI_PromoteRoundTripAndDiffAll(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	worktree := h.PrimeWorktree()
	stencilsDir := fabricengine.StencilsDir(h.Path)
	sourceRoot := filepath.Join(worktree, "contracts", "stencils")
	if err := os.MkdirAll(sourceRoot, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", sourceRoot, err)
	}

	if _, code, raw := runCLI(t, worktree, "sync"); code != 0 {
		t.Fatalf("stencil sync = %d; want 0. output: %s", code, raw)
	}

	name := stencils.Registry().Names()[4]
	boardPath := stencilstore.Path(stencilsDir, name)
	seeded, err := os.ReadFile(boardPath)
	if err != nil {
		t.Fatalf("read %s: %v", boardPath, err)
	}

	// Seed the worktree source tree from today's freshly-seeded board copy, minus its stamp --
	// the state a worktree in sync with the board should already be in.
	sourcePath := filepath.Join(sourceRoot, filepath.FromSlash(stencilstore.RelPath(name)))
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(sourcePath), err)
	}
	if err := os.WriteFile(sourcePath, stripStampLine(seeded), 0o644); err != nil {
		t.Fatalf("write %s: %v", sourcePath, err)
	}

	if _, code, raw := runCLI(t, worktree, "diff", "--all", "--exit-code"); code != 0 {
		t.Fatalf("diff --all --exit-code before any edit = %d; want 0. output: %s", code, raw)
	}

	// The operator edits the board copy directly.
	edited := append(append([]byte{}, seeded...), []byte("\n\nOPERATOR EDIT MARKER\n")...)
	if err := os.WriteFile(boardPath, edited, 0o644); err != nil {
		t.Fatalf("write %s: %v", boardPath, err)
	}

	env, code, raw := runCLI(t, worktree, "diff", "--all", "--exit-code")
	if code == 0 {
		t.Fatalf("diff --all --exit-code after an unported board edit = %d; want non-zero. output: %s", code, raw)
	}
	if !diffAllEntryDiffers(t, env, name) {
		t.Errorf("diff --all did not report %q as differing after the board edit; got %v", name, env)
	}

	env, code, raw = runCLI(t, worktree, "promote", name)
	if code != 0 {
		t.Fatalf("stencil promote %s = %d; want 0. output: %s", name, code, raw)
	}
	gotSource, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read %s: %v", sourcePath, err)
	}
	wantSource := stripStampLine(edited)
	if string(gotSource) != string(wantSource) {
		t.Errorf("promote wrote %q; want %q (the edit with only its stamp line stripped)", gotSource, wantSource)
	}

	if _, code, raw := runCLI(t, worktree, "diff", "--all", "--exit-code"); code != 0 {
		t.Fatalf("diff --all --exit-code immediately after promote = %d; want 0. output: %s", code, raw)
	}

	// Simulate the next deploy landing the promoted content as the new shipped default. This
	// drives internal/stencilstore.Reconcile directly rather than through the CLI seam, because
	// the CLI is permanently bound to the real compiled stencils.Registry() and has no way to be
	// handed a fake "next" default in-process.
	promoted, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read %s: %v", sourcePath, err)
	}
	fake := fakeRegistry{names: stencils.Registry().Names(), overrides: map[string][]byte{name: promoted}}
	if _, err := stencilstore.Reconcile(stencilsDir, fake, stencilstore.ModeProduction, ""); err != nil {
		t.Fatalf("stencilstore.Reconcile against the promoted default: %v", err)
	}

	restamped, err := os.ReadFile(boardPath)
	if err != nil {
		t.Fatalf("read %s: %v", boardPath, err)
	}
	if state := stencilstore.Classify(restamped, true, promoted); state != stencilstore.StateUntouched {
		t.Errorf("board copy state after reconciling against the promoted default = %v; want StateUntouched", state)
	}
}

// TestStencilCLI_PromoteAndDiffAllRequireSourceTree asserts that both promote and diff --all fail
// loudly, naming the missing tree, rather than no-op or create one, when the worktree carries no
// contracts/stencils/ source tree at all.
func TestStencilCLI_PromoteAndDiffAllRequireSourceTree(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	worktree := h.PrimeWorktree()

	if _, code, raw := runCLI(t, worktree, "sync"); code != 0 {
		t.Fatalf("stencil sync = %d; want 0. output: %s", code, raw)
	}

	sourceRoot := filepath.Join(worktree, "contracts", "stencils")
	if _, err := os.Stat(sourceRoot); !os.IsNotExist(err) {
		t.Fatalf("contracts/stencils/ source tree unexpectedly present at %s before the test", sourceRoot)
	}

	name := stencils.Registry().Names()[0]

	if _, code, raw := runCLI(t, worktree, "diff", "--all"); code == 0 {
		t.Errorf("diff --all with no contracts/stencils/ source tree = %d; want non-zero. output: %s", code, raw)
	}
	if _, code, raw := runCLI(t, worktree, "promote", name); code == 0 {
		t.Errorf("promote %s with no contracts/stencils/ source tree = %d; want non-zero. output: %s", name, code, raw)
	}

	if _, err := os.Stat(sourceRoot); !os.IsNotExist(err) {
		t.Errorf("promote/diff --all created %s; neither must ever create the source tree", sourceRoot)
	}
}

// TestStencilCLI_PromoteRequiresMatchingSourceFile asserts promote errors, naming the missing file,
// rather than creating one, when the worktree's contracts/stencils/ source tree exists but the target
// stencil's own family subfile is absent from it.
func TestStencilCLI_PromoteRequiresMatchingSourceFile(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	worktree := h.PrimeWorktree()

	if _, code, raw := runCLI(t, worktree, "sync"); code != 0 {
		t.Fatalf("stencil sync = %d; want 0. output: %s", code, raw)
	}

	sourceRoot := filepath.Join(worktree, "contracts", "stencils")
	if err := os.MkdirAll(sourceRoot, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", sourceRoot, err)
	}

	name := stencils.Registry().Names()[0]
	targetPath := filepath.Join(sourceRoot, filepath.FromSlash(stencilstore.RelPath(name)))
	if _, err := os.Stat(targetPath); !os.IsNotExist(err) {
		t.Fatalf("source file for %q unexpectedly present at %s before the test", name, targetPath)
	}

	if _, code, raw := runCLI(t, worktree, "promote", name); code == 0 {
		t.Errorf("promote %s with a contracts/stencils/ tree present but no matching source file = %d; want non-zero. output: %s", name, code, raw)
	}

	if _, err := os.Stat(targetPath); !os.IsNotExist(err) {
		t.Errorf("promote created %s for a missing source file; it must error instead", targetPath)
	}
}

// TestStencilCLI_DriftWarningNeverBlocksExitCode asserts the port-back drift warning fires as a
// plain logger.Warn line and never affects sync's exit code.
func TestStencilCLI_DriftWarningNeverBlocksExitCode(t *testing.T) {
	h := hubforge.NewHub(t, ".")
	worktree := h.PrimeWorktree()
	sourceRoot := filepath.Join(worktree, "contracts", "stencils")

	name := stencils.Registry().Names()[5]
	sourcePath := filepath.Join(sourceRoot, filepath.FromSlash(stencilstore.RelPath(name)))
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(sourcePath), err)
	}
	if err := os.WriteFile(sourcePath, []byte("<!-- stale worktree source -->\n\nSTALE BODY\n"), 0o644); err != nil {
		t.Fatalf("write %s: %v", sourcePath, err)
	}

	var buf bytes.Buffer
	logger.SetOutput(&buf)
	t.Cleanup(func() { logger.SetOutput(os.Stderr) })

	if _, code, raw := runCLI(t, worktree, "sync"); code != 0 {
		t.Fatalf("stencil sync = %d; want 0 even though the board copy has drifted from the worktree source. output: %s", code, raw)
	}

	if !strings.Contains(buf.String(), "board copy has drifted from worktree source") {
		t.Errorf("expected a port-back drift warning on stderr; got %q", buf.String())
	}
}
