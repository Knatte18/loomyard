// weftgit_exclude_test.go proves fabric's own lock artifacts (.weft/ from Fabric.Commit's write lock, and a legacy lock file left by an earlier binary) never surface as untracked dirt in the weft worktree: ensureWeftLockDir seeds them into the weft repo's info/exclude, so Remove's no-force dirty gate (a raw `git status --porcelain`, untracked included) never dead-ends on them, while a lock file that strays into _lym shows.
// It also proves every module's machine-local artifact — locks, pause flags, rendered prompts — is kept out of weft history structurally, by living under .lym and so never falling inside a weft-commit pathspec, rather than by an exclude-layer pattern.
//
// Package fabricengine_test to construct a real *fabricengine.Fabric against a real hubforge.NewHub
// hub; shares the TestMain in testmain_test.go. newFabricPair/writeWeftConfig below are this file's
// own fixture helpers, relocated here from weftgit_differential_test.go before its deletion (this
// file was already its only other consumer), then rebuilt onto a real hub in this batch: a real hub
// materializes config at BoardDir and WeftBase for real, so no stand-in-hub scaffolding is seeded by
// hand any more.
// gitStatusPorcelain moved to gitkit.GitStatusPorcelain, which this file now calls directly.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// newFabricPair returns a *fabricengine.Fabric obtained through fabricengine's ordinary exported
// constructor against a fresh hubforge.NewHub hub's own genuine warp/weft pair, plus the weft
// worktree path callers write directly into.
func newFabricPair(t *testing.T) (*fabricengine.Fabric, string) {
	t.Helper()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	f, err := fabricengine.Open(h.Location)
	if err != nil {
		t.Fatalf("fabricengine.Open: %v", err)
	}
	return f, h.PrimeRecords()
}

