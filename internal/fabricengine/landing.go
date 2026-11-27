// landing.go holds fabric's git steps on a branch a run landed on:
// the reads of the landing commits origin lacks, the records hold they cast,
// and the push of the prime's code branch with its refusal record.

package fabricengine

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/dotgit"
	"github.com/Knatte18/loomyard/internal/fabriclayout"
	"github.com/Knatte18/loomyard/internal/gitexec"
	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/pauserecord"
	"github.com/Knatte18/loomyard/internal/weftname"
)

// heldRecordsRefPrefix names a local ref of a records repository, one per run whose landing was dropped.
// The ref keeps the run's held-back records commit, and no push sends it.
const heldRecordsRefPrefix = "refs/held-records/"

// PushPrimeCodeTip pushes the code branch of the prime to tip, fast-forward only and never forced.
// It sends no commit past tip, and it makes one transport attempt, since its caller retries on its own schedule.
// A push that cannot reach origin returns an owed *PushFailedError, unrecorded when opts.OwedUnrecorded is set.
// A rejection that stays is recorded as one failed push of the prime's code branch.
// That record's way forward names the landing commits origin lacks.
// It does nothing when opts.SkipGit or opts.SkipPush is set.
func PushPrimeCodeTip(prime *lymcwd.Location, tip string, opts SyncOptions) error {
	if opts.SkipGit || opts.SkipPush {
		return nil
	}
	leaveGate := pauserecord.Admit(pushLockGateOp)
	defer leaveGate()

	commits, err := LandingCommits(prime)
	if err != nil {
		return err
	}
	opts.landings = slices.Sorted(maps.Values(commits))
	opts.SingleAttempt = true
	opts.ceiling = tip
	return pushClassified(NewMutations(prime.HubPath), prime.HubPath, fabriclayout.RepoCode, prime.WorktreePath(), opts, nil)
}

// PrimeCodeRefusalStands reports whether a refused-push record stands for the code branch of the prime.
func PrimeCodeRefusalStands(prime *lymcwd.Location) (bool, error) {
	records, err := ListFailedPushes(prime.HubPath)
	if err != nil {
		return false, err
	}
	var refused []string
	for _, record := range records {
		if record.Repo == fabriclayout.RepoCode && !isOwedClass(record.Class) {
			refused = append(refused, record.Branch)
		}
	}
	if len(refused) == 0 {
		return false, nil
	}
	branch, err := gitrepo.New(prime.WorktreePath()).CurrentBranch()
	if err != nil {
		return false, fmt.Errorf("fabricengine: read the code branch of %s: %w", prime.WorktreePath(), err)
	}
	return slices.Contains(refused, branch), nil
}

// ClearPrimeCodeRefusal removes the failed-push record of the prime's code branch, and does nothing when none exists.
func ClearPrimeCodeRefusal(prime *lymcwd.Location) error {
	branch, err := gitrepo.New(prime.WorktreePath()).CurrentBranch()
	if err != nil {
		return fmt.Errorf("fabricengine: read the code branch of %s: %w", prime.WorktreePath(), err)
	}
	return ClearPushFailure(prime.HubPath, fabriclayout.RepoCode, branch)
}

// branchCommit is one commit of a first-parent chain.
type branchCommit struct {
	sha     string
	message string
}

// commitsPastUpstream returns the first-parent commits of branch past its tracking ref, newest first.
// tracked is false for a branch with no tracking ref, which lists none.
func commitsPastUpstream(dir, branch string) (commits []branchCommit, tracked bool, err error) {
	upstream, ok, err := gitrepo.New(dir).Upstream(branch)
	if err != nil || !ok {
		return nil, false, err
	}
	out, err := gitexec.Run([]string{"log", "--first-parent", "-z", "--format=%H%n%B", upstream + "..refs/heads/" + branch, "--"}, dir)
	if err != nil {
		return nil, false, fmt.Errorf("fabricengine: list the commits of %s past its tracking ref in %s: %w", branch, dir, err)
	}
	for _, entry := range strings.Split(out, "\x00") {
		sha, message, _ := strings.Cut(strings.TrimLeft(entry, "\n"), "\n")
		if sha != "" {
			commits = append(commits, branchCommit{sha: sha, message: message})
		}
	}
	return commits, true, nil
}

