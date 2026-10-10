// reject.go implements the `reject` loom verb: the operator's recorded rejection of the task's open pull request, with the review findings read from a file.
// It writes the rejection record the PR-Gate row honours on its next run, removing any approval first so the latest decision wins,
// and never resumes the run itself, so a supervisor driving `lyx loom step` is never raced.
//
// The cobra shell is thin over rejectVerb, which takes every side effect as an injected closure so the refusal table is testable without git or network.

package loomcli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/githubclient"
	"github.com/Knatte18/loomyard/internal/landingshed"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomshed"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/google/go-github/v75/github"
	"github.com/spf13/cobra"
)

// rejectDeps is every side effect rejectVerb performs, injected so tests supply fakes.
type rejectDeps struct {
	// readStatus reads the run's status file under its lock; found is false when it is absent.
	readStatus func() (st shedengine.Status, found bool, err error)
	// branches resolves the task branch, the branch it lands on, and the origin URL.
	branches func() (taskBranch, parentBranch, originURL string, err error)
	// findPR looks up the pull request from head to base in owner/repo, nil when none exists.
	findPR func(ctx context.Context, owner, repo, head, base string) (*github.PullRequest, error)
	// headSHA reads the local task HEAD.
	headSHA func() (string, error)
	// readReviewFile reads the operator's review file.
	readReviewFile func(path string) (string, error)
	// removeApproval deletes any approval record; an absent record is success.
	removeApproval func() error
	// writeRejection records the rejection, replacing any earlier one.
	writeRejection func(landingshed.Rejection) error
	// routing is the run's recipe's routing, read for the PR-Gate bounce budget.
	routing shedengine.Routing
	// reworkRow is the recipe's row that answers a rejection: PR-Rework in loom, Darn in darn.
	reworkRow string
	// rejectionPending reports whether a rejection record is pending.
	// Only a run blocked at the Darn row reads it.
	rejectionPending func() (bool, error)
	// now supplies the rejection time.
	now func() time.Time
}

// rejectVerb runs the reject refusal table and, when every check passes, writes the record.
// It returns the process exit code and emits exactly one envelope on out.
func rejectVerb(ctx context.Context, out io.Writer, d rejectDeps, reviewFile string) int {
	st, found, err := d.readStatus()
	if err != nil {
		return output.Err(out, "loom: reject: read status file: "+err.Error())
	}
	if !found {
		return output.Err(out, `loom: reject: no status file; run "lyx loom start" first to bootstrap this task`)
	}
	halted := st.State == shedengine.StateAwaiting || st.State == shedengine.StateBlocked
	atGate := halted && st.CurrentProducer == loomshed.NamePRGate
	atRework := st.State == shedengine.StateBlocked && st.CurrentProducer == d.reworkRow
	if !atGate && !atRework {
		return output.Err(out, fmt.Sprintf("loom: reject: the run is not awaiting or blocked at %s, nor blocked at %s (state %q, producer %q)", loomshed.NamePRGate, d.reworkRow, st.State, st.CurrentProducer))
	}
	// A Darn row that halted on its own gate has no pull request yet, so only a pending rejection gives a rejection something to replace.
	if atRework && d.reworkRow == loomshed.NameDarn {
		pending, err := d.rejectionPending()
		if err != nil {
			return output.Err(out, "loom: reject: read the pending rejection: "+err.Error())
		}
		if !pending {
			return output.Err(out, fmt.Sprintf("loom: reject: the run is blocked at %s with no rejection pending, so it has no pull request to reject; fix the cause the status names, then run \"lyx loom resume\"", loomshed.NameDarn))
		}
	}
	if atGate {
		count, budget, inSegment := d.routing.Bounces(loomshed.NamePRGate, st.History)
		if inSegment && count >= budget {
			return output.Err(out, fmt.Sprintf("loom: reject: the rework budget is exhausted (%d of %d rounds used); \"lyx loom approve\" is the only way forward", count, budget))
		}
	}

	taskBranch, parentBranch, originURL, err := d.branches()
	if err != nil {
		return output.Err(out, "loom: reject: resolve branches: "+err.Error())
	}
	owner, repo, err := githubclient.ParseOwnerRepo(originURL)
	if err != nil {
		return output.Err(out, "loom: reject: origin URL unusable: "+err.Error())
	}
	pr, err := d.findPR(ctx, owner, repo, taskBranch, parentBranch)
	if err != nil {
		return output.Err(out, "loom: reject: look up the pull request: "+err.Error())
	}
	if pr == nil {
		return output.Err(out, fmt.Sprintf("loom: reject: no pull request from %s to %s in %s/%s", taskBranch, parentBranch, owner, repo))
	}
	if pr.GetState() != "open" || !pr.GetMergedAt().IsZero() {
		return output.Err(out, fmt.Sprintf("loom: reject: pull request #%d is not open (%s)", pr.GetNumber(), pr.GetHTMLURL()))
	}

	local, err := d.headSHA()
	if err != nil {
		return output.Err(out, "loom: reject: read the task HEAD: "+err.Error())
	}
	remote := pr.GetHead().GetSHA()
	if local != remote {
		return output.Err(out, fmt.Sprintf("loom: reject: local task HEAD %s differs from the pull request's head %s; push or sync first", local, remote))
	}

	findings, err := d.readReviewFile(reviewFile)
	if err != nil {
		return output.Err(out, "loom: reject: read the review file: "+err.Error())
	}
	if strings.TrimSpace(findings) == "" {
		return output.Err(out, fmt.Sprintf("loom: reject: review file %s is empty", reviewFile))
	}

	rec := landingshed.Rejection{
		PRNumber:   pr.GetNumber(),
		HeadSHA:    remote,
		RejectedAt: d.now().UTC().Format(time.RFC3339),
		Findings:   findings,
	}
	if err := d.removeApproval(); err != nil {
		return output.Err(out, "loom: reject: remove the approval: "+err.Error())
	}
	if err := d.writeRejection(rec); err != nil {
		return output.Err(out, "loom: reject: record the rejection: "+err.Error())
	}

	return output.Ok(out, map[string]any{
		"pr_number": rec.PRNumber,
		"pr_url":    pr.GetHTMLURL(),
		"head_sha":  rec.HeadSHA,
		"resume":    "lyx loom start",
	})
}

