// failedpush.go holds the failed-push record, one machine-local file per repository side and branch.
// It also holds the once-per-episode notice to the orch, sent through the push's notifier or the installed one.
// The way-forward line for each side and class is built there too.

package fabricengine

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/state"
)

// PushNotifier posts notice to the orch of the hub at hubPath and reports whether the mail was posted.
// A notifier reporting false, because no orch is recorded, leaves the failure to be mailed by a later one.
type PushNotifier func(hubPath string, notice fabriclayout.PushNotice) (posted bool, err error)

var installedPushNotifier atomic.Pointer[PushNotifier]

// InstallPushNotifier stores n as the process's push notifier and returns a function that reinstates the previous one.
func InstallPushNotifier(n PushNotifier) (restore func()) {
	previous := installedPushNotifier.Swap(&n)
	return func() { installedPushNotifier.Store(previous) }
}

// installedNotifier returns the process's push notifier, nil when none is installed.
func installedNotifier() PushNotifier {
	if n := installedPushNotifier.Load(); n != nil {
		return *n
	}
	return nil
}

// notifier returns the notifier a failed push of these options mails through:
// Notifier, or the process's installed one when Notifier is nil.
func (o SyncOptions) notifier() PushNotifier {
	if o.Notifier != nil {
		return o.Notifier
	}
	return installedNotifier()
}

func failedPushDir(hubPath string) string {
	return filepath.Join(fabriclayout.HubScratchDir(hubPath), lymdirs.FailedPushDirName)
}

// failedPushPath returns the record file for a repository side and branch.
// The branch is path-escaped so a slash in it stays one file name.
func failedPushPath(hubPath, repo, branch string) string {
	return filepath.Join(failedPushDir(hubPath), repo+"__"+url.PathEscape(branch)+".json")
}

func failedPushLockPath(path string) string {
	return path + ".lock"
}

// RecordPushFailure writes f as the failed-push record of its repository side and branch under the record's lock.
// The first failure creates the record.
// Later failures keep its first-seen time and notified flag, replace the other fields and bump the count.
// The runs of a later failure are unioned with the record's,
// so a second run's failure of one branch keeps the first run owed.
// While the record is not notified and its class is not deferred, the installed notifier is called once.
// The notice it receives names the branch, the class, the error and the way forward.
// It carries the side and branch as its topic and the pair's slug as its run.
// The record is marked notified only when the notifier reports the mail posted.
// A notifier error is logged and leaves the record unnotified, so the next failure mails again.
func RecordPushFailure(hubPath string, f fabriclayout.FailedPush, now time.Time) error {
	return recordPushFailure(hubPath, f, now, installedNotifier())
}

// recordPushFailure writes f as the failed-push record of its repository side and branch under the record's lock.
// It mails a record that is neither notified nor deferred through notify, once per episode; a nil notify mails nothing.
func recordPushFailure(hubPath string, f fabriclayout.FailedPush, now time.Time, notify PushNotifier) error {
	path := failedPushPath(hubPath, f.Repo, f.Branch)
	return state.UpdateJSON(path, failedPushLockPath(path), func(cur fabriclayout.FailedPush, found bool) (fabriclayout.FailedPush, error) {
		f.FirstSeen = now
		f.Count = 1
		if found {
			f.FirstSeen = cur.FirstSeen
			f.Count = cur.Count + 1
			f.Notified = cur.Notified
			f.Runs = unionRuns(cur.Runs, f.Runs)
		}
		f.LastSeen = now
		if f.Notified || f.Class == fabriclayout.PushFailureDeferred {
			return f, nil
		}
		if notify == nil {
			return f, nil
		}
		notice := fabriclayout.PushNotice{
			Line:  fmt.Sprintf("a push of %s branch %s failed (%s): %s; way forward: %s", f.Repo, f.Branch, f.Class, f.Error, FailedPushWayForward(f)),
			Topic: "push/" + f.Repo + "/" + f.Branch,
			Run:   f.PairSlug(),
		}
		posted, err := notify(hubPath, notice)
		if err != nil {
			logger.Warn("fabricengine: failed-push notice not posted", "repo", f.Repo, "branch", f.Branch, "error", err)
			return f, nil
		}
		f.Notified = posted
		return f, nil
	})
}

