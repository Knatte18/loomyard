// constructoranchoring_test.go pins every constructor batch 5 relocated out of internal/lyxcwd into
// its owning module, and every told-anchor path function an owning module declares outright (taking
// a plain anchor string rather than a *lyxcwd.Location — planparser.PlanDir/PlanOverview,
// pattern.File), to the anchoring table the
// overview's Shared Decisions record: there is no single base.
// It lives in cmd/lyx because this is the only package that may import every owning module at once
// (loomengine, websterengine, pattern, logger, fabricengine,
// planparser).
// Every case here is pure filepath.Join arithmetic -- no subprocess is spawned and no fixture tree
// is copied -- so this file stays untagged, per the Test Tier Purity Invariant.
//
// The check is anchor-aware, not byte-identical: two synthetic *lyxcwd.Location values are built,
// one unanchored (AnchorRel == ".")
// and one subpath-anchored (AnchorRel == "backend").
// For the unanchored fixture every constructor in both groups below is byte-identical to a plain
// filepath.Join computed independently in this file.
// For the subpath-anchored fixture, every worktree-level constructor -- the _lyx-durable group and
// the .lyx group alike -- moves down by AnchorRel, since this batch re-anchors the whole .lyx group
// onto AnchorPath; only fabricengine.HubLogsDir (hub-anchored through the board, one server per hub)
// stays byte-identical.
//
// As of this batch there are two groups, not three: the _lyx-durable group, and the .lyx group in
// full (loomengine.LoomStatusFile, loomengine.LoomStatusLock, loomengine.LoomRunLock,
// loomengine.LoomDriverLog, loomengine.LoomBootstrapLock,
// websterengine.PromptsDir/ScratchDir, logger.LogsDir) -- all
// AnchorPath-anchored, so every worktree-level .lyx entry sits under exactly one root:
// filepath.Join(anchor, ".lyx"). A prior slice split this into an already-migrated and a
// not-yet-migrated subset, each with its own base local; this batch collapses that split, since
// nothing under .lyx is left on WorktreePath() anymore.
package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/pattern"
	"github.com/Knatte18/loomyard/internal/planparser"
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

