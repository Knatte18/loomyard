// tierpurity_test.go enforces the Test Tier Purity Invariant.
// A *_test.go file whose tests are `tier0` (their opening tiergate.Require names tier 0) performs no expensive spawn.
// The banned spawns are gitexec.Run, exec.Command, exec.CommandContext and hubforge.NewHub, which builds a real hub.
// Every gitkit export is banned except the hermetic-environment helper.
// A support file, one holding no test function, is not scanned.
// One test binary holds every tier, so its helpers may serve any tier's tests.
// It also flags a tier0 file that holds a long literal time.Sleep(...).

package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/impactset"
	"github.com/Knatte18/loomyard/internal/testkit/scankit"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// tmuxTier is the lowest tier whose tests may start a tmux server.
const tmuxTier = 2

// allowedSpawners lists the files and directories that may contain a banned spawn token in a tier0 test file.
// Each entry is a module-relative, slash-separated file path, or a directory path ending in "/", with a one-line reason.
var allowedSpawners = []scankit.Entry{
	{Key: "internal/proc/", Why: "process control is the package's subject — its tests must spawn"},
	{Key: "cmd/lym/tierpurity_test.go", Why: "contains the banned token strings as its own test data"},
	{Key: "cmd/lym/llmtier_test.go", Why: "contains the banned `exec.Command` token string as its own fixture data (LLM-tier guard)"},
	{Key: "internal/testcover/spawnscan_test.go", Why: "contains the banned `exec.Command` token string as fixture source for the spawn scan, which only parses it"},
	{Key: "cmd/lym/hermeticenv_test.go", Why: "contains the banned token strings as its own test data (Hermetic Git Test Environment Invariant guard)"},
	{Key: "tools/sandbox/pathresolve_guard_test.go", Why: "contains the banned `exec.Command`/`exec.CommandContext` token strings as its own scan data (Dev/Prod Binary Separation guard)"},
	{Key: "cmd/lym/ghguard_test.go", Why: "contains the banned `exec.Command`/`exec.CommandContext` token strings as its own scan data (GitHub Auth Invariant guard)"},
	{Key: "cmd/lym/gitrepoboundary_test.go", Why: "names `gitexec.RunGit` in its own doc comment (gitrepo Client Boundary Invariant guard)"},
	{Key: "cmd/lym/boardguard_test.go", Why: "contains the banned `exec.Command`/`exec.CommandContext` token strings as its own scan data (Fabric Git Invariant board-guard)"},
	{Key: "cmd/lym/rawgitmutation_test.go", Why: "contains the banned `gitexec.Run`/`exec.Command` token strings as its own scan data (Fabric Git Invariant raw-git-mutation guard)"},
	{Key: "cmd/lym/checkedcall_test.go", Why: "contains the banned `gitexec.RunGit`/`exec.Command` token strings as its own scan data (gitexec Checked-Call Invariant guard)"},
	{Key: "internal/reedengine/socketfile_unix_test.go", Why: "references the kit's spawn-free BindStaleSocket, which binds and closes a datagram socket file and starts no server"},
	{Key: "internal/shuttleengines/shuttleengines_test.go", Why: "references the kit's spawn-free ServerEnviron, a pure environment filter that starts no server"},
	{Key: "cmd/lym/spawnobservability_test.go", Why: "contains the banned `exec.Command`/`exec.CommandContext` token strings as its own scan data (Live-Substrate Spawn Observability guard)"},
	{Key: "cmd/lym/interactivespawn_test.go", Why: "contains the banned `exec.Command`/`exec.CommandContext` token strings as fixture source for the headless-spawn scan, which only parses it (`PATTERN-reed-panes` guard)"},
}

// allowedTmuxkitOutsideTmuxTier lists tier1 test files that reference a tmuxkit export.
// Each entry gives a reason the file starts no server.
var allowedTmuxkitOutsideTmuxTier = []scankit.Entry{
	{Key: "internal/gatecli/test_integration_test.go", Why: "reads a process's state through the kit's spawn-free ProcState probe; starts no server"},
}

