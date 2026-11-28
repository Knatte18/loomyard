// failedpush.go holds the rules of the failed-push record.
// They cover how a new failure merges into the record, the notice it sends and the way forward by side and class.
// They also decide which records leave a push owed or refused.

package fabriccore

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/fabriclayout"
)

// VerbDirs maps a code worktree's root to the directory a lym verb of that worktree runs in.
// A worktree it does not hold runs its verbs at its root.
type VerbDirs map[string]string

// of returns the directory a lym verb of worktree runs in.
func (d VerbDirs) of(worktree string) string {
	if dir, ok := d[worktree]; ok {
		return dir
	}
	return worktree
}

// MergeFailedPush returns the record that stands once the failure next lands on current.
// found reports whether current is an existing record of the same repository side and branch.
// The first failure is counted once and first seen at now.
// A later one keeps the first-seen time and the notified flag, bumps the count and unions the runs, existing first.
// The record is last seen at now.
// notify reports whether the record is due its notice: it is neither notified nor deferred.
func MergeFailedPush(current fabriclayout.FailedPush, found bool, next fabriclayout.FailedPush, now time.Time) (merged fabriclayout.FailedPush, notify bool) {
	merged = next
	merged.FirstSeen = now
	merged.Count = 1
	if found {
		merged.FirstSeen = current.FirstSeen
		merged.Count = current.Count + 1
		merged.Notified = current.Notified
		merged.Runs = slices.Clone(current.Runs)
		for _, run := range next.Runs {
			if !slices.Contains(merged.Runs, run) {
				merged.Runs = append(merged.Runs, run)
			}
		}
	}
	merged.LastSeen = now
	return merged, !merged.Notified && merged.Class != fabriclayout.PushFailureDeferred
}

// FailedPushNotice returns the notice of the failed push f.
// Its line names the branch, the class, the error and the way forward.
// Its topic is the repository side and branch, and its run the pair's slug.
func FailedPushNotice(f fabriclayout.FailedPush, dirs VerbDirs) fabriclayout.PushNotice {
	return fabriclayout.PushNotice{
		Line:  fmt.Sprintf("a push of %s branch %s failed (%s): %s; way forward: %s", f.Repo, f.Branch, f.Class, f.Error, FailedPushWayForward(f, dirs)),
		Topic: "push/" + f.Repo + "/" + f.Branch,
		Run:   f.PairSlug(),
	}
}

// PushStep names the push that retries f, with the directory it runs in.
// A bolt record is pushed by the board sync, since the pair push never reaches Bolt.
// A prime's code branch is pushed by a plain git push, since the prime's pair push is records-only.
// Every other code branch, a beam parent's included, is pushed by the pair push.
func PushStep(f fabriclayout.FailedPush, dirs VerbDirs) string {
	switch {
	case f.Repo == fabriclayout.RepoBolt:
		return "`lym board sync` from the prime worktree"
	case f.Repo == fabriclayout.RepoCode && f.Prime:
		return fmt.Sprintf("`git push` in %s", f.RepoPath)
	case f.Repo == fabriclayout.RepoRecords:
		return fmt.Sprintf("`lym fabric push` in %s", dirs.of(f.CodeWorktree))
	default:
		return fmt.Sprintf("`lym fabric push` in %s", dirs.of(f.RepoPath))
	}
}

// FailedPushWayForward returns the runnable way forward for a failed push, by repository side and class.
func FailedPushWayForward(f fabriclayout.FailedPush, dirs VerbDirs) string {
	switch f.Class {
	case fabriclayout.PushFailureAuth:
		return fmt.Sprintf("fix this machine's permission to push %s on origin, then run %s", f.Branch, PushStep(f, dirs))
	case fabriclayout.PushFailureNotFastForward:
		if f.Repo == fabriclayout.RepoCode {
			// A refused landing push carries its own line, which names the landing commits too.
			if f.WayForward != "" {
				return f.WayForward
			}
			// Only a refused reset records origin's head on a diverged code branch; its commits are then origin's.
			var unmade []string
			if f.OriginHead != "" {
				unmade = f.Commits
			}
			return CodeDivergedWayForward(f.RepoPath, f.Branch, f.Prime, dirs, unmade...)
		}
		return RecordsConflictWayForward(f.RepoPath, f.Branch, f.Paths, f.Commits, PushStep(f, dirs))
	case fabriclayout.PushFailureRewound:
		return RewoundWayForward(f.RepoPath, f.Branch, f.OriginHead, f.Commits, dirs)
	case fabriclayout.PushFailureAnchorChange, fabriclayout.PushFailureHeldBack:
		return f.WayForward
	case fabriclayout.PushFailureTeardownLock:
		return fmt.Sprintf("the push retries once the removal or push holding the pair's teardown lock lets go, so run %s again then, "+
			"or end the stuck child whose pid `lym tender status` lists", PushStep(f, dirs))
	case fabriclayout.PushFailureGone:
		pairWorktree := f.RepoPath
		if f.Repo == fabriclayout.RepoRecords {
			pairWorktree = f.CodeWorktree
		}
		return fmt.Sprintf("origin no longer has branch %s, and its board entry is neither done nor removed on origin, since an ended entry writes no record; "+
			"decide whether the run ends: `lym fabric remove` of the pair at %s, run from the prime, removes the pair and clears this record; "+
			"once origin's board shows the entry done or removed, `lym cleanup --apply` tears it down instead",
			f.Branch, pairWorktree)
	default:
		// A refusal of the other class that a retry cannot clear carries its own line.
		if f.Class == fabriclayout.PushFailureOther && f.WayForward != "" {
			return f.WayForward
		}
		return fmt.Sprintf("run %s to retry the push", PushStep(f, dirs))
	}
}

