// suspect.go checks suspect paths against what the run recorded.
// A tracked file in the task worktree is checked against a commit its caller chooses.
// The run's start and last commits are picked by git ancestry, not by batch number.
// A plan file is checked against the run's plan hashes.
// Every other path has nothing recorded to compare with, so it is reported unverifiable rather than guessed at.
// It also holds the link-resolving path helpers those checks compare paths with.

package runplan

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

	"github.com/Knatte18/loomyard/internal/lymdirs"
)

// EvidenceBases holds the two commits the suspect-path evidence is checked against.
// They are picked by git ancestry rather than batch number.
type EvidenceBases struct {
	// Start is the run's start commit: the recorded batch StartSHA that is an ancestor of every other.
	Start string
	// Last is the last batch head: the recorded terminal Digest.HeadSHA every other terminal head is an ancestor of.
	Last string
	// Missing lists every recorded StartSHA and terminal HeadSHA absent from the repository, sorted and deduplicated.
	Missing []string
	// Starts lists every recorded StartSHA, sorted and deduplicated, to tell no recorded start from no qualifying one.
	Starts []string
}

// RunEvidenceBases picks st's start commit and last batch head by git ancestry.
// A batch number names a batch and does not date it, so the pick follows ancestry.
// A pick is "" when nothing is recorded or no candidate qualifies.
// It is also "" when any recorded commit of its kind is missing from the repository.
// A missing commit blanks the pick rather than being skipped: the pick among the rest would name an older commit.
// A state recording no SHA returns the zero value without running git.
// The error is an IsAncestor failure.
func RunEvidenceBases(geom Geometry, st *State) (EvidenceBases, error) {
	git, worktree := geom.Git, geom.WorktreeRoot
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
	var out EvidenceBases
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
			return EvidenceBases{}, err
		}
	}
	if !headsMissing {
		if out.Last, err = pickByAncestry(git, worktree, heads, false); err != nil {
			return EvidenceBases{}, err
		}
	}
	return out, nil
}

// pickByAncestry returns the commit of shas that is an ancestor of every other when oldest is true.
// When oldest is false it returns the commit that every other is an ancestor of.
// It returns "" when none qualifies.
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

// MissingCommitsClause names the recorded commits absent from the repository and the way forward.
// It returns "" for an empty list.
func MissingCommitsClause(missing []string) string {
	if len(missing) == 0 {
		return ""
	}
	return fmt.Sprintf("commit(s) %s recorded by this run are not in this repository; way forward: fetch the task branch from the machine that ran those batches with git, then re-run the verb", strings.Join(missing, ", "))
}

// SplitPlanPaths sorts paths into those under geom.PlanDir and the rest, each in input order.
// The error is a link-resolution failure.
func SplitPlanPaths(geom Geometry, paths []string) (plan, rest []string, err error) {
	planDir, err := CanonicalPath(geom.PlanDir)
	if err != nil {
		return nil, nil, err
	}
	for _, p := range paths {
		canon, err := CanonicalPath(ResolveWritePath(geom.PaneCwd, p))
		if err != nil {
			return nil, nil, err
		}
		if PathWithin(planDir, canon) {
			plan = append(plan, p)
		} else {
			rest = append(rest, p)
		}
	}
	return plan, rest, nil
}

// PlanFileName returns the slash-separated name of the plan path p relative to geom.PlanDir.
// It is the key State.PlanFileHashes is indexed by.
func PlanFileName(geom Geometry, p string) (string, error) {
	planDir, err := CanonicalPath(geom.PlanDir)
	if err != nil {
		return "", err
	}
	canon, err := CanonicalPath(ResolveWritePath(geom.PaneCwd, p))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(planDir, canon)
	if err != nil {
		return "", fmt.Errorf("runplan: relate %s to %s: %w", canon, planDir, err)
	}
	return filepath.ToSlash(rel), nil
}

