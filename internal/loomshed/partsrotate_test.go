//go:build tier0

// partsrotate_test.go exercises the Parts-Write and Parts-Plan-Write fresh-spawn rotations over a temp anchor with a fixed clock.

package loomshed

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/partsparser"
	"github.com/Knatte18/loomyard/internal/planparser"
)

// setupPartsDir writes an earlier attempt's parts directory under a new anchor: the parts file, two part directories each holding an overview, the join record and an earlier archive.
func setupPartsDir(t *testing.T) (anchorPath string) {
	t.Helper()
	anchorPath = t.TempDir()
	partsDir := partsparser.Dir(anchorPath)
	for _, rel := range []string{"parts.md", "join.md", "alpha/00-overview.md", "beta/00-overview.md", partsparser.ArchivePrefix + "20260101T000000Z/parts.md"} {
		path := filepath.Join(partsDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(rel), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) error = %v", path, err)
		}
	}
	return anchorPath
}

// partsDirEntries returns the top-level entry names of the parts directory under anchorPath, sorted.
func partsDirEntries(t *testing.T, anchorPath string) []string {
	t.Helper()
	entries, err := os.ReadDir(partsparser.Dir(anchorPath))
	if err != nil {
		t.Fatalf("ReadDir(parts directory) error = %v", err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

// requireFiles fails the test when any of rels, relative to dir, is not a file.
func requireFiles(t *testing.T, dir string, rels ...string) {
	t.Helper()
	for _, rel := range rels {
		if info, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel))); err != nil || info.IsDir() {
			t.Errorf("Stat(%s) = %v, %v; want a file", filepath.Join(dir, rel), info, err)
		}
	}
}