// RecordsConflictWayForward names a records branch origin moved past.
// It names the conflicting paths when a replay stopped on some, the local commits, and the two ways to settle it.
// The two ways run in worktree, and the second ends with pushVerb, the push step with its directory.
// It names no machine, since no commit names the machine that wrote it.
func RecordsConflictWayForward(worktree, branch string, paths, commits []string, pushVerb string) string {
	found := fmt.Sprintf("origin/%s holds commits this branch lacks (local commits: %s)", branch, joinOrNone(commits))
	settle := "settle any conflict"
	if len(paths) > 0 {
		found = fmt.Sprintf("origin/%s holds commits that change the same paths as this branch's (paths: %s; local commits: %s)", branch, strings.Join(paths, " "), joinOrNone(commits))
		settle = "settle the named paths"
	}
	return fmt.Sprintf("%s; "+
		"to keep origin's side, run `git reset --keep origin/%s` in %s, which drops the local commits; "+
		"to keep this machine's, run `git merge origin/%s` in %s, %s and commit, then run %s, whose push is then a fast-forward and meets no replay",
		found, branch, worktree, branch, worktree, settle, pushVerb)
}

// CodeDivergedWayForward names the merge that settles a code branch origin moved past.
// The merge runs at the worktree root and the push in the directory dirs gives the worktree.
// A prime's branch is pushed by a plain git push, since the pair push leaves the prime's code branch unpushed.
// unmade names the commits on origin's branch that this machine did not make, so the branch was not reset past them.
func CodeDivergedWayForward(worktree, branch string, prime bool, dirs VerbDirs, unmade ...string) string {
	push := fmt.Sprintf("`lym fabric push` in %s", dirs.of(worktree))
	if prime {
		push = fmt.Sprintf("`git push` in %s", worktree)
	}
	found := fmt.Sprintf("origin/%s holds commits this branch lacks", branch)
	if len(unmade) > 0 {
		found = fmt.Sprintf("origin/%s holds commits this machine did not make (%s), so it was not reset", branch, strings.Join(unmade, " "))
	}
	return fmt.Sprintf("%s; run `git merge origin/%s` in %s, settle any conflict and commit, then run %s",
		found, branch, worktree, push)
}

// RewoundWayForward names the two ways to settle a task branch origin rewound to originHead.
// dropped holds the commits the rewind removed from origin.
// The git steps run at the worktree root and the push in the directory dirs gives the worktree.
func RewoundWayForward(worktree, branch, originHead string, dropped []string, dirs VerbDirs) string {
	return fmt.Sprintf("origin rewound %s to %s, dropping %s; "+
		"to accept the rewind, run `git reset --keep %s` in %s, which drops the local commits past origin's head, then `lym fabric push` in %s; "+
		"to restore the dropped commits, run `git push origin %s` in %s, a plain fast-forward",
		branch, originHead, joinOrNone(dropped), originHead, worktree, dirs.of(worktree), branch, worktree)
}

func joinOrNone(items []string) string {
	if len(items) == 0 {
		return "none"
	}
	return strings.Join(items, " ")
}

// IsOwedClass reports whether class is one a caller retries until the push lands: transport, deferred or teardown_lock.
func IsOwedClass(class fabriclayout.PushFailureClass) bool {
	switch class {
	case fabriclayout.PushFailureTransport, fabriclayout.PushFailureDeferred, fabriclayout.PushFailureTeardownLock:
		return true
	}
	return false
}

// OwedBy returns the records that hold the run slug's push owed, newest last-seen first.
// A record holds a push owed when its class is a retried one and its Runs name the slug.
func OwedBy(records []fabriclayout.FailedPush, slug string) []fabriclayout.FailedPush {
	var owed []fabriclayout.FailedPush
	for _, record := range records {
		if IsOwedClass(record.Class) && slices.Contains(record.Runs, slug) {
			owed = append(owed, record)
		}
	}
	slices.SortStableFunc(owed, func(a, b fabriclayout.FailedPush) int { return b.LastSeen.Compare(a.LastSeen) })
	return owed
}

// RefusedPushStands reports whether records hold a refused push of the records branch or of the code branch.
// A refused push is a failed one of a class that is not retried.
// An empty codeBranch matches no code record.
func RefusedPushStands(records []fabriclayout.FailedPush, recordsBranch, codeBranch string) bool {
	return slices.ContainsFunc(records, func(record fabriclayout.FailedPush) bool {
		if IsOwedClass(record.Class) {
			return false
		}
		return (record.Repo == fabriclayout.RepoRecords && record.Branch == recordsBranch) ||
			(codeBranch != "" && record.Repo == fabriclayout.RepoCode && record.Branch == codeBranch)
	})
}
