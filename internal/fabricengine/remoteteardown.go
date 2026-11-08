// remoteteardown.go implements the remote half of a teardown, which no command waits on.
// A removal, a task-branch discard and a cleanup sweep finish their local steps;
// then they hand a job of remote steps to a detached child.
// The child takes the pair's teardown lock exclusive before the command's first mutation;
// it holds the lock until the job's last remote step ends;
// so no push, Add or other teardown of the slug runs in between.
// The file holds the job and the hold through which a command keeps the lock across the handover.
// It also holds the child's body and the notice of a failed remote step.

package fabricengine

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/lock"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/runproc"
	"github.com/Knatte18/loomyard/internal/selfexec"
)

// The hidden fabric verb and flags that start the teardown child.
const (
	// TeardownChildVerb is the hidden fabric subcommand that runs the remote half of a teardown.
	TeardownChildVerb = "remote-teardown"
	// TeardownChildSlugFlag names the slug whose teardown lock the child holds.
	TeardownChildSlugFlag = "slug"
	// TeardownChildTryFlag makes the child take the lock by one attempt instead of waiting for it.
	TeardownChildTryFlag = "try"
	// TeardownChildWaitFlag bounds the child's wait for the lock.
	TeardownChildWaitFlag = "wait"
)

// The one line the teardown child answers on its signal stream once it has tried the lock.
const (
	teardownHeldSignal = "held"
	teardownBusySignal = "busy"
)

// The kinds of remote teardown job, a closed set.
const (
	remoteTeardownRemove  = "remove"
	remoteTeardownCleanup = "cleanup"
	remoteTeardownDiscard = "discard"
)

// remoteTeardownJob is the remote half of one teardown.
// The command writes it, and the holder of the slug's teardown lock runs it.
type remoteTeardownJob struct {
	// Kind is one of the remoteTeardown kinds.
	Kind string `json:"kind"`
	Slug string `json:"slug"`
	// Worktree is the warp worktree the command ran from, the location every step runs from.
	Worktree string `json:"worktree"`
	// WeftBranch is the pair's records branch as the command read it from the pair;
	// since the pair's worktrees are gone when the job runs.
	WeftBranch string `json:"weft_branch,omitempty"`
	// WeftTip is the local weft tip the command tagged before it deleted the branch.
	// It is empty when no local branch was left, so the child reads origin's.
	WeftTip string `json:"weft_tip,omitempty"`
	// DeleteWeftOnOrigin asks for the weft branch's delete on origin, leased on the origin tip the archive read.
	DeleteWeftOnOrigin bool `json:"delete_weft_on_origin,omitempty"`
	// WarpBranch is the pair's task branch as the command read it from the pair;
	// since the pair's worktrees are gone when the job runs.
	WarpBranch string `json:"warp_branch,omitempty"`
	// ParentBranch is the pair's recorded parent, which gates the task branch's delete on origin.
	ParentBranch       string `json:"parent_branch,omitempty"`
	DeleteTaskOnOrigin bool   `json:"delete_task_on_origin,omitempty"`
	// TaskTip is the local task tip a discard tagged and deleted, empty when no local task branch existed.
	TaskTip string `json:"task_tip,omitempty"`
	// ExemptPause is set when the command that wrote the job runs exempt from every pause;
	// so the child's remote steps pass under a pause as the command's own did.
	ExemptPause bool `json:"exempt_pause,omitempty"`
}

