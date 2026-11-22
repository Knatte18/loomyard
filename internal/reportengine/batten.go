// batten.go reads a run's batten source: the status snapshots recorded in the run's span of the records branch.

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

const (
	// firstBattenLookback is how far before the report's window the first walk of the records branch reaches;
	// each widening of the walk doubles it.
	firstBattenLookback = 7 * 24 * time.Hour

	// battenSeedMargin is how long before a run's loom seed stamp its batten seed may lie.
	battenSeedMargin = 7 * 24 * time.Hour
)

// battenStatusRead is a slug's status file as read at one records commit.
// holds is false for a commit that holds no such file or whose file does not decode;
// a file that does not decode also carries decodeErr.
type battenStatusRead struct {
	holds     bool
	status    shedengine.Status
	decodeErr error
}

// opensRun reports whether the status read opens with the worktree-create row, as a run's first status does.
func (r battenStatusRead) opensRun() bool {
	history := r.status.History
	return r.holds && len(history) > 0 && history[0].Producer == battenrecipe.NameWorktreeCreate
}

// battenIndex lists the records commits that touched each slug's status file, oldest first.
// They come from one walk of the records branch's first-parent line, from floor to the tip.
// A zero floor walks the whole history.
// The walk opens firstBattenLookback before windowStart and widens only when a run's fresh seed lies before it.
type battenIndex struct {
	records     Reader
	tip         string
	windowStart time.Time
	lookback    time.Duration
	floor       time.Time

	// walked is the number of commits the last walk returned.
	walked   int
	touching map[string][]gitrepo.LogCommit
}

// newBattenIndex walks the records branch for the commits touching a run's directory.
// The walk starts firstBattenLookback before windowStart.
// A zero windowStart walks the whole history.
func newBattenIndex(src Sources, windowStart time.Time) (*battenIndex, error) {
	index := &battenIndex{records: src.Records, tip: src.RecordsTip, windowStart: windowStart, lookback: firstBattenLookback}
	if !windowStart.IsZero() {
		index.floor = windowStart.Add(-firstBattenLookback)
	}
	if err := index.walk(); err != nil {
		return nil, err
	}
	return index, nil
}

// walk reads the commits touching the run-records root from floor to the tip.
// It follows the first-parent line, which stops at the first commit older than floor.
// It groups the commits by slug, oldest first.
func (b *battenIndex) walk() error {
	root := filepath.ToSlash(shedrun.RunsRootRel())
	commits, err := b.records.Log(b.tip, gitrepo.LogQuery{FirstParent: true, PathPrefix: root, Since: b.floor})
	if err != nil {
		return fmt.Errorf("reportengine: walk the records branch: %w", err)
	}

	b.walked = len(commits)
	b.touching = map[string][]gitrepo.LogCommit{}
	for i := len(commits) - 1; i >= 0; i-- {
		for _, path := range commits[i].Paths {
			if slug, ok := statusPathSlug(root, path); ok {
				b.touching[slug] = append(b.touching[slug], commits[i])
			}
		}
	}
	return nil
}

// widen doubles the lookback and walks again, and reports false when the walk cannot reach further back:
// it already covers the whole history, or its floor is at or before bound.
// A non-zero bound caps the doubled floor, so the widening ends there.
// With a zero bound, a doubled walk that finds no commit the last one lacked gives way to a walk of the whole history,
// so a run with no fresh seed at all ends the widening in one more walk.
func (b *battenIndex) widen(bound time.Time) (bool, error) {
	if b.floor.IsZero() || (!bound.IsZero() && !b.floor.After(bound)) {
		return false, nil
	}
	previous := b.walked
	b.lookback *= 2
	b.floor = b.windowStart.Add(-b.lookback)
	if !bound.IsZero() && b.floor.Before(bound) {
		b.floor = bound
	}
	if err := b.walk(); err != nil {
		return false, err
	}
	if bound.IsZero() && b.walked == previous {
		b.floor = time.Time{}
		if err := b.walk(); err != nil {
			return false, err
		}
	}
	return true, nil
}

// statusPathSlug returns the slug whose status file is path, and false when path is not such a file under root.
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

// battenStatusReader reads one slug's status file at records commits, each commit at most once.
// It lives for one run's span read, so no parsed status outlives that read but the run's own snapshots.
type battenStatusReader struct {
	records   Reader
	statusRel string
	read      map[string]battenStatusRead
}

