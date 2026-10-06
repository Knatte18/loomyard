// planbaseline_test.go covers the plan baseline store and RestorePlan.
// Tier 1: no git, only t.TempDir().

package websterengine

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
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

//testtiming:keep pins each stored copy's bytes under its own hash and an existing copy left unrewritten by a second store; the covering begin-batch test only counts copies
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

// TestRestorePlan proves RestorePlan rewrites every edited or removed recorded file from its stored
// copy and removes files the record lacks, returning the touched names sorted, touches nothing on an
// unchanged plan, and refuses, naming the file and the fresh-run way forward and leaving the plan as
// it was, when a stored copy or the recorded hashes are missing.
func TestRestorePlan(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// prepare edits the fixture's plan or store.
		prepare func(t *testing.T, fx *baselineFixture)
		// wantRestored is the sorted names restored; ignored on a refusal.
		wantRestored []string
		// wantRefusal lists what an ErrPlanBaselineMissing refusal names; nil expects success.
		wantRefusal []string
		// wantFiles maps plan files to their content after the call; "" means absent.
		wantFiles map[string]string
	}{
		{
			name: "restores edited and removed files and removes unrecorded ones",
			prepare: func(t *testing.T, fx *baselineFixture) {
				fingerprintWriteFiles(t, fx.geom.PlanDir, map[string]string{"01-first.md": "edited", "03-new.md": "new"})
				if err := os.Remove(filepath.Join(fx.geom.PlanDir, "02-second.md")); err != nil {
					t.Fatal(err)
				}
			},
			wantRestored: []string{"01-first.md", "02-second.md", "03-new.md"},
			wantFiles:    map[string]string{"01-first.md": "first", "02-second.md": "second", "03-new.md": ""},
		},
		{
			name:      "an unchanged plan restores nothing",
			wantFiles: map[string]string{"01-first.md": "first", "02-second.md": "second"},
		},
		{
			name: "a missing stored copy refuses and leaves the plan",
			prepare: func(t *testing.T, fx *baselineFixture) {
				fingerprintWriteFiles(t, fx.geom.PlanDir, map[string]string{"01-first.md": "edited", "02-second.md": "also edited"})
				if err := os.Remove(planBaselinePath(fx.geom.WebsterDir, fx.st.PlanFileHashes["01-first.md"])); err != nil {
					t.Fatal(err)
				}
			},
			wantRefusal: []string{"01-first.md", "way forward:", "lyx webster run --fresh"},
			wantFiles:   map[string]string{"02-second.md": "also edited"},
		},
		{
			name:        "a state without hashes refuses",
			prepare:     func(t *testing.T, fx *baselineFixture) { fx.st = &State{} },
			wantRefusal: []string{"way forward:"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fx := newBaselineFixture(t)
			if tt.prepare != nil {
				tt.prepare(t, fx)
			}

			restored, err := RestorePlan(fx.st, fx.geom)
			if tt.wantRefusal != nil {
				if !errors.Is(err, ErrPlanBaselineMissing) {
					t.Fatalf("RestorePlan() error = %v; want ErrPlanBaselineMissing", err)
				}
				for _, want := range tt.wantRefusal {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("error %q missing %q", err, want)
					}
				}
			} else {
				if err != nil {
					t.Fatalf("RestorePlan() error = %v", err)
				}
				if !slices.Equal(restored, tt.wantRestored) {
					t.Errorf("restored = %v; want %v", restored, tt.wantRestored)
				}
			}
			for name, want := range tt.wantFiles {
				got, readErr := os.ReadFile(filepath.Join(fx.geom.PlanDir, name))
				if want == "" {
					if !os.IsNotExist(readErr) {
						t.Errorf("%s stat error = %v; want not-exist", name, readErr)
					}
					continue
				}
				if readErr != nil || string(got) != want {
					t.Errorf("%s = %q, %v; want %q", name, got, readErr, want)
				}
			}
		})
	}
}
