// pathrules_test.go is the machine half of two invariants over every module's exported path constructor: "every never-tracked file lives under .lyx, at the mirrored subpath of the _lyx content it relates to" and the anchoring rule that puts every worktree-level path under AnchorPath() rather than WorktreePath().
// Each row of pathRules names a constructor, a closure calling it on a *lyxcwd.Location, and its class;
// the expectations come from the class rule, never from a path literal on the row.
// It lives in cmd/lyx because this is the only package that may import every owning module at once.
// Every path is computed by pure filepath.Join arithmetic over a locationkit.Location,
// so no process is spawned and no fixture tree is copied, and the file stays untagged per the Test Tier Purity Invariant.
package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/battencli"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/pattern"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/testkit/locationkit"
	"github.com/Knatte18/loomyard/internal/treadleengine"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// pathClass is the placement rule a constructor's result must satisfy.
type pathClass int

const (
	// classDurable is tracked content under AnchorPath()/_lyx.
	classDurable pathClass = iota
	// classTransient is never-tracked content under AnchorPath()/.lyx.
	classTransient
	// classHub is content anchored at the hub, ignoring AnchorRel.
	classHub
)

// pathRule is one constructor under test.
// mirrors, when set, names the durable row whose path this transient row must equal once the _lyx segment is rewritten to .lyx.
type pathRule struct {
	name    string
	class   pathClass
	path    func(l *lyxcwd.Location) string
	mirrors string
}

// pathRules is the union of every constructor the path guards exercise.
//
// The planparser rows and the pattern.File row pass l.AnchorPath() in and are checked against an anchor-derived rule,
// so they are tautological with respect to anchoring and cannot catch a production call site that passes the wrong root.
// That proof lives in the subpath-anchored PlanSpec case in internal/loomengine/plan_test.go, the subpath-anchored PersistentPreRunE case in internal/webstercli/verbs_test.go, and TestPlanSpec_PatternDirectiveAnchoredUnderAnchorPath in internal/loomengine/plan_test.go.
var pathRules = []pathRule{
	{name: "planparser.PlanDir", class: classDurable, path: func(l *lyxcwd.Location) string { return planparser.PlanDir(l.AnchorPath()) }},
	{name: "planparser.PlanOverview", class: classDurable, path: func(l *lyxcwd.Location) string { return planparser.PlanOverview(l.AnchorPath()) }},
	{name: "pattern.File", class: classDurable, path: func(l *lyxcwd.Location) string { return pattern.File(l.AnchorPath()) }},
	{name: "loomengine.DiscussionDir", class: classDurable, path: loomengine.DiscussionDir},
	{name: "loomengine.DiscussionDecisionRecord", class: classDurable, path: loomengine.DiscussionDecisionRecord},
	{name: "loomengine.DiscussionSupportLog", class: classDurable, path: loomengine.DiscussionSupportLog},
	{name: "loomengine.LandingDir", class: classDurable, path: loomengine.LandingDir},
	{name: "loomengine.LoomReviewsDir", class: classDurable, path: loomengine.LoomReviewsDir},
	{name: "loomengine.LoomDurableDir", class: classDurable, path: loomengine.LoomDurableDir},
	{name: "loomengine.LoomFrictionDir", class: classDurable, path: loomengine.LoomFrictionDir},
	{name: "loomengine.LoomReworkDir", class: classDurable, path: loomengine.LoomReworkDir},
	{name: "shedrun.StatusFile", class: classDurable, path: func(l *lyxcwd.Location) string { return shedrun.StatusFile(l, shedrun.SelfRunID) }},
	{name: "shedrun.DriveReportsDir", class: classDurable, path: func(l *lyxcwd.Location) string { return shedrun.DriveReportsDir(l, shedrun.SelfRunID) }},
	{name: "websterengine.Dir", class: classDurable, path: func(l *lyxcwd.Location) string { return websterengine.Dir(l.AnchorPath()) }},
	{name: "websterengine.ReportsDir", class: classDurable, path: func(l *lyxcwd.Location) string { return websterengine.ReportsDir(l.AnchorPath()) }},
	{name: "websterengine.ReportsDir/blk", class: classDurable, path: func(l *lyxcwd.Location) string {
		return filepath.Join(websterengine.ReportsDir(l.AnchorPath()), "blk")
	}},
	// battencli.StatusFile is durable, fabric-synced state living at shedrun's _lyx-rooted run directory, unlike battencli.RunLock, StatusLock and PrimeRunLock below, which stay ephemeral.
	{name: "battencli.StatusFile", class: classDurable, path: func(l *lyxcwd.Location) string { return battencli.StatusFile(l, "slug") }},

	{name: "websterengine.ScratchDir", class: classTransient, mirrors: "websterengine.Dir", path: func(l *lyxcwd.Location) string { return websterengine.ScratchDir(l.AnchorPath()) }},
	{name: "websterengine.PromptsDir", class: classTransient, path: func(l *lyxcwd.Location) string { return websterengine.PromptsDir(l.AnchorPath()) }},
	{name: "shedrun.StatusLock", class: classTransient, path: func(l *lyxcwd.Location) string { return shedrun.StatusLock(l, shedrun.SelfRunID) }},
	{name: "shedrun.RunLock", class: classTransient, path: func(l *lyxcwd.Location) string { return shedrun.RunLock(l, shedrun.SelfRunID) }},
	{name: "loomengine.LoomDriverLog", class: classTransient, path: loomengine.LoomDriverLog},
	{name: "loomengine.LoomBootstrapLock", class: classTransient, path: loomengine.LoomBootstrapLock},
	{name: "loomengine.LoomSelfreportFiled", class: classTransient, path: loomengine.LoomSelfreportFiled},
	{name: "loomengine.LoomSelfreportFiledLock", class: classTransient, path: loomengine.LoomSelfreportFiledLock},
	{name: "loomengine.LoomApprovalPath", class: classTransient, path: loomengine.LoomApprovalPath},
	{name: "loomengine.LoomRejectionPath", class: classTransient, path: loomengine.LoomRejectionPath},
	{name: "loomengine.LoomReworkCoveragePath", class: classTransient, path: loomengine.LoomReworkCoveragePath},
	{name: "loomengine.LoomVerifyPendingPath", class: classTransient, path: loomengine.LoomVerifyPendingPath},
	{name: "loomengine.LoomVerifyOutputPath", class: classTransient, path: loomengine.LoomVerifyOutputPath},
	{name: "loomengine.LoomFrictionLock", class: classTransient, path: loomengine.LoomFrictionLock},
	{name: "logger.LogsDir", class: classTransient, path: logger.LogsDir},
	{name: "treadleengine.PauseFlagPath", class: classTransient, path: func(l *lyxcwd.Location) string {
		return treadleengine.PauseFlagPath(filepath.Join(websterengine.ScratchDir(l.AnchorPath()), "blk"))
	}},
	{name: "battencli.BattenDir", class: classTransient, path: func(l *lyxcwd.Location) string { return battencli.BattenDir(l, "slug") }},
	{name: "battencli.RunLock", class: classTransient, path: func(l *lyxcwd.Location) string { return battencli.RunLock(l, "slug") }},
	{name: "battencli.StatusLock", class: classTransient, path: func(l *lyxcwd.Location) string { return battencli.StatusLock(l, "slug") }},
	{name: "battencli.PrimeRunLock", class: classTransient, path: battencli.PrimeRunLock},

	// HubLogsDir is hub-anchored through the board, so one reed server per hub resolves to one place.
	{name: "fabricengine.HubLogsDir", class: classHub, path: func(l *lyxcwd.Location) string { return fabricengine.HubLogsDir(l.HubPath) }},
}