// writeWeftConfig overwrites the tracked _lym/config.yaml file a real hub's weft worktree ships
// with, the standard way this file dirties a weft worktree's pathspec-covered content.
func writeWeftConfig(t *testing.T, weftPath, content string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(weftPath, lymdirs.LymDirName, "config.yaml"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

// TestCommitWeft_LockArtifactsExcludedFromStatus commits scoped weft content (which creates the
// .weft lock dir) and drops a legacy push lock file, then asserts neither artifact appears in `git status
// --porcelain` — the exact check Remove's no-force weft dirty gate runs — while a stray lock file under _lym does.
func TestCommitWeft_LockArtifactsExcludedFromStatus(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	f, weftPath := newFabricPair(t)
	writeWeftConfig(t, weftPath, "modified for exclude test")

	if result, err := f.Commit([]string{lymdirs.LymDirName}, fabricengine.DefaultCommitMessage, nil, fabricengine.SyncOptions{}); err != nil {
		t.Fatalf("Commit: %v", err)
	} else if !result.WeftCommitted {
		t.Fatal("Commit WeftCommitted = false; want true")
	}

	// Precondition: the commit really did materialize the lock dir — the
	// artifact this test claims is excluded must actually exist on disk.
	if _, err := os.Stat(filepath.Join(weftPath, ".weft")); err != nil {
		t.Fatalf(".weft lock dir missing after Commit: %v", err)
	}

	// Materialize the push lock an earlier binary left at the worktree root.
	const legacyPushLock = ".gitrepo-push.lock"
	if err := os.WriteFile(filepath.Join(weftPath, legacyPushLock), nil, 0o644); err != nil {
		t.Fatalf("write legacy push lock: %v", err)
	}
	const strayLock = "stray-record.json.lock"
	if err := os.WriteFile(filepath.Join(weftPath, lymdirs.LymDirName, strayLock), nil, 0o644); err != nil {
		t.Fatalf("write stray lock: %v", err)
	}

	status := gitkit.GitStatusPorcelain(t, weftPath)
	if strings.Contains(status, ".weft") {
		t.Errorf("git status --porcelain reports .weft as dirt: %q; want it git-excluded", status)
	}
	if strings.Contains(status, legacyPushLock) {
		t.Errorf("git status --porcelain reports %s as dirt: %q; want it git-excluded", legacyPushLock, status)
	}
	if !strings.Contains(status, strayLock) {
		t.Errorf("git status --porcelain = %q; want the stray %s under %s shown", status, strayLock, lymdirs.LymDirName)
	}
}

// nonEmptyExcludeLines splits raw info/exclude content into its non-empty,
// non-comment lines, in file order — the shape seedWeftArtifactExcludes'
// entries take, with git's own boilerplate `#`-comment lines filtered out.
func nonEmptyExcludeLines(content string) []string {
	var lines []string
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		lines = append(lines, trimmed)
	}
	return lines
}

// TestCommitWeft_SeedsFabricArtifactsOnlyAndIsIdempotent proves seedWeftArtifactExcludes' shape: it seeds fabric's own .weft/ lock directory, lymdirs.DotLymDirName + "/" and the legacy lock file names into the weft repo's info/exclude, removes the retired *.lock / *.swaplock lines, and seeds none of the retired cross-module machine-local patterns.
// Re-seeding via a second commit leaves the file byte-identical.
func TestCommitWeft_SeedsFabricArtifactsOnlyAndIsIdempotent(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	f, weftPath := newFabricPair(t)
	writeWeftConfig(t, weftPath, "modified for exclude test")

	excludePath := filepath.Join(weftPath, ".git", "info", "exclude")
	seeded, err := os.ReadFile(excludePath)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("read info/exclude: %v", err)
	}
	if err := os.WriteFile(excludePath, append(seeded, []byte("*.lock\n*.swaplock\n")...), 0o644); err != nil {
		t.Fatalf("write info/exclude: %v", err)
	}

	if _, err := f.Commit([]string{lymdirs.LymDirName}, fabricengine.DefaultCommitMessage, nil, fabricengine.SyncOptions{}); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	first, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatalf("read info/exclude: %v", err)
	}

	wantLines := []string{".weft/", lymdirs.DotLymDirName + "/", ".gitrepo-push.lock", "board.lock", "board.push.lock", "board.json.swaplock", "tasks.json.swaplock", "notes.json.swaplock", "generation-record.json.lock", "settle-record.json.lock", "settle-record.patch.lock"}
	gotLines := nonEmptyExcludeLines(string(first))
	if len(gotLines) != len(wantLines) {
		t.Fatalf("info/exclude entries = %v; want exactly the entries of %v", gotLines, wantLines)
	}
	for _, want := range wantLines {
		if !slices.Contains(gotLines, want) {
			t.Errorf("info/exclude entries = %v; want %q among them", gotLines, want)
		}
	}

	retiredPatterns := []string{
		"*.lock",
		"*.swaplock",
		"**/" + lymdirs.LymDirName + "/*/**/*.lock",
		"**/" + lymdirs.LymDirName + "/*/pause",
		"**/" + lymdirs.LymDirName + "/*/prompts/",
	}
	for _, retired := range retiredPatterns {
		if slices.Contains(gotLines, retired) {
			t.Errorf("info/exclude seeds retired cross-module pattern %q; want it gone", retired)
		}
	}

	// A second Commit re-runs ensureWeftLockDir/seedWeftArtifactExcludes;
	// the exclude file must come out byte-identical, not merely
	// equivalent, proving the re-seed is a true no-op.
	writeWeftConfig(t, weftPath, "modified again for exclude test")
	if _, err := f.Commit([]string{lymdirs.LymDirName}, fabricengine.DefaultCommitMessage, nil, fabricengine.SyncOptions{}); err != nil {
		t.Fatalf("second Commit: %v", err)
	}
	second, err := os.ReadFile(excludePath)
	if err != nil {
		t.Fatalf("read info/exclude after second commit: %v", err)
	}
	if string(first) != string(second) {
		t.Errorf("info/exclude changed on re-seed:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

// mustWriteFile writes content to path, creating parent directories as
// needed, failing the test on any error.
func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile %s: %v", path, err)
	}
}

