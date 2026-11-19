// recordsduringmerge.go lands a records commit while a one-repository worktree is in the middle of a merge.
// There the records sit on the code branch, so a merge in progress would otherwise block every records commit.
// The file also names the git operation that a records commit cannot pass.

package fabricengine

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/dotgit"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymtemp"
)

// abortRecordsCommitMessage is the subject of the commit that keeps the uncommitted records through a merge abort.
const abortRecordsCommitMessage = "fabric merge: commit the records before the abort"

// recordsMergeKind classifies the merge state a records commit meets in a one-repository worktree.
type recordsMergeKind int

const (
	// recordsMergeNone means no merge state stands in the way, so the plain records commit applies.
	recordsMergeNone recordsMergeKind = iota
	// recordsMergePlain means a merge is staged whose index holds every records path as HEAD does.
	recordsMergePlain
	// recordsMergeUnsettled means a fabric merge stopped before its records step, so its index holds the source's records.
	recordsMergeUnsettled
	// recordsMergeBlocked means a git operation the records commit cannot pass.
	recordsMergeBlocked
)

// gitOperationMarkers maps each in-progress marker of a git directory to the operation it shows.
var gitOperationMarkers = []struct{ marker, operation string }{
	{"CHERRY_PICK_HEAD", "cherry-pick"},
	{"REVERT_HEAD", "revert"},
	{"rebase-merge", "rebase"},
	{"rebase-apply", "rebase"},
}

// gitOperationAt names the cherry-pick, revert or rebase in progress in the worktree at worktree.
// It returns an empty string when none is.
func gitOperationAt(worktree string) (string, error) {
	geometry, err := dotgit.Read(worktree)
	if err != nil {
		return "", fmt.Errorf("fabricengine: read the git directory of %s: %w", worktree, err)
	}
	for _, probe := range gitOperationMarkers {
		_, err := os.Stat(filepath.Join(geometry.GitDir, probe.marker))
		switch {
		case err == nil:
			return probe.operation, nil
		case !errors.Is(err, fs.ErrNotExist):
			return "", fmt.Errorf("fabricengine: look for %s in %s: %w", probe.marker, geometry.GitDir, err)
		}
	}
	return "", nil
}

// recordsIndexDiffersFromHead reports whether the index of the worktree holds any of paths unlike HEAD does.
// An unmerged entry counts as different.
func recordsIndexDiffersFromHead(worktree string, paths []string) (bool, error) {
	differing, err := gitexec.RunWithEnv(append([]string{"diff-index", "--cached", "--name-only", "-z", "HEAD", "--"}, paths...),
		worktree, mergeLiteralPathspecEnv)
	if err != nil {
		return false, fmt.Errorf("fabricengine: compare the index with HEAD in %s: %w", worktree, err)
	}
	return strings.Trim(differing, "\x00\n ") != "", nil
}

// recordsMergeKindAt classifies the merge state of the one-repository worktree at worktree.
// For recordsMergeBlocked it also returns the operation's name.
func recordsMergeKindAt(worktree string) (kind recordsMergeKind, operation string, err error) {
	operation, err = gitOperationAt(worktree)
	if err != nil {
		return recordsMergeNone, "", err
	}

	repo := gitrepo.New(worktree)
	mergeHead, err := repo.MergeHeadPresent()
	if err != nil {
		return recordsMergeNone, "", fmt.Errorf("fabricengine: check merge head: %w", err)
	}
	conflicted, err := repo.ConflictedFiles()
	if err != nil {
		return recordsMergeNone, "", fmt.Errorf("fabricengine: check conflicted files: %w", err)
	}
	record, err := readMergeRecordAt(worktree)
	if err != nil {
		return recordsMergeNone, "", err
	}

	switch {
	case operation != "":
		return recordsMergeBlocked, operation, nil
	case record != nil && record.RecordsUnsettled:
		return recordsMergeUnsettled, "", nil
	case !mergeHead && len(conflicted) == 0:
		return recordsMergeNone, "", nil
	}

	recordsPaths, err := RecordsPathsAt(worktree, "HEAD")
	if err != nil {
		return recordsMergeNone, "", err
	}
	differs, err := recordsIndexDiffersFromHead(worktree, recordsPaths)
	if err != nil {
		return recordsMergeNone, "", err
	}
	if differs {
		return recordsMergeBlocked, "merge", nil
	}
	return recordsMergePlain, "", nil
}

// readMergeRecordAt reads the fabric merge record of the one-repository worktree at worktree, nil when none stands.
func readMergeRecordAt(worktree string) (*mergeState, error) {
	f, err := newPaired(worktree, worktree)
	if err != nil {
		return nil, err
	}
	return f.loadMergeState()
}

