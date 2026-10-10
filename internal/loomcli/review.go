// review.go implements the `review` loom subtree: the verbs through which a run's parent answers the parent-review gate.
// `notify`, `delivered`, `approve` and `reject` sit under `review` so they never collide with PR-Gate's `lyx loom approve` and `lyx loom reject`.
//
// The group carries its own PersistentPreRunE, and the loom parent's pre-run skips the group, so arm never runs for it and never reads a slug as a run-id.
// The skip is what holds under cmd/lyx's cobra.EnableTraverseRunHooks, which runs every ancestor's pre-run.
// The pre-run resolves the target worktree and builds a parentreview.Store from that worktree's told directories.
// The verb bodies take that store and their own arguments, so every refusal is reachable from an untagged test.
//
// The verbs check no caller identity.
// The parent reviews every rewrite after a reject, and the cap's reject halts the run `blocked`.
// An unchecked verb reaches no further than this one advisory review: a reject can halt a run only on the cap's rejected round and only into `blocked`,
// and one approve lifts it by superseding that reject, gated by the halted state alone (status `blocked`, the store at the cap, the latest round the cap's reject);
// an approve never rewrites an earlier round or an open round's pending state.
// A delivered stamped without a send stops re-sends but leaves the wait visible in status until the bound,
// and a notify sends one prompt and changes no gate count; Discussion-Review runs after every outcome.

package loomcli

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/lyxcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/parentreview"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
	"github.com/spf13/cobra"
)

const (
	reviewWayNoOpen   = `way forward: only Discussion-Write's parent-review gate opens a request; "lyx loom status <run>" shows whether the run has reached it`
	reviewWaySettled  = `way forward: the round is settled and the run proceeds on its own; nothing more to submit, except that a run halted at the reject cap proceeds only after "lyx loom review approve <slug>"`
	reviewWayExpired  = `way forward: the run already passed on its wait bound and Discussion-Review still reviews the discussion; nothing more to submit`
	reviewWaySlugForm = `pass the task's slug as listed by "lyx board list"`
)

// reviewTargetDeps is every side effect resolveReviewTarget performs, injected so tests supply fakes.
type reviewTargetDeps struct {
	// primeName resolves the base name of the hub's main worktree.
	primeName func(*lyxcwd.Location) (string, error)
	// dirExists reports whether a directory exists on disk.
	dirExists func(path string) bool
	// resolveWorktree resolves a worktree root into a Location with no cwd gate.
	resolveWorktree func(worktreeRoot string) (*lyxcwd.Location, error)
}

// defaultReviewTargetDeps are the production seams.
func defaultReviewTargetDeps() reviewTargetDeps {
	return reviewTargetDeps{
		primeName: fabricengine.PrimeName,
		dirExists: func(path string) bool {
			info, err := os.Stat(path)
			return err == nil && info.IsDir()
		},
		resolveWorktree: lyxcwd.ResolveWorktree,
	}
}

// resolveReviewTarget returns the worktree a verb of the named group (`review` or `circling`) acts on.
// A given slug resolves to its sibling worktree of the hub; with no slug a task worktree addresses itself and the prime refuses.
func resolveReviewTarget(group, verb string, location *lyxcwd.Location, slug string, d reviewTargetDeps) (*lyxcwd.Location, error) {
	if slug == "" {
		prime, err := d.primeName(location)
		if err != nil {
			return nil, fmt.Errorf("loom: %s %s: %w", group, verb, err)
		}
		if location.WorktreeName == prime {
			return nil, fmt.Errorf("loom: %s %s: slug required from the prime; way forward: %s, e.g. \"lyx loom %s %s <slug>\"", group, verb, reviewWaySlugForm, group, verb)
		}
		return location, nil
	}
	root := fabricengine.WorktreePath(location, slug)
	if !d.dirExists(root) {
		return nil, fmt.Errorf("loom: %s %s: unknown slug %q: no worktree at %s; way forward: %s", group, verb, slug, root, reviewWaySlugForm)
	}
	target, err := d.resolveWorktree(root)
	if err != nil {
		return nil, fmt.Errorf("loom: %s %s: resolve worktree %q: %w", group, verb, slug, err)
	}
	return target, nil
}

