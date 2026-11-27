// refnames.go holds fabric's branch, tag, ref-namespace, trailer-key, flag and commit-subject names,
// and the pure functions that compose and parse them.

package fabriclayout

import (
	"strings"

	"github.com/Knatte18/loomyard/internal/weftname"
)

// WeftWarpSlug parses a weft sibling directory name and returns the warp slug it corresponds to.
// It reports whether name ends with weftname.Suffix and the stripped prefix is non-empty.
func WeftWarpSlug(name string) (slug string, ok bool) {
	if !strings.HasSuffix(name, weftname.Suffix) {
		return "", false
	}
	s := strings.TrimSuffix(name, weftname.Suffix)
	if s == "" {
		return "", false
	}
	return s, true
}

// ArchiveTagSHALen is how many leading hex digits of the tip SHA the archive tag name carries.
const ArchiveTagSHALen = 12

// ArchiveTagPrefix is the short-name prefix every archive tag carries.
const ArchiveTagPrefix = "archive/"

// ArchiveTagSlug returns the slug of the short tag name archive/<slug>/<sha>.
// The bool is false unless name has the archive prefix, a non-empty slug segment, and a non-empty final segment.
func ArchiveTagSlug(name string) (string, bool) {
	rest, ok := strings.CutPrefix(name, ArchiveTagPrefix)
	if !ok {
		return "", false
	}
	slug, tip, ok := strings.Cut(rest, "/")
	if !ok || slug == "" || tip == "" {
		return "", false
	}
	return slug, true
}

// ArchiveTagName returns the archive tag name of a tip: archive/<slug>/<first 12 hex of tip>.
// The tip must hold at least ArchiveTagSHALen characters.
func ArchiveTagName(slug, tip string) string {
	return ArchiveTagPrefix + slug + "/" + tip[:ArchiveTagSHALen]
}

// OwnerArchiveTagPrefix is the short-name prefix every owner archive tag carries.
// No reader of ArchiveTagPrefix tags matches it,
// so the owner archive of a live run or the prime never reads as an archived run.
const OwnerArchiveTagPrefix = "archive-owner/"

// OwnerArchiveTagName returns the owner archive tag name of a tip: archive-owner/<slug>/<first 12 hex of tip>.
// The tip must hold at least ArchiveTagSHALen characters.
func OwnerArchiveTagName(slug, tip string) string {
	return OwnerArchiveTagPrefix + slug + "/" + tip[:ArchiveTagSHALen]
}

// The repository sides an owner archive tag sits in.
const (
	OwnerArchiveSideCode    = "code"
	OwnerArchiveSideRecords = "records"
)

// The one-repository layout keeps a hub's records store under origin's refs/lym/ namespace.
const (
	// RecordsNamespaceRoot is the origin ref prefix that holds the records store's branches and tags.
	RecordsNamespaceRoot = "refs/lym/"
	// RecordsNamespaceHeads is the part of the namespace under RecordsNamespaceRoot that holds branches.
	RecordsNamespaceHeads = "heads/"
	// RecordsNamespaceTags is the part of the namespace under RecordsNamespaceRoot that holds tags.
	RecordsNamespaceTags = "tags/"
	// RecordsNamespaceCodeTags is the part under RecordsNamespaceRoot that holds the code store's archive tags.
	// No fetch refspec of either store maps it, so the records store never lists or fetches a code commit through it.
	RecordsNamespaceCodeTags = "code-tags/"
	// OneRepositoryBoardBranch is the fixed branch of the board in a one-repository hub.
	// It is a branch lym owns inside its records namespace, never read from git.
	OneRepositoryBoardBranch = "main"
	// PrimeRecordsBranchKey is the records store's git config key that names the prime's records branch.
	PrimeRecordsBranchKey = "lym.primeRecordsBranch"
)

// WarpBindingFileName is the filename of the recorded warp-URL binding at the board root (<boardDir>/.lym-warp).
// It holds only the warp URL string, plus a trailing newline.
// This is a structural per-repo record, written once at clone and read on every subsequent clone or reconcile.
// It is never a config or env override.
// The anchor file says where in warp lym is rooted; this file says which warp repo the weft pairs with.
// Exported so other packages and tests can name the file.
const WarpBindingFileName = ".lym-warp"

// ShortnameFileName is the filename of the recorded repo shortname at the board root (<boardDir>/.lym-shortname).
// It holds only the shortname, plus a trailing newline.
const ShortnameFileName = ".lym-shortname"

