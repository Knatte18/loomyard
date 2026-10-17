// runstart.go declares seedRunStart, which reads the run's start from the `started_at` stamp of the pair's own run seed.
// The start is the lower bound of the landing's pull-request lookup.
// It also declares findRunPullRequest, the run-bounded pull-request lookup the approve and reject verbs use.

package loomcli

import (
	"context"
	"fmt"
	"time"

	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/google/go-github/v75/github"
)

// findRunPullRequest looks up the run's pull request from head to base in owner/repo: the newest one created at or after since, whatever its state, or nil when none exists.
// A zero since keeps the unbounded lookup.
func findRunPullRequest(ctx context.Context, owner, repo, head, base string, since time.Time) (*github.PullRequest, error) {
	client, err := landingshed.NewGitHubClient()
	if err != nil {
		return nil, err
	}
	pr, _, err := landingshed.FindPullRequest(ctx, client, owner, repo, head, base, since)
	return pr, err
}

// seedRunStartWarning is the message logged when the run's seed yields no usable start stamp.
const seedRunStartWarning = "loom: the run's seed carries no usable start stamp, so the landing's pull-request lookup is not bounded by the run's start"

// seedRunStart returns the run's start, the `started_at` stamp of the pair's own run seed.
// A missing seed, an empty stamp and an unparseable stamp each return the zero time and a nil error after one warning.
// The zero time filters nothing, so the pull-request lookup stays unbounded.
// A seed that exists but cannot be read or decoded returns an error, so the caller fails rather than silently dropping the bound.
func seedRunStart(l *lyxcwd.Location) (time.Time, error) {
	seed, found, err := shedrun.ReadSeed(l, shedrun.SelfRunID)
	if err != nil {
		return time.Time{}, fmt.Errorf("loom: read the run's start from its seed %s: %w", shedrun.SeedFile(l, shedrun.SelfRunID), err)
	}
	if !found || seed.StartedAt == "" {
		logger.Warn(seedRunStartWarning, "run", shedrun.SelfRunID)
		return time.Time{}, nil
	}
	start, err := time.Parse(time.RFC3339, seed.StartedAt)
	if err != nil {
		logger.Warn(seedRunStartWarning, "run", shedrun.SelfRunID, "started_at", seed.StartedAt, "error", err)
		return time.Time{}, nil
	}
	return start, nil
}
