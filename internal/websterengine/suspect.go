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
	"sort"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
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

// worktreePathDiffers reports whether path differs from base in worktree: a changed tracked file or an untracked new one.
func worktreePathDiffers(worktree, base, path string) (bool, error) {
	_, stderr, code, err := gitexec.RunGit([]string{"diff", "--quiet", base, "--", path}, worktree)
	if err != nil {
		return false, fmt.Errorf("websterengine: git diff %s in %s: %w", path, worktree, err)
	}
	switch code {
	case 0:
	case 1:
		return true, nil
	default:
		return false, fmt.Errorf("websterengine: git diff %s in %s exited %d: %s", path, worktree, code, strings.TrimSpace(stderr))
	}
	stdout, err := gitexec.Run([]string{"ls-files", "--others", "--exclude-standard", "--", path}, worktree)
	if err != nil {
		return false, fmt.Errorf("websterengine: git ls-files %s in %s: %w", path, worktree, err)
	}
	return strings.TrimSpace(stdout) != "", nil
}
