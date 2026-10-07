//go:build integration

// calibrate_integration_test.go checks that the gitrepo-backed plan history and base tree give the
// calibration the same row as the in-memory ones.

package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/gitrepo"
)

func TestCalibrateGitBackedMatchesInMemory(t *testing.T) {
	t.Parallel()

	configDir := t.TempDir()
	seedProfile(t, configDir)

	dir := t.TempDir()
	gitkit.MustRun(t, dir, "git", "init", "-b", "main")
	write := func(rel, content string) {
		t.Helper()
		full := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	plan := planWith(map[int]string{1: "internal/a/a.go"})
	for name, data := range plan {
		write("_lyx/plan/"+name, data)
	}
	gitkit.Git(t, dir, "add", ".")
	gitkit.Git(t, dir, "commit", "-m", planCommitSubjectPrefix+"alpha")

	write("internal/a/a.go", lines(100))
	gitkit.Git(t, dir, "add", ".")
	gitkit.Git(t, dir, "commit", "-m", "base")
	repo := gitrepo.New(dir)
	baseSHA, err := repo.CurrentSHA()
	if err != nil {
		t.Fatal(err)
	}

	// The fork starts after any commit the fixture makes.
	started := time.Now().Add(time.Hour)
	runs := []RunTally{{Slug: "alpha", BaseSHA: baseSHA, Forks: []ForkTally{fork(started, 220, "01-c1")}}}

	got, err := Calibrate(runs, "fit", configDir, repo, repo)
	if err != nil {
		t.Fatalf("Calibrate over git: %v", err)
	}

	subject := planCommitSubjectPrefix + "alpha"
	planCommits, err := repo.CommitsWithSubject(subject)
	if err != nil || len(planCommits) != 1 {
		t.Fatalf("CommitsWithSubject = %v, %v; want the one plan commit", planCommits, err)
	}
	memHistory := fakeHistory{commits: map[string][]gitrepo.SubjectCommit{subject: planCommits}, files: map[string]string{}}
	for name, data := range plan {
		memHistory.files[planCommits[0].SHA+":_lyx/plan/"+name] = data
	}
	memBase := fakeBase{
		shas:  map[string]bool{baseSHA: true},
		files: map[string]string{baseSHA + ":internal/a/a.go": lines(100)},
	}
	want, err := Calibrate(runs, "fit", configDir, memHistory, memBase)
	if err != nil {
		t.Fatalf("Calibrate in memory: %v", err)
	}

	if len(want.Rows) != 1 || want.Rows[0].Estimate != 110 || want.Rows[0].Ratio != 2 {
		t.Fatalf("in-memory rows = %+v; want one row, estimate 110, ratio 2", want.Rows)
	}
	if len(got.Rows) != 1 || got.Rows[0] != want.Rows[0] || len(got.Skips) != 0 {
		t.Errorf("git-backed calibration rows = %+v, skips = %+v; want rows %+v and no skips", got.Rows, got.Skips, want.Rows)
	}
}