// landingCommitsOn returns the landing commits of branch past its tracking ref, keyed by the run each names.
// A run named twice keeps its newest commit.
func landingCommitsOn(dir, branch string) (map[string]string, error) {
	commits, _, err := commitsPastUpstream(dir, branch)
	if err != nil {
		return nil, err
	}
	landings := map[string]string{}
	for _, commit := range commits {
		run := trailerValue(commit.message, fabriclayout.LandingTrailerKey)
		if _, seen := landings[run]; run != "" && !seen {
			landings[run] = commit.sha
		}
	}
	return landings, nil
}

// LandingCommits returns the landing commits of the branch checked out at l that origin's tracking ref lacks.
// Each is keyed by the run its landing trailer names.
// Only the branch's first-parent chain is read.
// A detached HEAD and a branch with no tracking ref list none. It makes no network call.
func LandingCommits(l *lymcwd.Location) (map[string]string, error) {
	branch, detached, err := gitrepo.New(l.WorktreePath()).HeadRef()
	if err != nil {
		return nil, fmt.Errorf("fabricengine: read the branch of %s: %w", l.WorktreePath(), err)
	}
	if detached {
		return map[string]string{}, nil
	}
	return landingCommitsOn(l.WorktreePath(), branch)
}

// UnsettledLandings returns the landing commits LandingCommits lists at l whose run's board entry is not done.
// A worktree that does not exist lists none.
func UnsettledLandings(l *lymcwd.Location) (map[string]string, error) {
	if _, err := os.Stat(l.WorktreePath()); errors.Is(err, fs.ErrNotExist) {
		return map[string]string{}, nil
	}
	landings, err := LandingCommits(l)
	if err != nil {
		return nil, err
	}
	return unsettledOf(l.HubPath, landings)
}

// unsettledOf drops from landings every run whose board entry is done.
func unsettledOf(hubPath string, landings map[string]string) (map[string]string, error) {
	for run := range landings {
		done, err := landingRunDone(hubPath, run)
		if err != nil {
			return nil, err
		}
		if done {
			delete(landings, run)
		}
	}
	return landings, nil
}

// landingRunDone reports whether the hub's local board holds the entry slug as done.
// It reads the board's file first, since that file holds a done mark not yet committed.
// An entry the file does not list is read from the board's history.
func landingRunDone(hubPath, slug string) (bool, error) {
	boardDir := fabriclayout.BoltDir(hubPath)
	if _, err := os.Stat(boardDir); errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	ledger, _, err := runLedger()
	if err != nil {
		return false, err
	}
	data, err := os.ReadFile(filepath.Join(boardDir, filepath.FromSlash(ledger.File())))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false, fmt.Errorf("fabricengine: read the board for the run %q: %w", slug, err)
	}
	if err == nil {
		runs, err := ledger.Runs(data)
		if err != nil {
			return false, fmt.Errorf("fabricengine: read the board for the run %q: %w", slug, err)
		}
		for _, run := range runs {
			if run.Slug == slug {
				return run.Ended, nil
			}
		}
	}
	ended, err := readEndedSet(boardDir, "HEAD", []string{slug}, nil)
	if err != nil {
		return false, err
	}
	return ended[slug] == endedDone, nil
}

// AheadByOnly reports whether the branch at l is ahead of its tracking ref by commits among tips alone.
// Only first-parent commits count.
// A level branch reads true, and a branch with no tracking ref false.
func AheadByOnly(l *lymcwd.Location, tips []string) (bool, error) {
	branch, err := gitrepo.New(l.WorktreePath()).CurrentBranch()
	if err != nil {
		return false, fmt.Errorf("fabricengine: read the branch of %s: %w", l.WorktreePath(), err)
	}
	commits, tracked, err := commitsPastUpstream(l.WorktreePath(), branch)
	if err != nil || !tracked {
		return false, err
	}
	return !slices.ContainsFunc(commits, func(commit branchCommit) bool {
		return !slices.Contains(tips, commit.sha)
	}), nil
}

