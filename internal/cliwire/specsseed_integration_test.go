//go:build integration

// specsseed_integration_test.go pins ResolveStandalone's specs resolve-and-seed step against a real
// filesystem: a plain resolve populates and seeds Standalone.SpecsDir, a told --stencils-dir override
// does not suppress the specs seed (the scenario most likely to regress silently), and the resolved
// SpecsDir is always absolute.

package cliwire

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/standalonegeom"
	"github.com/Knatte18/loomyard/internal/stencilstore"
)

// TestResolveStandalone_SeedsTheSpecsDirectory asserts a plain standalone resolve populates
// Standalone.SpecsDir at standalonegeom.SpecsDir(res.StateDir), with both registered specs on disk
// and carrying a parseable stamp.
func TestResolveStandalone_SeedsTheSpecsDirectory(t *testing.T) {
	setStandaloneStateRoot(t)
	t.Cleanup(func() { logger.SetDurableSinkDir("") })
	target := t.TempDir()
	stateDir, _ := hash8AndStateDir(t, target)
	seedPlanDir(t, filepath.Join(stateDir, "_lyx", "plan"))

	res, err := websterFixture.ResolveStandalone(StandaloneRequest{Cwd: target})
	if err != nil {
		t.Fatalf("ResolveStandalone() = %v; want nil", err)
	}

	wantDir := standalonegeom.SpecsDir(res.StateDir)
	if res.SpecsDir != wantDir {
		t.Errorf("ResolveStandalone() SpecsDir = %q; want %q", res.SpecsDir, wantDir)
	}

	for _, name := range []string{"loom-plan-spec", "loom-plan-card-format"} {
		path := stencilstore.Path(res.SpecsDir, name)
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v; want the seeded spec %q to exist", path, err, name)
		}
		if hash, ok := stencilstore.ParseStamp(content); !ok || hash == "" {
			t.Errorf("stencilstore.ParseStamp(%s) = %q, %v; want a parseable, non-empty hash", path, hash, ok)
		}
	}
}

// TestResolveStandalone_ToldStencilsDirStillPopulatesSpecs is the key scenario and the one most
// likely to regress silently: a told --stencils-dir must not suppress the specs seed, which has no
// override flag of its own and inherits the stencils skip only if that inheritance is a bug.
func TestResolveStandalone_ToldStencilsDirStillPopulatesSpecs(t *testing.T) {
	setStandaloneStateRoot(t)
	t.Cleanup(func() { logger.SetDurableSinkDir("") })
	target := t.TempDir()
	stateDir, _ := hash8AndStateDir(t, target)
	seedPlanDir(t, filepath.Join(stateDir, "_lyx", "plan"))

	told := t.TempDir()
	curatedName := "curated-marker.md"
	if err := os.WriteFile(filepath.Join(told, curatedName), []byte("curated prompt set"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	res, err := websterFixture.ResolveStandalone(StandaloneRequest{Cwd: target, StencilsDirFlag: told})
	if err != nil {
		t.Fatalf("ResolveStandalone() = %v; want nil", err)
	}

	// The told stencils directory was NOT seeded: its contents are exactly what the test put there,
	// proving the existing skip still protects a curated prompt set.
	if res.StencilsDir != told {
		t.Errorf("ResolveStandalone() StencilsDir = %q; want the told directory %q, unchanged", res.StencilsDir, told)
	}
	entries, err := os.ReadDir(told)
	if err != nil {
		t.Fatalf("ReadDir(%s) error = %v", told, err)
	}
	if len(entries) != 1 || entries[0].Name() != curatedName {
		t.Errorf("told stencils directory entries = %v; want only the curated %q, unmodified", entries, curatedName)
	}

	// Standalone.SpecsDir still resolves, and the seeded spec files exist on disk under it -- a
	// resolvable-but-empty specs directory would reproduce the original dead reference while looking
	// correct from a path assertion alone, which is precisely what asserting file existence catches.
	wantDir := standalonegeom.SpecsDir(res.StateDir)
	if res.SpecsDir != wantDir {
		t.Errorf("ResolveStandalone() SpecsDir = %q; want %q", res.SpecsDir, wantDir)
	}
	for _, name := range []string{"loom-plan-spec", "loom-plan-card-format"} {
		path := stencilstore.Path(res.SpecsDir, name)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("stat %s: %v; want the seeded spec %q to exist even under a told --stencils-dir", path, err, name)
		}
	}
}

// TestResolveStandalone_SpecsDirIsAbsolute asserts filepath.IsAbs(res.SpecsDir): a deployed spec
// lives outside the agent's own worktree, so the rendered marker must be an absolute path for the
// agent to be able to open it at all.
func TestResolveStandalone_SpecsDirIsAbsolute(t *testing.T) {
	setStandaloneStateRoot(t)
	t.Cleanup(func() { logger.SetDurableSinkDir("") })
	target := t.TempDir()
	stateDir, _ := hash8AndStateDir(t, target)
	seedPlanDir(t, filepath.Join(stateDir, "_lyx", "plan"))

	res, err := websterFixture.ResolveStandalone(StandaloneRequest{Cwd: target})
	if err != nil {
		t.Fatalf("ResolveStandalone() = %v; want nil", err)
	}
	if !filepath.IsAbs(res.SpecsDir) {
		t.Errorf("ResolveStandalone() SpecsDir = %q; want an absolute path", res.SpecsDir)
	}
}