// TestConstructorAnchoring_Unanchored asserts every relocated constructor at AnchorRel == "."
// against a plain filepath.Join computed independently here, for both anchoring groups.
func TestConstructorAnchoring_Unanchored(t *testing.T) {
	hub := filepath.Join("home", "user", "repo-HUB")
	l := anchoringFixture(hub, "repo", ".")

	worktree := l.WorktreePath()
	anchor := l.AnchorPath()
	if anchor != worktree {
		t.Fatalf("AnchorPath() = %q; want it to equal WorktreePath() = %q at AnchorRel \".\"", anchor, worktree)
	}

	lyxBase := filepath.Join(anchor, lyxdirs.LyxDirName)

	// _lyx-durable group: AnchorPath-anchored.
	//
	// The two planparser rows below and the pattern.File row still pin the
	// join arithmetic and the _lyx-vs-.lyx group placement, but because they pass l.AnchorPath() in and
	// compare against an anchor-derived expectation, they are tautological with respect to anchoring
	// and can no longer catch a production call site that passes the wrong root. That proof now lives
	// in the subpath-anchored PlanSpec case in internal/loomengine/plan_test.go, the subpath-anchored
	// PersistentPreRunE case in internal/webstercli/verbs_test.go, and
	// TestPlanSpec_PatternDirectiveAnchoredUnderAnchorPath in internal/loomengine/plan_test.go.
	assertPath(t, "planparser.PlanDir", planparser.PlanDir(l.AnchorPath()), filepath.Join(lyxBase, planparser.PlanDirName))
	assertPath(t, "planparser.PlanOverview", planparser.PlanOverview(l.AnchorPath()), filepath.Join(lyxBase, planparser.PlanDirName, "00-overview.md"))
	assertPath(t, "loomengine.DiscussionDir", loomengine.DiscussionDir(l), filepath.Join(lyxBase, "discussion"))
	assertPath(t, "loomengine.DiscussionDecisionRecord", loomengine.DiscussionDecisionRecord(l), filepath.Join(lyxBase, "discussion", "decision-record.md"))
	assertPath(t, "loomengine.DiscussionSupportLog", loomengine.DiscussionSupportLog(l), filepath.Join(lyxBase, "discussion", "support-log.md"))
	assertPath(t, "websterengine.Dir", websterengine.Dir(l.AnchorPath()), filepath.Join(lyxBase, "webster"))
	assertPath(t, "websterengine.ReportsDir", websterengine.ReportsDir(l.AnchorPath()), filepath.Join(lyxBase, "webster", "reports"))
	assertPath(t, "pattern.File", pattern.File(l.AnchorPath()), filepath.Join(anchor, lyxdirs.LyxDirName, "PATTERN.md"))

	// .lyx group, now collapsed into one AnchorPath-anchored base: every
	// worktree-level .lyx entry, ephemeral and never git-tracked, joins onto
	// dotLyxBase.
	dotLyxBase := filepath.Join(anchor, ".lyx")
	assertPath(t, "loomengine.LoomStatusFile", loomengine.LoomStatusFile(l), filepath.Join(dotLyxBase, "loom", "status.json"))
	assertPath(t, "loomengine.LoomStatusLock", loomengine.LoomStatusLock(l), filepath.Join(dotLyxBase, "loom", "status.json.lock"))
	assertPath(t, "loomengine.LoomRunLock", loomengine.LoomRunLock(l), filepath.Join(dotLyxBase, "loom", "run.lock"))
	assertPath(t, "loomengine.LoomDriverLog", loomengine.LoomDriverLog(l), filepath.Join(dotLyxBase, "loom", "driver.log"))
	assertPath(t, "loomengine.LoomBootstrapLock", loomengine.LoomBootstrapLock(l), filepath.Join(dotLyxBase, "loom", "bootstrap.lock"))
	assertPath(t, "websterengine.PromptsDir", websterengine.PromptsDir(l.AnchorPath()), filepath.Join(dotLyxBase, "webster", "prompts"))
	assertPath(t, "websterengine.ScratchDir", websterengine.ScratchDir(l.AnchorPath()), filepath.Join(dotLyxBase, "webster"))
	assertPath(t, "logger.LogsDir", logger.LogsDir(l), filepath.Join(dotLyxBase, "logs"))

	// HubPath-anchored through the board: HubLogsDir alone, so one reed server per hub
	// resolves to one deterministic place.
	assertPath(t, "fabricengine.HubLogsDir", fabricengine.HubLogsDir(l.HubPath), filepath.Join(hub, "_board", ".lyx", "logs"))
}