// unionRuns returns the runs of existing followed by those of added it lacks, in order.
func unionRuns(existing, added []string) []string {
	union := slices.Clone(existing)
	for _, run := range added {
		if !slices.Contains(union, run) {
			union = append(union, run)
		}
	}
	return union
}

// readFailedPush returns the failed-push record of a repository side and branch, and whether one exists.
func readFailedPush(hubPath, repo, branch string) (fabriclayout.FailedPush, bool, error) {
	path := failedPushPath(hubPath, repo, branch)
	record, found, err := state.ReadJSON[fabriclayout.FailedPush](path, failedPushLockPath(path))
	if err != nil {
		return fabriclayout.FailedPush{}, false, fmt.Errorf("read failed-push record %s: %w", filepath.Base(path), err)
	}
	return record, found, nil
}

// ClearPushFailure removes the failed-push record of a repository side and branch, and does nothing when none exists.
func ClearPushFailure(hubPath, repo, branch string) error {
	path := failedPushPath(hubPath, repo, branch)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("clear failed-push record: %w", err)
	}
	return nil
}

// ListFailedPushes returns every failed-push record of the hub sorted by repository side and branch.
// A hub with no records returns none.
func ListFailedPushes(hubPath string) ([]fabriclayout.FailedPush, error) {
	dir := failedPushDir(hubPath)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("list failed-push records: %w", err)
	}
	var records []fabriclayout.FailedPush
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		record, found, err := state.ReadJSON[fabriclayout.FailedPush](path, failedPushLockPath(path))
		if err != nil {
			return nil, fmt.Errorf("read failed-push record %s: %w", entry.Name(), err)
		}
		if found {
			records = append(records, record)
		}
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].Repo != records[j].Repo {
			return records[i].Repo < records[j].Repo
		}
		return records[i].Branch < records[j].Branch
	})
	return records, nil
}

// FailedPushesOwedBy returns the failed-push records that hold the run slug's push owed, newest last-seen first.
// A record holds a push owed when its class is transport, deferred or teardown_lock and its Runs name the slug.
// A record of another class is no owed push, since such a push is no transient failure.
// A record whose Runs lack the slug is not the run's, even when it records the prime's records branch.
func FailedPushesOwedBy(hubPath, slug string) ([]fabriclayout.FailedPush, error) {
	records, err := ListFailedPushes(hubPath)
	if err != nil {
		return nil, err
	}
	var owed []fabriclayout.FailedPush
	for _, record := range records {
		switch record.Class {
		case fabriclayout.PushFailureTransport, fabriclayout.PushFailureDeferred, fabriclayout.PushFailureTeardownLock:
			if slices.Contains(record.Runs, slug) {
				owed = append(owed, record)
			}
		}
	}
	sort.SliceStable(owed, func(i, j int) bool { return owed[i].LastSeen.After(owed[j].LastSeen) })
	return owed, nil
}

// FailedPushFields renders the hub's failed-push records as the envelope rows a verb prints.
// Each row carries the record's repo side, branch, class and last error line.
// It also carries the first and last seen times, the count, the notified flag and the way forward for its class.
// A hub with no records returns an empty list, never nil, so the field prints as [].
func FailedPushFields(hubPath string) ([]map[string]any, error) {
	records, err := ListFailedPushes(hubPath)
	if err != nil {
		return nil, err
	}
	rows := make([]map[string]any, 0, len(records))
	for _, record := range records {
		rows = append(rows, map[string]any{
			"repo":        record.Repo,
			"branch":      record.Branch,
			"class":       string(record.Class),
			"error":       record.Error,
			"first_seen":  record.FirstSeen.Format(time.RFC3339),
			"last_seen":   record.LastSeen.Format(time.RFC3339),
			"count":       record.Count,
			"notified":    record.Notified,
			"way_forward": FailedPushWayForward(record),
		})
	}
	return rows, nil
}