// bannedTokens are the raw substrings a tier0 *_test.go file may not contain.
// Matching is raw-substring, not whole-token or AST, so exec.Command also matches exec.CommandContext.
// The gitkit spawn is not a token here;
// gitkitSpawnReference (cmd/lym/gitkitspawn_test.go) defines it once, for this scan and the Hermetic Env scan alike.
// The tmuxkit spawn is not a token here either;
// tmuxkitSpawnReference (cmd/lym/tmuxkitspawn_test.go) defines it once, as every tmuxkit export but Main.
// Comment and string-literal mentions trip the guard too, which is accepted:
// rename the mention or make the file's tests tier1.
// hubforge.NewHub is banned:
// it drives a real fabriccli.CloneAndWire clone, so a tier0 test calling it is an expensive-spawn violation.
// hubforge.SeedConfig and hubforge.SeedFabricConfig need no entries of their own:
// both take a *Hub that only NewHub can produce.
// hubforge.CopyHub and hubforge.SharedHub are banned the same way, since each builds a real hub on first use.
// Of quarry's methods, only DeltaGit spawns a process, so the token names DeltaGit alone.
// The token catches only a direct textual DeltaGit call.
// A test that calls a planglyph wrapper reaching DeltaGit transitively still passes.
// The guard narrows this gap but does not close it.
var bannedTokens = []string{
	"gitexec.Run",
	"exec.Command",
	"hubforge.NewHub",
	"hubforge.CopyHub",
	"hubforge.SharedHub",
	"DeltaGit",
	"lymbin.",
}

// TestTierPurity_UntaggedTestsSpawnNothing walks every `*_test.go` file under the module root.
// It fails if a tier0 file contains a banned spawn token as a raw substring.
// A tier0 file is one whose tests `impactset.FileTier` reads as tier 0.
// The file or its containing directory may still contain one if it is on the `allowedSpawners` allowlist.
// It also fails if a tier1 file references a tmuxkit spawn, unless `allowedTmuxkitOutsideTmuxTier` lists it.
// A support file holding no test function is skipped.
//
//lym:guard
func TestTierPurity_UntaggedTestsSpawnNothing(t *testing.T) {
	tiergate.Require(t, 0)
	spawners := scankit.NewAllowlist(allowedSpawners)
	sleepers := scankit.NewAllowlist(allowedLongSleepers)
	tmuxless := scankit.NewAllowlist(allowedTmuxkitOutsideTmuxTier)
	var failures []string

	scanned := scankit.Walk(t, scankit.Options{Filter: scankit.Test}, func(f *scankit.File) {
		file := f.AST(t, parser.ParseComments|parser.SkipObjectResolution)
		failures = append(failures, tierPurityFailures(f.Rel, f.Abs, f.Data, file, spawners, sleepers, tmuxless)...)
	})
	scankit.RequireFloor(t, scanned, 20, "tier purity guard")
	spawners.RequireNoStale(t)
	sleepers.RequireNoStale(t)
	tmuxless.RequireNoStale(t)

	if len(failures) > 0 {
		t.Errorf("`PATTERN-test-speed` violated:\n%s", strings.Join(failures, "\n"))
	}
}

// tierPurityFailures returns the Test Tier Purity failures of one test file:
// rel is its module-relative path, abs its absolute path, data its source and file its parse.
// A support file holding no test function yields none.
// A tier0 file fails on a banned spawn token or a long literal sleep unless the matching allowlist lists it;
// a tier1 file fails on a tmuxkit spawn reference unless tmuxless lists it.
func tierPurityFailures(rel, abs string, data []byte, file *ast.File, spawners, sleepers, tmuxless *scankit.Allowlist) []string {
	if len(testFunctions(file)) == 0 {
		return nil
	}
	var failures []string
	if tier := impactset.FileTier(file); tier > 0 {
		if ref, found := tmuxkitSpawnReference(string(data)); found && tier < tmuxTier && !tmuxless.Allowed(rel) {
			failures = append(failures, fmt.Sprintf(
				"%s: references tmuxkit spawn %q in a tier%d test file — make its tests tier2 with tiergate.Require(t, 2), or split the reed-starting half into a tier2 file",
				rel, ref, tier,
			))
		}
		return failures
	}

	bannedTok, bad := firstBannedToken(data)
	if !bad {
		bannedTok, bad = gitkitSpawnReference(string(data))
	}
	if !bad {
		bannedTok, bad = tmuxkitSpawnReference(string(data))
	}
	if bad && !spawners.Allowed(rel) {
		failures = append(failures, fmt.Sprintf(
			"%s: contains banned token %q in a tier0 test file — make the spawning tests tier1 or higher with tiergate.Require in a file of their own, or add an allowedSpawners entry in cmd/lym/tierpurity_test.go with a reason",
			rel, bannedTok,
		))
	}

	// Sleep guard is an independent check and must still run for every tier0 file.
	if !sleepers.Allowed(rel) {
		if evidence, found := findLongLiteralSleep(token.NewFileSet(), abs, data); found {
			failures = append(failures, fmt.Sprintf(
				"%s: contains a literal time.Sleep(...) of >= 1s in a tier0 test file (%s) — move it to a tier1 test, shrink the duration, or add an allowedLongSleepers entry in cmd/lym/tiersleep_test.go with a reason",
				rel, evidence,
			))
		}
	}
	return failures
}

