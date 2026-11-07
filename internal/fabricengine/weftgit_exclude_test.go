// weftgit_exclude_test.go tests that fabric's lock artifacts never show as untracked dirt in the weft worktree.
// ensureWeftLockDir seeds them into the weft repo's info/exclude, so Remove's no-force dirty gate never stops on them.
// Machine-local artifacts (locks, pause flags, rendered prompts) live under .lym.
// No _lym-scoped weft pathspec matches them.

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

// newFabricPair returns a *fabricengine.Fabric opened on a fresh hubforge hub.
// It also returns the hub's weft worktree path, where callers write files directly.
func newFabricPair(t *testing.T) (*fabricengine.Fabric, string) {
	t.Helper()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	f, err := fabricengine.Open(h.Location)
	if err != nil {
		t.Fatalf("fabricengine.Open: %v", err)
	}
	return f, h.PrimeRecords()
}

// writeWeftConfig overwrites the tracked _lym/config.yaml in a hub's weft worktree.
// The change falls under the _lym pathspec, so a commit has content to pick up.
func writeWeftConfig(t *testing.T, weftPath, content string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(weftPath, lymdirs.LymDirName, "config.yaml"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

// TestCommitWeft_LockArtifactsExcludedFromStatus commits scoped weft content, which creates the .weft lock dir.
// It then drops a legacy push lock file, and it asserts neither artifact appears in `git status --porcelain`.
// The check is the one Remove's no-force dirty gate runs; a stray lock file under _lym must still show.
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

	// Precondition: the lock dir exists on disk, so its absence from status proves an exclusion, not a missing file.
	if _, err := os.Stat(filepath.Join(weftPath, ".weft")); err != nil {
		t.Fatalf(".weft lock dir missing after Commit: %v", err)
	}

	// Create the legacy push lock an older binary left at the worktree root.
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

// nonEmptyExcludeLines returns the non-empty lines of raw info/exclude content, in file order.
// Lines starting with `#`, git's comment lines, are dropped.
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

// TestCommitWeft_SeedsFabricArtifactsOnlyAndIsIdempotent checks the entries seeded into the weft repo's info/exclude.
// They are fabric's own .weft/ lock dir, the .lym/ dir, and the legacy lock file names, and no cross-module pattern.
// The test first writes retired *.lock and *.swaplock lines, which the commit must remove.
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

	// A second Commit re-runs ensureWeftLockDir and seedWeftArtifactExcludes,
	// and the exclude file must come out byte-identical, not merely equivalent, so the re-seed is a true no-op.
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

// mustWriteFile writes content to path, creating parent directories, and fails the test on any error.
func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile %s: %v", path, err)
	}
}

// TestCommitWeft_MachineLocalArtifactsNeverEnterWeftTreeAtAnyDepth proves the sync never stages machine-local files.
// Another module's lock file, pause flag, or rendered-prompt directory is one such file.
// Those files live under .lym, outside the _lym-scoped pathspec, so no git-exclude pattern is needed.
//
// The test runs at the weft root and at two nested anchors, each on its own real hub.
// Each anchor's Fabric.Commit resolves its own anchor-relative path, so one Fabric cannot stand in for all three.
// Each depth also commits a durable state file under _lym next to the .lym artifacts,
// so the test shows the pathspec is exact: real state under _lym is still committed.
func TestCommitWeft_MachineLocalArtifactsNeverEnterWeftTreeAtAnyDepth(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	for _, anchor := range []string{".", "backend", "wts/some-task"} {
		t.Run(anchor, func(t *testing.T) {
			// Only the "." and "backend" anchors have a template, so the nested anchor is built with NewHub.
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

// TestCommit_EntryMatchingOnlyAnIgnoredFile_DegradesToCleanNoOp covers an entry matching only a git-ignored file.
// The entryMatchesWeft probe (`git ls-files --cached --others`) matches untracked files, including ignored ones.
// weftPathspecFilter then forwards that entry to `git add`;
// git refuses an ignored path and fails the whole StageAndCommit call.
// The --exclude-standard flag keeps ignored files out of the probe,
// so the entry is dropped and the call degrades to a clean no-op (WeftCommitted=false, no error).
// Without the flag, the call fails with a "gitrepo: git add:" error instead of a clean no-op.
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