// remoteOutcome is what the remote steps of a job did.
// Only an outcome whose steps ran in process carries anything;
// a handed-over job's outcome is the zero value with handedOff set.
type remoteOutcome struct {
	ran       bool
	handedOff bool
	// archive and archiveSkipped are the weft archive of a removal or a cleanup.
	archive        weftArchive
	archiveSkipped string
	// weft carries the outcome of the weft branch's delete on origin.
	weft CleanupBranchEntry
	// weftKeptOnOrigin is set when the weft branch lives only on origin and the job did not ask for its delete.
	weftKeptOnOrigin string
	taskDeleted      bool
	// taskKept is why the task branch on origin was left in place;
	// taskFailed marks it as a failed lookup, fetch or delete, not a refusal or a moved tip.
	taskKept   string
	taskFailed bool
	// discard carries the archive tags and the origin delete of a discard.
	discard DiscardResult
	// err is the failure that stopped the steps.
	err error
	// weftRootErr is the failure to find the records repository after the archive succeeded;
	// it is also err, and it tells the notice the archive itself did not fail.
	weftRootErr error
}

// runRemoteTeardown runs job's remote steps from the location l of job.Worktree, recording on rec.
// The sleep function is called between archive-tag push attempts.
// A removal and a cleanup archive the weft tips and then delete the weft branch on origin when asked.
// A removal pushes the weft tip first and then deletes the landed task branch on origin when asked.
// A discard archives the task branch's tips and deletes it on origin.
func runRemoteTeardown(rec *Mutations, l *lymcwd.Location, job remoteTeardownJob, sleep func(time.Duration)) remoteOutcome {
	out := remoteOutcome{ran: true, weft: CleanupBranchEntry{Branch: job.WeftBranch}}
	if job.Kind == remoteTeardownDiscard {
		out.discard, out.err = discardTaskBranchOnOrigin(rec, l, job, sleep)
		return out
	}

	out.archive, out.archiveSkipped, out.err = archiveWeftTip(rec, l, job.Slug, job.WeftBranch, job.WeftTip, job.Kind == remoteTeardownRemove, sleep)
	if out.err != nil {
		return out
	}
	if out.archiveSkipped == "" && out.archive.leaseTip != "" {
		if job.DeleteWeftOnOrigin {
			weftRoot, err := RecordsRepoRoot(l)
			if err != nil {
				out.weftRootErr = fmt.Errorf("resolve weft repo root: %w", err)
				out.err = out.weftRootErr
				return out
			}
			deleteWeftBranchOnRemote(rec, l, job.WeftBranch, weftRoot, out.archive.leaseTip, &out.weft)
		} else if job.WeftTip == "" {
			out.weftKeptOnOrigin = fmt.Sprintf("the sibling branch %q exists only on %q and was kept; pass --remote to delete it", job.WeftBranch, originRemoteName)
		}
	}
	if job.DeleteTaskOnOrigin {
		out.taskDeleted, out.taskKept, out.taskFailed = deleteTaskBranchOnOrigin(rec, l, job.WarpBranch, job.ParentBranch)
	}
	return out
}

// remoteStepFailure is one failed remote step of a teardown job and the way forward that settles it.
type remoteStepFailure struct {
	operation  string
	reason     string
	wayForward string
}

// discardRerunWayForward is the way forward of a discard's failed remote step, whose command has no fixed spelling.
const discardRerunWayForward = "run the command that discarded the task branch again"

// unknownKindWayForward returns the way forward for a remote teardown of slug whose job kind is not known:
// the re-run of each command that starts one.
func unknownKindWayForward(slug string) string {
	return fmt.Sprintf("run `lym fabric remove %s --remote` again, `lym fabric cleanup --apply --remote` when a cleanup sweep started it, or the command that discarded the task branch when a discard started it", slug)
}

// rerunWayForward returns the re-run that retries the remote steps of job:
// the cleanup sweep for a cleanup job, else the removal of its slug.
// It serves a step that leaves its branch on origin, which is what lets the re-run find the pair.
func rerunWayForward(job remoteTeardownJob) string {
	if job.Kind == remoteTeardownCleanup {
		return "run `lym fabric cleanup --apply --remote` again"
	}
	return fmt.Sprintf("run `lym fabric remove %s --remote` again", job.Slug)
}

