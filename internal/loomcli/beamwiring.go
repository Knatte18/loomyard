// beamwiring.go holds the wiring only a beam run needs: the BeamDescribe row's seams, which read the beam's board entry and the commits it adds over its parent.

package loomcli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/summaryparser"
)

// beamDescribeDeps assembles the BeamDescribe row's seams for the beam pair at location, whose slug names its board entry and whose parentBranch is the branch it lands on.
// Every closure reads or writes on demand and opens nothing at build time.
// An empty parentBranch, a pair with no origin record, is refused when the commit subjects are read.
func beamDescribeDeps(location *lyxcwd.Location, slug, parentBranch string) landingshed.BeamDescribeDeps {
	return landingshed.BeamDescribeDeps{
		Slug:            slug,
		DescriptionPath: summaryparser.Path(loomengine.LandingDir(location)),
		Entry: func() (string, string, error) {
			board, err := boardengine.OpenHub(location.HubPath)
			if err != nil {
				return "", "", err
			}
			task, found, err := board.GetTask(slug)
			if err != nil {
				return "", "", err
			}
			if !found {
				return "", "", fmt.Errorf("the board holds no entry %q; way forward: restore the entry with \"lyx board upsert\", then run \"lyx beam land %s\" again", slug, slug)
			}
			return task.Title, task.Brief, nil
		},
		CommitSubjects: func() ([]string, error) {
			return beamCommitSubjects(location, parentBranch)
		},
		// Commit mirrors CommitDescription, including its discard of (sha, committed), which makes a repeat commit over an unchanged directory a no-op.
		Commit: func() error {
			_, _, err := fabricengine.CommitAnchoredPaths(fabricengine.NewMutations(""), location, []string{loomengine.LandingDirRel()}, fmt.Sprintf("loom: change description for %s", slug), fabricengine.EnvSyncOptions())
			return err
		},
	}
}

// beamCommitSubjects lists the subjects of the commits the beam at location adds over its merge base with parentBranch, oldest first, merge commits left out.
func beamCommitSubjects(location *lyxcwd.Location, parentBranch string) ([]string, error) {
	if parentBranch == "" {
		return nil, errors.New("the beam's pair records no parent branch to measure its commits from; way forward: make the beam through \"lyx beam create\" or \"lyx beam promote\", which record its parent branch")
	}
	repo := gitrepo.New(location.WorktreePath())
	head, err := repo.CurrentSHA()
	if err != nil {
		return nil, err
	}
	parentTip, err := repo.ResolveSHA(parentBranch)
	if err != nil {
		return nil, err
	}
	base, err := repo.MergeBase(head, parentTip)
	if err != nil {
		return nil, err
	}
	added, err := repo.CommitsNotIn(head, base)
	if err != nil {
		return nil, err
	}

	// CommitsNotIn answers newest first.
	slices.Reverse(added)
	subjects := make([]string, 0, len(added))
	for _, sha := range added {
		parents, err := repo.CommitParents(sha)
		if err != nil {
			return nil, err
		}
		if len(parents) > 1 {
			continue
		}
		detail, err := repo.CommitDetail(sha)
		if err != nil {
			return nil, err
		}
		subject, _, _ := strings.Cut(strings.TrimSpace(detail.Message), "\n")
		subjects = append(subjects, subject)
	}
	return subjects, nil
}

// beamParentBranch returns the parent branch the beam pair at location records, or "" when the pair has no origin record yet.
// The record is stat'ed first so a pair without one reads as unrecorded, as the merge base reader's wiring does.
func beamParentBranch(location *lyxcwd.Location) (string, error) {
	if _, err := os.Stat(fabricengine.OriginRecordPath(location)); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("loomcli: stat the origin record: %w", err)
	}
	return readRecordedParentBranch(location)
}
