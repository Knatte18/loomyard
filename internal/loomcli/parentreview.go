// parentreview.go implements the `parent-review` loom group over a run's Discussion-Review asks to its parent orch.
// `answer` writes the parent's report for the run's latest ask and sets the ask done.
// `await` is the review chair's capped wait for that ask to end, setting it timeout once its deadline passed.
// Both move the ask only through burlerengine's ask record, under its one lock.
// The caller check guards against a mistaken call, not a forged strand name.

package loomcli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/agentname"
	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/hubgeom"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/orchcli"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/shuttleengines"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

const (
	parentReviewGroup      = "parent-review"
	parentReviewAnswerName = "answer"
	parentReviewAwaitName  = "await"

	// parentReviewPollInterval is how often await reads the ask record: the one-second floor of every wake.
	parentReviewPollInterval = time.Second

	parentReviewWayFlags         = "way forward: pass --ask <n> naming the ask the parent's request names, and exactly one of --satisfied or --findings-file <path>"
	parentReviewWayFindingsFile  = `way forward: write the --findings-file as YAML holding a non-empty "findings" list, each item with a "change" naming what to change and where, and an optional "severity" (BLOCKING, MEDIUM, LOW or NIT) and "class" (design, scope, decision or consistency), then re-run the verb`
	parentReviewWayParentAnswers = "way forward: the run's parent orch answers the ask; the operator's own unnamed shell may too"
	parentReviewWayRetry         = "way forward: re-run the verb; nothing was answered"
	parentReviewWayAwaitNoAsk    = "way forward: no ask went out for this run, so consolidate the round without a parent review"
	parentReviewWayAwaitRetry    = "way forward: re-run the verb"
)

// parentReviewDeps holds every side effect of a parent-review verb, injected so that tests can supply fakes.
type parentReviewDeps struct {
	// strandName is the full name reed exported to the calling session, empty for a shell reed did not spawn.
	strandName string
	// parent resolves the run's parent orch at call time.
	parent func() (hubgeom.Parent, error)
	// files locates the run's ask record and the parent's reports.
	files burlerengine.ParentReviewFiles
	// now is the clock every deadline is judged against.
	now func() time.Time
	// awaitCap is the longest one await call waits: the keep-warm cap of the calling strand's role.
	awaitCap time.Duration
	// sleep waits d, returning early with ctx's error when ctx ends.
	sleep func(ctx context.Context, d time.Duration) error
}

// parentAnswerInput holds the flag values of one `parent-review answer` call.
type parentAnswerInput struct {
	ask          int
	satisfied    bool
	findingsFile string
}

// parentFindingsFile is the shape of an answer's --findings-file.
type parentFindingsFile struct {
	Findings []struct {
		Change   string `yaml:"change"`
		Severity string `yaml:"severity"`
		Class    string `yaml:"class"`
	} `yaml:"findings"`
}

// parentFindingSeverities and parentFindingClasses are the review format's words a finding's severity and class take.
var (
	parentFindingSeverities = []burlerengine.Severity{burlerengine.SeverityBlocking, burlerengine.SeverityMedium, burlerengine.SeverityLow, burlerengine.SeverityNit}
	parentFindingClasses    = []burlerengine.Class{burlerengine.ClassDesign, burlerengine.ClassScope, burlerengine.ClassDecision, burlerengine.ClassConsistency}
)

// readParentFindings reads the findings of the file at path, or returns the refusal text.
// It refuses an unreadable or malformed file, a file with no item, an item with no change,
// and a severity or class outside the review format's words.
func readParentFindings(path string) ([]burlerengine.ParentFinding, string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Sprintf("read the --findings-file: %s; %s", err, parentReviewWayFindingsFile)
	}
	var file parentFindingsFile
	if err := yaml.NewDecoder(bytes.NewReader(data)).Decode(&file); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Sprintf("the --findings-file %s is not YAML holding a findings list: %s; %s", path, err, parentReviewWayFindingsFile)
	}
	if len(file.Findings) == 0 {
		return nil, fmt.Sprintf("the --findings-file %s holds no finding; %s", path, parentReviewWayFindingsFile)
	}
	findings := make([]burlerengine.ParentFinding, 0, len(file.Findings))
	for i, item := range file.Findings {
		n := i + 1
		if strings.TrimSpace(item.Change) == "" {
			return nil, fmt.Sprintf("finding %d of the --findings-file has no change; %s", n, parentReviewWayFindingsFile)
		}
		severity, class := burlerengine.Severity(item.Severity), burlerengine.Class(item.Class)
		if item.Severity != "" && !containsWord(parentFindingSeverities, severity) {
			return nil, fmt.Sprintf("finding %d of the --findings-file has the severity %q; %s", n, item.Severity, parentReviewWayFindingsFile)
		}
		if item.Class != "" && !containsWord(parentFindingClasses, class) {
			return nil, fmt.Sprintf("finding %d of the --findings-file has the class %q; %s", n, item.Class, parentReviewWayFindingsFile)
		}
		findings = append(findings, burlerengine.ParentFinding{Severity: severity, Class: class, Change: strings.TrimSpace(item.Change)})
	}
	return findings, ""
}