// TestConstructorAnchoring_SubpathAnchored asserts the anchor-aware move: every worktree-level
// constructor -- the _lyx-durable group and the .lyx group alike -- moves down by AnchorRel, while
// HubLogsDir stays byte-identical to its unanchored-fixture value.
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

	lyxBase := filepath.Join(anchor, lyxdirs.LyxDirName)

	// _lyx-durable group: moves down by AnchorRel, exactly like the .lyx
	// group below -- both groups now share one anchoring rule.
	//
	// The two planparser rows below and the pattern.File row still pin the
	// join arithmetic and the _lyx-vs-.lyx group placement, but because they pass l.AnchorPath() in and
	// compare against an anchor-derived expectation, they are tautological with respect to anchoring
	// and can no longer catch a production call site that passes the wrong root. That proof now lives
	// in the subpath-anchored PlanSpec case in internal/loomengine/plan_test.go, the subpath-anchored
	// PersistentPreRunE case in internal/webstercli/verbs_test.go, and
	// TestPlanSpec_PatternDirectiveAnchoredUnderAnchorPath in internal/loomengine/plan_test.go.
	assertPath(t, "planparser.PlanDir", planparser.PlanDir(l.AnchorPath()), filepath.Join(lyxBase, planparser.PlanDirName))
	assertPath(t, "planparser.PlanOverview", planparser.PlanOverview(l.AnchorPath()), filepath.Join(lyxBase, planparser.PlanDirName, "00-overview.md"))
	assertPath(t, "loomengine.DiscussionDir", loomengine.DiscussionDir(l), filepath.Join(lyxBase, "discussion"))
	assertPath(t, "loomengine.DiscussionDecisionRecord", loomengine.DiscussionDecisionRecord(l), filepath.Join(lyxBase, "discussion", "decision-record.md"))
	assertPath(t, "loomengine.DiscussionSupportLog", loomengine.DiscussionSupportLog(l), filepath.Join(lyxBase, "discussion", "support-log.md"))
	assertPath(t, "websterengine.Dir", websterengine.Dir(l.AnchorPath()), filepath.Join(lyxBase, "webster"))
	assertPath(t, "websterengine.ReportsDir", websterengine.ReportsDir(l.AnchorPath()), filepath.Join(lyxBase, "webster", "reports"))
	assertPath(t, "pattern.File", pattern.File(l.AnchorPath()), filepath.Join(anchor, lyxdirs.LyxDirName, "PATTERN.md"))

	// .lyx group: AnchorPath-anchored in full as of this batch, so every
	// entry moves down by AnchorRel here too, just like the _lyx-durable
	// group above.
	dotLyxBase := filepath.Join(anchor, ".lyx")
	assertPath(t, "loomengine.LoomStatusFile", loomengine.LoomStatusFile(l), filepath.Join(dotLyxBase, "loom", "status.json"))
	assertPath(t, "loomengine.LoomStatusLock", loomengine.LoomStatusLock(l), filepath.Join(dotLyxBase, "loom", "status.json.lock"))
	assertPath(t, "loomengine.LoomRunLock", loomengine.LoomRunLock(l), filepath.Join(dotLyxBase, "loom", "run.lock"))
	assertPath(t, "loomengine.LoomDriverLog", loomengine.LoomDriverLog(l), filepath.Join(dotLyxBase, "loom", "driver.log"))
	assertPath(t, "loomengine.LoomBootstrapLock", loomengine.LoomBootstrapLock(l), filepath.Join(dotLyxBase, "loom", "bootstrap.lock"))
	assertPath(t, "websterengine.PromptsDir", websterengine.PromptsDir(l.AnchorPath()), filepath.Join(dotLyxBase, "webster", "prompts"))
	assertPath(t, "websterengine.ScratchDir", websterengine.ScratchDir(l.AnchorPath()), filepath.Join(dotLyxBase, "webster"))
	assertPath(t, "logger.LogsDir", logger.LogsDir(l), filepath.Join(dotLyxBase, "logs"))

	// Hub-anchored through the board: stays byte-identical, ignoring AnchorRel entirely.
	assertPath(t, "fabricengine.HubLogsDir", fabricengine.HubLogsDir(l.HubPath), filepath.Join(hub, "_board", ".lyx", "logs"))

	// Regression guard for the two-roots bug this whole re-anchoring exists
	// to remove: every worktree-level .lyx constructor's result must have
	// filepath.Join(anchor, ".lyx") as a prefix, and none may have
	// filepath.Join(worktree, ".lyx") as its prefix. Asserting each
	// constructor's new value individually (above) would pass an
	// implementation that re-anchored some sites and missed others; this
	// prefix-exclusion form fails the moment any worktree-level consumer is
	// left behind.
	wrongRoot := filepath.Join(worktree, ".lyx")
	dotLyxConstructors := map[string]string{
		"loomengine.LoomStatusFile":    loomengine.LoomStatusFile(l),
		"loomengine.LoomStatusLock":    loomengine.LoomStatusLock(l),
		"loomengine.LoomRunLock":       loomengine.LoomRunLock(l),
		"loomengine.LoomDriverLog":     loomengine.LoomDriverLog(l),
		"loomengine.LoomBootstrapLock": loomengine.LoomBootstrapLock(l),
		"websterengine.PromptsDir":     websterengine.PromptsDir(l.AnchorPath()),
		"websterengine.ScratchDir":     websterengine.ScratchDir(l.AnchorPath()),
		"logger.LogsDir":               logger.LogsDir(l),
	}
	for name, got := range dotLyxConstructors {
		if !strings.HasPrefix(got, dotLyxBase) {
			t.Errorf("%s = %q; want it under the one .lyx root %q", name, got, dotLyxBase)
		}
		if strings.HasPrefix(got, wrongRoot) {
			t.Errorf("%s = %q; want it NOT under the WorktreePath-based .lyx root %q -- a subpath-anchored repo must have exactly one .lyx root", name, got, wrongRoot)
		}
	}
}

// assertPath fails t if got != want, naming which constructor produced the mismatch.
func assertPath(t *testing.T, name, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("%s = %q; want %q", name, got, want)
	}
}
