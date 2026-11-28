// failedpush.go holds the failed-push record, one machine-local file per repository side and branch.
// It also holds the once-per-episode notice to the orch, sent through the push's notifier or the installed one.
// It reads the directory each way-forward line runs its verbs in.

package fabricengine

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Knatte18/loomyard/internal/fabriccore"
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
		merged, due := fabriccore.MergeFailedPush(cur, found, f, now)
		if !due || notify == nil {
			return merged, nil
		}
		notice := fabriccore.FailedPushNotice(merged, verbDirsOf(merged.RepoPath, merged.CodeWorktree))
		posted, err := notify(hubPath, notice)
		if err != nil {
			logger.Warn("fabricengine: failed-push notice not posted", "repo", merged.Repo, "branch", merged.Branch, "error", err)
			return merged, nil
		}
		merged.Notified = posted
		return merged, nil
	})
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
	return fabriccore.OwedBy(records, slug), nil
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

// verbDirsOf returns the directory a lym verb of each non-empty code worktree runs in,
// so a way forward is built from directories already read.
func verbDirsOf(worktrees ...string) fabriccore.VerbDirs {
	dirs := fabriccore.VerbDirs{}
	for _, worktree := range worktrees {
		if worktree != "" {
			dirs[worktree] = lymVerbDir(worktree)
		}
	}
	return dirs
}

// pushStep names the push that retries f, with the directory it runs in.
func pushStep(f fabriclayout.FailedPush) string {
	return fabriccore.PushStep(f, verbDirsOf(f.RepoPath, f.CodeWorktree))
}

// FailedPushWayForward returns the runnable way forward for a failed push, by repository side and class.
func FailedPushWayForward(f fabriclayout.FailedPush) string {
	return fabriccore.FailedPushWayForward(f, verbDirsOf(f.RepoPath, f.CodeWorktree))
}

// recordsConflictWayForward names a records branch origin moved past and the two ways to settle it in worktree.
func recordsConflictWayForward(worktree, branch string, paths, commits []string, pushVerb string) string {
	return fabriccore.RecordsConflictWayForward(worktree, branch, paths, commits, pushVerb)
}

// codeDivergedWayForward names the merge that settles a code branch origin moved past.
// unmade names the commits on origin's branch that this machine did not make.
func codeDivergedWayForward(worktree, branch string, prime bool, unmade ...string) string {
	return fabriccore.CodeDivergedWayForward(worktree, branch, prime, verbDirsOf(worktree), unmade...)
}

// rewoundWayForward names the two ways to settle a task branch origin rewound to originHead.
func rewoundWayForward(worktree, branch, originHead string, dropped []string) string {
	return fabriccore.RewoundWayForward(worktree, branch, originHead, dropped, verbDirsOf(worktree))
}

func joinOrNone(items []string) string {
	if len(items) == 0 {
		return "none"
	}
	return strings.Join(items, " ")
}