// reviewStoreFor builds the parentreview.Store for a target worktree from its told directories.
func reviewStoreFor(target *lyxcwd.Location) parentreview.Store {
	return parentreview.Store{
		Root:    loomengine.LoomParentReviewDir(target),
		LockDir: loomengine.LoomParentReviewLockDir(target),
	}
}

// reviewStoreErr maps a store error onto an envelope whose message ends in a way-forward clause.
func reviewStoreErr(out io.Writer, verb string, err error) int {
	switch {
	case errors.Is(err, parentreview.ErrNoOpenRequest):
		return output.Err(out, fmt.Sprintf("loom: review %s: no open review request; %s", verb, reviewWayNoOpen))
	case errors.Is(err, parentreview.ErrVerdictRecorded):
		return output.Err(out, fmt.Sprintf("loom: review %s: a verdict is already recorded for this round; %s", verb, reviewWaySettled))
	case errors.Is(err, parentreview.ErrExpired):
		return output.Err(out, fmt.Sprintf("loom: review %s: the review request expired; %s", verb, reviewWayExpired))
	case errors.Is(err, parentreview.ErrEmptyReviewFile), errors.Is(err, fs.ErrNotExist):
		return output.Err(out, fmt.Sprintf("loom: review %s: the review file is missing or empty; way forward: write the review to a non-empty file and re-run \"lyx loom review %s\" with its path", verb, verb))
	default:
		return output.Err(out, fmt.Sprintf("loom: review %s: %s", verb, err.Error()))
	}
}

// reviewNotifyVerb adds one waiting notify to the latest open request.
func reviewNotifyVerb(out io.Writer, store parentreview.Store, slug string) int {
	if err := store.AddNotify(); err != nil {
		return reviewStoreErr(out, "notify", err)
	}
	return output.Ok(out, map[string]any{"slug": slug, "action": "notify"})
}

// reviewDeliveredVerb stamps delivery on the latest open request, or records failedReason when it is non-empty.
func reviewDeliveredVerb(out io.Writer, store parentreview.Store, slug, failedReason string) int {
	if err := store.RecordDelivered(failedReason); err != nil {
		return reviewStoreErr(out, "delivered", err)
	}
	return output.Ok(out, map[string]any{"slug": slug, "action": "delivered", "failed": failedReason != ""})
}

// reviewStatusReader reads the target worktree's default run status; found is false when the run has no status file.
type reviewStatusReader func() (shedengine.Status, bool, error)

// reviewApproveVerb records an approve verdict, copying reviewFile when given.
// When the latest round carries a reject it takes the supersede path instead, which lifts a run halted at the reject cap.
func reviewApproveVerb(out io.Writer, store parentreview.Store, slug, reviewFile string, readStatus reviewStatusReader) int {
	latest, found, err := store.Latest()
	if err != nil {
		return reviewStoreErr(out, "approve", err)
	}
	if found && latest.Verdict != nil && latest.Verdict.Kind == parentreview.VerdictReject {
		return reviewSupersedeVerb(out, store, slug, reviewFile, readStatus)
	}
	if err := store.RecordVerdict(parentreview.VerdictApprove, reviewFile); err != nil {
		return reviewStoreErr(out, "approve", err)
	}
	return output.Ok(out, map[string]any{"slug": slug, "action": "approve"})
}

