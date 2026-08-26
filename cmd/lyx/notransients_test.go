// notransients_test.go is the machine half of the "every never-tracked file lives under .lyx"
// invariant: it walks every module's exported path constructor and asserts, for two synthetic
// *lyxcwd.Location fixtures (AnchorRel == "." and AnchorRel == "backend"), that no _lyx-rooted
// path resolves as a transient and every transient path resolves under .lyx at the mirrored
// subpath of the _lyx-rooted content it relates to.
// It lives in cmd/lyx because this is the only package that may import every owning module at
// once (loomengine, websterengine, treadleengine,
// logger, planparser), the same reason constructoranchoring_test.go lives here.
// It stays untagged: every constructor exercised here is pure filepath.Join arithmetic over a
// hand-built *lyxcwd.Location, so no process is spawned and no fixture tree is copied, per the
// Test Tier Purity Invariant.
// It builds its own fixture locally rather than reaching for constructoranchoring_test.go's
// unexported anchoringFixture, so neither file constrains the other.
package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/lyxdirs"
	"github.com/Knatte18/loomyard/internal/planparser"
	"github.com/Knatte18/loomyard/internal/treadleengine"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

// notransientsFixture builds a synthetic *lyxcwd.Location by hand, mirroring the field
// derivation Resolve performs, without spawning git.
func notransientsFixture(hubPath, worktreeName, anchorRel string) *lyxcwd.Location {
	return &lyxcwd.Location{
		HubPath:      hubPath,
		WorktreeName: worktreeName,
		AnchorRel:    anchorRel,
	}
}

// namedPath pairs a constructor's name (for failure messages) with its resolved path.
type namedPath struct {
	name string
	path string
}

// durableSet returns every _lyx-rooted path a module exposes for l.
//
// The two planparser rows below share the weakening annotated on
// cmd/lyx/constructoranchoring_test.go's plan rows: they still pin that no durable path resolves as
// a transient at both AnchorRel fixtures, but because each row builds l.AnchorPath() itself instead
// of calling a constructor that anchored internally, neither can catch a production call site that
// passes the wrong root. That proof lives in the subpath-anchored PlanSpec case in
// internal/loomengine/plan_test.go and the subpath-anchored PersistentPreRunE case in
// internal/webstercli/verbs_test.go.
func durableSet(l *lyxcwd.Location) []namedPath {
	return []namedPath{
		{"planparser.PlanDir", planparser.PlanDir(l.AnchorPath())},
		{"planparser.PlanOverview", planparser.PlanOverview(l.AnchorPath())},
		{"loomengine.DiscussionDir", loomengine.DiscussionDir(l)},
		{"websterengine.Dir", websterengine.Dir(l.AnchorPath())},
		{"websterengine.ReportsDir", websterengine.ReportsDir(l.AnchorPath())},
		{"websterengine.ReportsDir/blk", filepath.Join(websterengine.ReportsDir(l.AnchorPath()), "blk")},
	}
}

// transientSet returns every relocated, never-tracked artifact a module exposes for l.
func transientSet(l *lyxcwd.Location) []namedPath {
	return []namedPath{
		{"websterengine.ScratchDir", websterengine.ScratchDir(l.AnchorPath())},
		{"websterengine.PromptsDir", websterengine.PromptsDir(l.AnchorPath())},
		{"loomengine.LoomStatusFile", loomengine.LoomStatusFile(l)},
		{"loomengine.LoomStatusLock", loomengine.LoomStatusLock(l)},
		{"loomengine.LoomRunLock", loomengine.LoomRunLock(l)},
		{"loomengine.LoomDriverLog", loomengine.LoomDriverLog(l)},
		{"loomengine.LoomBootstrapLock", loomengine.LoomBootstrapLock(l)},
		{"logger.LogsDir", logger.LogsDir(l)},
		{"treadleengine.PauseFlagPath", treadleengine.PauseFlagPath(filepath.Join(websterengine.ScratchDir(l.AnchorPath()), "blk"))},
	}
}

