//go:build tier0

// tierpurity_test.go enforces the Test Tier Purity Invariant: untagged *_test.go files (the ones that run in `tier0`, without `-tags tier1`, `tier2` or `tier3`) perform no expensive spawns — no gitexec.Run, no exec.Command/CommandContext, no gitkit spawn (every gitkit export but the hermetic-environment helper), and no hubforge.NewHub real-hub fixture build.
// This is the repo-wide grep-guard that keeps the offline Tier 1 loop's premise from rotting silently again, machine-enforcing what was previously review discipline only.
// See `PATTERN-test-speed`.
// It also flags an untagged file containing a long literal time.Sleep(...)
// (see cmd/lym/tiersleep_test.go).

package main

import (
	"fmt"
	"go/build/constraint"
	"go/token"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/testkit/scankit"
)

// allowedSpawners is the Test Tier Purity Invariant allowlist: module-relative,
// slash-separated file paths, or directory paths ending in "/", that are permitted to contain
// a banned spawn token in an untagged test file, each with a one-line reason —
// mirroring sandbox_coverage_test.go's excludedModules style.
var allowedSpawners = []scankit.Entry{
	{Key: "internal/proc/", Why: "process control is the package's subject — its tests must spawn"},
	{Key: "cmd/lym/tierpurity_test.go", Why: "contains the banned token strings as its own test data"},
	{Key: "cmd/lym/llmtier_test.go", Why: "contains the banned `exec.Command` token string as its own fixture data (LLM-tier guard)"},
	{Key: "cmd/testtiming/spawnscan_test.go", Why: "contains the banned `exec.Command` token string as fixture source for the spawn scan, which only parses it"},
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
	{Key: "cmd/lym/interactivespawn_test.go", Why: "contains the banned `exec.Command`/`exec.CommandContext` token strings as fixture source for the headless-spawn scan, which only parses it (`PATTERN-interactive-spawn` guard)"},
}

// allowedTmuxkitOutsideTmuxTier lists tagged files that compile in tier 2 and reference a tmuxkit export, each with a reason it starts no server there.
var allowedTmuxkitOutsideTmuxTier = []scankit.Entry{
	{Key: "internal/loomcli/smoke_helpers_test.go", Why: "reads /proc through the kit's spawn-free Pids, ProcArgv, ProcCwd, ProcPPID and IsWatchdog probes to find a fixture's driver and watchdog processes; starts no server"},
	{Key: "internal/shedverbs/loop_integration_test.go", Why: "reads a process's state through the kit's spawn-free ProcState probe; starts no server"},
	{Key: "internal/gatecli/test_integration_test.go", Why: "reads a process's state through the kit's spawn-free ProcState probe; starts no server"},
}

// knownTierTags are the build tags that mark a *_test.go file as tagged (i.e. not run in `tier0`) for Test Tier Purity purposes:
// the tier tags `tier1`, `tier2` and `tier3`.
// A `tier0` tag is no entry: a file tagged `tier0` runs in `tier0` and is held to the untagged rules.
// isTierTagged reads a file as tagged when its constraint can hold only with one of these tags, so adding a new tier tag here is the single place
// that both the purity guard and its doc comments need to stay in sync with.
var knownTierTags = []string{"tier1", "tier2", "tier3"}

// bannedTokens are the raw substrings an untagged *_test.go file may not contain.
// Matching is deliberately raw-substring, not whole-token or AST: exec.Command also matches exec.CommandContext.
// A gitkit spawn is not a token here: gitkitSpawnReference (cmd/lym/gitkitspawn_test.go) defines it once, for this scan and the Hermetic Env scan alike.
// A tmuxkit spawn is not a token here either: tmuxkitSpawnReference (cmd/lym/tmuxkitspawn_test.go) defines it once, as every tmuxkit export but Main.
// Comment or string-literal mentions trip the guard too — that is
// accepted (rename the mention or tag the file).
// hubforge.NewHub is banned by the same rule: it drives a real fabriccli.CloneAndWire clone, so an
// untagged test calling it is exactly the expensive-spawn violation this guard exists to catch.
// hubforge.SeedConfig and hubforge.SeedFabricConfig need no separate entries — both take a *Hub that
// only NewHub can produce, so this token already covers every package that can reach them.
// hubforge.CopyHub and hubforge.SharedHub are banned the same way, since each builds a real hub on first use.
// DeltaGit is deliberately narrow and deliberately NOT quarry.Open: only DeltaGit spawns a
// process, while Resolve, TOC, Glyphs and Expand read files and Name performs no I/O at all, so
// banning the constructor would force integration tags onto tests that spawn nothing. Raw-substring
// is the right shape here, matching this guard's own documented design.
// bannedTokens scans untagged *_test.go files, so it catches a DIRECT TEXTUAL DeltaGit call site
// only — a test calling a planglyph wrapper that reaches DeltaGit transitively contains no such
// token and still passes. The existing tokens accept exactly this limit and this one inherits it;
// the guard narrows the gap, it does not close it, and a later change must not restate this as full
// coverage.
var bannedTokens = []string{
	"gitexec.Run",
	"exec.Command",
	"hubforge.NewHub",
	"hubforge.CopyHub",
	"hubforge.SharedHub",
	"DeltaGit",
	"lymbin.",
}

