// tierpurity_test.go enforces the Test Tier Purity Invariant: untagged *_test.go files (the ones that run in every plain `go test`, without `-tags integration`, `tmux` or `llm`) perform no expensive spawns — no gitexec.Run, no exec.Command/CommandContext, no gitkit spawn (every gitkit export but the hermetic-environment helper), and no hubforge.NewHub real-hub fixture build.
// This is the repo-wide grep-guard that keeps the offline Tier 1 loop's premise from rotting
// silently again, machine-enforcing what was previously review discipline only.
// See `PATTERN-test-speed`.
// It also flags an untagged file containing a long literal time.Sleep(...) (see
// cmd/lyx/tiersleep_test.go).

package main

import (
	"fmt"
	"go/token"
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
	{Key: "cmd/lyx/tierpurity_test.go", Why: "contains the banned token strings as its own test data"},
	{Key: "cmd/lyx/hermeticenv_test.go", Why: "contains the banned token strings as its own test data (Hermetic Git Test Environment Invariant guard)"},
	{Key: "tools/sandbox/pathresolve_guard_test.go", Why: "contains the banned `exec.Command`/`exec.CommandContext` token strings as its own scan data (Dev/Prod Binary Separation guard)"},
	{Key: "cmd/lyx/ghguard_test.go", Why: "contains the banned `exec.Command`/`exec.CommandContext` token strings as its own scan data (GitHub Auth Invariant guard)"},
	{Key: "cmd/lyx/gitrepoboundary_test.go", Why: "names `gitexec.RunGit` in its own doc comment (gitrepo Client Boundary Invariant guard)"},
	{Key: "cmd/lyx/boardguard_test.go", Why: "contains the banned `exec.Command`/`exec.CommandContext` token strings as its own scan data (Fabric Git Invariant board-guard)"},
	{Key: "cmd/lyx/rawgitmutation_test.go", Why: "contains the banned `gitexec.Run`/`exec.Command` token strings as its own scan data (Fabric Git Invariant raw-git-mutation guard)"},
	{Key: "cmd/lyx/checkedcall_test.go", Why: "contains the banned `gitexec.RunGit`/`exec.Command` token strings as its own scan data (gitexec Checked-Call Invariant guard)"},
	{Key: "cmd/lyx/spawnobservability_test.go", Why: "contains the banned `exec.Command`/`exec.CommandContext` token strings as its own scan data (Live-Substrate Spawn Observability guard)"},
}

// knownTierTags are the `//go:build` constraint substrings that mark a *_test.go file
// as tagged (i.e. excluded from a plain `go test` run) for Test Tier Purity purposes.
// isTierTagged matches on any entry, so adding a new tier tag here is the single place
// that both the purity guard and its doc comments need to stay in sync with.
var knownTierTags = []string{"integration", "tmux", "llm"}

// bannedTokens are the raw substrings an untagged *_test.go file may not contain.
// Matching is deliberately raw-substring, not whole-token or AST: exec.Command also matches exec.CommandContext.
// A gitkit spawn is not a token here: gitkitSpawnReference (cmd/lyx/gitkitspawn_test.go) defines it once, for this scan and the Hermetic Env scan alike.
// A tmuxkit spawn is not a token here either: tmuxkitSpawnReference (cmd/lyx/tmuxkitspawn_test.go) defines it once, as every tmuxkit export but Main.
// Comment or string-literal mentions trip the guard too — that is
// accepted (rename the mention or tag the file).
// hubforge.NewHub is banned by the same rule: it drives a real fabriccli.CloneAndWire clone, so an
// untagged test calling it is exactly the expensive-spawn violation this guard exists to catch.
// hubforge.SeedConfig and hubforge.SeedFabricConfig need no separate entries — both take a *Hub that
// only NewHub can produce, so this token already covers every package that can reach them.
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
	"DeltaGit",
	"lyxbin.",
}

// TestTierPurity_UntaggedTestsSpawnNothing walks every *_test.go file under the module root and
// fails if any untagged file — one whose first non-empty line is not a `//go:build` constraint
// mentioning any of knownTierTags — contains a banned spawn token as a raw substring, unless the
// file (or its containing directory) is on the allowedSpawners allowlist.
// Platform-only constraints (e.g. `//go:build windows`) count as untagged: they still run in Tier 1
// on that platform.
func TestTierPurity_UntaggedTestsSpawnNothing(t *testing.T) {
	spawners := scankit.NewAllowlist(allowedSpawners)
	sleepers := scankit.NewAllowlist(allowedLongSleepers)
	var failures []string

	scanned := scankit.Walk(t, scankit.Options{Filter: scankit.Test}, func(f *scankit.File) {
		if isTierTagged(f.Data) {
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
				"%s: contains banned token %q in an untagged test file — move it behind one of knownTierTags' `//go:build` constraints (integration, tmux or llm), or add an allowedSpawners entry in cmd/lyx/tierpurity_test.go with a reason",
				f.Rel, bannedTok,
			))
		}

		// Sleep guard is an independent check and must still run for every untagged file.
		if !sleepers.Allowed(f.Rel) {
			if evidence, found := findLongLiteralSleep(token.NewFileSet(), f.Abs, f.Data); found {
				failures = append(failures, fmt.Sprintf(
					"%s: contains a literal time.Sleep(...) of >= 1s in an untagged test file (%s) — move it behind a build tag, shrink the duration, or add an allowedLongSleepers entry in cmd/lyx/tiersleep_test.go with a reason",
					f.Rel, evidence,
				))
			}
		}
	})
	scankit.RequireFloor(t, scanned, 20, "tier purity guard")
	spawners.RequireNoStale(t)
	sleepers.RequireNoStale(t)

	if len(failures) > 0 {
		t.Errorf("`PATTERN-test-speed` violated:\n%s", strings.Join(failures, "\n"))
	}
}

// isTierTagged reports whether data's first line is a `//go:build` constraint with a known tier tag.
func isTierTagged(data []byte) bool {
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if !strings.HasPrefix(trimmed, "//go:build") {
			return false
		}
		for _, tag := range knownTierTags {
			if strings.Contains(trimmed, tag) {
				return true
			}
		}
		return false
	}
	return false
}

// TestIsTierTagged_RecognizesKnownTagsList verifies isTierTagged recognizes all known tier tags.
func TestIsTierTagged_RecognizesKnownTagsList(t *testing.T) {
	tests := []struct {
		name string
		line string
		want bool
	}{
		{"integration", "//go:build integration", true},
		{"tmux", "//go:build tmux", true},
		{"llm", "//go:build llm", true},
		{"shared_helper_disjunction", "//go:build tmux || llm", true},
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

// TestFirstBannedToken_RecognizesDeltaGit proves the new token detection fires: an untagged test
// file's raw source containing the literal DeltaGit token trips the guard exactly like every other
// bannedTokens entry.
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