// under reports whether path is root or lies beneath it, by whole path segments.
func under(path, root string) bool {
	return path == root || strings.HasPrefix(path, root+string(filepath.Separator))
}

func TestPathRules(t *testing.T) {
	hub := filepath.Join("home", "user", "repo-LYXHUB")

	fixtures := []struct {
		name      string
		anchorRel string
	}{
		{"unanchored", "."},
		{"subpath-anchored", "backend"},
	}

	if len(pathRules) == 0 {
		t.Fatal("pathRules is empty; the guard would pass vacuously")
	}
	byName := make(map[string]pathRule, len(pathRules))
	for _, r := range pathRules {
		byName[r.name] = r
	}

	for _, fx := range fixtures {
		t.Run(fx.name, func(t *testing.T) {
			l := locationkit.Location(hub, "repo", fx.anchorRel)
			anchor := l.AnchorPath()
			worktree := l.WorktreePath()
			if (fx.anchorRel == ".") != (anchor == worktree) {
				t.Fatalf("AnchorPath() = %q, WorktreePath() = %q; want them equal exactly when AnchorRel is \".\"", anchor, worktree)
			}
			lyxRoot := filepath.Join(anchor, lyxdirs.LyxDirName)
			dotLyxRoot := filepath.Join(anchor, lyxdirs.DotLyxDirName)

			for _, r := range pathRules {
				got := r.path(l)
				switch r.class {
				case classDurable:
					if !under(got, lyxRoot) {
						t.Errorf("%s = %q; want it under the durable root %q", r.name, got, lyxRoot)
					}
					if strings.HasSuffix(got, ".lock") {
						t.Errorf("%s = %q; a durable (_lyx) path must never end in .lock", r.name, got)
					}
					if filepath.Base(got) == "pause" {
						t.Errorf("%s = %q; a durable (_lyx) path must never have base name \"pause\"", r.name, got)
					}
					for _, seg := range strings.Split(filepath.ToSlash(got), "/") {
						if seg == "prompts" {
							t.Errorf("%s = %q; a durable (_lyx) path must never have a \"prompts\" path segment", r.name, got)
						}
					}
				case classTransient:
					if !under(got, dotLyxRoot) {
						t.Errorf("%s = %q; want it under the scratch root %q", r.name, got, dotLyxRoot)
					}
					if under(got, lyxRoot) {
						t.Errorf("%s = %q; want it NOT under the durable root %q", r.name, got, lyxRoot)
					}
					// A subpath-anchored repo has exactly one .lyx root: a path left on the WorktreePath-based root is the two-roots bug.
					if wrong := filepath.Join(worktree, lyxdirs.DotLyxDirName); wrong != dotLyxRoot && under(got, wrong) {
						t.Errorf("%s = %q; want it NOT under the WorktreePath-based root %q", r.name, got, wrong)
					}
				case classHub:
					if !under(got, hub) {
						t.Errorf("%s = %q; want it under the hub %q", r.name, got, hub)
					}
				}

				// Mirrored subpath: rewriting the durable sibling's _lyx segment to .lyx must give the transient path byte-for-byte.
				if r.mirrors != "" {
					sibling, ok := byName[r.mirrors]
					if !ok || sibling.class != classDurable {
						t.Fatalf("%s mirrors %q, which is not a durable row", r.name, r.mirrors)
					}
					rewritten := strings.Replace(sibling.path(l), lyxdirs.LyxDirName, lyxdirs.DotLyxDirName, 1)
					if rewritten != got {
						t.Errorf("%s: rewriting %s's %q segment to %q gave %q; want it to equal %q", r.name, r.mirrors, lyxdirs.LyxDirName, lyxdirs.DotLyxDirName, rewritten, got)
					}
				}
			}
		})
	}
}