// TestPartsRotators verifies Parts-Write's rotation archives the whole parts directory, Parts-Plan-Write's keeps the parts file and rotates the plan, and a retry after a failed plan rotation still announces the prior plan.
func TestPartsRotators(t *testing.T) {
	t.Parallel()
	stamp := fixedPlanClock().UTC().Format(archiveTimestampFormat)
	archiveName := partsparser.ArchivePrefix + stamp

	t.Run("Parts-Write archives every entry into one archive and takes a -1 suffix in the same second", func(t *testing.T) {
		t.Parallel()
		anchorPath := setupPartsDir(t)
		rotate := NewPartsDirRotator(anchorPath, fixedPlanClock)

		amendment, err := rotate()
		if err != nil || amendment != "" {
			t.Fatalf("rotate() = %q, %v; want an empty amendment and nil error", amendment, err)
		}
		partsDir := partsparser.Dir(anchorPath)
		if got, want := partsDirEntries(t, anchorPath), []string{partsparser.ArchivePrefix + "20260101T000000Z", archiveName}; !slices.Equal(got, want) {
			t.Errorf("parts directory entries = %v; want %v", got, want)
		}
		requireFiles(t, filepath.Join(partsDir, archiveName), "parts.md", "join.md", "alpha/00-overview.md", "beta/00-overview.md")
		requireFiles(t, filepath.Join(partsDir, partsparser.ArchivePrefix+"20260101T000000Z"), "parts.md")

		// A second attempt in the same second archives into the first free suffix.
		if err := os.WriteFile(partsparser.File(anchorPath), []byte("again"), 0o644); err != nil {
			t.Fatalf("WriteFile(parts file) error = %v", err)
		}
		if _, err := rotate(); err != nil {
			t.Fatalf("second rotate() error = %v; want nil", err)
		}
		requireFiles(t, filepath.Join(partsDir, archiveName+"-1"), "parts.md")
		if _, err := os.Stat(filepath.Join(partsDir, archiveName, archiveName+"-1")); !os.IsNotExist(err) {
			t.Errorf("an archive nested inside an earlier archive; want archives side by side")
		}
	})

	t.Run("Parts-Plan-Write keeps the parts file, rotates the plan and returns the prior-plan amendment", func(t *testing.T) {
		t.Parallel()
		anchorPath := setupPartsDir(t)
		planDir := planparser.PlanDir(anchorPath)
		if err := os.MkdirAll(planDir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) error = %v", planDir, err)
		}
		if err := os.WriteFile(filepath.Join(planDir, "00-overview.md"), []byte("overview"), 0o644); err != nil {
			t.Fatalf("WriteFile(plan overview) error = %v", err)
		}

		amendment, err := NewPartsPlanRotator(anchorPath, seedPriorPlanStencils(t), noWebsterArchive, fixedPlanClock)()
		if err != nil {
			t.Fatalf("rotate() error = %v; want nil", err)
		}
		if !strings.Contains(amendment, "00-overview.md") {
			t.Errorf("rotate() amendment = %q; want the prior-plan block naming 00-overview.md", amendment)
		}
		if got, want := partsDirEntries(t, anchorPath), []string{partsparser.ArchivePrefix + "20260101T000000Z", archiveName, "parts.md"}; !slices.Equal(got, want) {
			t.Errorf("parts directory entries = %v; want the parts file kept and the rest archived %v", got, want)
		}
		requireFiles(t, filepath.Join(partsparser.Dir(anchorPath), archiveName), "join.md", "alpha/00-overview.md", "beta/00-overview.md")
		if _, err := os.Stat(filepath.Join(planDir, "00-overview.md")); !os.IsNotExist(err) {
			t.Errorf("plan overview still in place after the rotation; want it archived")
		}
	})

	t.Run("a failed plan rotation leaves the plan in place and the retry still announces it", func(t *testing.T) {
		t.Parallel()
		anchorPath := setupPartsDir(t)
		planDir := planparser.PlanDir(anchorPath)
		if err := os.MkdirAll(planDir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) error = %v", planDir, err)
		}
		if err := os.WriteFile(filepath.Join(planDir, "00-overview.md"), []byte("overview"), 0o644); err != nil {
			t.Fatalf("WriteFile(plan overview) error = %v", err)
		}
		stencilsDir := seedPriorPlanStencils(t)

		failing := func(string) error { return errors.New("webster holds its run lock") }
		if _, err := NewPartsPlanRotator(anchorPath, stencilsDir, failing, fixedPlanClock)(); err == nil {
			t.Fatal("rotate() with a failing archiveWebster = nil error; want the failure")
		}
		requireFiles(t, planDir, "00-overview.md")
		requireFiles(t, filepath.Join(partsparser.Dir(anchorPath), archiveName), "join.md", "alpha/00-overview.md")

		amendment, err := NewPartsPlanRotator(anchorPath, stencilsDir, noWebsterArchive, fixedPlanClock)()
		if err != nil {
			t.Fatalf("retried rotate() error = %v; want nil", err)
		}
		if !strings.Contains(amendment, "00-overview.md") {
			t.Errorf("retried rotate() amendment = %q; want the prior-plan block naming 00-overview.md", amendment)
		}
	})

	t.Run("an absent parts directory is a no-op for both rotations", func(t *testing.T) {
		t.Parallel()
		for name, rotate := range map[string]func(anchorPath string) func() (string, error){
			"Parts-Write": func(anchorPath string) func() (string, error) {
				return NewPartsDirRotator(anchorPath, fixedPlanClock)
			},
			"Parts-Plan-Write": func(anchorPath string) func() (string, error) {
				return NewPartsPlanRotator(anchorPath, seedPriorPlanStencils(t), noWebsterArchive, fixedPlanClock)
			},
		} {
			anchorPath := t.TempDir()
			amendment, err := rotate(anchorPath)()
			if err != nil || amendment != "" {
				t.Errorf("%s rotate() = %q, %v; want an empty amendment and nil error", name, amendment, err)
			}
			if _, err := os.Stat(partsparser.Dir(anchorPath)); !os.IsNotExist(err) {
				t.Errorf("%s rotate() created the parts directory; want a no-op", name)
			}
		}
	})
}
