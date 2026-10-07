// suspect.go checks suspect paths against what the run recorded, the evidence accept-audit, run --fresh and recover-batch share.
// A tracked file in the task worktree is checked against a commit its caller chooses:
// the last batch head for accept-audit, the run's start commit for run --fresh (both picked by git ancestry, see runEvidenceBases), and the recovery report's head for recover-batch.
// A plan file is checked against the run's plan hashes.
// Every other path has nothing recorded to compare with, so it is reported unverifiable rather than guessed at.
// suspectBlobs and checkRecoveredSuspects add the recovery half: the flagged blob a failed batch records, and the check that a recovery left none of it behind.

package websterengine

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/Knatte18/loomyard/internal/lyxdirs"
)

// evidenceBases are the two commits the suspect-path evidence is checked against, picked by git ancestry rather than batch number.
type evidenceBases struct {
	// Start is the run's start commit: the recorded batch StartSHA that is an ancestor of every other.
	Start string
	// Last is the last batch head: the recorded terminal Digest.HeadSHA every other terminal head is an ancestor of.
	Last string
	// Missing lists every recorded StartSHA and terminal HeadSHA absent from the repository, sorted and deduplicated.
	Missing []string
	// Starts lists every recorded StartSHA, sorted and deduplicated, so a caller can tell an empty Start with nothing recorded from one where no candidate qualifies.
	Starts []string
}

// runEvidenceBases picks st's start commit and last batch head by git ancestry, the integration key excluded.
// A batch number names a batch and does not date it, so the pick follows ancestry.
// A pick is "" when nothing is recorded, no candidate qualifies, or any recorded commit of its kind is missing from the repository:
// a missing commit blanks the pick rather than being skipped, since the pick among the rest would name an older commit.
// A state recording no SHA returns the zero value without running git.
// The error is an IsAncestor failure.
func runEvidenceBases(geom Geometry, st *State) (evidenceBases, error) {
	git, worktree := geom.git(), geom.WorktreeRoot
	var starts, heads []string
	for _, bs := range st.Batches {
		if bs == nil {
			continue
		}
		if bs.StartSHA != "" && !slices.Contains(starts, bs.StartSHA) {
			starts = append(starts, bs.StartSHA)
		}
		if bs.Terminal && bs.Digest != nil && bs.Digest.HeadSHA != "" && !slices.Contains(heads, bs.Digest.HeadSHA) {
			heads = append(heads, bs.Digest.HeadSHA)
		}
	}
	var out evidenceBases
	if len(starts) == 0 && len(heads) == 0 {
		return out, nil
	}
	out.Starts = slices.Sorted(slices.Values(starts))
	startsMissing, headsMissing := false, false
	for _, sha := range starts {
		if !git.SHAExists(worktree, sha) {
			startsMissing = true
			out.Missing = append(out.Missing, sha)
		}
	}
	for _, sha := range heads {
		if !git.SHAExists(worktree, sha) {
			headsMissing = true
			if !slices.Contains(out.Missing, sha) {
				out.Missing = append(out.Missing, sha)
			}
		}
	}
	sort.Strings(out.Missing)
	var err error
	if !startsMissing {
		if out.Start, err = pickByAncestry(git, worktree, starts, true); err != nil {
			return evidenceBases{}, err
		}
	}
	if !headsMissing {
		if out.Last, err = pickByAncestry(git, worktree, heads, false); err != nil {
			return evidenceBases{}, err
		}
	}
	return out, nil
}

// pickByAncestry returns the commit of shas that is an ancestor of every other (oldest) or that every other is an ancestor of (!oldest), or "" when none qualifies.
func pickByAncestry(git Git, worktree string, shas []string, oldest bool) (string, error) {
	for _, cand := range shas {
		qualifies := true
		for _, other := range shas {
			if other == cand {
				continue
			}
			sha, ref := cand, other
			if !oldest {
				sha, ref = other, cand
			}
			ok, err := git.IsAncestor(worktree, sha, ref)
			if err != nil {
				return "", err
			}
			if !ok {
				qualifies = false
				break
			}
		}
		if qualifies {
			return cand, nil
		}
	}
	return "", nil
}

// missingCommitsClause names the recorded commits absent from the repository and the way forward, or "" for an empty list.
func missingCommitsClause(missing []string) string {
	if len(missing) == 0 {
		return ""
	}
	return fmt.Sprintf("commit(s) %s recorded by this run are not in this repository; way forward: fetch the task branch from the machine that ran those batches with git, then re-run the verb", strings.Join(missing, ", "))
}

// planPathClause is the way forward for plan paths that differ from the plan the run recorded, ending in the verb to re-run as next.
func planPathClause(next string) string {
	return fmt.Sprintf("restore them with \"lyx webster restore-plan\", or accept an edit to a card no batch has begun with \"lyx webster rebaseline --card NN\"; then re-run %s", next)
}

