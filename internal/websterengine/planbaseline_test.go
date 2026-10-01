// planbaseline_test.go covers the plan baseline store and RestorePlan.
// Tier 1: no git, only t.TempDir().

package websterengine

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// baselineFixture is a plan directory restamped into a webster dir.
type baselineFixture struct {
	st   *State
	geom Geometry
}

func newBaselineFixture(t *testing.T) *baselineFixture {
	t.Helper()
	planDir := t.TempDir()
	fingerprintWriteFiles(t, planDir, map[string]string{
		"00-overview.md": "overview",
		"01-first.md":    "first",
		"02-second.md":   "second",
	})
	geom := Geometry{PlanDir: planDir, WebsterDir: t.TempDir()}
	st := &State{}
	if err := restampFingerprint(st, planDir, geom.WebsterDir); err != nil {
		t.Fatalf("restampFingerprint() error = %v", err)
	}
	return &baselineFixture{st: st, geom: geom}
}

func TestStorePlanBaseline_ContentAddressed(t *testing.T) {
	t.Parallel()
	planDir := t.TempDir()
	fingerprintWriteFiles(t, planDir, map[string]string{"01-a.md": "alpha", "02-b.md": "beta"})
	hashes, err := planFileHashes(planDir)
	if err != nil {
		t.Fatal(err)
	}
	websterDir := t.TempDir()
	if err := storePlanBaseline(websterDir, planDir, hashes); err != nil {
		t.Fatalf("storePlanBaseline() error = %v", err)
	}
	for name, want := range map[string]string{"01-a.md": "alpha", "02-b.md": "beta"} {
		got, err := os.ReadFile(planBaselinePath(websterDir, hashes[name]))
		if err != nil || string(got) != want {
			t.Errorf("stored copy of %s = %q, %v; want %q", name, got, err, want)
		}
	}
	entries, err := os.ReadDir(filepath.Join(websterDir, planBaselineDirName))
	if err != nil || len(entries) != len(hashes) {
		t.Fatalf("store entries = %d, %v; want one per hash (%d)", len(entries), err, len(hashes))
	}

	path := planBaselinePath(websterDir, hashes["01-a.md"])
	past := time.Now().Add(-time.Hour).Truncate(time.Second)
	if err := os.Chtimes(path, past, past); err != nil {
		t.Fatal(err)
	}
	if err := storePlanBaseline(websterDir, planDir, hashes); err != nil {
		t.Fatalf("second storePlanBaseline() error = %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().Equal(past) {
		t.Errorf("modification time = %v; want unchanged %v", info.ModTime(), past)
	}
}

// TestStorePlanBaseline_RefusesBytesNotMatchingTheirHash proves a file edited after its hash was computed is refused and nothing is stored under that hash.
func TestStorePlanBaseline_RefusesBytesNotMatchingTheirHash(t *testing.T) {
	t.Parallel()
	planDir := t.TempDir()
	fingerprintWriteFiles(t, planDir, map[string]string{"01-a.md": "alpha"})
	hashes, err := planFileHashes(planDir)
	if err != nil {
		t.Fatal(err)
	}
	fingerprintWriteFiles(t, planDir, map[string]string{"01-a.md": "edited"})
	websterDir := t.TempDir()

	err = storePlanBaseline(websterDir, planDir, hashes)
	if err == nil || !strings.Contains(err.Error(), "01-a.md") {
		t.Fatalf("storePlanBaseline() error = %v; want a refusal naming 01-a.md", err)
	}
	if _, statErr := os.Stat(planBaselinePath(websterDir, hashes["01-a.md"])); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("stored copy stat = %v; want none stored", statErr)
	}
}

func TestStorePlanBaseline_EmptyWebsterDirIsAnError(t *testing.T) {
	t.Parallel()
	if err := storePlanBaseline("", t.TempDir(), map[string]string{}); err == nil {
		t.Fatal("storePlanBaseline(\"\") error = nil; want a wiring error")
	}
}

func TestRestorePlan_RestoresEditedAndRemovesUnrecorded(t *testing.T) {
	t.Parallel()
	fx := newBaselineFixture(t)
	dir := fx.geom.PlanDir
	fingerprintWriteFiles(t, dir, map[string]string{"01-first.md": "edited", "03-new.md": "new"})
	if err := os.Remove(filepath.Join(dir, "02-second.md")); err != nil {
		t.Fatal(err)
	}

	restored, err := RestorePlan(fx.st, fx.geom)
	if err != nil {
		t.Fatalf("RestorePlan() error = %v", err)
	}
	if got := strings.Join(restored, ","); got != "01-first.md,02-second.md,03-new.md" {
		t.Errorf("restored = %q; want all three files, sorted", got)
	}
	for name, want := range map[string]string{"01-first.md": "first", "02-second.md": "second"} {
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || string(got) != want {
			t.Errorf("%s = %q, %v; want %q", name, got, err, want)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "03-new.md")); !os.IsNotExist(err) {
		t.Errorf("03-new.md stat error = %v; want not-exist", err)
	}
}

func TestRestorePlan_RefusesMissingCopy(t *testing.T) {
	t.Parallel()
	fx := newBaselineFixture(t)
	dir := fx.geom.PlanDir
	fingerprintWriteFiles(t, dir, map[string]string{"01-first.md": "edited", "02-second.md": "also edited"})
	if err := os.Remove(planBaselinePath(fx.geom.WebsterDir, fx.st.PlanFileHashes["01-first.md"])); err != nil {
		t.Fatal(err)
	}

	_, err := RestorePlan(fx.st, fx.geom)
	if !errors.Is(err, ErrPlanBaselineMissing) {
		t.Fatalf("RestorePlan() error = %v; want ErrPlanBaselineMissing", err)
	}
	for _, want := range []string{"01-first.md", "way forward:", "lyx webster run --fresh"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err, want)
		}
	}
	got, readErr := os.ReadFile(filepath.Join(dir, "02-second.md"))
	if readErr != nil || string(got) != "also edited" {
		t.Errorf("02-second.md = %q, %v; want it unchanged by a refused restore", got, readErr)
	}
}

func TestRestorePlan_RefusesStateWithoutHashes(t *testing.T) {
	t.Parallel()
	geom := Geometry{PlanDir: t.TempDir(), WebsterDir: t.TempDir()}
	if _, err := RestorePlan(&State{}, geom); !errors.Is(err, ErrPlanBaselineMissing) {
		t.Fatalf("RestorePlan() error = %v; want ErrPlanBaselineMissing", err)
	}
}

func TestRestorePlan_NothingChanged(t *testing.T) {
	t.Parallel()
	fx := newBaselineFixture(t)
	restored, err := RestorePlan(fx.st, fx.geom)
	if err != nil {
		t.Fatalf("RestorePlan() error = %v", err)
	}
	if len(restored) != 0 {
		t.Errorf("restored = %v; want none", restored)
	}
}