// KeepHeldRecords keeps commit, a records commit of the run slug, under a local ref of the records repository of pair.
// While the ref stands no push of the run's records sends that commit.
func KeepHeldRecords(pair *lymcwd.Location, slug, commit string) error {
	if _, err := gitexec.Run([]string{"update-ref", heldRecordsRefPrefix + slug, commit}, RecordsWorktree(pair)); err != nil {
		return fmt.Errorf("fabricengine: keep the held records commit %s of %q: %w", commit, slug, err)
	}
	return nil
}

// dropHeldRecordsRef deletes the held-records ref of slug from the records repository of l's hub.
// A failure is logged, since a ref of a removed pair holds nothing back.
func dropHeldRecordsRef(l *lymcwd.Location, slug string) {
	root, err := RecordsRepoRoot(l)
	if err == nil {
		_, err = gitexec.Run([]string{"update-ref", "-d", heldRecordsRefPrefix + slug}, root)
	}
	if err != nil {
		logger.Warn("fabricengine: held-records ref of a removed pair not deleted", "slug", slug, "error", err)
	}
}

// OldestUnpushedRecords returns the oldest records commit of pair that no origin branch holds, "" when there is none.
// It reads local refs only.
func OldestUnpushedRecords(pair *lymcwd.Location) (string, error) {
	return oldestUnpushed(RecordsWorktree(pair))
}

// oldestUnpushed returns the oldest first-parent commit of the records worktree at dir that no origin branch holds.
// It returns "" when there is none.
func oldestUnpushed(dir string) (string, error) {
	out, err := gitexec.Run([]string{"rev-list", "--first-parent", "HEAD", "--not", "--remotes=" + originRemoteName}, dir)
	if err != nil {
		return "", fmt.Errorf("fabricengine: list the unpushed commits of %s: %w", dir, err)
	}
	commits := strings.Fields(out)
	if len(commits) == 0 {
		return "", nil
	}
	return commits[len(commits)-1], nil
}

// isCheckout reports whether dir holds a usable git checkout.
// A directory a removal left behind, with no repository in it, is none.
func isCheckout(dir string) bool {
	geometry, err := dotgit.Read(dir)
	return err == nil && dotgit.IsRepository(geometry)
}

// heldByRefOrLanding returns the records commit of the run slug that git itself holds back.
// The commit the run's held-records ref names comes first.
// Without that ref, a landing commit of a run not yet done, on its parent past origin, holds the records.
// The held commit is then the oldest unpushed one.
// It reads local refs only.
// A slug with no records checkout holds nothing, and one with no code checkout holds nothing but by its ref.
func heldByRefOrLanding(hubPath, slug string) (hold string, held bool, err error) {
	records := weftname.SiblingPath(hubPath, slug)
	if !isCheckout(records) {
		return "", false, nil
	}
	hold, err = gitrepo.New(records).RefSHA(heldRecordsRefPrefix + slug)
	switch {
	case err == nil:
		return hold, true, nil
	case !errors.Is(err, gitrepo.ErrRefNotFound):
		return "", false, fmt.Errorf("fabricengine: read the held-records ref of %q: %w", slug, err)
	}

	code := filepath.Join(hubPath, slug)
	if !isCheckout(code) {
		return "", false, nil
	}
	pair, err := lymcwd.NamedWorktree(hubPath, slug)
	if err != nil {
		return "", false, err
	}
	origin, found, err := ReadOriginFor(pair, slug)
	if err != nil {
		// A half-made pair has no readable parent, and its removal must still run.
		logger.Warn("fabricengine: origin record not read, so no landing holds the pair's records", "slug", slug, "error", err)
		return "", false, nil
	}
	if !found || origin.ParentBranch == "" {
		return "", false, nil
	}
	landings, err := landingCommitsOn(code, origin.ParentBranch)
	if err != nil {
		return "", false, err
	}
	if _, landed := landings[slug]; !landed {
		return "", false, nil
	}
	done, err := landingRunDone(hubPath, slug)
	if err != nil || done {
		return "", false, err
	}
	hold, err = oldestUnpushed(records)
	if err != nil {
		return "", false, err
	}
	return hold, hold != "", nil
}
