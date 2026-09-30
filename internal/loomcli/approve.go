// approve.go implements the `approve` loom verb: the operator's recorded approval of the task's open pull request.
// It writes the approval record the PR-Gate row honours on its next run, removing any pending rejection first so the latest decision wins,
// and never resumes the run itself, so a supervisor driving `lyx loom step` is never raced.
//
// The cobra shell is thin over approveVerb, which takes every side effect as an injected closure so
// the refusal table is testable without git or network.

package loomcli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
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

// approveDeps is every side effect approveVerb performs, injected so tests supply fakes.
type approveDeps struct {
	// readStatus reads the run's status file under its lock; found is false when it is absent.
	readStatus func() (st shedengine.Status, found bool, err error)
	// branches resolves the task branch, the branch it lands on, and the origin URL.
	branches func() (taskBranch, parentBranch, originURL string, err error)
	// findPR looks up the pull request from head to base in owner/repo, nil when none exists.
	findPR func(ctx context.Context, owner, repo, head, base string) (*github.PullRequest, error)
	// headSHA reads the local task HEAD.
	headSHA func() (string, error)
	// removeRejection deletes any pending rejection record; an absent record is success.
	removeRejection func() error
	// writeApproval records the approval, replacing any earlier one.
	writeApproval func(landingshed.Approval) error
	// now supplies the approval time.
	now func() time.Time
}

// approveVerb runs the approve refusal table and, when every check passes, writes the record.
// It returns the process exit code and emits exactly one envelope on out.
func approveVerb(ctx context.Context, out io.Writer, d approveDeps) int {
	st, found, err := d.readStatus()
	if err != nil {
		return output.Err(out, "loom: approve: read status file: "+err.Error())
	}
	if !found {
		return output.Err(out, `loom: approve: no status file; run "lyx loom start" first to bootstrap this task`)
	}
	halted := st.State == shedengine.StateAwaiting || st.State == shedengine.StateBlocked
	if !halted || st.CurrentProducer != loomshed.NamePRGate {
		return output.Err(out, fmt.Sprintf("loom: approve: the run is not awaiting or blocked at %s (state %q, producer %q); approval applies only to a run awaiting or blocked at %s", loomshed.NamePRGate, st.State, st.CurrentProducer, loomshed.NamePRGate))
	}

	taskBranch, parentBranch, originURL, err := d.branches()
	if err != nil {
		return output.Err(out, "loom: approve: resolve branches: "+err.Error())
	}
	owner, repo, err := githubclient.ParseOwnerRepo(originURL)
	if err != nil {
		return output.Err(out, "loom: approve: origin URL unusable: "+err.Error())
	}
	pr, err := d.findPR(ctx, owner, repo, taskBranch, parentBranch)
	if err != nil {
		return output.Err(out, "loom: approve: look up the pull request: "+err.Error())
	}
	if pr == nil {
		return output.Err(out, fmt.Sprintf("loom: approve: no pull request from %s to %s in %s/%s", taskBranch, parentBranch, owner, repo))
	}
	if pr.GetState() != "open" || !pr.GetMergedAt().IsZero() {
		return output.Err(out, fmt.Sprintf("loom: approve: pull request #%d is not open (%s)", pr.GetNumber(), pr.GetHTMLURL()))
	}

	local, err := d.headSHA()
	if err != nil {
		return output.Err(out, "loom: approve: read the task HEAD: "+err.Error())
	}
	remote := pr.GetHead().GetSHA()
	if local != remote {
		return output.Err(out, fmt.Sprintf("loom: approve: local task HEAD %s differs from the pull request's head %s; push or sync first", local, remote))
	}

	rec := landingshed.Approval{
		PRNumber:   pr.GetNumber(),
		HeadSHA:    remote,
		ApprovedAt: d.now().UTC().Format(time.RFC3339),
	}
	if err := d.removeRejection(); err != nil {
		return output.Err(out, "loom: approve: remove the pending rejection: "+err.Error())
	}
	if err := d.writeApproval(rec); err != nil {
		return output.Err(out, "loom: approve: record the approval: "+err.Error())
	}

	return output.Ok(out, map[string]any{
		"pr_number": rec.PRNumber,
		"pr_url":    pr.GetHTMLURL(),
		"head_sha":  rec.HeadSHA,
		"resume":    "lyx loom start",
	})
}

// approveCmd builds the `approve` subcommand.
func (c *loomCLI) approveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "approve",
		Short: "record approval of the task's open pull request at the PR-Gate row so the next lyx loom start lands it",
		Long: `approve records the operator's approval of the task's open pull request,
for a run awaiting or blocked at PR-Gate, and removes any pending rejection. It refuses unless the run is awaiting or blocked at PR-Gate,
the pull request is open, and the local task HEAD equals the pull request's
head commit. It writes the approval record and never resumes the run; run
"lyx loom start" afterwards to land it.

Example:
  lyx loom approve`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			ctx := cmd.Context()
			location := c.location

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

			d := approveDeps{
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
				writeApproval: func(a landingshed.Approval) error {
					return landingshed.WriteApproval(loomengine.LoomApprovalPath(location), a)
				},
				now: time.Now,
			}
			clihelp.SetExit(ctx, approveVerb(ctx, cmd.OutOrStdout(), d))
			return nil
		},
	}
}
