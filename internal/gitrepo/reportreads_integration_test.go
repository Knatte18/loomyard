//go:build integration

// reportreads_integration_test.go covers the reads a report over finished runs builds on — TagRefs, Log and SubdirsAtRevision — against one real git repository whose commits carry fixed committer dates, reusing blobread_integration_test.go's commitAllAt and gitrepo_test.go's newRepo and writeFile fixture helpers.

package gitrepo_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/gitrepo"
)

// The fixed committer dates of the commits TestReportReads builds, one day apart.
const (
	dateA = "2026-01-01T12:00:00Z"
	dateB = "2026-01-02T12:00:00Z"
	dateS = "2026-01-03T12:00:00Z"
	dateM = "2026-01-04T12:00:00Z"
)

// mustParseDate parses an RFC 3339 fixture date.
func mustParseDate(t *testing.T, date string) time.Time {
	t.Helper()

	parsed, err := time.Parse(time.RFC3339, date)
	if err != nil {
		t.Fatalf("parse date %q: %v", date, err)
	}
	return parsed
}

// mergeNoFastForwardAt merges branch into the checked-out branch of dir with a merge commit whose author and committer date is committerDate.
func mergeNoFastForwardAt(t *testing.T, dir, branch, committerDate string) {
	t.Helper()

	cmd := exec.Command("git", "merge", "--no-ff", "-m", "merge "+branch, branch)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_COMMITTER_DATE="+committerDate, "GIT_AUTHOR_DATE="+committerDate)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git merge %s in %s: %v; output: %s", branch, dir, err, out)
	}
}

// logSHAs returns the SHAs of commits in order.
func logSHAs(commits []gitrepo.LogCommit) []string {
	shas := make([]string, len(commits))
	for i, commit := range commits {
		shas[i] = commit.SHA
	}
	return shas
}

