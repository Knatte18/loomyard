// pullrequest.go closes a torn-down pair's open pull request, so a run that never landed leaves nothing on GitHub that looks like pending work.

package pairteardown

import (
	"context"
	"time"

	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/pullrequest"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

// closePullRequestTimeout bounds the GitHub calls of one pair's pull request close, so a teardown never hangs on the network.
const closePullRequestTimeout = 30 * time.Second

// tornDownPullRequestComment is the comment a pull request is closed with when its pair is torn down before it landed.
const tornDownPullRequestComment = "lym closed this pull request: the run's pair was torn down before it landed"

// closePairPullRequest closes the open pull request the pair's run recorded, if any.
// It reads whether the task worktree is gone, and when gone derives the task branch from the fabric config; when present it resolves the task location, reads the record under the run's scratch directory and the pair's origin URL, then closes through closeOpenPullRequest.
// Every failure is logged as a warning naming the slug, and none stops the teardown.
func (t *Teardown) closePairPullRequest(slug string) {
	gone, err := t.gone(slug)
	if err != nil {
		logger.Warn("pairteardown: could not tell whether the task worktree is gone, so its pull request is left alone", "slug", slug, "cause", err)
		return
	}
	openClient := func(originURL string) (pullrequest.API, error) {
		client, err := pullrequest.Open(originURL)
		if err != nil {
			return nil, err
		}
		return client, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), closePullRequestTimeout)
	defer cancel()
	if gone {
		// An unreadable config leaves the branch unnamed; the warning still goes out.
		taskBranch := ""
		cfg, err := fabricengine.LoadConfig(fabricengine.ConfigDirs(t.prime))
		if err != nil {
			logger.Warn("pairteardown: could not read the fabric config to name the task branch", "slug", slug, "cause", err)
		} else {
			taskBranch = cfg.BranchPrefix + slug
		}
		closeOpenPullRequest(ctx, slug, taskBranch, true, pullrequest.Record{}, false, openClient, "")
		return
	}

	task, err := t.taskLocation(slug)
	if err != nil {
		logger.Warn("pairteardown: could not resolve the task worktree, so its pull request is left alone", "slug", slug, "cause", err)
		return
	}
	rec, found, err := pullrequest.ReadRecord(shedrun.PullRequestRecord(task, shedrun.SelfRunID), shedrun.PullRequestRecordLock(task, shedrun.SelfRunID))
	if err != nil {
		logger.Warn("pairteardown: could not read the pull request record, so its pull request is left alone", "slug", slug, "cause", err)
		return
	}
	if !found {
		return
	}
	fabric, err := fabricengine.Open(task)
	if err != nil {
		logger.Warn("pairteardown: could not open the pair, so its pull request is left alone", "slug", slug, "url", rec.URL, "cause", err)
		return
	}
	originURL, err := fabric.OriginURL()
	if err != nil {
		logger.Warn("pairteardown: could not read the pair's origin URL, so its pull request is left alone", "slug", slug, "url", rec.URL, "cause", err)
		return
	}
	closeOpenPullRequest(ctx, slug, "", false, rec, true, openClient, originURL)
}

// closeOpenPullRequest closes the pull request rec records when GitHub reports it open, with one comment saying the pair was torn down before it landed.
// A merged or closed pull request is left as it is, so a landed run's merged pull request stays.
// A gone task worktree has no record to read, so it logs a warning naming slug and taskBranch and calls nothing; no record calls nothing.
// Any failure of the client, the status read or the close is logged as a warning naming the pull request's URL and returns, and never stops the teardown.
func closeOpenPullRequest(ctx context.Context, slug, taskBranch string, gone bool, rec pullrequest.Record, found bool, api func(originURL string) (pullrequest.API, error), originURL string) {
	if gone {
		logger.Warn("pairteardown: the task worktree is gone, so a pull request from its task branch, if the run opened one, may stay open", "slug", slug, "branch", taskBranch)
		return
	}
	if !found {
		return
	}
	client, err := api(originURL)
	if err != nil {
		logger.Warn("pairteardown: could not open the GitHub client, so the run's pull request stays open", "slug", slug, "url", rec.URL, "cause", err)
		return
	}
	status, err := client.Status(ctx, rec.Number)
	if err != nil {
		logger.Warn("pairteardown: could not read the run's pull request, so it stays open", "slug", slug, "url", rec.URL, "cause", err)
		return
	}
	if status.State != pullrequest.StateOpen {
		return
	}
	if err := client.Close(ctx, rec.Number, tornDownPullRequestComment); err != nil {
		logger.Warn("pairteardown: could not close the run's pull request", "slug", slug, "url", rec.URL, "cause", err)
		return
	}
	logger.Info("pairteardown: closed the run's pull request", "slug", slug, "url", rec.URL)
}
