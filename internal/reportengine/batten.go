// batten.go reads the batten source of a run: the batten status snapshots recorded in the run's span of the records branch.

package reportengine

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/battenrecipe"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// battenCommit is a records commit that touched a slug's status file, read.
// holds is false for a commit that deleted the file or whose file does not decode, the latter also carrying decodeErr.
type battenCommit struct {
	committed time.Time
	holds     bool
	status    shedengine.Status
	decodeErr error

	// fresh marks a commit whose status begins a new batten run: its history opens with the worktree-create row and the previous commit touching the file held no file or a longer history.
	fresh bool
}

// battenIndex lists the records commits that touched each slug's status file, read lazily per slug.
type battenIndex struct {
	records  Reader
	touching map[string][]gitrepo.LogCommit
	read     map[string][]battenCommit
}

// newBattenIndex walks the records branch once for the commits touching a run's directory and groups them by slug, oldest first.
func newBattenIndex(src Sources) (*battenIndex, error) {
	root := filepath.ToSlash(shedrun.RunsRootRel())
	commits, err := src.Records.Log(src.RecordsTip, gitrepo.LogQuery{PathPrefix: root})
	if err != nil {
		return nil, fmt.Errorf("reportengine: walk the records branch: %w", err)
	}

	index := &battenIndex{records: src.Records, touching: map[string][]gitrepo.LogCommit{}, read: map[string][]battenCommit{}}
	for i := len(commits) - 1; i >= 0; i-- {
		for _, path := range commits[i].Paths {
			if slug, ok := statusPathSlug(root, path); ok {
				index.touching[slug] = append(index.touching[slug], commits[i])
			}
		}
	}
	return index, nil
}

// statusPathSlug returns the slug whose status file path names, for a path under root.
func statusPathSlug(root, path string) (string, bool) {
	rest, ok := strings.CutPrefix(path, root+"/")
	if !ok {
		return "", false
	}
	slug, _, ok := strings.Cut(rest, "/")
	if !ok {
		return "", false
	}
	rels, err := shedrun.TreeRunRels(slug)
	if err != nil || rels.Status != path {
		return "", false
	}
	return slug, true
}

// commitsOf reads the status file at every commit that touched slug's, oldest first.
func (b *battenIndex) commitsOf(slug, statusRel string)([]battenCommit, error) {
	if read, ok := b.read[slug]; ok {
		return read, nil
	}
	var read []battenCommit
	for _, commit := range b.touching[slug] {
		entry := battenCommit{committed: commit.Committed}
		data, err := b.records.FileAtRevision(commit.SHA, statusRel)
		switch {
		case errors.Is(err, gitrepo.ErrPathNotAtRevision):
		case err != nil:
			return nil, fmt.Errorf("reportengine: read %s at %s: %w", statusRel, commit.SHA, err)
		default:
			if entry.status, entry.decodeErr = shedengine.ParseStatusTolerant(data); entry.decodeErr == nil {
				entry.holds = true
			}
		}
		read = append(read, entry)
	}
	for i := range read {
		history := read[i].status.History
		if !read[i].holds || len(history) == 0 || history[0].Producer != battenrecipe.NameWorktreeCreate {
			continue
		}
		read[i].fresh = i == 0 || !read[i-1].holds || len(read[i-1].status.History) > len(history)
	}
	b.read[slug] = read
	return read, nil
}

// snapshotsOfRun returns the batten status snapshots of the run archived under tag, oldest first, and a finding for each snapshot that does not decode.
// The run's own fresh seed is the newest fresh seed committed after earlier, when there is one, and at or before until; its span runs up to the slug's next fresh seed or to the records tip.
// No fresh seed in that interval returns no snapshots.
func (b *battenIndex) snapshotsOfRun(slug, tag string, earlier *time.Time, until time.Time) ([]shedengine.Status, []Finding, error) {
	rels, err := shedrun.TreeRunRels(slug)
	if err != nil {
		return nil, nil, err
	}
	commits, err := b.commitsOf(slug, rels.Status)
	if err != nil {
		return nil, nil, err
	}

	start := -1
	for i := len(commits) - 1; i >= 0; i-- {
		commit := commits[i]
		if commit.fresh && (earlier == nil || commit.committed.After(*earlier)) && !commit.committed.After(until) {
			start = i
			break
		}
	}
	if start < 0 {
		return nil, nil, nil
	}
	end := len(commits)
	for i := start + 1; i < len(commits); i++ {
		if commits[i].fresh {
			end = i
			break
		}
	}

	var snapshots []shedengine.Status
	var findings []Finding
	for _, commit := range commits[start:end] {
		switch {
		case commit.decodeErr != nil:
			findings = append(findings, Finding{Tag: tag, File: rels.Status, Error: commit.decodeErr.Error()})
		case commit.holds:
			snapshots = append(snapshots, commit.status)
		}
	}
	return snapshots, findings, nil
}