// reviewSupersedeVerb replaces the cap's reject with a superseding approve.
// The halted state is its only gate: the run's status is blocked, the store is at the cap and the latest round is the cap's reject.
func reviewSupersedeVerb(out io.Writer, store parentreview.Store, slug, reviewFile string, readStatus reviewStatusReader) int {
	if reviewFile != "" {
		return output.Err(out, fmt.Sprintf("loom: review approve: --review is refused when approving over a reject, since the cap's review file is kept; %s", reviewWayNotAtCap("re-run \"lyx loom review approve\" without --review")))
	}
	st, found, err := readStatus()
	if err != nil {
		return output.Err(out, fmt.Sprintf("loom: review approve: read the run status: %s", err.Error()))
	}
	if !found || st.State != shedengine.StateBlocked {
		return output.Err(out, "loom: review approve: approve supersedes a reject only on a run halted at the reject cap, and this run is not blocked; "+reviewWayNotAtCap(""))
	}
	if err := store.SupersedeCapReject(); err != nil {
		if errors.Is(err, parentreview.ErrNotAtCap) {
			return output.Err(out, "loom: review approve: the latest round's reject is not the cap's; "+reviewWayNotAtCap(""))
		}
		return reviewStoreErr(out, "approve", err)
	}
	return output.Ok(out, map[string]any{
		"slug":       slug,
		"action":     "approve",
		"superseded": true,
		"message":    `the cap's reject is superseded; way forward: run "lyx loom start" in the task worktree`,
	})
}

// reviewWayNotAtCap is the way-forward clause for a supersede refusal; lead, when set, names the step to take first.
func reviewWayNotAtCap(lead string) string {
	if lead != "" {
		lead += "; "
	}
	return `way forward: ` + lead + `"lyx loom status <slug>" shows the run's state; below the cap the gate re-prompts the writer itself, and the next round's request is approved normally`
}

// reviewRejectVerb records a reject verdict with the findings in reviewFile.
func reviewRejectVerb(out io.Writer, store parentreview.Store, slug, reviewFile string) int {
	if err := store.RecordVerdict(parentreview.VerdictReject, reviewFile); err != nil {
		return reviewStoreErr(out, "reject", err)
	}
	return output.Ok(out, map[string]any{"slug": slug, "action": "reject"})
}

// reviewSlugArg picks the slug out of a review verb's positional arguments.
// `reject` takes its review file last, so a slug is present only with two arguments.
func reviewSlugArg(verb string, args []string) string {
	if verb == "reject" {
		if len(args) == 2 {
			return args[0]
		}
		return ""
	}
	if len(args) >= 1 {
		return args[0]
	}
	return ""
}

// reviewState carries what the review group's pre-run resolved for its verbs.
type reviewState struct {
	store      parentreview.Store
	readStatus reviewStatusReader
	slug       string
	cwd        string
}

// resolveFile resolves a review-file argument against the invoking cwd.
func (s *reviewState) resolveFile(path string) string {
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(s.cwd, path)
}

// preRun resolves the invoking cwd, then the target worktree, then the store.
func (s *reviewState) preRun(cmd *cobra.Command, args []string) error {
	if cmd.Name() == "review" {
		return nil
	}
	ctx := cmd.Context()
	out := cmd.OutOrStdout()

	cwd, err := lyxcwd.CwdFrom(ctx)
	if err != nil {
		output.Err(out, err.Error())
		clihelp.Abort(ctx, 1)
		return nil
	}
	location, err := lyxcwd.Resolve(cwd)
	if err != nil {
		output.Err(out, err.Error())
		clihelp.Abort(ctx, 1)
		return nil
	}
	target, err := resolveReviewTarget("review", cmd.Name(), location, reviewSlugArg(cmd.Name(), args), defaultReviewTargetDeps())
	if err != nil {
		output.Err(out, err.Error())
		clihelp.Abort(ctx, 1)
		return nil
	}
	s.cwd = cwd
	s.slug = target.WorktreeName
	s.store = reviewStoreFor(target)
	statusPath := shedrun.StatusFile(target, shedrun.SelfRunID)
	statusLock := shedrun.StatusLock(target, shedrun.SelfRunID)
	s.readStatus = func() (shedengine.Status, bool, error) {
		return state.ReadJSONStrict[shedengine.Status](statusPath, statusLock)
	}
	return nil
}

