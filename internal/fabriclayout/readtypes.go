// readtypes.go holds the plain result types of the read verbs, their constants and pure methods.

package fabriclayout

import "time"

// CheckoutState is the state of one checkout of a pair.
// Every path list is slash-separated, relative to Root, sorted and never nil.
type CheckoutState struct {
	// Root is the checkout's absolute directory.
	Root string
	// Head is the checkout's HEAD commit, empty when it has no commit yet.
	Head string
	// Changed holds the tracked paths that differ from HEAD, staged, unstaged or deleted.
	Changed []string
	// Untracked holds every untracked path outside the ignore rules and `.git/info/exclude`.
	Untracked []string
}

// CodeWorktree names one code worktree of a hub.
type CodeWorktree struct {
	// Path is the worktree root, cleaned and in native path separators.
	Path string
	// Anchor is the worktree's anchor dir, where its _lym/config lives.
	Anchor string
	// Main is true for the hub's main worktree, the prime.
	Main bool
}

// WorktreeEntry represents a single worktree of a repository, as `git worktree list` reports it.
type WorktreeEntry struct {
	Path     string `json:"path"`
	Head     string `json:"head"`
	Branch   string `json:"branch"`
	Main     bool   `json:"main"`
	Prunable bool   `json:"prunable"`
}

// WorktreeEvent is what the worktree seam is told about one worktree.
// Root is the worktree's root.
// Anchor is its anchor directory, where the repository's own files sit; it equals Root in a worktree with no anchor.
// ScratchDir is the pair's never-tracked `.lym` directory.
// BoltDir is the hub's Bolt checkout.
// Excluded holds the anchor-relative paths the warp exclude list names;
// they are already seeded into the repository's exclude file.
// Paths holds the root-relative paths that differ between the old and the new head, empty for "made".
type WorktreeEvent struct {
	Root       string
	Anchor     string
	ScratchDir string
	BoltDir    string
	Excluded   []string
	Paths      []string
}

// AnchoredFileVersion is one committed version of an anchored file on the records branch's first-parent line.
type AnchoredFileVersion struct {
	// SHA is the full object name of the commit that changed the file.
	SHA string
	// Position is that commit's first-parent position on the records branch:
	// the number of first-parent steps from the branch's root commit, so the root is 0 and each later commit is one more.
	Position int
	// Data is the file's contents at that commit.
	Data []byte
}

// ConfigChanges is the set of per-worktree config files a task changed on its weft branch.
// A change counts only when the task's copy differs from both its fork point and its parent's current copy.
type ConfigChanges struct {
	// Files are the changed files, repository-relative and slash-separated, sorted.
	Files []string
	// Base is the merge-base SHA of the task's and the parent's weft branches, the task's fork point.
	Base string
	// Tip is the SHA of the task's weft branch tip.
	Tip string
	// ParentTip is the SHA of the parent's weft branch tip.
	ParentTip string
	// Contents holds, per file in Files, the file's bytes at each of the three revisions.
	Contents map[string]ConfigFileContents
}

// ConfigFileContents is one config file's bytes at the task's fork point, the task's tip and the parent's tip;
// a file absent at a revision is nil there.
type ConfigFileContents struct {
	Base, Tip, Parent []byte
}

// ChangeSide names which repo of a warp<->weft pair a changed path belongs to.
type ChangeSide string

// The two sides of a warp<->weft pair that a changed path can be labelled with.
const (
	SideWarp ChangeSide = "warp"
	SideWeft ChangeSide = "weft"
)

// ChangeEntry is one repo-relative path that changed on one side of a warp<->weft pair.
// Its Side field names that side.
type ChangeEntry struct {
	Path string
	Side ChangeSide
}

// DiffResult is the merged, side-labelled set of paths that changed across a warp<->weft pair since a warp SHA.
// NoWeftCorrespondence reports whether that warp SHA predates any recorded weft correspondence;
// in that case Entries holds only warp-side changes.
type DiffResult struct {
	Entries              []ChangeEntry
	NoWeftCorrespondence bool
}

// HealthCause names the condition that made a health check fail,
// so a caller can classify the failure without parsing a display string.
type HealthCause string

// The closed set of causes a failed health check can report, one per drift shape.
const (
	// CauseBranchMismatch reports that the weft worktree is not on the paired weft branch.
	CauseBranchMismatch HealthCause = "branch-mismatch"
	// CauseConfigLoadFailed reports that the wired name-set could not be loaded from fabric.yaml.
	CauseConfigLoadFailed HealthCause = "config-load-failed"
	// CauseJunctionMissing reports that a warp junction entry does not exist on disk.
	CauseJunctionMissing HealthCause = "junction-missing"
	// CauseNotAJunction reports that a warp junction entry exists but is not a link.
	CauseNotAJunction HealthCause = "not-a-junction"
	// CauseJunctionPointsElsewhere reports that a warp junction link resolves to the wrong target.
	CauseJunctionPointsElsewhere HealthCause = "junction-points-elsewhere"
)

// HealthReason describes why a health check failed.
// Cause is the drift shape a caller switches on.
// Detail is a display string, worded for fabric, that a caller may print verbatim.
// The zero value carries no reason, as it does for a passing check or an error.
type HealthReason struct {
	Cause  HealthCause
	Detail string
}