// TestNoTransientsUnderLyx is the machine guard: no durable (_lyx-rooted) constructor's result
// is a transient, and every transient constructor's result resolves under .lyx, at the same
// relative subpath as its durable counterpart. It runs the whole table against two fixtures,
// AnchorRel == "." and AnchorRel == "backend", so the assertion holds at both the unanchored and
// subpath-anchored geometries.
func TestNoTransientsUnderLyx(t *testing.T) {
	hub := filepath.Join("home", "user", "repo-HUB")

	fixtures := []struct {
		name      string
		anchorRel string
	}{
		{"unanchored", "."},
		{"subpath-anchored", "backend"},
	}

	var scanned int

	for _, fx := range fixtures {
		t.Run(fx.name, func(t *testing.T) {
			l := notransientsFixture(hub, "repo", fx.anchorRel)
			lyxRoot := filepath.Join(l.AnchorPath(), lyxdirs.LyxDirName)
			dotLyxRoot := filepath.Join(l.AnchorPath(), lyxdirs.DotLyxDirName)

			durable := durableSet(l)
			transient := transientSet(l)
			scanned += len(durable) + len(transient)

			// Direction 1: no durable path is a transient -- it must sit under
			// _lyx, and it must never look like a transient artifact (a .lock
			// suffix, a "pause" base name, or a "prompts" path segment).
			for _, np := range durable {
				if !strings.HasPrefix(np.path, lyxRoot) {
					t.Errorf("%s = %q; want it prefixed by the durable root %q", np.name, np.path, lyxRoot)
				}
				if strings.HasSuffix(np.path, ".lock") {
					t.Errorf("%s = %q; a durable (_lyx) path must never end in .lock", np.name, np.path)
				}
				if filepath.Base(np.path) == "pause" {
					t.Errorf("%s = %q; a durable (_lyx) path must never have base name \"pause\"", np.name, np.path)
				}
				for _, seg := range strings.Split(filepath.ToSlash(np.path), "/") {
					if seg == "prompts" {
						t.Errorf("%s = %q; a durable (_lyx) path must never have a \"prompts\" path segment", np.name, np.path)
					}
				}
			}

			// Direction 2: every transient resolves under .lyx, and never
			// under the durable root -- catches an implementation that left
			// a copy under _lyx as well as moving it.
			for _, np := range transient {
				if !strings.HasPrefix(np.path, dotLyxRoot) {
					t.Errorf("%s = %q; want it prefixed by the scratch root %q", np.name, np.path, dotLyxRoot)
				}
				if strings.HasPrefix(np.path, lyxRoot) {
					t.Errorf("%s = %q; want it NOT prefixed by the durable root %q", np.name, np.path, lyxRoot)
				}
			}

			// Direction 3: the mirrored-subpath equality check. For each
			// module pair, the durable and scratch paths differ in
			// exactly the one directory-name segment -- rewriting the durable
			// path's LyxDirName segment to DotLyxDirName must produce the
			// scratch path byte-for-byte. This is what makes "same relative
			// subpath" a machine fact rather than a convention.
			mirroredPairs := []struct {
				name    string
				durable string
				scratch string
			}{
				{"websterengine.Dir/ScratchDir", websterengine.Dir(l.AnchorPath()), websterengine.ScratchDir(l.AnchorPath())},
			}
			for _, mp := range mirroredPairs {
				rewritten := strings.Replace(mp.durable, lyxdirs.LyxDirName, lyxdirs.DotLyxDirName, 1)
				if rewritten != mp.scratch {
					t.Errorf("%s: rewriting the durable path's %q segment to %q gave %q; want it to equal the scratch path %q", mp.name, lyxdirs.LyxDirName, lyxdirs.DotLyxDirName, rewritten, mp.scratch)
				}
			}
		})
	}

	// Sanity check: a table accidentally emptied by a refactor must not pass
	// vacuously, mirroring the guard style already used in
	// internal/lyxcwd/enforcement_test.go's "scanned_non_empty" sub-test.
	t.Run("scanned_non_empty", func(t *testing.T) {
		if scanned == 0 {
			t.Error("no-transients-under-.lyx guard: no constructor paths were scanned; the table may be misconfigured")
		}
	})
}