// splitPlanPaths sorts paths into those under geom.PlanDir and the rest, each in input order.
// The error is a link-resolution failure.
func splitPlanPaths(geom Geometry, paths []string) (plan, rest []string, err error) {
	planDir, err := canonicalPath(geom.PlanDir)
	if err != nil {
		return nil, nil, err
	}
	for _, p := range paths {
		canon, err := canonicalPath(resolveWritePath(geom.WorktreeRoot, p))
		if err != nil {
			return nil, nil, err
		}
		if pathWithin(planDir, canon) {
			plan = append(plan, p)
		} else {
			rest = append(rest, p)
		}
	}
	return plan, rest, nil
}

// planFileName returns the slash-separated name of the plan path p relative to geom.PlanDir, the key State.PlanFileHashes uses.
func planFileName(geom Geometry, p string) (string, error) {
	planDir, err := canonicalPath(geom.PlanDir)
	if err != nil {
		return "", err
	}
	canon, err := canonicalPath(resolveWritePath(geom.WorktreeRoot, p))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(planDir, canon)
	if err != nil {
		return "", fmt.Errorf("websterengine: relate %s to %s: %w", canon, planDir, err)
	}
	return filepath.ToSlash(rel), nil
}

// checkSuspectPaths sorts each of paths into differing or unverifiable, both returned sorted.
// A plan file differs when its hash is not the one st.PlanFileHashes recorded, and is unverifiable when no hashes were recorded.
// A tracked-or-new file in the task worktree outside its _lyx, geom.ScratchDir and git-ignored paths differs when it changed against base, and is unverifiable when base is empty or not in the repository.
// Any other path is unverifiable.
// The error is a git probe's or a read's failure other than not-exist;
// the check changes nothing.
func checkSuspectPaths(geom Geometry, st *State, base string, paths []string) (differing, unverifiable []string, err error) {
	worktree, err := canonicalPath(geom.WorktreeRoot)
	if err != nil {
		return nil, nil, err
	}
	planDir, err := canonicalPath(geom.PlanDir)
	if err != nil {
		return nil, nil, err
	}
	scratch, err := canonicalPath(geom.ScratchDir)
	if err != nil {
		return nil, nil, err
	}
	lyx, err := canonicalPath(filepath.Join(geom.WorktreeRoot, lyxdirs.LyxDirName))
	if err != nil {
		return nil, nil, err
	}
	git := geom.git()
	if base != "" && !git.SHAExists(geom.WorktreeRoot, base) {
		base = ""
	}
	for _, p := range paths {
		lexical := resolveWritePath(geom.WorktreeRoot, p)
		canon, err := canonicalPath(lexical)
		if err != nil {
			return nil, nil, err
		}
		switch {
		case pathWithin(planDir, canon):
			differs, ok, err := planFileDiffers(st, planDir, canon)
			if err != nil {
				return nil, nil, err
			}
			if !ok {
				unverifiable = append(unverifiable, p)
			} else if differs {
				differing = append(differing, p)
			}
		case !pathWithin(worktree, canon),
			pathWithin(lyx, canon), pathWithin(filepath.Join(geom.WorktreeRoot, lyxdirs.LyxDirName), lexical),
			pathWithin(scratch, canon):
			unverifiable = append(unverifiable, p)
		default:
			ignored, err := git.IgnoredPath(geom.WorktreeRoot, canon)
			if err != nil {
				return nil, nil, err
			}
			if ignored || base == "" {
				unverifiable = append(unverifiable, p)
				continue
			}
			differs, err := git.PathDiffers(geom.WorktreeRoot, base, canon)
			if err != nil {
				return nil, nil, err
			}
			if differs {
				differing = append(differing, p)
			}
		}
	}
	sort.Strings(differing)
	sort.Strings(unverifiable)
	return differing, unverifiable, nil
}

// planFileDiffers compares the plan file at canon against st.PlanFileHashes.
// ok is false when no hashes were recorded.
// An absent file matches only an absent entry.
func planFileDiffers(st *State, planDir, canon string) (differs, ok bool, err error) {
	if len(st.PlanFileHashes) == 0 {
		return false, false, nil
	}
	rel, err := filepath.Rel(planDir, canon)
	if err != nil {
		return false, false, fmt.Errorf("websterengine: relate %s to %s: %w", canon, planDir, err)
	}
	want, recorded := st.PlanFileHashes[filepath.ToSlash(rel)]
	data, err := os.ReadFile(canon)
	if errors.Is(err, os.ErrNotExist) {
		return recorded, true, nil
	}
	if err != nil {
		return false, false, fmt.Errorf("websterengine: read %s: %w", canon, err)
	}
	sum := sha256.Sum256(data)
	return !recorded || want != hex.EncodeToString(sum[:]), true, nil
}

