// geometry.go declares Geometry, the paths, Git and code index the shared batch machinery is told by its implementer.
// It also declares Git, the questions the machinery asks a worktree's repository.

package runplan

import (
	"errors"

	"github.com/Knatte18/loomyard/internal/planindex"
)

// errNoIndex is returned by a call that needs the code index when Geometry.Index was never wired.
var errNoIndex = errors.New("runplan: the geometry carries no code index; the implementer's CLI layer must set Geometry.Index")

// Geometry is the set of paths an implementer tells the shared batch machinery.
// It also carries the Git and code index the machinery reads through.
// No method validates or recomputes a field: the caller populates every path field with a usable absolute path.
type Geometry struct {
	// WorktreeRoot is the git worktree's root; every git call opens the repository here.
	WorktreeRoot string
	// PaneCwd is the directory the implementer's sessions run in.
	// A relative recorded path resolves against it, and the run's `_lym` is read under it.
	PaneCwd string
	// PlanDir is the plan directory the run builds.
	PlanDir string
	// RunDir is the implementer's durable run directory, which holds its state file and the plan baseline store.
	RunDir string
	// ReportsDir is the directory holding the implementer's per-batch report files and its verify-failure record.
	ReportsDir string
	// PromptsDir is the directory holding the implementer's rendered batch prompts.
	PromptsDir string
	// ScratchDir is the implementer's never-tracked directory.
	// It holds its locks and its state-mutation lease.
	ScratchDir string
	// Git is the git surface every worktree read goes through; the caller always sets it.
	Git Git
	// Index is the code index the dispatch re-resolution and the post-batch checks call.
	// A call that reaches a nil Index returns an error naming the missing wiring.
	Index planindex.Index
}

// index returns the geometry's code index, or errNoIndex when none was wired.
func (g Geometry) index() (planindex.Index, error) {
	if g.Index == nil {
		return nil, errNoIndex
	}
	return g.Index, nil
}

// Git is every question the shared batch machinery puts to a worktree's git repository.
// Each method takes the worktree to ask about, so one Git serves a run and its linked worktrees alike.
type Git interface {
	// SHAExists reports whether sha names a commit in the worktree's repository.
	SHAExists(worktree, sha string) bool
	// IsAncestor reports whether sha is an ancestor of ref.
	IsAncestor(worktree, sha, ref string) (bool, error)
	// IgnoredPath reports whether git ignores path in the worktree.
	IgnoredPath(worktree, path string) (bool, error)
	// PathDiffers reports whether path differs from base in the worktree: a changed tracked file or an untracked new one.
	PathDiffers(worktree, base, path string) (bool, error)
	// WorktreeBlob returns the blob id of path's content in the worktree, or "" when the file is absent.
	WorktreeBlob(worktree, path string) (string, error)
	// CommitBlob returns the blob id of path at commit, or "" when the commit does not hold the path.
	CommitBlob(worktree, commit, path string) (string, error)
	// TreePathsWithBlob returns, sorted, every path in commit's tree whose object id is blob.
	TreePathsWithBlob(worktree, commit, blob string) ([]string, error)
	// CommitLog returns, oldest first, the non-merge commits on head's first-parent line after base, both full SHAs.
	// A base that is not on that line, or a SHA the repository does not hold, is an error.
	CommitLog(worktree, base, head string) ([]CommitLogEntry, error)
}

// CommitLogEntry is one commit in a commit log, with the departures its message declares.
type CommitLogEntry struct {
	SHA     string
	Subject string
	// Departures holds the value of each `Pattern-Departure:` trailer line of the commit message, in order and verbatim.
	Departures []string
}

// DepartureLines renders each departure as "<subject>: <value>", in order.
func (e CommitLogEntry) DepartureLines() []string {
	lines := make([]string, 0, len(e.Departures))
	for _, departure := range e.Departures {
		lines = append(lines, e.Subject+": "+departure)
	}
	return lines
}