// TestCommitWeft_MachineLocalArtifactsNeverEnterWeftTreeAtAnyDepth proves the structural replacement for F-B's fix (formerly the Weft Git Invariant's "Cross-module exclusions"): fabric's OWN sync pathspec — fabricengine.ScopedPathspec(relPath, []string{lymdirs.LymDirName}), positive entries only, no exclusions, the exact shape internal/fabriccli/weft_verbs.go builds for `lym fabric sync`/`lym config <module> --set ...`
// — never stages another module's lock file, pause flag, or rendered-prompt directory, because those now live under .lym and so fall outside the _lym-scoped pathspec entirely rather than being caught by a git-exclude pattern.
//
// Exercised at the weft worktree root AND at two nested anchor depths, one real hub per anchor —
// proving the exclusion holds at every depth Fabric.Commit's classification can be resolved at, not
// just the root. hubforge's warp template carries "backend" and "wts/some-task" as real anchors
// alongside ".", which is what makes each depth here a genuine hubforge.NewHub hub rather than the
// old fixture's hand-written .lym-anchor marker naming an arbitrary "sub" path no real hub can
// produce; each anchor's Fabric.Commit call resolves its own AnchorRel from lymcwd, which is what
// classifyPaths needs to route an anchored pathspec to the weft side at all, so one Fabric handle
// cannot stand in for all three depths the way a single shared weft checkout would suggest. A
// durable per-module state file is written under _lym and committed alongside the .lym artifacts at
// every depth, proving the property is exact and does not over-match real state.
func TestCommitWeft_MachineLocalArtifactsNeverEnterWeftTreeAtAnyDepth(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	for _, anchor := range []string{".", "backend", "wts/some-task"} {
		t.Run(anchor, func(t *testing.T) {
			// The shape set holds only "." and "backend"; the nested anchor has no template.
			var h *hubforge.Hub
			if anchor == "wts/some-task" {
				h = hubforge.NewHub(t, anchor)
			} else {
				h = hubforge.CopyHub(t, hubforge.Shape{Anchor: anchor})
			}
			f, err := fabricengine.Open(h.Location)
			if err != nil {
				t.Fatalf("fabricengine.Open: %v", err)
			}
			weftPath := h.PrimeRecords()
			anchorRel := filepath.FromSlash(anchor)

			dotLymDir := filepath.Join(weftPath, anchorRel, lymdirs.DotLymDirName)
			mustWriteFile(t, filepath.Join(dotLymDir, "webster", "pause"), "")
			mustWriteFile(t, filepath.Join(dotLymDir, "webster", "prompts", "01.md"), "prompt")

			lymDir := filepath.Join(weftPath, anchorRel, lymdirs.LymDirName)
			mustWriteFile(t, filepath.Join(lymDir, "webster", "state.json"), "{}")

			pathspec := fabricengine.ScopedPathspec(anchor, []string{lymdirs.LymDirName})
			if _, err := f.Commit(pathspec, fabricengine.DefaultCommitMessage, nil, fabricengine.SyncOptions{}); err != nil {
				t.Fatalf("Commit(anchor=%q, pathspec=%v): %v", anchor, pathspec, err)
			}

			tracked := strings.Join(gitkit.LsFiles(t, weftPath), "\n")
			lymRel := filepath.ToSlash(filepath.Join(anchorRel, lymdirs.LymDirName))
			durable := lymRel + "/webster/state.json"
			if !strings.Contains(tracked, durable) {
				t.Errorf("git ls-files at anchor=%q does not track durable %q; want it committed\nfull ls-files:\n%s", anchor, durable, tracked)
			}

			for _, forbidden := range []string{".lock", "pause", "prompts"} {
				if strings.Contains(tracked, forbidden) {
					t.Errorf("git ls-files tracks a %q-named entry; want none\nfull ls-files:\n%s", forbidden, tracked)
				}
			}
		})
	}
}

// TestCommit_EntryMatchingOnlyAnIgnoredFile_DegradesToCleanNoOp is the --exclude-standard
// regression: without it, entryMatchesWeft's `git ls-files --cached --others` probe matches a file
// that exists only because it is untracked, INCLUDING one hidden by .gitignore/info-exclude, so
// weftPathspecFilter forwards a doomed entry to `git add`, which then refuses an ignored path and
// fails the whole StageAndCommit call with exit 1 — toppling a commit that had no other content in
// this call. With --exclude-standard, the ignored-only entry is filtered out, positive stays false,
// and the call degrades to a clean no-op (WeftCommitted=false, no error).
// This test has a verified pre-fix failure: on the pre-card-41 code it fails with a
// "gitrepo: git add:" error instead of landing a clean no-op.
func TestCommit_EntryMatchingOnlyAnIgnoredFile_DegradesToCleanNoOp(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	f, weftPath := newFabricPair(t)

	ignoredRel := filepath.ToSlash(filepath.Join(lymdirs.LymDirName, "ignored.tmp"))
	excludePath := filepath.Join(weftPath, ".git", "info", "exclude")
	existing, err := os.ReadFile(excludePath)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("read info/exclude: %v", err)
	}
	if err := os.WriteFile(excludePath, append(existing, []byte("ignored.tmp\n")...), 0o644); err != nil {
		t.Fatalf("write info/exclude: %v", err)
	}
	mustWriteFile(t, filepath.Join(weftPath, lymdirs.LymDirName, "ignored.tmp"), "ignored, never committed")

	result, err := f.Commit([]string{ignoredRel}, fabricengine.DefaultCommitMessage, nil, fabricengine.SyncOptions{})
	if err != nil {
		t.Fatalf("Commit(%q) error = %v; want a clean no-op, not a git add failure on an ignored-only entry", ignoredRel, err)
	}
	if result.WeftCommitted {
		t.Errorf("Commit(%q) = %+v; want WeftCommitted=false (nothing but an ignored file matched)", ignoredRel, result)
	}
}