// containsWord reports whether words holds word.
func containsWord[T comparable](words []T, word T) bool {
	for _, candidate := range words {
		if candidate == word {
			return true
		}
	}
	return false
}

// parentAnswerCallerRefusal returns the refusal text for a caller that may not answer asked, or empty when it may.
// The operator's own unnamed shell always may, and so does the run's parent orch as resolved at call time.
// So does the orch strand asked went to, which keeps an orch that came up or went down since the ask from refusing it.
func parentAnswerCallerRefusal(caller string, parent hubgeom.Parent, parentErr error, asked burlerengine.ParentAsk) string {
	if caller == "" || (asked.Strand != "" && caller == asked.Strand) {
		return ""
	}
	if parentErr != nil {
		return fmt.Sprintf("the caller %q is not the strand the ask went to, and the run's parent orch could not be resolved (%s); %s", caller, parentErr, parentReviewWayParentAnswers)
	}
	if parent.Name != "" && caller == parent.Name {
		return ""
	}
	return fmt.Sprintf("the caller %q is neither the run's parent orch nor the strand the ask went to; %s", caller, parentReviewWayParentAnswers)
}

// parentReviewWayDecision returns the way forward of a refused answer that no later answer can take:
// a call the parent still wants made goes in with `lym loom decision add`.
// It names who runs that verb as its guard resolves it: the run's parent orch,
// or the operator's own unnamed shell when no parent orch resolves or the resolved one has no name.
func parentReviewWayDecision(slug string, parent hubgeom.Parent, parentErr error) string {
	who := "the operator's own unnamed shell"
	if parentErr == nil && parent.Name != "" {
		who = "the run's parent orch " + parent.Name
	}
	return fmt.Sprintf(`way forward: a call the parent still wants made goes in with "lym loom decision add %s --by parent", run by %s`, slug, who)
}

// parentAnswerRefusalText returns the refusal text of an answer to the ask numbered number that refusal turned down.
func parentAnswerRefusalText(slug string, number int, refusal *burlerengine.ParentAnswerRefusal, parent hubgeom.Parent, parentErr error) string {
	latest := refusal.Latest
	wayDecision := parentReviewWayDecision(slug, parent, parentErr)
	switch refusal.Kind {
	case burlerengine.ParentAnswerNoAsk:
		return "the run has no parent-review ask; " + wayDecision
	case burlerengine.ParentAnswerNotLatest:
		if !latest.State.Ended() {
			return fmt.Sprintf("ask %d is not the run's latest ask: ask %d is, and it awaits an answer, so the decision record may have changed since ask %d; way forward: review the record again under ask %d's own request, then answer with --ask %d",
				number, latest.Number, number, latest.Number, latest.Number)
		}
		return fmt.Sprintf("ask %d is not the run's latest ask, which is ask %d, already %s; %s", number, latest.Number, latest.State, wayDecision)
	case burlerengine.ParentAnswerPastDeadline:
		return fmt.Sprintf("ask %d reached its deadline at %s with no answer and is now %s; %s", number, latest.Deadline.UTC().Format(time.RFC3339), latest.State, wayDecision)
	}
	return fmt.Sprintf("ask %d is already %s; %s", number, latest.State, wayDecision)
}

