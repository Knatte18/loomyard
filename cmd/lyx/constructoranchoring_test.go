// constructoranchoring_test.go pins every constructor batch 5 relocated out
// of internal/lyxcwd into its owning module to the anchoring table the
// overview's Shared Decisions record: there is no single base. It lives in
// cmd/lyx because this is the only package that may import every owning
// module at once (loomengine, builderengine, websterengine, perchengine,
// scoutengine, pattern, logger, reedengine, planparser). Every case here is
// pure filepath.Join arithmetic -- no subprocess is spawned and no fixture
// tree is copied -- so this file stays untagged, per the Test Tier Purity
// Invariant.
//
// The check is anchor-aware, not byte-identical: two synthetic
// *lyxcwd.Location values are built, one unanchored (AnchorRel == ".") and
// one subpath-anchored (AnchorRel == "backend"). For the unanchored fixture
// every constructor in all three groups is byte-identical to a plain
// filepath.Join computed independently in this file. For the
// subpath-anchored fixture, the _lyx-durable group (AnchorPath-anchored)
// intentionally moves down by AnchorRel, while the .lyx group and
// HubLogsDir (WorktreePath/HubPath-anchored) stay byte-identical to their
// unanchored-fixture values.
package main

import (
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/builderengine"
	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/pattern"
	"github.com/Knatte18/loomyard/internal/perchengine"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/reedengine"
	"github.com/Knatte18/loomyard/internal/scoutengine"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// anchoringFixture builds a synthetic *lyxcwd.Location by hand, mirroring
// the field derivation Resolve performs, without spawning git.
func anchoringFixture(hubPath, worktreeName, anchorRel string) *lyxcwd.Location {
	return &lyxcwd.Location{
		HubPath:      hubPath,
		WorktreeName: worktreeName,
		AnchorRel:    anchorRel,
	}
}

// TestConstructorAnchoring_Unanchored asserts every relocated constructor at
// AnchorRel == "." against a plain filepath.Join computed independently
// here, for all three anchoring groups.
func TestConstructorAnchoring_Unanchored(t *testing.T) {
	hub := filepath.Join("home", "user", "repo-HUB")
	l := anchoringFixture(hub, "repo", ".")

	worktree := l.WorktreePath()
	anchor := l.AnchorPath()
	if anchor != worktree {
		t.Fatalf("AnchorPath() = %q; want it to equal WorktreePath() = %q at AnchorRel \".\"", anchor, worktree)
	}

	lyxBase := filepath.Join(anchor, configengine.LyxDirName)

	// _lyx-durable group: AnchorPath-anchored.
	assertPath(t, "loomengine.PlanDir", loomengine.PlanDir(l), filepath.Join(lyxBase, planparser.PlanDirName))
	assertPath(t, "loomengine.PlanOverview", loomengine.PlanOverview(l), filepath.Join(lyxBase, planparser.PlanDirName, "00-overview.md"))
	assertPath(t, "loomengine.DiscussionDir", loomengine.DiscussionDir(l), filepath.Join(lyxBase, "discussion"))
	assertPath(t, "loomengine.DiscussionDecisionRecord", loomengine.DiscussionDecisionRecord(l), filepath.Join(lyxBase, "discussion", "decision-record.md"))
	assertPath(t, "loomengine.DiscussionSupportLog", loomengine.DiscussionSupportLog(l), filepath.Join(lyxBase, "discussion", "support-log.md"))
	assertPath(t, "loomengine.LoomStatusFile", loomengine.LoomStatusFile(l), filepath.Join(lyxBase, "status.json"))
	assertPath(t, "loomengine.LoomStatusLock", loomengine.LoomStatusLock(l), filepath.Join(lyxBase, "status.json.lock"))
	assertPath(t, "builderengine.Dir", builderengine.Dir(l), filepath.Join(lyxBase, "builder"))
	assertPath(t, "builderengine.ReportsDir", builderengine.ReportsDir(l), filepath.Join(lyxBase, "builder", "reports"))
	assertPath(t, "websterengine.Dir", websterengine.Dir(l), filepath.Join(lyxBase, "webster"))
	assertPath(t, "websterengine.ReportsDir", websterengine.ReportsDir(l), filepath.Join(lyxBase, "webster", "reports"))
	assertPath(t, "websterengine.PromptsDir", websterengine.PromptsDir(l), filepath.Join(lyxBase, "webster", "prompts"))
	assertPath(t, "perchengine.RunsDir", perchengine.RunsDir(l), filepath.Join(lyxBase, "perch"))
	assertPath(t, "pattern.FileHere", pattern.FileHere(l), filepath.Join(anchor, pattern.DirName, "PATTERN.md"))

	// .lyx ephemeral group: WorktreePath-anchored, never git-tracked.
	dotLyxBase := filepath.Join(worktree, ".lyx")
	assertPath(t, "logger.WorktreeLogsDir", logger.WorktreeLogsDir(l), filepath.Join(dotLyxBase, "logs"))
	assertPath(t, "scoutengine.DaemonStateFile", scoutengine.DaemonStateFile(worktree, "go"), filepath.Join(dotLyxBase, "scout", "go", "daemon.json"))
	assertPath(t, "scoutengine.DaemonLock", scoutengine.DaemonLock(worktree, "go"), filepath.Join(dotLyxBase, "scout", "go", "daemon.lock"))

	// HubPath-anchored: HubLogsDir alone, so one reed server per hub
	// resolves to one deterministic place.
	assertPath(t, "reedengine.HubLogsDir", reedengine.HubLogsDir(l), filepath.Join(hub, ".lyx", "logs"))
}

// TestConstructorAnchoring_SubpathAnchored asserts the anchor-aware move: the
// _lyx-durable group moves down by AnchorRel, while the .lyx group and
// HubLogsDir stay byte-identical to their unanchored-fixture values.
func TestConstructorAnchoring_SubpathAnchored(t *testing.T) {
	hub := filepath.Join("home", "user", "repo-HUB")
	anchorRel := "backend"
	l := anchoringFixture(hub, "repo", anchorRel)

	worktree := l.WorktreePath()
	anchor := l.AnchorPath()
	if anchor == worktree {
		t.Fatalf("AnchorPath() = %q; want it to differ from WorktreePath() = %q at a nested AnchorRel", anchor, worktree)
	}
	if anchor != filepath.Join(worktree, anchorRel) {
		t.Fatalf("AnchorPath() = %q; want %q", anchor, filepath.Join(worktree, anchorRel))
	}

	lyxBase := filepath.Join(anchor, configengine.LyxDirName)

	// _lyx-durable group: moves down by AnchorRel, unlike the other two groups.
	assertPath(t, "loomengine.PlanDir", loomengine.PlanDir(l), filepath.Join(lyxBase, planparser.PlanDirName))
	assertPath(t, "loomengine.PlanOverview", loomengine.PlanOverview(l), filepath.Join(lyxBase, planparser.PlanDirName, "00-overview.md"))
	assertPath(t, "loomengine.DiscussionDir", loomengine.DiscussionDir(l), filepath.Join(lyxBase, "discussion"))
	assertPath(t, "loomengine.DiscussionDecisionRecord", loomengine.DiscussionDecisionRecord(l), filepath.Join(lyxBase, "discussion", "decision-record.md"))
	assertPath(t, "loomengine.DiscussionSupportLog", loomengine.DiscussionSupportLog(l), filepath.Join(lyxBase, "discussion", "support-log.md"))
	assertPath(t, "loomengine.LoomStatusFile", loomengine.LoomStatusFile(l), filepath.Join(lyxBase, "status.json"))
	assertPath(t, "loomengine.LoomStatusLock", loomengine.LoomStatusLock(l), filepath.Join(lyxBase, "status.json.lock"))
	assertPath(t, "builderengine.Dir", builderengine.Dir(l), filepath.Join(lyxBase, "builder"))
	assertPath(t, "builderengine.ReportsDir", builderengine.ReportsDir(l), filepath.Join(lyxBase, "builder", "reports"))
	assertPath(t, "websterengine.Dir", websterengine.Dir(l), filepath.Join(lyxBase, "webster"))
	assertPath(t, "websterengine.ReportsDir", websterengine.ReportsDir(l), filepath.Join(lyxBase, "webster", "reports"))
	assertPath(t, "websterengine.PromptsDir", websterengine.PromptsDir(l), filepath.Join(lyxBase, "webster", "prompts"))
	assertPath(t, "perchengine.RunsDir", perchengine.RunsDir(l), filepath.Join(lyxBase, "perch"))
	assertPath(t, "pattern.FileHere", pattern.FileHere(l), filepath.Join(anchor, pattern.DirName, "PATTERN.md"))

	// .lyx ephemeral group: stays WorktreePath-anchored, ignoring AnchorRel
	// entirely -- byte-identical to the unanchored fixture's own dotLyxBase.
	dotLyxBase := filepath.Join(worktree, ".lyx")
	assertPath(t, "logger.WorktreeLogsDir", logger.WorktreeLogsDir(l), filepath.Join(dotLyxBase, "logs"))
	assertPath(t, "scoutengine.DaemonStateFile", scoutengine.DaemonStateFile(worktree, "go"), filepath.Join(dotLyxBase, "scout", "go", "daemon.json"))
	assertPath(t, "scoutengine.DaemonLock", scoutengine.DaemonLock(worktree, "go"), filepath.Join(dotLyxBase, "scout", "go", "daemon.lock"))

	// HubPath-anchored: stays byte-identical too, ignoring AnchorRel entirely.
	assertPath(t, "reedengine.HubLogsDir", reedengine.HubLogsDir(l), filepath.Join(hub, ".lyx", "logs"))
}

// assertPath fails t if got != want, naming which constructor produced the mismatch.
func assertPath(t *testing.T, name, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("%s = %q; want %q", name, got, want)
	}
}