// trackedRel returns the slash-separated path of p relative to worktree when p lies in the task worktree's tracked tree:
// inside the worktree, outside its _lyx and not git-ignored.
// ok is false for every other path, which has no blob to compare.
func trackedRel(git Git, worktree, p string) (rel string, ok bool, err error) {
	root, err := canonicalPath(worktree)
	if err != nil {
		return "", false, err
	}
	lexical := resolveWritePath(worktree, p)
	canon, err := canonicalPath(lexical)
	if err != nil {
		return "", false, err
	}
	lyxLink := filepath.Join(worktree, lyxdirs.LyxDirName)
	lyxReal, err := canonicalPath(lyxLink)
	if err != nil {
		return "", false, err
	}
	if !pathWithin(root, canon) || pathWithin(lyxReal, canon) || pathWithin(lyxLink, lexical) {
		return "", false, nil
	}
	ignored, err := git.IgnoredPath(worktree, canon)
	if err != nil || ignored {
		return "", false, err
	}
	r, err := filepath.Rel(root, canon)
	if err != nil {
		return "", false, fmt.Errorf("websterengine: relate %s to %s: %w", canon, root, err)
	}
	return filepath.ToSlash(r), true, nil
}

// uncheckableSuspects returns the paths of paths that checkRecoveredSuspects cannot check:
// neither under geom.PlanDir with st.PlanFileHashes recorded, nor accepted by trackedRel.
// uncheckableReason classifies each path, so the reason a path cannot be checked comes from the same cases;
// the record keeps the bare paths, and a refusal asks uncheckableReason for each path's reason.
// The error is a link-resolution or git probe failure.
func uncheckableSuspects(geom Geometry, st *State, paths []string) ([]string, error) {
	var out []string
	for _, p := range paths {
		_, uncheckable, err := uncheckableReason(geom, st, p)
		if err != nil {
			return nil, err
		}
		if uncheckable {
			out = append(out, p)
		}
	}
	return out, nil
}

// suspectBlobs records, for each of paths, the blob of its worktree content now.
// A path outside the tracked tree, or an absent file, gets an empty Blob.
func suspectBlobs(git Git, worktree string, paths []string) ([]SuspectPath, error) {
	var out []SuspectPath
	for _, p := range paths {
		sp := SuspectPath{Path: p}
		rel, ok, err := trackedRel(git, worktree, p)
		if err != nil {
			return nil, err
		}
		if ok {
			if sp.Blob, err = git.WorktreeBlob(worktree, filepath.Join(worktree, filepath.FromSlash(rel))); err != nil {
				return nil, err
			}
		}
		out = append(out, sp)
	}
	return out, nil
}

// checkRecoveredSuspects returns one reason per suspect path the recovery left unresolved at head:
// a plan file differing from the run's recorded hashes, a tracked path differing from head, or the flagged blob still held at head under any path that did not hold it at the start commit.
// The blob search covers the whole head tree, so a strand that moves the flagged file keeps the finding.
// It runs for every path recording a blob, which suspectBlobs records only for a path in the tracked tree when the batch failed,
// so a strand that untracks the original path still has its moved copy found.
// A path checkSuspectPaths reports unverifiable is not checked here:
// RecoverSpawnOrAttach refuses a batch recording one, so none reaches this check.
// number is the batch's number, named in the plan-path way forward.
func checkRecoveredSuspects(geom Geometry, st *State, bs *BatchState, number int, head string) ([]string, error) {
	var paths []string
	blobs := map[string]string{}
	for _, sp := range bs.SuspectPaths {
		paths = append(paths, sp.Path)
		blobs[sp.Path] = sp.Blob
	}
	planDiff, _, err := checkSuspectPaths(geom, st, "", paths)
	if err != nil {
		return nil, err
	}
	headDiff, _, err := checkSuspectPaths(geom, st, head, paths)
	if err != nil {
		return nil, err
	}
	var reasons []string
	isPlan := map[string]bool{}
	for _, p := range planDiff {
		isPlan[p] = true
		reasons = append(reasons, fmt.Sprintf("suspect path %s still differs from the plan as the run recorded it; way forward: %s", p, planPathClause(fmt.Sprintf("\"lyx webster recover-batch %02d\"", number))))
	}
	for _, p := range headDiff {
		if !isPlan[p] {
			reasons = append(reasons, fmt.Sprintf("suspect path %s has changes the recovery report's head %s does not hold", p, head))
		}
	}
	for _, p := range paths {
		if isPlan[p] || blobs[p] == "" || slices.Contains(headDiff, p) {
			continue
		}
		listed, err := geom.git().TreePathsWithBlob(geom.WorktreeRoot, head, blobs[p])
		if err != nil {
			return nil, err
		}
		for _, lp := range listed {
			// With no start commit recorded, nothing was held at the start;
			// commitBlob would otherwise read ":<path>", the index entry.
			atStart := ""
			if bs.StartSHA != "" {
				if atStart, err = geom.git().CommitBlob(geom.WorktreeRoot, bs.StartSHA, lp); err != nil {
					return nil, err
				}
			}
			if atStart != blobs[p] {
				reasons = append(reasons, fmt.Sprintf("suspect path %s still holds the content the audit flagged, at %s; revert it to %s or re-derive it", p, lp, bs.StartSHA))
			}
		}
	}
	return reasons, nil
}