// parentReviewAnswerVerb records the parent's answer in input to the run's latest ask and sets the ask done.
// A refusal carries a way forward and writes nothing.
// An ask found at or past its deadline is the exception: the call sets it timeout.
func parentReviewAnswerVerb(out io.Writer, slug string, deps parentReviewDeps, input parentAnswerInput) int {
	refuse := func(format string, args ...any) int {
		return output.Err(out, fmt.Sprintf("loom: %s %s: ", parentReviewGroup, parentReviewAnswerName)+fmt.Sprintf(format, args...))
	}
	if input.ask < 1 {
		return refuse("--ask is required and numbers an ask from 1 up; %s", parentReviewWayFlags)
	}
	if input.satisfied == (input.findingsFile != "") {
		return refuse("pass exactly one of --satisfied or --findings-file; %s", parentReviewWayFlags)
	}
	var findings []burlerengine.ParentFinding
	if input.findingsFile != "" {
		var refusal string
		if findings, refusal = readParentFindings(input.findingsFile); refusal != "" {
			return refuse("%s", refusal)
		}
	}

	record, _, err := burlerengine.ReadParentAsks(deps.files)
	if err != nil {
		return refuse("read the run's parent-review asks: %s; %s", err, parentReviewWayRetry)
	}
	asked, _ := record.Ask(input.ask)
	parent, parentErr := deps.parent()
	if refusal := parentAnswerCallerRefusal(deps.strandName, parent, parentErr, asked); refusal != "" {
		return refuse("%s", refusal)
	}

	ask, err := burlerengine.AnswerParentAsk(deps.files, input.ask, deps.now(), findings)
	var refusal *burlerengine.ParentAnswerRefusal
	if errors.As(err, &refusal) {
		return refuse("%s", parentAnswerRefusalText(slug, input.ask, refusal, parent, parentErr))
	}
	if err != nil {
		return refuse("%s; %s", err, parentReviewWayRetry)
	}
	return output.Ok(out, map[string]any{
		"slug":     slug,
		"ask":      ask.Number,
		"state":    string(ask.State),
		"report":   ask.Report,
		"findings": len(findings),
	})
}

// parentReviewAwaitVerb waits for the run's latest ask to end, capped at deps.awaitCap.
// It reads the ask record once per poll interval.
// An ask still pending-post or open at or past its deadline is set timeout, which ends it.
// It answers ready true with the ask's state once the ask has ended.
// That answer carries the reason of a not-sent ask and the report of a done one.
// It answers ready false once the cap runs out first, and the caller runs it again.
// A run with no ask is refused.
func parentReviewAwaitVerb(ctx context.Context, out io.Writer, deps parentReviewDeps) int {
	refuse := func(format string, args ...any) int {
		return output.Err(out, fmt.Sprintf("loom: %s %s: ", parentReviewGroup, parentReviewAwaitName)+fmt.Sprintf(format, args...))
	}
	end := deps.now().Add(deps.awaitCap)
	for {
		ask, found, err := burlerengine.ExpireParentAsk(deps.files, deps.now())
		if err != nil {
			return refuse("%s; %s", err, parentReviewWayAwaitRetry)
		}
		if !found {
			return refuse("the run has no parent-review ask to wait on; %s", parentReviewWayAwaitNoAsk)
		}
		if ask.State.Ended() {
			result := map[string]any{"ready": true, "ask": ask.Number, "state": string(ask.State)}
			if ask.State == burlerengine.ParentAskNotSent {
				result["reason"] = ask.Reason
			}
			if ask.State == burlerengine.ParentAskDone {
				result["report"] = ask.Report
			}
			return output.Ok(out, result)
		}
		remaining := end.Sub(deps.now())
		if remaining <= 0 {
			return output.Ok(out, map[string]any{
				"ready":    false,
				"ask":      ask.Number,
				"state":    string(ask.State),
				"deadline": ask.Deadline.UTC().Format(time.RFC3339),
				"message":  "the parent has not answered and the ask's deadline has not passed; call this command again",
			})
		}
		if err := deps.sleep(ctx, min(parentReviewPollInterval, remaining)); err != nil {
			return refuse("the wait was cut short: %s; %s", err, parentReviewWayAwaitRetry)
		}
	}
}

// parentReviewState carries a parent-review verb's resolved target and side effects from the group's pre-run.
type parentReviewState struct {
	slug   string
	deps   parentReviewDeps
	answer parentAnswerInput
}