// at returns the status file read at the commit sha.
func (r *battenStatusReader) at(sha string) (battenStatusRead, error) {
	if read, ok := r.read[sha]; ok {
		return read, nil
	}
	var read battenStatusRead
	data, err := r.records.FileAtRevision(sha, r.statusRel)
	switch {
	case errors.Is(err, gitrepo.ErrPathNotAtRevision):
	case err != nil:
		return battenStatusRead{}, fmt.Errorf("reportengine: read %s at %s: %w", r.statusRel, sha, err)
	default:
		if read.status, read.decodeErr = shedengine.ParseStatusTolerant(data); read.decodeErr == nil {
			read.holds = true
		}
	}
	r.read[sha] = read
	return read, nil
}

// isFreshSeed reports whether commits[k] opens a run:
// its status opens with worktree-create, and the status before it is absent, does not decode or holds a longer history.
// The status before the walk's oldest commit is read at that commit's first parent,
// and a commit with nothing before it in a whole-history walk is a seed.
func (b *battenIndex) isFreshSeed(commits []gitrepo.LogCommit, k int, reader *battenStatusReader) (bool, error) {
	read, err := reader.at(commits[k].SHA)
	if err != nil || !read.opensRun() {
		return false, err
	}
	var prior battenStatusRead
	switch {
	case k > 0:
		prior, err = reader.at(commits[k-1].SHA)
	case b.floor.IsZero() || len(commits[k].Parents) == 0:
		return true, nil
	default:
		prior, err = reader.at(commits[k].Parents[0])
	}
	if err != nil {
		return false, err
	}
	return !prior.holds || len(prior.status.History) > len(read.status.History), nil
}

// newestSeed returns the index in commits of the newest fresh seed.
// The seed is committed after earlier, when there is one, and at or before until.
// It returns -1 when the walked commits hold none.
func (b *battenIndex) newestSeed(commits []gitrepo.LogCommit, reader *battenStatusReader, earlier *time.Time, until time.Time) (int, error) {
	for k := len(commits) - 1; k >= 0; k-- {
		committed := commits[k].Committed
		if committed.After(until) || (earlier != nil && !committed.After(*earlier)) {
			continue
		}
		fresh, err := b.isFreshSeed(commits, k, reader)
		if err != nil {
			return -1, err
		}
		if fresh {
			return k, nil
		}
	}
	return -1, nil
}

// snapshotsOfRun returns the batten status snapshots of the run archived under tag, oldest first.
// It also returns the commit holding the last of them and a finding for each snapshot that does not decode.
// The run's own fresh seed is the newest fresh seed committed after earlier, when there is one, and at or before until.
// Its span runs up to the slug's next fresh seed, or to the records tip.
// With no fresh seed in that interval, it returns no snapshots and an empty commit.
// It widens the walk while the seed may lie before the walk's floor.
// It stops at earlier or, when there is none, at battenSeedMargin before startedAt, the run's loom seed stamp.
// An empty or unparseable stamp sets no stop.
// It reads the status file only at the span's commits and the one before its seed.
func (b *battenIndex) snapshotsOfRun(slug, tag string, earlier *time.Time, startedAt string, until time.Time) ([]shedengine.Status, string, []Finding, error) {
	rels, err := shedrun.TreeRunRels(slug)
	if err != nil {
		return nil, "", nil, err
	}
	var reach time.Time
	if started, parseErr := time.Parse(time.RFC3339, startedAt); parseErr == nil {
		reach = started.Add(-battenSeedMargin)
	}
	if earlier != nil {
		reach = *earlier
	}
	reader := &battenStatusReader{records: b.records, statusRel: rels.Status, read: map[string]battenStatusRead{}}

	var commits []gitrepo.LogCommit
	start := -1
	for {
		commits = b.touching[slug]
		if start, err = b.newestSeed(commits, reader, earlier, until); err != nil {
			return nil, "", nil, err
		}
		if start >= 0 {
			break
		}
		widened, err := b.widen(reach)
		if err != nil {
			return nil, "", nil, err
		}
		if !widened {
			break
		}
	}
	if start < 0 {
		return nil, "", nil, nil
	}

	end := len(commits)
	for i := start + 1; i < len(commits); i++ {
		fresh, err := b.isFreshSeed(commits, i, reader)
		if err != nil {
			return nil, "", nil, err
		}
		if fresh {
			end = i
			break
		}
	}

	var snapshots []shedengine.Status
	var findings []Finding
	recordCommit := ""
	for _, commit := range commits[start:end] {
		read, err := reader.at(commit.SHA)
		if err != nil {
			return nil, "", nil, err
		}
		switch {
		case read.decodeErr != nil:
			findings = append(findings, Finding{Tag: tag, File: rels.Status, Error: read.decodeErr.Error()})
		case read.holds:
			snapshots = append(snapshots, read.status)
			recordCommit = commit.SHA
		}
	}
	return snapshots, recordCommit, findings, nil
}
