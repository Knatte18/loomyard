// archive_test.go covers the shared run-file archive (absent no-op, rename/preserve and collision
// suffixing over state.json, outcome.yaml and summary.md), archiveReportsDir's recreate-after-archive
// and absent-dir-still-recreates behavior, and ArchiveRunRecord's move, idempotence and refusals.
// Tier 1: no git, only t.TempDir() plus an injected now.

package websterengine

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/summaryparser"
)

// archiveFixedClock returns a func() time.Time that always returns t.
func archiveFixedClock(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

// TestArchiveRunFile pins the archive every run-scoped file shares: an absent file is a no-op, a
// present one is renamed to <name>-<UTC stamp><ext> with its content preserved, and a stamp already
// taken, with or without suffixes, takes the first free numeric suffix instead of clobbering.
func TestArchiveRunFile(t *testing.T) {
	t.Parallel()

	stamp := time.Date(2026, 7, 11, 13, 45, 0, 0, time.UTC)
	artifacts := []struct {
		name    string
		file    string
		archive func(dir string, now func() time.Time) (string, error)
		// archived is the archive name for suffix, under the fixed stamp.
		archived func(suffix string) string
	}{
		{"state.json", "state.json", archiveStateFile, func(suffix string) string { return "state-20260711T134500Z" + suffix + ".json" }},
		{"outcome.yaml", "outcome.yaml", archiveStaleOutcome, func(suffix string) string { return "outcome-20260711T134500Z" + suffix + ".yaml" }},
		{"summary.md", summaryparser.FileName, ArchiveStaleSummary, func(suffix string) string { return "summary-20260711T134500Z" + suffix + ".md" }},
	}
	cases := []struct {
		name string
		// absent leaves the file out.
		absent bool
		// taken lists the archive suffixes already occupied under the stamp.
		taken      []string
		wantSuffix string
	}{
		{name: "absent file is a no-op", absent: true},
		{name: "renames and preserves content"},
		{name: "a taken stamp takes the first free suffix", taken: []string{"", "-1"}, wantSuffix: "-2"},
	}
	for _, a := range artifacts {
		for _, tc := range cases {
			t.Run(a.name+": "+tc.name, func(t *testing.T) {
				t.Parallel()

				dir := t.TempDir()
				original := filepath.Join(dir, a.file)
				const content = "the run's own content\n"
				if !tc.absent {
					if err := os.WriteFile(original, []byte(content), 0o644); err != nil {
						t.Fatal(err)
					}
				}
				for _, suffix := range tc.taken {
					if err := os.WriteFile(filepath.Join(dir, a.archived(suffix)), []byte("taken"), 0o644); err != nil {
						t.Fatal(err)
					}
				}

				got, err := a.archive(dir, archiveFixedClock(stamp))
				if err != nil {
					t.Fatalf("archive error = %v; want nil", err)
				}
				if tc.absent {
					if got != "" {
						t.Errorf("archive = %q; want \"\" for an absent file", got)
					}
					return
				}
				if want := filepath.Join(dir, a.archived(tc.wantSuffix)); got != want {
					t.Errorf("archive = %q; want %q", got, want)
				}
				if _, err := os.Stat(original); !os.IsNotExist(err) {
					t.Errorf("original %s still exists after archiving; want it renamed away", a.file)
				}
				archived, err := os.ReadFile(got)
				if err != nil {
					t.Fatalf("ReadFile(%q): %v", got, err)
				}
				if string(archived) != content {
					t.Errorf("archived content = %q; want %q", archived, content)
				}
			})
		}
	}
}

func TestArchiveReportsDir(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// seedReport is the content of an existing 01-first.yaml; empty leaves the dir absent.
		seedReport string
	}{
		{name: "absent dir is still recreated empty"},
		{name: "existing content is archived wholesale and the dir recreated empty", seedReport: "status: done\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			reportsDir := filepath.Join(root, "reports")
			if tt.seedReport != "" {
				if err := os.MkdirAll(reportsDir, 0o755); err != nil {
					t.Fatalf("MkdirAll(reportsDir): %v", err)
				}
				if err := os.WriteFile(filepath.Join(reportsDir, "01-first.yaml"), []byte(tt.seedReport), 0o644); err != nil {
					t.Fatalf("write report: %v", err)
				}
			}
			clk := archiveFixedClock(time.Date(2026, 7, 11, 13, 45, 0, 0, time.UTC))

			if err := archiveReportsDir(reportsDir, clk); err != nil {
				t.Fatalf("archiveReportsDir() error = %v; want nil", err)
			}

			info, err := os.Stat(reportsDir)
			if err != nil {
				t.Fatalf("Stat(reportsDir) after archiveReportsDir(): %v", err)
			}
			if !info.IsDir() {
				t.Errorf("reportsDir = %q; want a directory", reportsDir)
			}
			entries, err := os.ReadDir(reportsDir)
			if err != nil {
				t.Fatalf("ReadDir(reportsDir): %v", err)
			}
			if len(entries) != 0 {
				t.Errorf("recreated reportsDir has %d entries; want empty", len(entries))
			}
			if tt.seedReport == "" {
				return
			}
			archivedReport := filepath.Join(root, "reports-20260711T134500Z", "01-first.yaml")
			content, err := os.ReadFile(archivedReport)
			if err != nil {
				t.Fatalf("ReadFile(%q): %v; want the archived report preserved", archivedReport, err)
			}
			if string(content) != tt.seedReport {
				t.Errorf("archived report content = %q; want %q", content, tt.seedReport)
			}
		})
	}
}