// preRun resolves the invoking cwd, then the target worktree, then the verb's side effects.
func (s *parentReviewState) preRun(cmd *cobra.Command, args []string) error {
	if cmd.Name() == parentReviewGroup {
		return nil
	}
	ctx := cmd.Context()
	out := cmd.OutOrStdout()
	abort := func(msg string) error {
		output.Err(out, msg)
		clihelp.Abort(ctx, 1)
		return nil
	}

	cwd, err := lymcwd.CwdFrom(ctx)
	if err != nil {
		return abort(err.Error())
	}
	location, err := lymcwd.Resolve(cwd)
	if err != nil {
		return abort(err.Error())
	}
	slug := ""
	if len(args) >= 1 {
		slug = args[0]
	}
	target, err := resolveReviewTarget(parentReviewGroup, cmd.Name(), location, slug, defaultReviewTargetDeps())
	if err != nil {
		return abort(err.Error())
	}
	recipe, err := seedRecipe(target)
	if err != nil {
		return abort(fmt.Sprintf("loom: %s %s: read the run's seed: %s; way forward: restore the run's seed, or begin the run again with `lym loom start`", parentReviewGroup, cmd.Name(), err))
	}
	record := runRecordOf(target, recipe)
	strand := os.Getenv(agentname.StrandNameEnv)
	s.slug = target.WorktreeName
	s.deps = parentReviewDeps{
		strandName: strand,
		parent:     func() (hubgeom.Parent, error) { return decisionParent(target, record, orchcli.OrchLive) },
		files:      burlerengine.ParentReviewFilesOf(target.AnchorPath()),
		now:        time.Now,
		awaitCap:   shuttleengines.KeepWarm(fabricengine.ConfigDirs(target), strand).Cap,
		sleep:      sleepContext,
	}
	return nil
}

// parentReviewCmd builds the `parent-review` subtree.
func (c *loomCLI) parentReviewCmd() *cobra.Command {
	st := &parentReviewState{}

	group := &cobra.Command{
		Use:   parentReviewGroup,
		Short: "answer or await a Discussion-Review round's ask to the parent orch",
		Long: `parent-review holds the verbs of a Discussion-Review round's ask to the run's
parent orch. "parent-review answer" records the parent's review of the decision
record; "parent-review await" is the review chair's wait for that answer.

The verb takes an optional slug naming the task worktree. A task worktree
addresses itself; from the prime the slug is required. The ask record and the
parent's reports are never tracked.

Example:
  lym loom parent-review answer <slug> --ask 1 --satisfied
  lym loom parent-review await <slug>`,
		RunE:              clihelp.GroupRunE,
		PersistentPreRunE: st.preRun,
	}

	answer := &cobra.Command{
		Use:         parentReviewAnswerName + " [<slug>]",
		Short:       "record the parent orch's review of a Discussion-Review round's ask",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceOperator},
		Long: `answer writes the parent's report for the ask --ask names and sets the ask
done. --ask is required, with exactly one of --satisfied, which writes a report
with no findings, or --findings-file <path>, a YAML file holding a "findings"
list. Each item has a required "change", naming what to change and where, and
an optional "severity" (BLOCKING, MEDIUM, LOW or NIT) and "class" (design,
scope, decision or consistency). The report numbers the items P1, P2 and so on
in the file's order.

It answers only the run's latest ask while that ask is pending-post or open
and before its deadline; an ask at or past its deadline is set timeout and the
answer refused. A call the parent still wants made after a refusal goes in with
"lym loom decision add <slug> --by parent".

The callers are the run's parent orch, resolved at call time, the orch strand
the ask went to, and the operator's own unnamed shell; the strand name is not
verified. It edits no tracked file and never the decision record.

Example:
  lym loom parent-review answer <slug> --ask 2 --findings-file findings.yaml`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			clihelp.SetExit(cmd.Context(), parentReviewAnswerVerb(cmd.OutOrStdout(), st.slug, st.deps, st.answer))
			return nil
		},
	}
	answer.Flags().IntVar(&st.answer.ask, "ask", 0, "the number of the ask the answer is for, as the parent's request names it")
	answer.Flags().BoolVar(&st.answer.satisfied, "satisfied", false, "the parent has no findings")
	answer.Flags().StringVar(&st.answer.findingsFile, "findings-file", "", "a YAML file holding the parent's findings list")

	await := &cobra.Command{
		Use:         parentReviewAwaitName + " [<slug>]",
		Short:       "wait, capped, until the run's latest parent-review ask has ended",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceRole},
		Long: `await waits until the run's latest parent-review ask has ended, and prints ok
with ready true, the ask's state, the reason of a not-sent ask and the report
of a done one. An ask still pending-post or open at or past its deadline is set
timeout, which ends it. When the calling session's keep-warm cap elapses first
it prints ok with ready false and exits 0: run it again, in a turn that does
nothing else, with a Bash tool timeout above the cap. A run with no ask is
refused. Any caller may run it; it writes only timeout, and only to an ask at
or past its deadline.

Example:
  lym loom parent-review await <slug>`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			clihelp.SetExit(cmd.Context(), parentReviewAwaitVerb(cmd.Context(), cmd.OutOrStdout(), st.deps))
			return nil
		},
	}

	group.AddCommand(answer, await)
	return group
}