// TestTierPurity_UntaggedTestsSpawnNothing walks every *_test.go file under the module root.
// It fails if any untagged file contains a banned spawn token as a raw substring, unless the file (or its containing directory) is on the allowedSpawners allowlist.
// An untagged file is one whose `//go:build` constraint, if it has one, can hold without any of knownTierTags.
// Platform-only constraints (e.g. `//go:build windows`) count as untagged: they still run in `tier0` on that platform, and so does `tier0`.
//
//lym:guard
func TestTierPurity_UntaggedTestsSpawnNothing(t *testing.T) {
	spawners := scankit.NewAllowlist(allowedSpawners)
	sleepers := scankit.NewAllowlist(allowedLongSleepers)
	tmuxless := scankit.NewAllowlist(allowedTmuxkitOutsideTmuxTier)
	var failures []string

	scanned := scankit.Walk(t, scankit.Options{Filter: scankit.Test}, func(f *scankit.File) {
		if isTierTagged(f.Data) {
			if ref, found := tmuxkitSpawnReference(string(f.Data)); found && !isTmuxTier(f.Data) && !tmuxless.Allowed(f.Rel) {
				failures = append(failures, fmt.Sprintf(
					"%s: references tmuxkit spawn %q in a file that compiles in the `tier0` or `tier1` build — constrain the file to `tier2` and `tier3` only, or split the reed-starting half into a `tier2` file",
					f.Rel, ref,
				))
			}
			return
		}

		bannedTok, bad := firstBannedToken(f.Data)
		if !bad {
			bannedTok, bad = gitkitSpawnReference(string(f.Data))
		}
		if !bad {
			bannedTok, bad = tmuxkitSpawnReference(string(f.Data))
		}
		if bad && !spawners.Allowed(f.Rel) {
			failures = append(failures, fmt.Sprintf(
				"%s: contains banned token %q in an untagged test file — move it behind one of knownTierTags' `//go:build` constraints (tier1, tier2 or tier3), or add an allowedSpawners entry in cmd/lym/tierpurity_test.go with a reason",
				f.Rel, bannedTok,
			))
		}

		// Sleep guard is an independent check and must still run for every untagged file.
		if !sleepers.Allowed(f.Rel) {
			if evidence, found := findLongLiteralSleep(token.NewFileSet(), f.Abs, f.Data); found {
				failures = append(failures, fmt.Sprintf(
					"%s: contains a literal time.Sleep(...) of >= 1s in an untagged test file (%s) — move it behind a build tag, shrink the duration, or add an allowedLongSleepers entry in cmd/lym/tiersleep_test.go with a reason",
					f.Rel, evidence,
				))
			}
		}
	})
	scankit.RequireFloor(t, scanned, 20, "tier purity guard")
	spawners.RequireNoStale(t)
	sleepers.RequireNoStale(t)
	tmuxless.RequireNoStale(t)

	if len(failures) > 0 {
		t.Errorf("`PATTERN-test-speed` violated:\n%s", strings.Join(failures, "\n"))
	}
}

// isTierTagged reports whether data's `//go:build` constraint can hold only with one of knownTierTags.
// A file tagged `tier0` or not at all is untagged here.
func isTierTagged(data []byte) bool {
	return holdsOnlyWithOneOf(data, knownTierTags)
}

