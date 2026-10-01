// suspect.go checks a run-exit finding's suspect paths against what the run recorded, the evidence accept-audit and the run-entry paths share.
// A path is verifiable only against the last batch head (a tracked file in the task worktree) or the run's plan hashes (a plan file);
// every other path has nothing recorded to compare with, so it is reported unverifiable rather than guessed at.

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

	"github.com/Knatte18/loomyard/internal/lyxdirs"
)

// lastBatchHead returns the Digest.HeadSHA of the highest-numbered terminal batch that carries one, the integration key excluded.
// It returns "" when no batch does.
func lastBatchHead(st *State) string {
	best, head := 0, ""
	for n, bs := range st.Batches {
		if n == integrationBatchKey || bs == nil || !bs.Terminal || bs.Digest == nil || bs.Digest.HeadSHA == "" {
			continue
		}
		if n > best {
			best, head = n, bs.Digest.HeadSHA
		}
	}
	return head
}

// runStartCommit returns the StartSHA of the lowest-numbered batch record that carries one, or "" when none does.
func runStartCommit(st *State) string {
	best, sha := 0, ""
	for n, bs := range st.Batches {
		if n == integrationBatchKey || bs == nil || bs.StartSHA == "" {
			continue
		}
		if sha == "" || n < best {
			best, sha = n, bs.StartSHA
		}
	}
	return sha
}

// checkSuspectPaths sorts each of paths into differing or unverifiable, both returned sorted.
// A plan file differs when its hash is not the one st.PlanFileHashes recorded, and is unverifiable when no hashes were recorded.
// A tracked-or-new file in the task worktree outside its _lyx, geom.ScratchDir and git-ignored paths differs when it changed against base, and is unverifiable when base is empty.
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
			ignored, err := ignoredPath(geom.WorktreeRoot, canon)
			if err != nil {
				return nil, nil, err
			}
			if ignored || base == "" {
				unverifiable = append(unverifiable, p)
				continue
			}
			differs, err := worktreePathDiffers(geom.WorktreeRoot, base, canon)
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
func trackedRel(worktree, p string) (rel string, ok bool, err error) {
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
	ignored, err := ignoredPath(worktree, canon)
	if err != nil || ignored {
		return "", false, err
	}
	r, err := filepath.Rel(root, canon)
	if err != nil {
		return "", false, fmt.Errorf("websterengine: relate %s to %s: %w", canon, root, err)
	}
	return filepath.ToSlash(r), true, nil
}

// suspectBlobs records, for each of paths, the blob of its worktree content now.
// A path outside the tracked tree, or an absent file, gets an empty Blob.
func suspectBlobs(worktree string, paths []string) ([]SuspectPath, error) {
	var out []SuspectPath
	for _, p := range paths {
		sp := SuspectPath{Path: p}
		rel, ok, err := trackedRel(worktree, p)
		if err != nil {
			return nil, err
		}
		if ok {
			if sp.Blob, err = worktreeBlob(worktree, filepath.Join(worktree, filepath.FromSlash(rel))); err != nil {
				return nil, err
			}
		}
		out = append(out, sp)
	}
	return out, nil
}

// checkRecoveredSuspects returns one reason per suspect path the recovery left unresolved at head:
// a plan file differing from the run's recorded hashes, a tracked path differing from head, or a tracked path whose head content is still the flagged blob and not the start commit's.
// A path checkSuspectPaths reports unverifiable is not checked.
func checkRecoveredSuspects(geom Geometry, st *State, bs *BatchState, head string) ([]string, error) {
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
		reasons = append(reasons, fmt.Sprintf("suspect path %s still differs from the plan as the run recorded it", p))
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
		rel, ok, err := trackedRel(geom.WorktreeRoot, p)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		atHead, err := commitBlob(geom.WorktreeRoot, head, rel)
		if err != nil {
			return nil, err
		}
		if atHead != blobs[p] {
			continue
		}
		atStart, err := commitBlob(geom.WorktreeRoot, bs.StartSHA, rel)
		if err != nil {
			return nil, err
		}
		if atStart != atHead {
			reasons = append(reasons, fmt.Sprintf("suspect path %s still holds the content the audit flagged; revert it to %s or re-derive it", p, bs.StartSHA))
		}
	}
	return reasons, nil
}
