// trailercommit_integration_test.go covers trailer commits and the reads that find them.
// The subjects are CommitAnchoredPathsWithTrailers and CommitEmptyWithTrailers.
// The reads are FindTrailerCommit, TrailerCommits and AnchoredFileAt.
// It runs them against a wired hub fixture.
//
// Package fabricengine_test; shares the single TestMain in testmain_test.go.

package fabricengine_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitkit"
	"github.com/Knatte18/loomyard/internal/hubforge"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/testkit/tiergate"
)

// writeAnchored writes content to the anchor-relative rel in the weft worktree of l.
func writeAnchored(t *testing.T, l *lymcwd.Location, rel, content string) {
	t.Helper()
	path := filepath.Join(fabricengine.RecordsWorktree(l), l.AnchorRel, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

// warpHead returns the warp checkout's HEAD of l.
func warpHead(t *testing.T, l *lymcwd.Location) string {
	t.Helper()
	f, err := fabricengine.Open(l)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	sha, err := f.HeadSHA()
	if err != nil {
		t.Fatalf("HeadSHA: %v", err)
	}
	return sha
}

// firstParentPosition returns the first-parent position of the weft commit sha: its first-parent steps from the root.
func firstParentPosition(t *testing.T, l *lymcwd.Location, sha string) int {
	t.Helper()
	return gitkit.RevListCount(t, fabricengine.RecordsWorktree(l), "--first-parent", sha) - 1
}

// anchoredWeftPath returns the weft-relative slash path of the anchor-relative rel.
func anchoredWeftPath(l *lymcwd.Location, rel string) string {
	return filepath.ToSlash(filepath.Join(l.AnchorRel, rel))
}

// TestTrailerCommitsListsKeyCarriers lists the commits carrying a key over one hub.
func TestTrailerCommitsListsKeyCarriers(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	rec := fabricengine.NewMutations(l.HubPath)
	const probe = "_lym/trailer-list/probe.txt"
	const other = "_lym/trailer-list/other.txt"

	commit := func(rel, content, msg string, trailers ...fabricengine.Trailer) string {
		t.Helper()
		writeAnchored(t, l, rel, content)
		sha, committed, err := fabricengine.CommitAnchoredPathsWithTrailers(rec, l, []string{rel}, msg, trailers, fabricengine.SyncOptions{})
		if err != nil || !committed {
			t.Fatalf("commit %q committed = %v, err = %v", msg, committed, err)
		}
		return sha
	}
	olderSHA := commit(probe, "one", "older", fabricengine.Trailer{Key: "Drift-Seal", Value: "seal-1"})
	olderWarp := warpHead(t, l)
	commit(other, "two", "unmarked")
	newerSHA := commit(probe, "three", "newer", fabricengine.Trailer{Key: "Drift-Seal", Value: "seal-2"})
	newerWarp := warpHead(t, l)

	got, err := fabricengine.TrailerCommits(l, "Drift-Seal")
	if err != nil {
		t.Fatalf("TrailerCommits error = %v", err)
	}
	want := []fabricengine.TrailerCommit{
		{SHA: newerSHA, Value: "seal-2", CodeHead: newerWarp, Position: firstParentPosition(t, l, newerSHA)},
		{SHA: olderSHA, Value: "seal-1", CodeHead: olderWarp, Position: firstParentPosition(t, l, olderSHA)},
	}
	if !slices.Equal(got, want) {
		t.Errorf("TrailerCommits(Drift-Seal) = %+v; want %+v", got, want)
	}
	if got[0].Position <= got[1].Position {
		t.Errorf("positions = %d then %d; want the newer commit's position higher", got[0].Position, got[1].Position)
	}

	t.Run("UnbornWeftIsEmpty", func(t *testing.T) {
		unborn := *l
		unborn.WorktreeName = "unborn"
		weft := fabricengine.RecordsWorktree(&unborn)
		if err := os.MkdirAll(weft, 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		gitkit.Git(t, weft, "init")

		got, err := fabricengine.TrailerCommits(&unborn, "Drift-Seal")
		if err != nil || len(got) != 0 {
			t.Errorf("TrailerCommits on an unborn weft = (%+v, %v); want empty and nil error", got, err)
		}
	})
}

// TestCommitEmptyWithTrailers drives the empty trailer commit over one hub, in order.
// Its subtests share the hub's weft history and index and run in sequence, so none is parallel.
func TestCommitEmptyWithTrailers(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	weft := fabricengine.RecordsWorktree(l)
	rec := fabricengine.NewMutations(l.HubPath)
	const probe = "_lym/empty-trailer/probe.txt"
	const bystander = "_lym/empty-trailer/bystander.txt"

	writeAnchored(t, l, probe, "content")
	if _, committed, err := fabricengine.CommitAnchoredPaths(rec, l, []string{probe}, "seed", fabricengine.SyncOptions{}); err != nil || !committed {
		t.Fatalf("seed commit committed = %v, err = %v", committed, err)
	}
	trailers := []fabricengine.Trailer{{Key: "Drift-Seal", Value: "seal-9"}}

	tests := []struct {
		name        string
		stageOther  bool
		wantStaged  string
		wantTrailer string
	}{
		{"UnchangedFileNothingStaged", false, "", "Drift-Seal: seal-9"},
		{"UnchangedFileWithAnotherFileStaged", true, anchoredWeftPath(l, bystander), "Drift-Seal: seal-9"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.stageOther {
				writeAnchored(t, l, bystander, "staged only")
				gitkit.Git(t, weft, "add", "--", anchoredWeftPath(l, bystander))
			}
			parent := gitkit.RevParse(t, weft, "HEAD")

			sha, committed, err := fabricengine.CommitEmptyWithTrailers(rec, l, probe, "empty", trailers, fabricengine.SyncOptions{})
			if err != nil || !committed {
				t.Fatalf("CommitEmptyWithTrailers committed = %v, err = %v", committed, err)
			}
			if head := gitkit.RevParse(t, weft, "HEAD"); head != sha || sha == parent {
				t.Errorf("HEAD = %s, returned %s; want a new commit that is HEAD (parent %s)", head, sha, parent)
			}
			if message := gitkit.Git(t, weft, "log", "-1", "--format=%B", sha); !strings.Contains(message, tt.wantTrailer) {
				t.Errorf("commit message = %q; want it to carry %q", message, tt.wantTrailer)
			}
			if got, want := gitkit.RevParse(t, weft, sha+"^{tree}"), gitkit.RevParse(t, weft, parent+"^{tree}"); got != want {
				t.Errorf("commit tree = %s; want parent's tree %s", got, want)
			}
			if staged := gitkit.Git(t, weft, "diff", "--cached", "--name-only"); staged != tt.wantStaged {
				t.Errorf("staged paths = %q; want %q", staged, tt.wantStaged)
			}
		})
	}

	t.Run("InvalidTrailerKeyIsRefused", func(t *testing.T) {
		before := gitkit.RevParse(t, weft, "HEAD")

		sha, committed, err := fabricengine.CommitEmptyWithTrailers(rec, l, probe, "refused", []fabricengine.Trailer{{Key: "Drift Seal", Value: "a"}}, fabricengine.SyncOptions{})
		if err == nil || !strings.Contains(err.Error(), "Drift Seal") {
			t.Fatalf("CommitEmptyWithTrailers error = %v; want one naming the key", err)
		}
		if sha != "" || committed {
			t.Errorf("CommitEmptyWithTrailers = (%q, %v); want (\"\", false)", sha, committed)
		}
		if after := gitkit.RevParse(t, weft, "HEAD"); after != before {
			t.Errorf("weft HEAD = %s; want %s unchanged", after, before)
		}
	})
}

// TestTrailerCommits runs the trailer commit, find and read-at-commit rows over one hub, in order.
// Its subtests share the hub's weft history and run in sequence, so none is parallel.
func TestTrailerCommits(t *testing.T) {
	tiergate.Require(t, 1)
	t.Parallel()

	h := hubforge.CopyHub(t, hubforge.Shape{Anchor: "."})
	l := h.Location
	rec := fabricengine.NewMutations(l.HubPath)
	const probe = "_lym/trailer-probe/probe.txt"

	writeAnchored(t, l, probe, "old")
	firstWarp := warpHead(t, l)
	firstSHA, committed, err := fabricengine.CommitAnchoredPathsWithTrailers(rec, l, []string{probe}, "first",
		[]fabricengine.Trailer{{Key: "Drift-Seal", Value: "seal-1"}, {Key: "Drift-Records", Value: "records-1"}}, fabricengine.SyncOptions{})
	if err != nil || !committed {
		t.Fatalf("first commit committed = %v, err = %v", committed, err)
	}

	gitkit.CommitFile(t, l.WorktreePath(), "trailer-warp.txt", "x", "move the warp head")

	writeAnchored(t, l, probe, "new")
	secondWarp := warpHead(t, l)
	secondSHA, committed, err := fabricengine.CommitAnchoredPathsWithTrailers(rec, l, []string{probe}, "second",
		[]fabricengine.Trailer{{Key: "Drift-Records", Value: "records-2"}}, fabricengine.SyncOptions{})
	if err != nil || !committed {
		t.Fatalf("second commit committed = %v, err = %v", committed, err)
	}

	t.Run("FindPicksNewestAndOldest", func(t *testing.T) {
		tests := []struct {
			name   string
			key    string
			newest bool
			want   fabricengine.TrailerCommit
		}{
			{"RecordsNewest", "Drift-Records", true, fabricengine.TrailerCommit{SHA: secondSHA, Value: "records-2", CodeHead: secondWarp, Position: firstParentPosition(t, l, secondSHA)}},
			{"RecordsOldest", "Drift-Records", false, fabricengine.TrailerCommit{SHA: firstSHA, Value: "records-1", CodeHead: firstWarp, Position: firstParentPosition(t, l, firstSHA)}},
			{"SealNewest", "Drift-Seal", true, fabricengine.TrailerCommit{SHA: firstSHA, Value: "seal-1", CodeHead: firstWarp, Position: firstParentPosition(t, l, firstSHA)}},
			{"SealOldest", "Drift-Seal", false, fabricengine.TrailerCommit{SHA: firstSHA, Value: "seal-1", CodeHead: firstWarp, Position: firstParentPosition(t, l, firstSHA)}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got, found, err := fabricengine.FindTrailerCommit(l, tt.key, tt.newest)
				if err != nil || !found {
					t.Fatalf("FindTrailerCommit(%q, %v) found = %v, err = %v", tt.key, tt.newest, found, err)
				}
				if got != tt.want {
					t.Errorf("FindTrailerCommit(%q, %v) = %+v; want %+v", tt.key, tt.newest, got, tt.want)
				}
			})
		}
	})

	t.Run("MissingKeyIsNotFound", func(t *testing.T) {
		got, found, err := fabricengine.FindTrailerCommit(l, "Drift-Absent", true)
		if err != nil || found {
			t.Errorf("FindTrailerCommit(Drift-Absent) = (%+v, %v, %v); want not found, nil error", got, found, err)
		}
	})

	t.Run("AnchoredFileAtReadsTheCommit", func(t *testing.T) {
		tests := []struct {
			name      string
			rev       string
			path      string
			wantFound bool
			want      string
		}{
			{"OlderCommit", firstSHA, probe, true, "old"},
			{"NewerCommit", secondSHA, probe, true, "new"},
			{"AbsentPath", firstSHA, "_lym/trailer-probe/never.txt", false, ""},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				data, found, err := fabricengine.AnchoredFileAt(l, tt.rev, tt.path)
				if err != nil {
					t.Fatalf("AnchoredFileAt(%q) error = %v", tt.path, err)
				}
				if found != tt.wantFound || string(data) != tt.want {
					t.Errorf("AnchoredFileAt(%q) = (%q, %v); want (%q, %v)", tt.path, data, found, tt.want, tt.wantFound)
				}
			})
		}
	})

	t.Run("RefusedTrailerStagesNothing", func(t *testing.T) {
		writeAnchored(t, l, probe, "refused")
		before := gitkit.RevParse(t, fabricengine.RecordsWorktree(l), "HEAD")
		tests := []struct {
			name    string
			trailer fabricengine.Trailer
			want    string
		}{
			{"ValueWithNewline", fabricengine.Trailer{Key: "Drift-Seal", Value: "a\nWarp-SHA: forged"}, "Drift-Seal"},
			{"EmptyValue", fabricengine.Trailer{Key: "Drift-Seal", Value: ""}, "Drift-Seal"},
			{"KeyOutsideCharset", fabricengine.Trailer{Key: "Drift Seal", Value: "a"}, "Drift Seal"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				sha, committed, err := fabricengine.CommitAnchoredPathsWithTrailers(rec, l, []string{probe}, "refused", []fabricengine.Trailer{tt.trailer}, fabricengine.SyncOptions{})
				if err == nil || !strings.Contains(err.Error(), tt.want) {
					t.Fatalf("CommitAnchoredPathsWithTrailers error = %v; want one naming %q", err, tt.want)
				}
				if sha != "" || committed {
					t.Errorf("CommitAnchoredPathsWithTrailers = (%q, %v); want (\"\", false)", sha, committed)
				}
				if after := gitkit.RevParse(t, fabricengine.RecordsWorktree(l), "HEAD"); after != before {
					t.Errorf("weft HEAD = %s; want %s unchanged", after, before)
				}
				if staged := gitkit.Git(t, fabricengine.RecordsWorktree(l), "diff", "--cached", "--name-only"); strings.TrimSpace(staged) != "" {
					t.Errorf("staged paths = %q; want none", staged)
				}
			})
		}
	})

	t.Run("UnbornWeftIsNotFound", func(t *testing.T) {
		unborn := *l
		unborn.WorktreeName = "unborn"
		weft := fabricengine.RecordsWorktree(&unborn)
		if err := os.MkdirAll(weft, 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		gitkit.Git(t, weft, "init")

		got, found, err := fabricengine.FindTrailerCommit(&unborn, "Drift-Seal", true)
		if err != nil || found {
			t.Errorf("FindTrailerCommit on an unborn weft = (%+v, %v, %v); want not found, nil error", got, found, err)
		}
	})
}