// mergeRouteApplies reports whether a records commit in the worktree at worktree goes through commitRecordsAroundMerge.
// That is a one-repository worktree with merge state present, a refused operation included.
func mergeRouteApplies(worktree string) (bool, error) {
	if !oneRepositoryAt(worktree) {
		return false, nil
	}
	kind, _, err := recordsMergeKindAt(worktree)
	return kind != recordsMergeNone, err
}

// MergeStateOperation names the git operation a records commit in l's worktree cannot pass.
// It answers "merge" for a live MERGE_HEAD or a conflicted index.
// It answers "cherry-pick", "revert" or "rebase" for those operations in progress.
// It answers "" when none blocks.
// In a one-repository worktree a merge blocks only when its index holds a records path unlike HEAD's.
// It also blocks when the index is conflicted on a records path.
// It does not block while a fabric merge record stands with its records unsettled.
// A cherry-pick, revert or rebase blocks there too.
// In a two-repository worktree any merge state of the records worktree blocks.
func MergeStateOperation(l *lymcwd.Location) (string, error) {
	worktree := RecordsWorktree(l)
	if oneRepositoryAt(worktree) {
		kind, operation, err := recordsMergeKindAt(worktree)
		if err != nil || kind != recordsMergeBlocked {
			return "", err
		}
		return operation, nil
	}

	operation, err := gitOperationAt(worktree)
	if err != nil || operation != "" {
		return operation, err
	}
	repo := gitrepo.New(worktree)
	mergeHead, err := repo.MergeHeadPresent()
	if err != nil {
		return "", fmt.Errorf("fabricengine: check merge head: %w", err)
	}
	conflicted, err := repo.ConflictedFiles()
	if err != nil {
		return "", fmt.Errorf("fabricengine: check conflicted files: %w", err)
	}
	if mergeHead || len(conflicted) > 0 {
		return "merge", nil
	}
	return "", nil
}

// refuseMarkerCommitDuringMerge returns an error naming the merge state that bars a marker commit in the worktree.
// It applies to a one-repository worktree with MERGE_HEAD present, a fabric merge record standing,
// or an operation MergeStateOperation names.
// The marker sits at the worktree root, outside the anchored records paths a records commit takes around a merge.
func refuseMarkerCommitDuringMerge(worktree string) error {
	if !oneRepositoryAt(worktree) {
		return nil
	}
	kind, operation, err := recordsMergeKindAt(worktree)
	if err != nil {
		return err
	}
	record, err := readMergeRecordAt(worktree)
	if err != nil {
		return err
	}
	mergeHead, err := gitrepo.New(worktree).MergeHeadPresent()
	if err != nil {
		return fmt.Errorf("fabricengine: check merge head: %w", err)
	}

	state := ""
	switch {
	case kind == recordsMergeBlocked:
		state = operation
	case mergeHead:
		state = "merge"
	case record != nil:
		state = "fabric merge"
	}
	if state == "" {
		return nil
	}
	return fmt.Errorf("fabricengine: a %s is in progress in %s, so the %s marker is not committed; way forward: conclude or abort it, then run the verb again",
		state, worktree, lymcwd.AnchorFileName)
}

// commitRecordsAroundMerge commits the records paths relPaths, scoped under anchorRel, in a worktree that is mid-merge.
// The caller holds the write lock of the worktree.
// The worktree is a one-repository worktree with merge state present.
// It lands the commit without concluding the merge:
// the commit is built in a temporary index and HEAD moves onto it, so concluding the merge carries the records.
// A merge whose index holds every records path as HEAD does takes the worktree's content of relPaths.
// A fabric merge stopped before its records step takes the worktree's content of each records path.
// That holds for each path the merge left as HEAD has it.
// The commit takes HEAD's version of every other.
// It then aborts that merge to the new commit and deletes the merge record.
// A cherry-pick, revert or rebase is refused with the operation named.
// So is a merge fabric did not start whose index differs from HEAD on a records path.
// It returns ("", false, nil) when relPaths change nothing.
func commitRecordsAroundMerge(rec *Mutations, worktree, anchorRel string, relPaths []string, msg string) (sha string, committed bool, err error) {
	return commitRecordsAroundMergeWith(rec, worktree, anchorRel, relPaths, msg, false)
}