// lymVerbDir returns the directory a lym verb of the code worktree at worktreeRoot runs in: its anchored directory.
// Every lym verb refuses a cwd other than that directory.
// A way forward naming the root would fail as written in an anchored hub.
// The anchor is read from the worktree's recorded marker when the line is built.
// A record written before an anchor change therefore names the current anchor.
// A worktree with no marker runs its verbs at the root, and so does one whose marker cannot be read, logged at Warn.
// An empty worktreeRoot, a records worktree with no paired code worktree, stays empty.
func lymVerbDir(worktreeRoot string) string {
	if worktreeRoot == "" {
		return ""
	}
	l, err := lymcwd.NamedWorktree(filepath.Dir(worktreeRoot), filepath.Base(worktreeRoot))
	if err != nil {
		logger.Warn("fabricengine: anchor of a way forward's worktree not read, the way forward names its root", "worktree", worktreeRoot, "error", err)
		return worktreeRoot
	}
	return l.AnchorPath()
}

// pushStep names the push that retries f, with the directory it runs in.
// A bolt record is pushed by the board sync, since the pair push never reaches Bolt.
// A prime's code branch is pushed by a plain git push, since the prime's pair push is records-only.
// Every other code branch, a beam parent's included, is pushed by the pair push.
func pushStep(f fabriclayout.FailedPush) string {
	switch {
	case f.Repo == fabriclayout.RepoBolt:
		return "`lym board sync` from the prime worktree"
	case f.Repo == fabriclayout.RepoCode && f.Prime:
		return fmt.Sprintf("`git push` in %s", f.RepoPath)
	case f.Repo == fabriclayout.RepoRecords:
		return fmt.Sprintf("`lym fabric push` in %s", lymVerbDir(f.CodeWorktree))
	default:
		return fmt.Sprintf("`lym fabric push` in %s", lymVerbDir(f.RepoPath))
	}
}

// FailedPushWayForward returns the runnable way forward for a failed push, by repository side and class.
func FailedPushWayForward(f fabriclayout.FailedPush) string {
	switch f.Class {
	case fabriclayout.PushFailureAuth:
		return fmt.Sprintf("fix this machine's permission to push %s on origin, then run %s", f.Branch, pushStep(f))
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
			return codeDivergedWayForward(f.RepoPath, f.Branch, f.Prime, unmade...)
		}
		return recordsConflictWayForward(f.RepoPath, f.Branch, f.Paths, f.Commits, pushStep(f))
	case fabriclayout.PushFailureRewound:
		return rewoundWayForward(f.RepoPath, f.Branch, f.OriginHead, f.Commits)
	case fabriclayout.PushFailureAnchorChange, fabriclayout.PushFailureHeldBack:
		return f.WayForward
	case fabriclayout.PushFailureTeardownLock:
		return fmt.Sprintf("the push retries once the removal or push holding the pair's teardown lock lets go, so run %s again then, "+
			"or end the stuck child whose pid `lym tender status` lists", pushStep(f))
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
		return fmt.Sprintf("run %s to retry the push", pushStep(f))
	}
}

// recordsConflictWayForward names a records branch origin moved past.
// It names the conflicting paths when a replay stopped on some, the local commits, and the two ways to settle it.
// The two ways run in worktree, and the second ends with pushVerb, the push step with its directory.
// It names no machine, since no commit names the machine that wrote it.
func recordsConflictWayForward(worktree, branch string, paths, commits []string, pushVerb string) string {
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

// codeDivergedWayForward names the merge that settles a code branch origin moved past.
// The merge runs at the worktree root and the push in its anchored directory.
// A prime's branch is pushed by a plain git push, since the pair push leaves the prime's code branch unpushed.
// unmade names the commits on origin's branch that this machine did not make, so the branch was not reset past them.
func codeDivergedWayForward(worktree, branch string, prime bool, unmade ...string) string {
	push := fmt.Sprintf("`lym fabric push` in %s", lymVerbDir(worktree))
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

// rewoundWayForward names the two ways to settle a task branch origin rewound to originHead.
// dropped holds the commits the rewind removed from origin.
// The git steps run at the worktree root and the push in its anchored directory.
func rewoundWayForward(worktree, branch, originHead string, dropped []string) string {
	return fmt.Sprintf("origin rewound %s to %s, dropping %s; "+
		"to accept the rewind, run `git reset --keep %s` in %s, which drops the local commits past origin's head, then `lym fabric push` in %s; "+
		"to restore the dropped commits, run `git push origin %s` in %s, a plain fast-forward",
		branch, originHead, joinOrNone(dropped), originHead, worktree, lymVerbDir(worktree), branch, worktree)
}

func joinOrNone(items []string) string {
	if len(items) == 0 {
		return "none"
	}
	return strings.Join(items, " ")
}