// failedRemoteSteps returns every failed remote step of job's outcome, in the order the steps ran.
// A step that stopped the job leaves the later steps unrun, so their outcome fields are empty.
// weftRoot is the records repository's root, empty when it could not be found.
// In that case no manual tag push is named for the records archive.
func failedRemoteSteps(job remoteTeardownJob, out remoteOutcome, weftRoot string) []remoteStepFailure {
	var failures []remoteStepFailure
	switch {
	case out.err != nil && job.Kind == remoteTeardownDiscard:
		wayForward := discardRerunWayForward
		if job.TaskTip != "" {
			wayForward = fmt.Sprintf("push the archive tag with `git -C %s push %s refs/tags/%s`, then %s", job.Worktree, originRemoteName, ArchiveTagName(job.Slug, job.TaskTip), wayForward)
		}
		failures = append(failures, remoteStepFailure{"the archive and origin delete of its task branch", out.err.Error(), wayForward})
	case out.weftRootErr != nil:
		failures = append(failures, remoteStepFailure{"the lookup of its records repository", out.weftRootErr.Error(), rerunWayForward(job)})
	case out.err != nil:
		wayForward := rerunWayForward(job)
		if job.WeftTip != "" {
			if job.Kind == remoteTeardownRemove {
				// A removal whose branches never reached origin leaves its re-run nothing to find.
				wayForward = "when origin still holds a branch of the pair, " + wayForward
			}
			if weftRoot != "" {
				wayForward = fmt.Sprintf("push the archive tag with `git -C %s push %s refs/tags/%s`, then %s", weftRoot, originRemoteName, ArchiveTagName(job.Slug, job.WeftTip), wayForward)
			}
		}
		failures = append(failures, remoteStepFailure{"the archive of its records", out.err.Error(), wayForward})
	}
	if out.weft.RemoteError != "" {
		failures = append(failures, remoteStepFailure{"the delete of its records branch on origin", out.weft.RemoteError, rerunWayForward(job)})
	}
	if out.taskFailed {
		failures = append(failures, remoteStepFailure{"the delete of its task branch on origin", out.taskKept, rerunWayForward(job)})
	}
	return failures
}

// noticeRemoteTeardown logs every failed remote step of job and mails the hub's orch one line.
// The line names the slug and, for each failed step, the operation and the way forward.
// A kept branch is no failure and mails nothing.
// The weftRoot argument is the records repository's root, empty when it could not be found.
func noticeRemoteTeardown(hubPath string, job remoteTeardownJob, out remoteOutcome, weftRoot string) {
	failures := failedRemoteSteps(job, out, weftRoot)
	if len(failures) == 0 {
		logger.Info("fabricengine: remote teardown done", "slug", job.Slug, "kind", job.Kind, "weft_kept", out.weft.RemoteKeptReason, "task_kept", out.taskKept)
		return
	}
	clauses := make([]string, len(failures))
	for i, failure := range failures {
		clauses[i] = fmt.Sprintf("at %s: %s; way forward: %s", failure.operation, failure.reason, failure.wayForward)
	}
	postRemoteTeardownNotice(hubPath, job.Slug, fmt.Sprintf("the remote teardown of %s failed %s", job.Slug, strings.Join(clauses, "; and ")))
}

// noticeUnreadRemoteTeardown logs and mails that the remote steps of slug's teardown never ran, because their job could not be read.
func noticeUnreadRemoteTeardown(hubPath, slug string, readErr error) {
	postRemoteTeardownNotice(hubPath, slug, fmt.Sprintf("the remote teardown of %s failed at reading its job: %v; way forward: %s", slug, readErr, unknownKindWayForward(slug)))
}

// NoticeRemoteTeardownEnded logs and mails that the remote teardown of slug ended before its last step, for the reason given.
// The way forward names each command that starts a remote teardown.
// A caller ended by a signal, or unable to write the job, does not know the job's kind.
func NoticeRemoteTeardownEnded(hubPath, slug, reason string) {
	postRemoteTeardownNotice(hubPath, slug, fmt.Sprintf("the remote teardown of %s ended before its last step: %s; way forward: %s", slug, reason, unknownKindWayForward(slug)))
}

