// failedpush.go holds the failed-push record, one machine-local file per repository side and branch.
// It also holds the once-per-episode notice to the orch, sent through the push's notifier or the installed one.
// The way-forward line for each side and class is built there too.

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

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/lymdirs"
	"github.com/Knatte18/loomyard/internal/state"
)

// PushFailureClass names why a push did not land.
type PushFailureClass string

const (
	// PushFailureNotFastForward means origin holds commits the pushed branch lacks.
	PushFailureNotFastForward PushFailureClass = "not_fast_forward"
	// PushFailureTransport means origin could not be reached.
	PushFailureTransport PushFailureClass = "transport"
	// PushFailureAuth means origin refused this machine's credentials or permissions.
	PushFailureAuth PushFailureClass = "auth"
	// PushFailureGone means origin no longer has the branch.
	PushFailureGone PushFailureClass = "gone"
	// PushFailureOther means the push failed for a reason no other class names, so the record carries git's own error.
	PushFailureOther PushFailureClass = "other"
	// PushFailureDeferred means the push was held back, not refused, and is retried by the next push of the branch.
	PushFailureDeferred PushFailureClass = "deferred"
)

// The repository sides a failed push is recorded against.
const (
	failedPushSideCode    = "code"
	failedPushSideRecords = "records"
	failedPushSideBolt    = "bolt"
)

// The repository sides a branch outcome or failed-push record names in its Repo field.
const (
	// RepoCode is the Repo of a branch of a pair's code repository.
	RepoCode = failedPushSideCode
	// RepoRecords is the Repo of a branch of a pair's records repository.
	RepoRecords = failedPushSideRecords
	// RepoBolt is the Repo of a branch of the records repository's board branch.
	RepoBolt = failedPushSideBolt
)

// FailedPush is the record of a push origin refused, one per repository side and branch.
// Repo is "code", "records" or "bolt", the last being the records repository's board branch.
// CodeWorktree is set for a records record only: the root of the pair's code worktree.
// FabricDir is set for a code or records record: the anchored directory of the pair's code worktree,
// the one directory a fabric verb accepts as its cwd;
// a record written without it reads as anchored at the code worktree's root.
// Prime marks a code record of the hub prime's own branch.
// Paths, Commits and OriginHead are set where the failure has them, such as a replay stop or a rewind.
// WayForward is set for the anchor-change class only, which carries its own commands.
type FailedPush struct {
	Repo         string           `json:"repo"`
	RepoPath     string           `json:"repo_path"`
	CodeWorktree string           `json:"code_worktree,omitempty"`
	FabricDir    string           `json:"fabric_dir,omitempty"`
	Branch       string           `json:"branch"`
	Prime        bool             `json:"prime,omitempty"`
	Class        PushFailureClass `json:"class"`
	Error        string           `json:"error,omitempty"`
	Paths        []string         `json:"paths,omitempty"`
	Commits      []string         `json:"commits,omitempty"`
	OriginHead   string           `json:"origin_head,omitempty"`
	WayForward   string           `json:"way_forward,omitempty"`
	FirstSeen    time.Time        `json:"first_seen"`
	LastSeen     time.Time        `json:"last_seen"`
	Count        int              `json:"count"`
	Notified     bool             `json:"notified"`
}

// PushNotifier posts line to the orch of the hub at hubPath and reports whether the mail was posted.
// A notifier reporting false, because no orch is recorded, leaves the failure to be mailed by a later one.
type PushNotifier func(hubPath, line string) (posted bool, err error)

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
	return filepath.Join(HubScratchDir(hubPath), lymdirs.FailedPushDirName)
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
// While the record is not notified and its class is not deferred, the installed notifier is called once.
// The line it receives names the branch, the class, the error and the way forward.
// The record is marked notified only when the notifier reports the mail posted.
// A notifier error is logged and leaves the record unnotified, so the next failure mails again.
func RecordPushFailure(hubPath string, f FailedPush, now time.Time) error {
	return recordPushFailure(hubPath, f, now, installedNotifier())
}