// commitRecordsAroundMergeWith is commitRecordsAroundMerge with the choice of landing an empty commit.
// With allowEmpty the commit lands whether relPaths change anything, so a trailer has a commit to sit on.
// An empty commit needs relPaths to match a tracked or present file when it names any.
func commitRecordsAroundMergeWith(rec *Mutations, worktree, anchorRel string, relPaths []string, msg string, allowEmpty bool) (sha string, committed bool, err error) {
	kind, operation, err := recordsMergeKindAt(worktree)
	if err != nil {
		return "", false, err
	}
	switch kind {
	case recordsMergeBlocked:
		return "", false, fmt.Errorf("fabricengine: a git %s is in progress in %s, so its records cannot be committed; way forward: conclude or abort it with plain git, then retry",
			operation, worktree)
	case recordsMergeNone:
		return "", false, fmt.Errorf("fabricengine: no merge is in progress in %s, so there is nothing to commit the records around", worktree)
	}

	pathspec := weftCommitPathspec(anchorRel, relPaths)
	filtered, positive, err := weftPathspecFilter(worktree, pathspec)
	if err != nil {
		return "", false, err
	}
	if !positive {
		if !allowEmpty {
			return "", false, nil
		}
		if len(relPaths) > 0 {
			return "", false, fmt.Errorf("fabricengine: %s match no tracked or present file in %s", strings.Join(pathspec, ", "), worktree)
		}
	}

	head, err := gitrepo.New(worktree).CurrentSHA()
	if err != nil {
		return "", false, err
	}
	scratch, err := lymtemp.MkdirTemp("records-around-merge-*")
	if err != nil {
		return "", false, fmt.Errorf("fabricengine: create the temporary index in %s: %w", worktree, err)
	}
	defer func() { _ = os.RemoveAll(scratch) }()
	tempEnv := indexEnv(filepath.Join(scratch, "index"))

	tree, err := buildRecordsTree(worktree, head, tempEnv, filtered, kind == recordsMergeUnsettled)
	if err != nil {
		return "", false, err
	}
	headTree, err := gitexec.Run([]string{"rev-parse", head + "^{tree}"}, worktree)
	if err != nil {
		return "", false, fmt.Errorf("fabricengine: read the tree of HEAD in %s: %w", worktree, err)
	}
	if tree == strings.TrimSpace(headTree) && !allowEmpty {
		return "", false, nil
	}

	created, err := gitexec.Run([]string{"-c", "commit.gpgsign=false", "commit-tree", tree, "-p", head, "-m", msg}, worktree)
	if err != nil {
		return "", false, fmt.Errorf("fabricengine: write the records commit in %s: %w", worktree, err)
	}
	sha = strings.TrimSpace(created)
	err = retryHeldIndexLock(indexLockRetryBound, time.Sleep, func() error {
		_, updateErr := gitexec.Run([]string{"update-ref", "-m", "records commit during a merge", "HEAD", sha, head}, worktree)
		return updateErr
	})
	if err != nil {
		return "", false, fmt.Errorf("fabricengine: move HEAD onto the records commit in %s: %w", worktree, err)
	}

	f, err := newPaired(worktree, worktree)
	if err != nil {
		return undoRecordsCommit(worktree, head, sha, err)
	}
	st, err := f.loadMergeState()
	if err != nil {
		return undoRecordsCommit(worktree, head, sha, err)
	}
	if st != nil {
		st.WarpStart, st.WeftStart = sha, sha
		if err := f.saveMergeState(st); err != nil {
			return undoRecordsCommit(worktree, head, sha, err)
		}
	}

	if kind == recordsMergeUnsettled {
		if err := f.resetMergeSides(rec, sha); err != nil {
			return sha, true, err
		}
		return sha, true, f.deleteMergeState()
	}
	if len(filtered) > 0 {
		err = retryHeldIndexLock(indexLockRetryBound, time.Sleep, func() error {
			_, addErr := gitexec.RunWithEnv(append([]string{"add", "-A", "--"}, filtered...), worktree, mergeLiteralPathspecEnv)
			return addErr
		})
		if err != nil {
			return sha, true, fmt.Errorf("fabricengine: stage the records in %s: %w", worktree, err)
		}
	}
	return sha, true, nil
}

// undoRecordsCommit moves HEAD of worktree from the records commit sha back to head.
// It runs after cause kept the merge record from following the commit.
// A merge record still naming head would otherwise make a later abort reset to head and drop the commit.
// It returns ("", false, cause) once HEAD is back.
// When HEAD cannot be moved back, it reports the commit as made, beside both errors.
func undoRecordsCommit(worktree, head, sha string, cause error) (string, bool, error) {
	_, err := gitexec.Run([]string{"update-ref", "-m", "undo the records commit during a merge", "HEAD", head, sha}, worktree)
	if err != nil {
		return sha, true, errors.Join(cause, fmt.Errorf("fabricengine: move HEAD back off the records commit in %s: %w", worktree, err))
	}
	return "", false, cause
}