// postRemoteTeardownNotice logs line at Warn and mails it to the hub's orch through the installed notifier; with none installed it only logs.
func postRemoteTeardownNotice(hubPath, slug, line string) {
	logger.Warn("fabricengine: "+line, "slug", slug)
	notifier := installedPushNotifier.Load()
	if notifier == nil {
		return
	}
	if _, err := (*notifier)(hubPath, line); err != nil {
		logger.Warn("fabricengine: remote teardown notice not posted", "slug", slug, "error", err)
	}
}

// teardownHold keeps a slug's teardown lock exclusive until the job handed to finish has run.
type teardownHold interface {
	// finish runs or hands over job and releases the lock once its last remote step ends; a nil job releases it at once.
	finish(job *remoteTeardownJob) remoteOutcome
}

// holdTeardown takes slug's teardown lock exclusive for a teardown whose remote steps follow its local ones.
// It waits for the lock, or with try set makes one attempt and refuses when the lock is held.
// With handOff set, outside a test binary, a detached child takes the lock and holds it:
// finish hands the child the job and returns at once, and the child releases the lock after the job's last remote step.
// Otherwise the lock is held in process, and finish runs the job before it releases the lock,
// so the verb's result carries the remote outcome.
// A lock file the call brings into being is recorded in lockRec, and an in-process job's steps in rec.
func (t *Topology) holdTeardown(lockRec, rec *Mutations, l *lymcwd.Location, slug string, try bool, rerun string, handOff bool) (teardownHold, error) {
	if !handOff || testing.Testing() {
		release, err := t.lockPairTeardown(lockRec, l, slug, try, rerun)
		if err != nil {
			return nil, err
		}
		return &inProcessHold{rec: rec, location: l, release: release, sleep: t.archiveSleep}, nil
	}
	exe, err := selfexec.LymBinary()
	if err != nil {
		return nil, fmt.Errorf("start the teardown child of %q: %w", slug, err)
	}
	return spawnTeardownHold(lockRec, l, slug, try, exe, teardownChildArgs(slug, try, t.teardownWait), rerun)
}

// inProcessHold is a teardown lock this process holds.
type inProcessHold struct {
	rec      *Mutations
	location *lymcwd.Location
	release  func()
	sleep    func(time.Duration)
}

// finish runs job in process, then releases the lock.
func (h *inProcessHold) finish(job *remoteTeardownJob) remoteOutcome {
	defer h.release()
	if job == nil {
		return remoteOutcome{}
	}
	return runRemoteTeardown(h.rec, h.location, *job, h.sleep)
}

// childHold is a teardown lock a detached child holds for this process.
type childHold struct {
	hubPath string
	slug    string
	cmd     *exec.Cmd
	stdin   io.WriteCloser
}

// finish writes job to the child and returns at once, leaving the child to run it.
// The job is marked with whether this process runs exempt from every pause.
// A nil job closes the child's input and waits for it to release the lock.
// A job that cannot be written is logged and mailed with its way forward.
// The child then reads no job and releases the lock.
func (h *childHold) finish(job *remoteTeardownJob) remoteOutcome {
	if job == nil {
		_ = h.stdin.Close()
		_ = h.cmd.Wait()
		return remoteOutcome{}
	}
	handed := *job
	handed.ExemptPause = pauserecord.GateExemptsAll()
	if err := json.NewEncoder(h.stdin).Encode(handed); err != nil {
		NoticeRemoteTeardownEnded(h.hubPath, h.slug, fmt.Sprintf("its job could not be handed to the child: %v", err))
	}
	_ = h.stdin.Close()
	go func() { _ = h.cmd.Wait() }()
	return remoteOutcome{handedOff: true}
}