// WarpSHATrailerKey is the git-trailer key fabric writes into every weft commit message as "Warp-SHA: <sha>".
const WarpSHATrailerKey = "Warp-SHA"

// TransitionTrailerKey is the git-trailer key of a records commit marking a run's transition.
// It is written as "Run-Transition: <run id>".
// Only a run's transition commits, its code-tip give-up and webster's reset of the code branch write it.
// The newest commit carrying it thus names the run's last transition.
// Fabric's other records commits carry a Warp-SHA trailer too, but never this one.
const TransitionTrailerKey = "Run-Transition"

// SnapshotTrailerKey is the git-trailer key fabric writes into a weft commit message.
// It is written once per caller-supplied snapshot tag, alongside the Warp-SHA trailer.
const SnapshotTrailerKey = "Snapshot"

// originRecordSubjectPrefix is the commit subject prefix of the commit that records a new pair's origin.
const originRecordSubjectPrefix = "fabric: record parent branch for "

// OriginRecordSubject returns the commit subject of the origin-record commit that opens the pair for slug.
// That commit is the oldest commit of the pair's own records history: everything below it is the parent's.
func OriginRecordSubject(slug string) string {
	return originRecordSubjectPrefix + slug
}

// OriginRecordSlug returns the slug an origin-record commit subject names.
// The second result is false for any other subject and for an empty slug.
func OriginRecordSlug(subject string) (string, bool) {
	slug, ok := strings.CutPrefix(subject, originRecordSubjectPrefix)
	if !ok || slug == "" {
		return "", false
	}
	return slug, true
}

// DefaultCommitMessage is the commit message for a weft commit that does not need a custom one.
const DefaultCommitMessage = "weft sync"

const (
	// PauseSlugFlag is the hidden flag naming the slug a push child's network calls are held under; empty for the prime.
	PauseSlugFlag = "pause-slug"
	// TeardownSlugFlag is the hidden fabric flag that tells a detached push child the pair whose teardown it yields to.
	TeardownSlugFlag = "teardown-slug"
	// ParentLockedFlag is the hidden flag marking a child spawned under a held parent lock; it never exits for coverage.
	ParentLockedFlag = "parent-locked"
)

// The hidden fabric verb and flags that start the teardown child.
const (
	// TeardownChildVerb is the hidden fabric subcommand that runs the remote half of a teardown.
	TeardownChildVerb = "remote-teardown"
	// TeardownChildSlugFlag names the slug whose teardown lock the child holds.
	TeardownChildSlugFlag = "slug"
	// TeardownChildTryFlag makes the child take the lock by one attempt instead of waiting for it.
	TeardownChildTryFlag = "try"
	// TeardownChildWaitFlag bounds the child's wait for the lock.
	TeardownChildWaitFlag = "wait"
)

// The verbs a fabric merge-state record carries.
const (
	// MergeVerbMergeIn is the verb of a record written by a merge-in of a source branch into the current pair.
	MergeVerbMergeIn = "merge-in"
	// MergeVerbMerge is the verb of a record written by a merge of a source branch into a target pair.
	MergeVerbMerge = "merge"
)

// The kinds of anchor change a move record stands for.
const (
	// MoveKindMove re-roots the weft's content at the new anchor.
	MoveKindMove = "move"
	// MoveKindIntegrate re-roots the weft's content and carries its local commits across an upstream anchor change.
	MoveKindIntegrate = "integrate"
)

// FabricReaderVerbs returns the names of the `lym fabric` verbs that only read:
// list, pairs, status, diff, shortname, prune and cleanup.
// It declares names only; the arguments each verb may take without mutating stay with the consumer that applies them.
// Every call returns a fresh slice, so no caller can change another's set.
func FabricReaderVerbs() []string {
	return []string{"list", "pairs", "status", "diff", "shortname", "prune", "cleanup"}
}

// SeedSubjectPrefix opens the commit subject of a seeding commit.
const SeedSubjectPrefix = "lym: seed "

// syncSubjectPrefix opens the commit subject of an operator-requested sync commit.
const syncSubjectPrefix = "lym: sync "

// SeedCommitMessage returns the subject of a seeding commit for a subtree (`stencils` or `specs`).
// label is the parenthesised binary label.
func SeedCommitMessage(subtree, label string) string {
	return SeedSubjectPrefix + subtree + " " + label
}

// SyncCommitMessage returns the subject of a commit that an operator-requested `lym stencil sync` makes for a subtree.
// label is the parenthesised binary label.
func SyncCommitMessage(subtree, label string) string {
	return syncSubjectPrefix + subtree + " " + label
}
