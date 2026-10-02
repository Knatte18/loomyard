// review.go implements the `review` loom subtree: the verbs through which a run's parent answers the parent-review gate.
// `notify`, `delivered`, `approve` and `reject` sit under `review` so they never collide with PR-Gate's `lyx loom approve` and `lyx loom reject`.
//
// The group carries its own PersistentPreRunE, so the loom parent's arm never runs for it and never reads a slug as a run-id.
// The pre-run resolves the target worktree and builds a parentreview.Store from that worktree's told directories.
// The verb bodies take that store and their own arguments, so every refusal is reachable from an untagged test.
//
// The verbs check no caller identity.
// An unchecked verb reaches no further than this one advisory review: an approve skips at most the pass-on-cap parent review,
// a reject spends at most its single re-prompt, a delivered stamped without a send stops re-sends but leaves the wait visible in status until the bound,
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
	"github.com/spf13/cobra"
)

const (
	reviewWayNoOpen   = `way forward: only Discussion-Write's parent-review gate opens a request; "lyx loom status <run>" shows whether the run has reached it`
	reviewWaySettled  = `way forward: the round is settled and the run proceeds on its own; nothing more to submit`
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

// resolveReviewTarget returns the worktree a review verb acts on.
// A given slug resolves to its sibling worktree of the hub; with no slug a task worktree addresses itself and the prime refuses.
func resolveReviewTarget(verb string, location *lyxcwd.Location, slug string, d reviewTargetDeps) (*lyxcwd.Location, error) {
	if slug == "" {
		prime, err := d.primeName(location)
		if err != nil {
			return nil, fmt.Errorf("loom: review %s: %w", verb, err)
		}
		if location.WorktreeName == prime {
			return nil, fmt.Errorf("loom: review %s: slug required from the prime; way forward: %s, e.g. \"lyx loom review %s <slug>\"", verb, reviewWaySlugForm, verb)
		}
		return location, nil
	}
	root := fabricengine.WorktreePath(location, slug)
	if !d.dirExists(root) {
		return nil, fmt.Errorf("loom: review %s: unknown slug %q: no worktree at %s; way forward: %s", verb, slug, root, reviewWaySlugForm)
	}
	target, err := d.resolveWorktree(root)
	if err != nil {
		return nil, fmt.Errorf("loom: review %s: resolve worktree %q: %w", verb, slug, err)
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

// reviewApproveVerb records an approve verdict, copying reviewFile when given.
func reviewApproveVerb(out io.Writer, store parentreview.Store, slug, reviewFile string) int {
	if err := store.RecordVerdict(parentreview.VerdictApprove, reviewFile); err != nil {
		return reviewStoreErr(out, "approve", err)
	}
	return output.Ok(out, map[string]any{"slug": slug, "action": "approve"})
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
	store parentreview.Store
	slug  string
	cwd   string
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
	target, err := resolveReviewTarget(cmd.Name(), location, reviewSlugArg(cmd.Name(), args), defaultReviewTargetDeps())
	if err != nil {
		output.Err(out, err.Error())
		clihelp.Abort(ctx, 1)
		return nil
	}
	s.cwd = cwd
	s.slug = target.WorktreeName
	s.store = reviewStoreFor(target)
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

Example:
  lyx loom review notify <slug>
  lyx loom review delivered <slug>
  lyx loom review approve <slug>
  lyx loom review reject <slug> review.md`,
		RunE:              clihelp.GroupRunE,
		PersistentPreRunE: st.preRun,
	}

	notify := &cobra.Command{
		Use:   "notify [slug]",
		Short: "ask the writer to re-prompt the reviewer about the open parent-review request",
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
		Use:   "delivered [slug] [--failed <reason>]",
		Short: "record that the review notice reached the reviewer, or that it could not be sent",
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
		Use:   "approve [slug] [--review <file>]",
		Short: "approve the discussion for the run's parent-review request",
		Long: `approve records an approve verdict on the latest open review request, so the
parent-review gate lets the run through. --review copies a review file
into the round; a relative path resolves against the current directory.

Example:
  lyx loom review approve <slug>
  lyx loom review approve <slug> --review review.md`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			clihelp.SetExit(cmd.Context(), reviewApproveVerb(cmd.OutOrStdout(), st.store, st.slug, st.resolveFile(approveFile)))
			return nil
		},
	}
	approve.Flags().StringVar(&approveFile, "review", "", "review file to copy into the round")

	reject := &cobra.Command{
		Use:   "reject [slug] <review-file>",
		Short: "reject the discussion with the findings in a review file, re-prompting the writer once",
		Long: `reject records a reject verdict on the latest open review request with the
findings in the review file, which must exist and be non-empty. The
parent-review gate sends the findings to the still-live writer once; a
relative path resolves against the current directory.

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