// teardownChildArgs returns the arguments of the teardown child for slug.
func teardownChildArgs(slug string, try bool, wait time.Duration) []string {
	args := []string{"fabric", TeardownChildVerb, "--" + TeardownChildSlugFlag, slug, "--" + TeardownChildWaitFlag, wait.String()}
	if try {
		args = append(args, "--"+TeardownChildTryFlag)
	}
	return args
}

// spawnTeardownHold starts the detached teardown child of slug, running exe with args in the hub's Bolt checkout.
// It returns the child's hold once the child answers that it holds the lock.
// A child that answers busy returns the refusal for a held lock, after the child has exited;
// any other answer is an error naming it.
// The lock file the child brings into being is recorded in rec.
// The child is registered under the slug in the hub's owner record with the teardown role.
// A teardown of the slug's pair spares that role.
func spawnTeardownHold(rec *Mutations, l *lymcwd.Location, slug string, try bool, exe string, args []string, rerun string) (teardownHold, error) {
	lockPath := TeardownLockPath(l.HubPath, slug)
	existed := pathPresent(lockPath)
	cmd := exec.Command(exe, args...)
	cmd.Dir = BoltDir(l.HubPath)
	proc.Detach(cmd)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("start the teardown child of %q: %w", slug, err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("start the teardown child of %q: %w", slug, err)
	}
	if err := cmd.Start(); err != nil {
		logger.Warn("fabricengine: spawn teardown child failed", "exe", exe, "args", args, "error", err)
		return nil, fmt.Errorf("start the teardown child of %q: %w", slug, err)
	}
	pid := cmd.Process.Pid
	logger.Info("fabricengine: spawned teardown child", "exe", exe, "args", args, "dir", cmd.Dir, "pid", pid)
	if err := runproc.Register(RunProcDir(filepath.Dir(cmd.Dir)), slug, runproc.ChildRecord(pid, runproc.RoleTeardown, args)); err != nil {
		logger.Warn("fabricengine: teardown child not registered", "slug", slug, "pid", pid, "error", err)
	}

	line, readErr := bufio.NewReader(stdout).ReadString('\n')
	if !existed && pathPresent(lockPath) {
		rec.Append(KindFileWritten, lockPath, "")
	}
	answer := strings.TrimSpace(line)
	if answer == teardownHeldSignal {
		return &childHold{hubPath: l.HubPath, slug: slug, cmd: cmd, stdin: stdin}, nil
	}
	_ = stdin.Close()
	_ = cmd.Wait()
	if answer == teardownBusySignal {
		return nil, teardownLockRefusal(slug, try, rerun)
	}
	if answer == "" && readErr != nil {
		answer = readErr.Error()
	}
	return nil, fmt.Errorf("the teardown child of %q did not take the teardown lock: %s", slug, answer)
}