// buildRecordsTree writes the tree of HEAD with the worktree's content of paths laid over it, and returns its id.
// The tree is built in the throwaway index tempEnv names, so the real index and the worktree stay as they are.
// With keepHeadOfMerged set, a path whose index entry differs from HEAD's keeps HEAD's version:
// the merge staged the source's records there.
func buildRecordsTree(worktree, head string, tempEnv, paths []string, keepHeadOfMerged bool) (string, error) {
	if _, err := gitexec.RunWithEnv([]string{"read-tree", head}, worktree, tempEnv); err != nil {
		return "", fmt.Errorf("fabricengine: read the tree of HEAD into the temporary index in %s: %w", worktree, err)
	}
	if len(paths) > 0 {
		if _, err := gitexec.RunWithEnv(append([]string{"add", "-A", "--"}, paths...), worktree, tempEnv); err != nil {
			return "", fmt.Errorf("fabricengine: take the records of %s into the temporary index: %w", worktree, err)
		}
	}
	if keepHeadOfMerged {
		if err := restoreHeadVersionOfMergedRecords(worktree, head, tempEnv, paths); err != nil {
			return "", err
		}
	}
	tree, err := gitexec.RunWithEnv([]string{"write-tree"}, worktree, tempEnv)
	if err != nil {
		return "", fmt.Errorf("fabricengine: write the records tree in %s: %w", worktree, err)
	}
	return strings.TrimSpace(tree), nil
}

// restoreHeadVersionOfMergedRecords sets every file under paths back to HEAD's version.
// It works in the throwaway index tempEnv names.
// It touches each file whose entry in the real index differs from HEAD's.
// A file HEAD does not hold is removed.
func restoreHeadVersionOfMergedRecords(worktree, head string, tempEnv, paths []string) error {
	recordsPaths, err := RecordsPathsAt(worktree, "HEAD")
	if err != nil {
		return err
	}
	differing, err := gitexec.RunWithEnv(append([]string{"diff-index", "--cached", "--name-only", "-z", "HEAD", "--"}, recordsPaths...),
		worktree, mergeLiteralPathspecEnv)
	if err != nil {
		return fmt.Errorf("fabricengine: list the records the merge staged in %s: %w", worktree, err)
	}

	slashed := make([]string, len(paths))
	for i, path := range paths {
		slashed[i] = filepath.ToSlash(path)
	}
	for _, path := range strings.Split(differing, "\x00") {
		if path == "" || !underRecordsPath(path, slashed) {
			continue
		}
		listed, err := gitexec.RunWithEnv([]string{"ls-tree", head, "--", path}, worktree, tempEnv)
		if err != nil {
			return fmt.Errorf("fabricengine: list %s at HEAD in %s: %w", path, worktree, err)
		}
		entry, _, _ := strings.Cut(strings.TrimSpace(listed), "\t")
		args := []string{"rm", "--cached", "-f", "-q", "--ignore-unmatch", "--", path}
		if fields := strings.Fields(entry); len(fields) == 3 {
			args = []string{"update-index", "--add", "--cacheinfo", fields[0] + "," + fields[2] + "," + path}
		}
		if _, err := gitexec.RunWithEnv(args, worktree, tempEnv); err != nil {
			return fmt.Errorf("fabricengine: set %s back to HEAD's version in the temporary index of %s: %w", path, worktree, err)
		}
	}
	return nil
}

// commitWeftAroundMerge is commitWeftLocked's body for a one-repository worktree that is mid-merge.
// The write lock is held.
// With forceEmpty an empty commit lands when nothing in pathspec changed, so the trailers have a commit to sit on.
func (f *Fabric) commitWeftAroundMerge(pathspec []string, message, warpSHA string, forceEmpty bool) (sha string, committed bool, err error) {
	filtered, positive, err := weftPathspecFilter(f.weftPath, pathspec)
	if err != nil {
		return "", false, err
	}
	relPaths := filtered
	if !positive {
		if !forceEmpty {
			return "", false, nil
		}
		relPaths = []string{lymcwd.AnchorFileName}
	}

	rec := NewMutations(filepath.Dir(f.warpPath))
	sha, committed, err = commitRecordsAroundMergeWith(rec, f.weftPath, ".", relPaths, message, forceEmpty)
	if err != nil || !committed {
		return "", false, err
	}
	if err := f.RecordCorrespondence(warpSHA, sha); err != nil {
		return sha, true, err
	}
	return sha, true, nil
}

// commitRecordsBeforeAbort commits the uncommitted records of f's one-repository worktree ahead of a merge abort.
// It reports aborted true when that commit also ended the merge, which an unsettled fabric merge does.
// The caller holds the write lock.
func (f *Fabric) commitRecordsBeforeAbort(rec *Mutations) (aborted bool, err error) {
	routed, err := mergeRouteApplies(f.warpPath)
	if err != nil || !routed {
		return false, err
	}
	recordsPaths, err := RecordsPathsAt(f.warpPath, "HEAD")
	if err != nil {
		return false, err
	}
	sha, committed, err := commitRecordsAroundMerge(rec, f.warpPath, ".", recordsPaths, abortRecordsCommitMessage)
	if err != nil {
		return false, err
	}
	if committed {
		rec.Append(KindCommitCreated, f.warpPath, sha)
	}
	exists, err := f.mergeRecordExists()
	return !exists, err
}