// rejectCmd builds the `reject` subcommand.
func (c *loomCLI) rejectCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "reject <review-file>",
		Short: "record rejection of the task's open pull request with findings from a file so the next lyx loom start reworks it",
		Long: `reject records the operator's rejection of the task's open pull request,
with the review findings read from <review-file>, and removes any approval.
It applies to a run awaiting or blocked at PR-Gate, or blocked at the recipe's
rework row (the rework session stopped; a new rejection replaces the pending
one). In the loom recipe the rework row is PR-Rework: the next "lyx loom start"
sends the findings to it, and it archives the built plan generation into the
round and plans a new one, then re-runs the main line. In the darn recipe the
rework row is Darn: the next "lyx loom start" spawns a fresh Darn session
that is told the findings, and a run blocked at Darn needs a pending rejection
for a new one to replace.

It refuses unless the pull request is open, the local task HEAD equals the
pull request's head commit, and the review file is readable and non-empty.
At PR-Gate it also refuses once the rework budget is exhausted; "lyx loom
approve" is then the only way forward. At PR-Rework the budget is not
consulted. reject writes the rejection record and never resumes the run; run
"lyx loom start" afterwards.

Example:
  lyx loom reject review.md`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			ctx := cmd.Context()
			location := c.location

			routing, err := c.recipeRouting()
			if err != nil {
				clihelp.SetExit(ctx, output.Err(cmd.OutOrStdout(), "loom: reject: "+err.Error()))
				return nil
			}

			var handle *fabricengine.Fabric
			open := func() (*fabricengine.Fabric, error) {
				if handle != nil {
					return handle, nil
				}
				f, err := fabricengine.Open(location)
				if err != nil {
					return nil, err
				}
				handle = f
				return f, nil
			}

			d := rejectDeps{
				readStatus: func() (shedengine.Status, bool, error) {
					if err := os.MkdirAll(filepath.Dir(c.shedPaths.StatusLockPath), 0o755); err != nil {
						return shedengine.Status{}, false, err
					}
					return state.ReadJSONStrict[shedengine.Status](c.shedPaths.StatusPath, c.shedPaths.StatusLockPath)
				},
				branches: func() (string, string, string, error) {
					f, err := open()
					if err != nil {
						return "", "", "", err
					}
					task, parent, err := landingBranches(f, location)
					if err != nil {
						return "", "", "", err
					}
					originURL, err := f.OriginURL()
					if err != nil {
						return "", "", "", err
					}
					return task, parent, originURL, nil
				},
				findPR: func(ctx context.Context, owner, repo, head, base string) (*github.PullRequest, error) {
					client, err := landingshed.NewGitHubClient()
					if err != nil {
						return nil, err
					}
					return landingshed.FindPullRequest(ctx, client, owner, repo, head, base)
				},
				headSHA: func() (string, error) {
					f, err := open()
					if err != nil {
						return "", err
					}
					return f.HeadSHA()
				},
				readReviewFile: func(path string) (string, error) {
					data, err := os.ReadFile(path)
					return string(data), err
				},
				removeApproval: func() error {
					return landingshed.RemoveRecord(loomengine.LoomApprovalPath(location))
				},
				writeRejection: func(r landingshed.Rejection) error {
					return landingshed.WriteRejection(loomengine.LoomRejectionPath(location), r)
				},
				routing:   routing,
				reworkRow: reworkRowFor(c.recipe),
				rejectionPending: func() (bool, error) {
					return rejectionPending(location), nil
				},
				now: time.Now,
			}
			clihelp.SetExit(ctx, rejectVerb(ctx, cmd.OutOrStdout(), d, args[0]))
			return nil
		},
	}
}