// RunTeardownChild is the body of the detached teardown child of slug in the hub at hubPath.
// It takes the slug's teardown lock exclusive: with try set, in one attempt; otherwise waiting at most wait.
// It writes one line to signal: held, busy, or the error that kept it from the lock.
// Holding the lock, it reads the job from jobIn until the parent closes it.
// It runs the job's remote steps, exempt from every pause when the job says its command was, and releases the lock.
// An empty input means the parent ended its teardown without remote steps; the child then releases the lock at once.
// A failed remote step, or a job it cannot read, is logged and mailed to the hub's orch as one line.
// The line names the slug, the operation and the way forward.
func RunTeardownChild(hubPath, slug string, try bool, wait time.Duration, jobIn io.Reader, signal io.Writer) error {
	lockPath := TeardownLockPath(hubPath, slug)
	held, acquired, err := takeTeardownChildLock(hubPath, slug, lockPath, try, wait)
	if err != nil {
		fmt.Fprintf(signal, "error %v\n", err)
		return err
	}
	if !acquired {
		fmt.Fprintln(signal, teardownBusySignal)
		return nil
	}
	defer func() { _ = held.Release() }()
	if _, err := fmt.Fprintln(signal, teardownHeldSignal); err != nil {
		err = fmt.Errorf("answer the teardown parent of %q: %w", slug, err)
		NoticeRemoteTeardownEnded(hubPath, slug, err.Error())
		return err
	}

	raw, err := io.ReadAll(jobIn)
	if err != nil {
		err = fmt.Errorf("read the remote teardown job of %q: %w", slug, err)
		noticeUnreadRemoteTeardown(hubPath, slug, err)
		return err
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		logger.Info("fabricengine: the teardown ended with no remote steps", "slug", slug)
		return nil
	}
	var job remoteTeardownJob
	if err := json.Unmarshal(raw, &job); err != nil {
		err = fmt.Errorf("read the remote teardown job of %q: %w", slug, err)
		noticeUnreadRemoteTeardown(hubPath, slug, err)
		return err
	}
	if job.ExemptPause {
		defer pauserecord.SetGateExemptAll()()
	}
	l, err := lymcwd.ResolveWorktree(job.Worktree)
	if err != nil {
		out := remoteOutcome{err: fmt.Errorf("resolve the worktree %s: %w", job.Worktree, err)}
		noticeRemoteTeardown(hubPath, job, out, "")
		return out.err
	}
	out := runRemoteTeardown(NewMutations(hubPath), l, job, nil)
	weftRoot := ""
	if out.err != nil {
		if root, rootErr := RecordsRepoRoot(l); rootErr == nil {
			weftRoot = root
		}
	}
	noticeRemoteTeardown(hubPath, job, out, weftRoot)
	return nil
}

// takeTeardownChildLock takes the teardown lock at lockPath for the child, creating its directory first.
func takeTeardownChildLock(hubPath, slug, lockPath string, try bool, wait time.Duration) (*lock.FileLock, bool, error) {
	if err := ensureTeardownLockDir(hubPath, slug); err != nil {
		return nil, false, err
	}
	if try {
		return lock.TryAcquireWriteLock(lockPath)
	}
	return lock.AcquireWriteLockWithin(lockPath, wait)
}

// discardTaskBranchOnOrigin is the remote half of a discard.
// It reads the task branch's origin tip and archives the tips the discard must keep;
// it deletes the branch on origin, leased on that tip.
// job.TaskTip, the local tip the discard tagged and deleted, is archived only when origin's tip does not contain it.
func discardTaskBranchOnOrigin(rec *Mutations, l *lymcwd.Location, job remoteTeardownJob, sleep func(time.Duration)) (res DiscardResult, err error) {
	repoDir := l.WorktreePath()
	originTip := ""
	if _, urlErr := gitrepo.New(repoDir).RemoteURL(originRemoteName); urlErr == nil {
		originTip, err = remoteHeadTip(repoDir, job.WarpBranch)
		if err != nil {
			return DiscardResult{}, err
		}
	}
	if originTip == "" && job.TaskTip == "" {
		return DiscardResult{}, nil
	}
	if originTip != "" {
		if _, err := gitrepo.New(repoDir).FetchWithArgs(originRemoteName, "refs/heads/"+job.WarpBranch); err != nil {
			return DiscardResult{}, fmt.Errorf("fetch task branch %q from %q: %w", job.WarpBranch, originRemoteName, err)
		}
	}

	localTag, originTag, err := archiveDiscardedTips(rec, repoDir, job.Slug, job.TaskTip, originTip, sleep)
	if err != nil {
		return DiscardResult{}, err
	}
	for _, tag := range []string{localTag, originTag} {
		if tag != "" && (len(res.ArchiveTags) == 0 || res.ArchiveTags[0] != tag) {
			res.ArchiveTags = append(res.ArchiveTags, tag)
		}
	}
	_, res.OriginDeleted, err = deleteArchivedTaskBranch(rec, repoDir, job.WarpBranch, "", originTag, originTip)
	return res, err
}