// TestReportReads drives the three reads over one repository:
// commit A adds runs/a/status.json holding four lines, B rewrites it to three lines and adds docs/x.md, a side branch adds side.txt in S, and M merges S into the main line with --no-ff;
// an annotated tag archive/a/1 points at A, a lightweight archive/b/2 at M and other at B.
// Every step only reads, so the steps run serially in one order over the shared repository.
func TestReportReads(t *testing.T) {
	t.Parallel()

	dir, repo := newRepo(t)
	if err := os.MkdirAll(filepath.Join(dir, "runs", "a"), 0o755); err != nil {
		t.Fatalf("mkdir runs/a: %v", err)
	}
	writeFile(t, dir, "runs/a/status.json", "l1\nl2\nl3\nl4\n")
	gitkit.Git(t, dir, "add", ".")
	commitAllAt(t, dir, "commit A", dateA)
	shaA := requireCurrentSHA(t, repo)

	writeFile(t, dir, "runs/a/status.json", "l1\nL2\nl4\n")
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	writeFile(t, dir, "docs/x.md", "doc")
	gitkit.Git(t, dir, "add", ".")
	commitAllAt(t, dir, "commit B", dateB)
	shaB := requireCurrentSHA(t, repo)

	gitkit.Git(t, dir, "checkout", "-b", "side")
	writeFile(t, dir, "side.txt", "side")
	gitkit.Git(t, dir, "add", ".")
	commitAllAt(t, dir, "commit S", dateS)
	shaS := requireCurrentSHA(t, repo)
	gitkit.Git(t, dir, "checkout", "main")
	mergeNoFastForwardAt(t, dir, "side", dateM)
	shaM := requireCurrentSHA(t, repo)

	gitkit.Git(t, dir, "tag", "-a", "-m", "archive a", "archive/a/1", shaA)
	gitkit.Git(t, dir, "tag", "archive/b/2", shaM)
	gitkit.Git(t, dir, "tag", "other", shaB)

	steps := []struct {
		name string
		run  func(t *testing.T)
	}{
		{"TagRefs lists the prefixed tags peeled and ordered by date", func(t *testing.T) {
			got, err := repo.TagRefs("archive/")
			if err != nil {
				t.Fatalf("TagRefs error = %v", err)
			}
			want := []gitrepo.TagRef{
				{Name: "archive/a/1", Commit: shaA, Committed: mustParseDate(t, dateA)},
				{Name: "archive/b/2", Commit: shaM, Committed: mustParseDate(t, dateM)},
			}
			if len(got) != len(want) {
				t.Fatalf("TagRefs(archive/) = %+v; want %+v", got, want)
			}
			for i := range want {
				if got[i].Name != want[i].Name || got[i].Commit != want[i].Commit || !got[i].Committed.Equal(want[i].Committed) {
					t.Errorf("TagRefs(archive/)[%d] = %+v; want %+v", i, got[i], want[i])
				}
			}

			none, err := repo.TagRefs("nope/")
			if err != nil || len(none) != 0 {
				t.Errorf("TagRefs(nope/) = (%+v, %v); want none", none, err)
			}
		}},
		{"Log walks from a tip as the query shapes it", func(t *testing.T) {
			tests := []struct {
				name  string
				query gitrepo.LogQuery
				want  []string
			}{
				{"every parent", gitrepo.LogQuery{}, []string{shaM, shaS, shaB, shaA}},
				{"first parent only", gitrepo.LogQuery{FirstParent: true}, []string{shaM, shaB, shaA}},
				{"first parent since a date", gitrepo.LogQuery{FirstParent: true, Since: mustParseDate(t, dateB)}, []string{shaM, shaB}},
				{"every parent since a date", gitrepo.LogQuery{Since: mustParseDate(t, dateS)}, []string{shaM, shaS}},
				{"path prefix", gitrepo.LogQuery{PathPrefix: "runs"}, []string{shaB, shaA}},
				{"limit", gitrepo.LogQuery{Limit: 1}, []string{shaM}},
			}
			for _, tt := range tests {
				got, err := repo.Log(shaM, tt.query)
				if err != nil {
					t.Fatalf("Log(%s) error = %v", tt.name, err)
				}
				if gotSHAs := logSHAs(got); !reflect.DeepEqual(gotSHAs, tt.want) {
					t.Errorf("Log(%s) = %v; want %v", tt.name, gotSHAs, tt.want)
				}
			}

			byPath, err := repo.Log(shaM, gitrepo.LogQuery{PathPrefix: "runs"})
			if err != nil {
				t.Fatalf("Log(path prefix) error = %v", err)
			}
			for _, commit := range byPath {
				if !reflect.DeepEqual(commit.Paths, []string{"runs/a/status.json"}) {
					t.Errorf("Log(path prefix) commit %s Paths = %v; want [runs/a/status.json]", commit.SHA, commit.Paths)
				}
			}

			first, err := repo.Log(shaM, gitrepo.LogQuery{Limit: 1})
			if err != nil || len(first) != 1 {
				t.Fatalf("Log(limit) = (%+v, %v); want one commit", first, err)
			}
			if !reflect.DeepEqual(first[0].Parents, []string{shaB, shaS}) || first[0].Paths != nil || !first[0].Committed.Equal(mustParseDate(t, dateM)) {
				t.Errorf("Log(limit)[0] = %+v; want parents [B S], no paths and M's date", first[0])
			}

			if _, err := repo.Log("not-a-sha", gitrepo.LogQuery{}); !errors.Is(err, gitrepo.ErrInvalidSHA) {
				t.Errorf("Log(not-a-sha) error = %v; want ErrInvalidSHA", err)
			}
		}},
		{"SubdirsAtRevision lists the directories of one directory", func(t *testing.T) {
			tests := []struct {
				dir  string
				want []string
			}{
				{"runs", []string{"a"}},
				{"", []string{"docs", "runs"}},
				{"absent", nil},
			}
			for _, tt := range tests {
				got, err := repo.SubdirsAtRevision(shaB, tt.dir)
				if err != nil {
					t.Fatalf("SubdirsAtRevision(%q) error = %v", tt.dir, err)
				}
				if !reflect.DeepEqual(got, tt.want) {
					t.Errorf("SubdirsAtRevision(B, %q) = %v; want %v", tt.dir, got, tt.want)
				}
			}

			if _, err := repo.SubdirsAtRevision("not-a-sha", ""); !errors.Is(err, gitrepo.ErrInvalidSHA) {
				t.Errorf("SubdirsAtRevision(not-a-sha) error = %v; want ErrInvalidSHA", err)
			}
		}},
	}
	for _, step := range steps {
		t.Run(step.name, step.run)
	}
}