// TestTierPurityFailures proves the per-file checks of `tierPurityFailures` on source text.
// It checks which files are skipped, flagged and let through.
func TestTierPurityFailures(t *testing.T) {
	tiergate.Require(t, 0)
	t.Parallel()

	const rel = "p/a_test.go"
	const requireImport = "package p\n\nimport \"github.com/Knatte18/loomyard/internal/testkit/tiergate\"\n\n"
	tests := []struct {
		name         string
		src          string
		allowTmuxkit bool
		want         string
	}{
		{"SupportFileIsSkipped", "package p\n\nfunc helper() { exec.Command(\"git\") }\n", false, ""},
		{"Tier0FileWithoutSpawn", "package p\n\nfunc TestA(t *testing.T) {}\n", false, ""},
		{"Tier0FileWithBannedToken", "package p\n\nfunc TestA(t *testing.T) { exec.Command(\"git\") }\n", false, "banned token \"exec.Command\""},
		{"Tier1FileReferencingTmuxkitSpawn", requireImport + "func TestA(t *testing.T) {\n\ttiergate.Require(t, 1)\n\ttmuxkit.Socket(t)\n}\n", false, "tmuxkit spawn \"tmuxkit.Socket\""},
		{"Tier1FileOnTheTmuxkitAllowlist", requireImport + "func TestA(t *testing.T) {\n\ttiergate.Require(t, 1)\n\ttmuxkit.Socket(t)\n}\n", true, ""},
		{"Tier2FileReferencingTmuxkitSpawn", requireImport + "func TestA(t *testing.T) {\n\ttiergate.Require(t, 2)\n\ttmuxkit.Socket(t)\n}\n", false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			file, err := parser.ParseFile(token.NewFileSet(), rel, tt.src, parser.ParseComments|parser.SkipObjectResolution)
			if err != nil {
				t.Fatal(err)
			}
			var tmuxEntries []scankit.Entry
			if tt.allowTmuxkit {
				tmuxEntries = append(tmuxEntries, scankit.Entry{Key: rel, Why: "fixture"})
			}
			got := tierPurityFailures(rel, rel, []byte(tt.src), file, scankit.NewAllowlist(nil), scankit.NewAllowlist(nil), scankit.NewAllowlist(tmuxEntries))
			if tt.want == "" {
				if len(got) != 0 {
					t.Errorf("tierPurityFailures() = %q; want none", got)
				}
				return
			}
			if len(got) != 1 || !strings.Contains(got[0], tt.want) {
				t.Errorf("tierPurityFailures() = %q; want one failure containing %q", got, tt.want)
			}
		})
	}
}

// TestFirstBannedToken_RecognizesDeltaGit proves `firstBannedToken` finds the literal DeltaGit token in test source.
//
//testtiming:keep proves the TestTierPurity_UntaggedTestsSpawnNothing guard fires on a tier0 file carrying the DeltaGit token
func TestFirstBannedToken_RecognizesDeltaGit(t *testing.T) {
	tiergate.Require(t, 0)
	tests := []struct {
		name string
		src  string
		want bool
	}{
		{"direct DeltaGit call", "package p\n\nfunc f() { repo.DeltaGit(from, to, \".\") }\n", true},
		{"no banned token at all", "package p\n\nfunc f() { repo.Resolve(targets) }\n", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, got := firstBannedToken([]byte(tt.src))
			if got != tt.want {
				t.Errorf("firstBannedToken(%q) bad = %v; want %v", tt.src, got, tt.want)
			}
		})
	}
}

// firstBannedToken returns the first `bannedTokens` entry, in declared order, that appears in data as a raw substring.
// It reports whether an entry was found.
func firstBannedToken(data []byte) (string, bool) {
	content := string(data)
	for _, token := range bannedTokens {
		if strings.Contains(content, token) {
			return token, true
		}
	}
	return "", false
}
