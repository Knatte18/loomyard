// gitwrap.go implements webster's own git-query helpers over internal/gitrepo: headSHA captures a
// batch's start-SHA and the report cross-check's actual HEAD, and dirty is the half-done-work
// signal.
// refuseMidMerge and reconcileReportHead are the read-only probes record-batch and recover-batch share:
// the first refuses while a git merge is in progress, the second accepts a HEAD that is only merge commits past the report's head_sha.
// Per the Shared Decision git-verification-via-gitrepo, every helper here goes through gitrepo.Repo
// except dirty, which gitrepo exposes no porcelain/status method for and so wraps gitexec.Run
// directly — the one carved-out exception the decision names.

package websterengine

import (
	"fmt"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
)

// headSHA returns worktree's current HEAD commit SHA via gitrepo.Repo.
// An unborn HEAD surfaces as gitrepo.ErrNoCommits.
func headSHA(worktree string) (string, error) {
	sha, err := gitrepo.New(worktree).CurrentSHA()
	if err != nil {
		return "", fmt.Errorf("websterengine: head sha in %s: %w", worktree, err)
	}
	return sha, nil
}

// refuseMidMerge returns an error when worktree has a git merge in progress, and nil otherwise.
// It asks git whether MERGE_HEAD resolves rather than statting a file, because a linked worktree keeps MERGE_HEAD in its per-worktree git dir.
func refuseMidMerge(worktree string) error {
	present, err := gitrepo.New(worktree).MergeHeadPresent()
	if err != nil {
		return fmt.Errorf("websterengine: probe merge in progress in %s: %w", worktree, err)
	}
	if present {
		return fmt.Errorf("webster: worktree %s has a git merge in progress; conclude it first "+
			"(`lyx fabric merge --continue` / `lyx fabric merge --abort` in a hub, "+
			"`git merge --continue` / `git merge --abort` for a standalone run)", worktree)
	}
	return nil
}

// reconcileReportHead cross-checks a report's head_sha against worktree's actual HEAD, tolerating merge commits only.
// HEAD equal to reportHead is the unchanged fast path and returns an empty warning.
// Otherwise it walks HEAD's first-parent chain, comparing each commit with reportHead BEFORE looking at its parent count —
// so a reportHead that is itself a merge commit is accepted, not stepped past.
// A commit that is not reportHead must be a merge (two or more parents) to be walked over;
// a non-merge commit or the root ends the walk in refusal.
// On acceptance the warning names subject, both heads and every walked merge SHA in walk order.
func reconcileReportHead(worktree, reportHead, subject string) (warning string, err error) {
	head, err := headSHA(worktree)
	if err != nil {
		return "", err
	}
	if head == reportHead {
		return "", nil
	}

	repo := gitrepo.New(worktree)
	var merges []string
	for cur := head; ; {
		if cur == reportHead {
			return fmt.Sprintf("webster: %s: head_sha %q differs from the worktree's HEAD %q; only merge commits (%s) sit between them, so the batch is recorded at the report's head_sha %q",
				subject, reportHead, head, strings.Join(merges, ", "), reportHead), nil
		}
		parents, err := repo.CommitParents(cur)
		if err != nil {
			return "", fmt.Errorf("websterengine: walk first-parent chain from %s in %s: %w", head, worktree, err)
		}
		if len(parents) < 2 {
			return "", fmt.Errorf("webster: %s: head_sha %q does not match the worktree's actual HEAD %q; "+
				"only merge commits, such as a parent merge-in, may sit between a fork's reported head and HEAD",
				subject, reportHead, head)
		}
		merges = append(merges, cur)
		cur = parents[0]
	}
}

// dirty reports whether worktree has any uncommitted or untracked changes.
// It wraps gitexec.Run directly since gitrepo.Repo exposes no porcelain/status method.
func dirty(worktree string) (bool, error) {
	stdout, err := gitexec.Run([]string{"status", "--porcelain"}, worktree)
	if err != nil {
		return false, fmt.Errorf("websterengine: git status --porcelain in %s: %w", worktree, err)
	}
	return strings.TrimSpace(stdout) != "", nil
}