// recordPushFailure writes f as the failed-push record of its repository side and branch under the record's lock.
// It mails a record that is neither notified nor deferred through notify, once per episode; a nil notify mails nothing.
func recordPushFailure(hubPath string, f FailedPush, now time.Time, notify PushNotifier) error {
	path := failedPushPath(hubPath, f.Repo, f.Branch)
	return state.UpdateJSON(path, failedPushLockPath(path), func(cur FailedPush, found bool) (FailedPush, error) {
		f.FirstSeen = now
		f.Count = 1
		if found {
			f.FirstSeen = cur.FirstSeen
			f.Count = cur.Count + 1
			f.Notified = cur.Notified
		}
		f.LastSeen = now
		if f.Notified || f.Class == PushFailureDeferred {
			return f, nil
		}
		if notify == nil {
			return f, nil
		}
		line := fmt.Sprintf("a push of %s branch %s failed (%s): %s; way forward: %s", f.Repo, f.Branch, f.Class, f.Error, FailedPushWayForward(f))
		posted, err := notify(hubPath, line)
		if err != nil {
			logger.Warn("fabricengine: failed-push notice not posted", "repo", f.Repo, "branch", f.Branch, "error", err)
			return f, nil
		}
		f.Notified = posted
		return f, nil
	})
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
func ListFailedPushes(hubPath string) ([]FailedPush, error) {
	dir := failedPushDir(hubPath)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("list failed-push records: %w", err)
	}
	var records []FailedPush
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		record, found, err := state.ReadJSON[FailedPush](path, failedPushLockPath(path))
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

// anchoredFabricDir returns the anchored directory of the code worktree at codeWorktree, where every fabric verb runs.
// The anchor is the worktree's own recorded one, read from the hub at hubPath.
// An anchor that cannot be read is logged and leaves the worktree's root.
func anchoredFabricDir(hubPath, codeWorktree string) string {
	l, err := lymcwd.NamedWorktree(hubPath, filepath.Base(codeWorktree))
	if err != nil {
		logger.Warn("fabricengine: anchor of the code worktree not read, its root is named instead", "worktree", codeWorktree, "error", err)
		return codeWorktree
	}
	return filepath.Join(codeWorktree, l.AnchorRel)
}

// fabricDir returns the directory a fabric verb named for the record runs in:
// FabricDir, or the code worktree's root for a record written without it.
func (f FailedPush) fabricDir() string {
	switch {
	case f.FabricDir != "":
		return f.FabricDir
	case f.Repo == failedPushSideRecords:
		return f.CodeWorktree
	default:
		return f.RepoPath
	}
}

// pushStep names the push that retries f, with the directory it runs in.
// A bolt record is pushed by the board sync, since the pair push never reaches Bolt.
// A prime's code branch is pushed by a plain git push, since the prime's pair push is records-only.
// Any other record is pushed by `lym fabric push` in the anchored directory of its code worktree.
func pushStep(f FailedPush) string {
	switch {
	case f.Repo == failedPushSideBolt:
		return "`lym board sync` from the prime worktree"
	case f.Repo == failedPushSideCode && f.Prime:
		return fmt.Sprintf("`git push` in %s", f.RepoPath)
	default:
		return fmt.Sprintf("`lym fabric push` in %s", f.fabricDir())
	}
}

// FailedPushWayForward returns the runnable way forward for a failed push, by repository side and class.
func FailedPushWayForward(f FailedPush) string {
	switch f.Class {
	case PushFailureAuth:
		return fmt.Sprintf("fix this machine's permission to push %s on origin, then run %s", f.Branch, pushStep(f))
	case PushFailureNotFastForward:
		if f.Repo == failedPushSideCode {
			return codeDivergedWayForward(f.RepoPath, f.fabricDir(), f.Branch, f.Prime)
		}
		return recordsConflictWayForward(f.RepoPath, f.Branch, f.Paths, f.Commits, pushStep(f))
	case PushFailureRewound:
		return rewoundWayForward(f.RepoPath, f.fabricDir(), f.Branch, f.OriginHead, f.Commits)
	case PushFailureAnchorChange:
		return f.WayForward
	case PushFailureGone:
		pairWorktree := f.RepoPath
		if f.Repo == failedPushSideRecords {
			pairWorktree = f.CodeWorktree
		}
		return fmt.Sprintf("origin no longer has branch %s, and its board entry is neither done nor removed on origin, since an ended entry writes no record; "+
			"decide whether the run ends: `lym fabric remove` of the pair at %s, run from the prime, removes the pair and clears this record; "+
			"once origin's board shows the entry done or removed, `lym cleanup --apply` tears it down instead",
			f.Branch, pairWorktree)
	default:
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
// The merge runs in worktree, and the push that follows in fabricDir, the worktree's anchored directory.
// A prime's branch is pushed by a plain git push in worktree, since the pair push leaves the prime's code branch unpushed.
func codeDivergedWayForward(worktree, fabricDir, branch string, prime bool) string {
	push := fmt.Sprintf("`lym fabric push` in %s", fabricDir)
	if prime {
		push = fmt.Sprintf("`git push` in %s", worktree)
	}
	return fmt.Sprintf("origin/%s holds commits this branch lacks; run `git merge origin/%s` in %s, settle any conflict and commit, then run %s",
		branch, branch, worktree, push)
}

// rewoundWayForward names the two ways to settle a task branch origin rewound to originHead.
// dropped holds the commits the rewind removed from origin.
// The git steps run in worktree, and `lym fabric push` in fabricDir, the worktree's anchored directory.
func rewoundWayForward(worktree, fabricDir, branch, originHead string, dropped []string) string {
	return fmt.Sprintf("origin rewound %s to %s, dropping %s; "+
		"to accept the rewind, run `git reset --keep %s` in %s, which drops the local commits past origin's head, then `lym fabric push` in %s; "+
		"to restore the dropped commits, run `git push origin %s` in %s, a plain fast-forward",
		branch, originHead, joinOrNone(dropped), originHead, worktree, fabricDir, branch, worktree)
}

func joinOrNone(items []string) string {
	if len(items) == 0 {
		return "none"
	}
	return strings.Join(items, " ")
}