// LineChange is the tree difference between two commits of a pair's code repository.
type LineChange struct {
	// Paths are the sorted changed paths, a rename counted at both its old and its new path.
	Paths []string
	// Lines is the added plus the deleted lines.
	// A pure rename adds none, and a binary file counts as one line, so it is never free.
	Lines int
	// AddedLines is the text of every added line, without its leading plus.
	AddedLines []string
}

// DiffSize is the size of the tree difference between two commits of a pair's code repository.
type DiffSize struct {
	// LinesAdded and LinesDeleted are the added and deleted lines, a pure rename adding and deleting none.
	LinesAdded   int
	LinesDeleted int
	// FilesChanged counts each changed file once, renamed and binary files included; binary files add and delete no lines.
	FilesChanged int
}

// FileDiff is one file's committed change: its repository-relative, slash-separated Path and its own Diff text.
type FileDiff struct {
	Path string
	Diff string
}

// TaskPatch is the warp working tree's change against a merge base.
// Head is the commit HEAD stands at, Full the full diff bytes in a stable order, and FullHash the hex SHA-256 of Full.
// PatchID is git's stable patch-id of the zero-context form of the same diff.
// It survives a rebase that moves the context around the change, and it is empty when the diff has no change.
type TaskPatch struct {
	Head     string
	Full     []byte
	FullHash string
	PatchID  string
}

// MoveRecord is the record of an anchor change under way.
// It is written before the weft is touched and removed once the change is finished or undone.
// A is the anchor the weft is moving from and B the one it is moving to.
// Kind is MoveKindMove or MoveKindIntegrate.
// U is the upstream tip an integration rebuilds on, empty until that step is reached.
// PID and StartTime identify the process that holds the move lock,
// so a reader can tell a live change from an interrupted one.
type MoveRecord struct {
	A         string `json:"a"`
	B         string `json:"b"`
	Kind      string `json:"kind"`
	U         string `json:"u,omitempty"`
	PID       int    `json:"pid"`
	StartTime string `json:"start_time,omitempty"`
}

// Origin is fabric's provenance record for one worktree pair.
// It is written once at pair creation and read thereafter, never inferred.
// It names the branch and the worktree the pair was created from.
type Origin struct {
	// ParentBranch is the warp branch the pair was forked from.
	ParentBranch string `json:"parent_branch"`
	// ParentWorktree is the WorktreeName of the worktree the pair was added from.
	// A run's parent agent name is resolved from it.
	// A record that lacks the field, such as a legacy-worktree repair whose creator is unknown, leaves it empty.
	ParentWorktree string `json:"parent_worktree,omitempty"`
	// Beam marks the pair as a beam.
	// A record that lacks the field decodes with it false.
	Beam bool `json:"beam,omitempty"`
}

// PairParts reports which parts of a pair are present, read locally with no network call.
type PairParts struct {
	// TaskWorktree reports a registered linked worktree at the pair's location.
	TaskWorktree bool
	// RecordsWorktree reports a directory at the pair's records worktree path.
	RecordsWorktree bool
	// RecordsBranch reports the local sibling branch.
	RecordsBranch bool
	// TaskBranch reports the local task branch.
	TaskBranch bool
	// StrayPath reports an unregistered path at the pair's task worktree location, such as one a late writer refilled.
	StrayPath bool
}

// Any reports whether any part of the pair is present.
func (p PairParts) Any() bool {
	return p.TaskWorktree || p.RecordsWorktree || p.RecordsBranch || p.TaskBranch || p.StrayPath
}

// RecordsTag is one tag of a slug's records history:
// an archive tag a teardown pushed, or a set-aside tag a local removal kept.
type RecordsTag struct {
	// Name is the tag's short name.
	Name string
	// Commit is the full hex object name of the commit the tag names, an annotated tag peeled.
	Commit string
	// OnOrigin is true when origin holds the tag.
	OnOrigin bool
	// OriginOnly is true when only origin holds the tag.
	OriginOnly bool
	// CommitterTime is the committer time of Commit.
	CommitterTime time.Time
	// Files holds the content of each requested path found at Commit, keyed by the requested path;
	// a path absent there is left out.
	Files map[string][]byte
}

// Trailer is one git trailer a commit message ends with.
type Trailer struct {
	// Key is the trailer's key, matching [A-Za-z][A-Za-z0-9-]*.
	Key string
	// Value is the trailer's value: non-empty and on one line.
	Value string
}

// TrailerCommit is a weft commit that carries a trailer key.
type TrailerCommit struct {
	// SHA is the weft commit's full object name.
	SHA string
	// Value is the last value of the searched key in the commit's message.
	Value string
	// CodeHead is the commit's Warp-SHA value, the code checkout's HEAD the commit was made at, empty when it carries none.
	CodeHead string
	// Position is the commit's first-parent position on the records branch:
	// the number of first-parent steps from the branch's root commit, so the root is 0 and each later commit is one more.
	// Positions from different reads of the same branch compare as long as no history is rewritten.
	Position int
}

// PollutionEntry describes a path tracked in the warp index that belongs in the weft, such as one under _lym.
type PollutionEntry struct {
	// Path is the relative path reported by git ls-files.
	Path string `json:"path"`
	// Remedy is the command that untracks the path and keeps it on disk. Empty when no remedy applies.
	Remedy string `json:"remedy,omitempty"`
}