// archiveRecordGeom builds a Geometry over temp directories with a populated webster dir.
func archiveRecordGeom(t *testing.T) Geometry {
	t.Helper()
	root := t.TempDir()
	geom := Geometry{
		WebsterDir: filepath.Join(root, "webster"),
		ReportsDir: filepath.Join(root, "webster", "reports"),
		PromptsDir: filepath.Join(root, "scratch", "prompts"),
		ScratchDir: filepath.Join(root, "scratch"),
	}
	if err := SaveState(geom.WebsterDir, geom.ScratchDir, &State{RunGUID: "g1"}); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	if err := os.MkdirAll(geom.ReportsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(geom.ReportsDir, "01.yaml"), []byte("status: OK\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(geom.WebsterDir, "outcome.yaml"), []byte("outcome: done\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(geom.PromptsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(geom.PromptsDir, "01.md"), []byte("p"), 0o644); err != nil {
		t.Fatal(err)
	}
	return geom
}

func TestArchiveRunRecord(t *testing.T) {
	t.Parallel()

	t.Run("moves the whole record and clears prompts", func(t *testing.T) {
		t.Parallel()

		geom := archiveRecordGeom(t)
		dest := filepath.Join(t.TempDir(), "dest")

		if err := ArchiveRunRecord(geom, dest); err != nil {
			t.Fatalf("ArchiveRunRecord() error = %v; want nil", err)
		}

		for _, rel := range []string{"state.json", "outcome.yaml", filepath.Join("reports", "01.yaml")} {
			if _, err := os.Stat(filepath.Join(dest, rel)); err != nil {
				t.Errorf("archived %s missing: %v", rel, err)
			}
		}
		st, err := LoadState(geom.WebsterDir, geom.ScratchDir)
		if err != nil || st != nil {
			t.Errorf("LoadState after archive = (%v, %v); want (nil, nil)", st, err)
		}
		if _, err := os.Stat(geom.PromptsDir); !os.IsNotExist(err) {
			t.Errorf("prompts dir still present: %v", err)
		}
	})

	t.Run("a second call is a no-op", func(t *testing.T) {
		t.Parallel()

		geom := archiveRecordGeom(t)
		dest := filepath.Join(t.TempDir(), "dest")

		if err := ArchiveRunRecord(geom, dest); err != nil {
			t.Fatalf("first call: %v", err)
		}
		if err := ArchiveRunRecord(geom, dest); err != nil {
			t.Fatalf("second call error = %v; want nil", err)
		}
		if _, err := os.Stat(filepath.Join(dest, "state.json")); err != nil {
			t.Errorf("archived state.json missing after second call: %v", err)
		}
	})

	t.Run("an absent webster dir is a no-op", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		geom := Geometry{
			WebsterDir: filepath.Join(root, "webster"),
			PromptsDir: filepath.Join(root, "scratch", "prompts"),
			ScratchDir: filepath.Join(root, "scratch"),
		}
		dest := filepath.Join(root, "dest")

		if err := ArchiveRunRecord(geom, dest); err != nil {
			t.Fatalf("ArchiveRunRecord() error = %v; want nil", err)
		}
		if _, err := os.Stat(dest); !os.IsNotExist(err) {
			t.Errorf("dest created for an absent webster dir: %v", err)
		}
	})

	t.Run("a collision errors and moves nothing", func(t *testing.T) {
		t.Parallel()

		geom := archiveRecordGeom(t)
		dest := filepath.Join(t.TempDir(), "dest")
		if err := os.MkdirAll(dest, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dest, "outcome.yaml"), []byte("stale"), 0o644); err != nil {
			t.Fatal(err)
		}

		err := ArchiveRunRecord(geom, dest)
		if err == nil {
			t.Fatal("ArchiveRunRecord() error = nil; want collision error")
		}
		if !strings.Contains(err.Error(), "outcome.yaml") || !strings.Contains(err.Error(), "remove whichever copy is stale, then re-step") {
			t.Errorf("error = %v; want it to name the entry and the re-step way forward", err)
		}
		if _, statErr := os.Stat(filepath.Join(geom.WebsterDir, "state.json")); statErr != nil {
			t.Errorf("state.json moved despite collision: %v", statErr)
		}
	})

	t.Run("a held run lock returns ErrRunBusy", func(t *testing.T) {
		t.Parallel()

		geom := archiveRecordGeom(t)
		held, locked, err := lock.TryAcquireWriteLock(filepath.Join(geom.ScratchDir, runLockName))
		if err != nil || !locked {
			t.Fatalf("hold run lock: locked=%v err=%v", locked, err)
		}
		defer held.Release()

		err = ArchiveRunRecord(geom, filepath.Join(t.TempDir(), "dest"))
		if !errors.Is(err, ErrRunBusy) {
			t.Errorf("ArchiveRunRecord() error = %v; want ErrRunBusy", err)
		}
	})
}