// reviewCmd builds the `review` subtree.
func (c *loomCLI) reviewCmd() *cobra.Command {
	st := &reviewState{}

	group := &cobra.Command{
		Use:   "review",
		Short: "answer a run's parent-review request: notify, delivered, approve or reject",
		Long: `review holds the verbs through which a run's parent answers the parent-review
step of Discussion-Write's gate. They sit under review so they never collide
with PR-Gate's "lyx loom approve" and "lyx loom reject".

Each verb takes an optional slug naming the task worktree. A task worktree
addresses itself; from the prime the slug is required. The verbs check no
caller identity, and an unchecked verb reaches no further than this one
advisory review: Discussion-Review runs after every outcome.

The parent reviews every rewrite after a reject. The cap's reject halts the
run blocked, and approve on that halted run supersedes the reject.

Example:
  lyx loom review notify <slug>
  lyx loom review delivered <slug>
  lyx loom review approve <slug>
  lyx loom review reject <slug> review.md`,
		RunE:              clihelp.GroupRunE,
		PersistentPreRunE: st.preRun,
	}

	notify := &cobra.Command{
		Use:         "notify [<slug>]",
		Short:       "ask the writer to re-prompt the reviewer about the open parent-review request",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceRole},
		Long: `notify adds one waiting notify to the latest open review request; the
parent-review gate carries it to the live writer on its next pass. It changes
no gate count.

Example:
  lyx loom review notify <slug>`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			clihelp.SetExit(cmd.Context(), reviewNotifyVerb(cmd.OutOrStdout(), st.store, st.slug))
			return nil
		},
	}

	var failed string
	delivered := &cobra.Command{
		Use:         "delivered [<slug>]",
		Short:       "record that the review notice reached the reviewer, or that it could not be sent",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceRole},
		Long: `delivered stamps the latest open review request as delivered. With --failed
it records the reason instead, logs a warning and leaves the request
undelivered, so the wait stays visible in status.

Example:
  lyx loom review delivered <slug>
  lyx loom review delivered <slug> --failed "no such agent"`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			clihelp.SetExit(cmd.Context(), reviewDeliveredVerb(cmd.OutOrStdout(), st.store, st.slug, failed))
			return nil
		},
	}
	delivered.Flags().StringVar(&failed, "failed", "", "record this failure reason instead of stamping delivery")

	var approveFile string
	approve := &cobra.Command{
		Use:         "approve [<slug>]",
		Short:       "approve the discussion for the run's parent-review request",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `approve records an approve verdict on the latest open review request, so the
parent-review gate lets the run through. --review copies a review file
into the round; a relative path resolves against the current directory.

When the latest round carries a reject, approve instead supersedes it, and
only on a run halted blocked at the reject cap; --review is refused there,
since the cap's review file is kept. Then run "lyx loom start" in the task
worktree.

Example:
  lyx loom review approve <slug>
  lyx loom review approve <slug> --review review.md`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			clihelp.SetExit(cmd.Context(), reviewApproveVerb(cmd.OutOrStdout(), st.store, st.slug, st.resolveFile(approveFile), st.readStatus))
			return nil
		},
	}
	approve.Flags().StringVar(&approveFile, "review", "", "review file to copy into the round")

	reject := &cobra.Command{
		Use:         "reject [<slug>] <review-file>",
		Short:       "reject the discussion with the findings in a review file, re-prompting the writer",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `reject records a reject verdict on the latest open review request with the
findings in the review file, which must exist and be non-empty. The
parent-review gate sends the findings to the still-live writer and reviews
the rewrite as the next round; a relative path resolves against the current
directory. A reject on the cap's round halts the run blocked, and one
"lyx loom review approve" lifts it.

Example:
  lyx loom review reject <slug> review.md`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			clihelp.SetExit(cmd.Context(), reviewRejectVerb(cmd.OutOrStdout(), st.store, st.slug, st.resolveFile(args[len(args)-1])))
			return nil
		},
	}

	group.AddCommand(notify, delivered, approve, reject)
	return group
}