// isTmuxTier reports whether data's `//go:build` constraint is false in every build that lacks the `tier2` and `tier3` tags.
// A file with no constraint line compiles everywhere and is never tmux-tier.
func isTmuxTier(data []byte) bool {
	return holdsOnlyWithOneOf(data, []string{"tier2", "tier3"})
}

// holdsOnlyWithOneOf reports whether data's `//go:build` constraint is false in every build that lacks all of tags.
// It evaluates the expression under every assignment of its other tags, platform tags included, so the host's GOOS plays no part.
// A file with no constraint line compiles everywhere and holds with none of them.
func holdsOnlyWithOneOf(data []byte, tags []string) bool {
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if !constraint.IsGoBuild(trimmed) {
			return false
		}
		expr, err := constraint.Parse(trimmed)
		if err != nil {
			return false
		}
		var others []string
		seen := map[string]bool{}
		walkTags(expr, func(tag string) {
			if !slices.Contains(tags, tag) && !seen[tag] {
				seen[tag] = true
				others = append(others, tag)
			}
		})
		for assignment := 0; assignment < 1<<len(others); assignment++ {
			enabled := map[string]bool{}
			for i, tag := range others {
				enabled[tag] = assignment&(1<<i) != 0
			}
			if expr.Eval(func(tag string) bool { return enabled[tag] }) {
				return false
			}
		}
		return true
	}
	return false
}

// walkTags calls visit for every tag leaf of expr.
func walkTags(expr constraint.Expr, visit func(tag string)) {
	switch e := expr.(type) {
	case *constraint.TagExpr:
		visit(e.Tag)
	case *constraint.NotExpr:
		walkTags(e.X, visit)
	case *constraint.AndExpr:
		walkTags(e.X, visit)
		walkTags(e.Y, visit)
	case *constraint.OrExpr:
		walkTags(e.X, visit)
		walkTags(e.Y, visit)
	}
}

// TestIsTierTagged_RecognizesKnownTagsList verifies isTierTagged recognizes all known tier tags, and isTmuxTier separates the tmux-only constraints from those that compile in tier 2.
//
//testtiming:keep proves the TestTierPurity_UntaggedTestsSpawnNothing guard classifies every known tier tag as tagged
func TestIsTierTagged_RecognizesKnownTagsList(t *testing.T) {
	tmuxTier := []struct {
		line string
		want bool
	}{
		{"//go:build tier1 || tier2", false},
		{"//go:build tier1 && windows", false},
		{"//go:build tier0", false},
		{"//go:build tier1", false},
		{"//go:build tier2", true},
		{"//go:build tier3", true},
		{"//go:build tier2 || tier3", true},
		{"//go:build tier2 && !windows", true},
	}
	for _, tt := range tmuxTier {
		if got := isTmuxTier([]byte(tt.line)); got != tt.want {
			t.Errorf("isTmuxTier(%q) = %v; want %v", tt.line, got, tt.want)
		}
	}

	tests := []struct {
		name string
		line string
		want bool
	}{
		{"shared_helper_disjunction", "//go:build tier2 || tier3", true},
		{"tier0_untagged", "//go:build tier0", false},
		{"tier0_or_tier1_untagged", "//go:build tier0 || tier1", false},
		{"tier1", "//go:build tier1", true},
		{"tier2", "//go:build tier2", true},
		{"tier3", "//go:build tier3", true},
		{"tier2_not_windows", "//go:build tier2 && !windows", true},
		{"platform_only_untagged", "//go:build windows", false},
		{"empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isTierTagged([]byte(tt.line))
			if got != tt.want {
				t.Errorf("isTierTagged(%q) = %v; want %v", tt.line, got, tt.want)
			}
		})
	}
}

// TestFirstBannedToken_RecognizesDeltaGit proves the new token detection fires: an untagged test file's raw source containing the literal DeltaGit token trips the guard exactly like every other bannedTokens entry.
//
//testtiming:keep proves the TestTierPurity_UntaggedTestsSpawnNothing guard fires on an untagged file carrying the DeltaGit token
func TestFirstBannedToken_RecognizesDeltaGit(t *testing.T) {
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

// firstBannedToken returns the first entry of bannedTokens (in declared order) that
// appears as a raw substring of data, and whether any was found.
func firstBannedToken(data []byte) (string, bool) {
	content := string(data)
	for _, token := range bannedTokens {
		if strings.Contains(content, token) {
			return token, true
		}
	}
	return "", false
}