// CheckSuspectPaths sorts each of paths into differing or unverifiable, both returned sorted.
// A plan file differs when its hash is not the one st.PlanFileHashes recorded.
// It is unverifiable when no hashes were recorded.
// A tracked or new file in the task worktree differs when it changed against base.
// It is unverifiable when base is empty or not in the repository.
// Paths under its _lym or under geom.ScratchDir, and git-ignored paths, are also unverifiable.
// Any other path is unverifiable.
// The error is a git probe's or a read's failure other than not-exist;
// the check changes nothing.
func CheckSuspectPaths(geom Geometry, st *State, base string, paths []string) (differing, unverifiable []string, err error) {
	worktree, err := CanonicalPath(geom.WorktreeRoot)
	if err != nil {
		return nil, nil, err
	}
	planDir, err := CanonicalPath(geom.PlanDir)
	if err != nil {
		return nil, nil, err
	}
	scratch, err := CanonicalPath(geom.ScratchDir)
	if err != nil {
		return nil, nil, err
	}
	lym, err := CanonicalPath(filepath.Join(geom.PaneCwd, lymdirs.LymDirName))
	if err != nil {
		return nil, nil, err
	}
	git := geom.Git
	if base != "" && !git.SHAExists(geom.WorktreeRoot, base) {
		base = ""
	}
	for _, p := range paths {
		lexical := ResolveWritePath(geom.PaneCwd, p)
		canon, err := CanonicalPath(lexical)
		if err != nil {
			return nil, nil, err
		}
		switch {
		case PathWithin(planDir, canon):
			differs, ok, err := planFileDiffers(st, planDir, canon)
			if err != nil {
				return nil, nil, err
			}
			if !ok {
				unverifiable = append(unverifiable, p)
			} else if differs {
				differing = append(differing, p)
			}
		case !PathWithin(worktree, canon),
			PathWithin(lym, canon), PathWithin(filepath.Join(geom.PaneCwd, lymdirs.LymDirName), lexical),
			PathWithin(scratch, canon):
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
		return false, false, fmt.Errorf("runplan: relate %s to %s: %w", canon, planDir, err)
	}
	want, recorded := st.PlanFileHashes[filepath.ToSlash(rel)]
	data, err := os.ReadFile(canon)
	if errors.Is(err, os.ErrNotExist) {
		return recorded, true, nil
	}
	if err != nil {
		return false, false, fmt.Errorf("runplan: read %s: %w", canon, err)
	}
	sum := sha256.Sum256(data)
	return !recorded || want != hex.EncodeToString(sum[:]), true, nil
}

// TrackedRel returns the slash-separated path of p relative to worktree when p lies in the task worktree's tracked tree.
// That tree is the part inside the worktree, outside the _lym under paneCwd and not git-ignored.
// A relative p is resolved against paneCwd, the directory the session that recorded it ran in.
// ok is false for every other path, which has no blob to compare.
func TrackedRel(git Git, paneCwd, worktree, p string) (rel string, ok bool, err error) {
	root, err := CanonicalPath(worktree)
	if err != nil {
		return "", false, err
	}
	lexical := ResolveWritePath(paneCwd, p)
	canon, err := CanonicalPath(lexical)
	if err != nil {
		return "", false, err
	}
	lymLink := filepath.Join(paneCwd, lymdirs.LymDirName)
	lymReal, err := CanonicalPath(lymLink)
	if err != nil {
		return "", false, err
	}
	if !PathWithin(root, canon) || PathWithin(lymReal, canon) || PathWithin(lymLink, lexical) {
		return "", false, nil
	}
	ignored, err := git.IgnoredPath(worktree, canon)
	if err != nil || ignored {
		return "", false, err
	}
	r, err := filepath.Rel(root, canon)
	if err != nil {
		return "", false, fmt.Errorf("runplan: relate %s to %s: %w", canon, root, err)
	}
	return filepath.ToSlash(r), true, nil
}

// SuspectBlobs records, for each path, the blob of its worktree content at call time.
// A relative path is resolved against paneCwd.
// A path outside the tracked tree, or an absent file, gets an empty Blob.
func SuspectBlobs(git Git, paneCwd, worktree string, paths []string) ([]SuspectPath, error) {
	var out []SuspectPath
	for _, p := range paths {
		sp := SuspectPath{Path: p}
		rel, ok, err := TrackedRel(git, paneCwd, worktree, p)
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

// CheckRecoveredSuspects returns one reason per suspect path the recovery leaves unresolved at head.
// A path differing from the run's recorded plan hashes, or differing from head, is unresolved.
// A path is also unresolved when its flagged blob is still at head under another path:
// a path that did not hold that blob at the start commit.
// The blob search covers the whole head tree, so a strand that moves the flagged file keeps the finding.
// Only a path in the tracked tree when the batch failed records a blob,
// so a strand that untracks the original path still has its moved copy found.
// A suspect path that cannot be verified is not checked here:
// the recovery entry point refuses a batch that records one, so none reaches this check.
// Each reason about a plan file ends in planWayForward, the caller's way to settle a plan edit and recover again.
func CheckRecoveredSuspects(geom Geometry, st *State, bs *BatchState, head, planWayForward string) ([]string, error) {
	var paths []string
	blobs := map[string]string{}
	for _, sp := range bs.SuspectPaths {
		paths = append(paths, sp.Path)
		blobs[sp.Path] = sp.Blob
	}
	planDiff, _, err := CheckSuspectPaths(geom, st, "", paths)
	if err != nil {
		return nil, err
	}
	headDiff, _, err := CheckSuspectPaths(geom, st, head, paths)
	if err != nil {
		return nil, err
	}
	var reasons []string
	isPlan := map[string]bool{}
	for _, p := range planDiff {
		isPlan[p] = true
		reasons = append(reasons, fmt.Sprintf("suspect path %s still differs from the plan as the run recorded it; way forward: %s", p, planWayForward))
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
		listed, err := geom.Git.TreePathsWithBlob(geom.WorktreeRoot, head, blobs[p])
		if err != nil {
			return nil, err
		}
		for _, lp := range listed {
			// With no start commit recorded, nothing was held at the start;
			// the commit lookup would otherwise read ":<path>", the index entry.
			atStart := ""
			if bs.StartSHA != "" {
				if atStart, err = geom.Git.CommitBlob(geom.WorktreeRoot, bs.StartSHA, lp); err != nil {
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

// ResolveWritePath canonicalizes a recorded write path: cleaned, and, when relative, resolved against workdir.
// This prevents false matches when agents mix absolute and relative spellings for the same file.
func ResolveWritePath(workdir, path string) string {
	cleaned := filepath.Clean(path)
	if isTranscriptPathAbsolute(path) {
		return cleaned
	}
	return filepath.Join(workdir, cleaned)
}

// isTranscriptPathAbsolute reports whether a recorded write path is absolute:
// either in the running OS's sense or as a POSIX path with a leading "/".
// Recorded write paths and the workdir and contract paths are always POSIX-style, whatever the platform.
// On Windows, filepath.IsAbs alone misses a leading "/".
// Joining such a path onto workdir again yields a path that never matches the contract paths.
func isTranscriptPathAbsolute(path string) bool {
	return filepath.IsAbs(path) || strings.HasPrefix(path, "/")
}

// PathWithin reports whether path is dir itself or lies beneath it;
// both must already be canonical.
func PathWithin(dir, path string) bool {
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

// CanonicalPath resolves links in path through its nearest existing ancestor.
// It re-joins the missing tail, so a path that does not exist yet still compares against a link-resolved directory.
func CanonicalPath(path string) (string, error) {
	cleaned := filepath.Clean(path)
	var tail []string
	cur := cleaned
	for {
		resolved, err := filepath.EvalSymlinks(cur)
		if err == nil {
			for i := len(tail) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, tail[i])
			}
			return resolved, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("runplan: resolve links in %s: %w", cur, err)
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return cleaned, nil
		}
		tail = append(tail, filepath.Base(cur))
		cur = parent
	}
}
