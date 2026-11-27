// prfindings.go implements the `pr-findings` loom verb: the PR-Review agent's report of a findings set.
// The verb decides whether the set goes back to Code-Review or the agent fixes it in place.
// A send-back stores the findings file, writes a friction note and marks the pull-request record.
// The PR-Review row then reroutes.
// A fix-here answer only counts the findings, so the findings fixed between two send-backs stay bounded.

package loomcli

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/burlerengine"
	"github.com/Knatte18/loomyard/internal/clihelp"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/friction"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/loomrecipe"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/output"
	"github.com/Knatte18/loomyard/internal/pullrequest"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/verifytree"
	"github.com/spf13/cobra"
)

const (
	// prReviewEngine is the recipe engine name of the PR-Review row.
	prReviewEngine = "PRReview"
	// prFindingsFormatStencil is the stencil, under the stencils directory, that defines burler's review format.
	prFindingsFormatStencil = "burler/burler-review-format.md"
	// prFindingsNotePrefix frames the name of a send-back's friction note around the send-back's number.
	prFindingsNotePrefix = "pr-review-send-back-"
)

// prFindingsDeps carries the injected seams through which the pr-findings verb reads and writes.
type prFindingsDeps struct {
	// dirty answers the task worktree's uncommitted paths.
	dirty func() ([]string, error)
	// recordPath and recordLockPath are the pull-request record and its lock.
	recordPath     string
	recordLockPath string
	// scratchDir holds the stored findings files, frictionDir the friction notes and stencilsDir the hub's seeded stencils.
	// An empty frictionDir means Tier 2 is off.
	scratchDir  string
	frictionDir string
	stencilsDir string
	// smallMax answers the run's pr_review_small_max.
	smallMax func() (int, error)
	// edge answers whether the send-back edge can fire, and why not when it cannot.
	edge func() (bool, string, error)
	// commit commits the pull-request record to the pair's records.
	commit func() error
}

// prFindingsReadFailure is the error envelope of a seam that could not be read or written, naming a re-run.
func prFindingsReadFailure(out io.Writer, file, what string, err error) int {
	return output.Err(out, "loom: pr-findings: "+what+": "+err.Error()+"; way forward: run lym loom pr-findings "+file+" again, and tell the operator when it keeps failing")
}

// sendBackEdge reports whether the send-back edge of the PR-Review row named row can fire, and why not when it cannot.
// It reads the built rows, so the answer follows the recipe and the run's settings.
// The edge cannot fire when the row declares no OnReroute.
// It cannot fire when the target row's entry gate does not hold.
// It cannot fire when the target row's bounce budget is 0.
// A budget of 0 makes the target approve without review.
// A row the producers lack, or an entry gate that fails to answer, is an error.
func sendBackEdge(producers []shedengine.ProducerDef, row string) (bool, string, error) {
	find := func(name string) (shedengine.ProducerDef, error) {
		at := slices.IndexFunc(producers, func(def shedengine.ProducerDef) bool { return def.Name == name })
		if at < 0 {
			return shedengine.ProducerDef{}, fmt.Errorf("the run's recipe has no %s row", name)
		}
		return producers[at], nil
	}
	source, err := find(row)
	if err != nil {
		return false, "", err
	}
	if source.OnReroute == "" {
		return false, "the " + row + " row declares no reroute edge", nil
	}
	target, err := find(source.OnReroute)
	if err != nil {
		return false, "", err
	}
	if target.EntryGate != nil {
		holds, reason, err := target.EntryGate()
		if err != nil {
			return false, "", fmt.Errorf("read the %s row's entry gate: %w", target.Name, err)
		}
		if !holds {
			return false, fmt.Sprintf("the %s row's entry gate does not hold: %s", target.Name, reason), nil
		}
	}
	if target.MaxBounces == 0 {
		return false, "the " + target.Name + " row's bounce budget is 0, so it approves without review", nil
	}
	return true, "", nil
}

// writeSendBackNote writes the friction note of send-back n under frictionDir.
// The note says Code-Review approved the diff the findings were raised against.
// It counts the findings by severity, lists each finding's summary and names the stored findings file at findingsPath.
// The note takes a free name, so a later send-back or an agent's own note never overwrites it.
// It writes nothing when frictionDir is empty or send-back n already has its note,
// so a re-run after a crash between the note and the record update keeps the first note.
func writeSendBackNote(frictionDir string, n int, findingsPath string, findings []burlerengine.Finding) error {
	if frictionDir == "" {
		return nil
	}
	if _, err := os.Stat(filepath.Join(frictionDir, prFindingsNotePrefix+fmt.Sprint(n)+".md")); err == nil {
		return nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	path := friction.NotePath(frictionDir, prFindingsNotePrefix+fmt.Sprint(n))
	if path == "" {
		return nil
	}
	friction.EnsureDir(frictionDir)

	severities := []burlerengine.Severity{burlerengine.SeverityBlocking, burlerengine.SeverityMedium, burlerengine.SeverityLow, burlerengine.SeverityNit}
	counts := make([]string, 0, len(severities))
	for _, severity := range severities {
		matching := 0
		for _, finding := range findings {
			if finding.Severity == severity {
				matching++
			}
		}
		counts = append(counts, fmt.Sprintf("%d %s", matching, severity))
	}

	var note strings.Builder
	fmt.Fprintf(&note, "# PR-Review sent findings back to Code-Review (send-back %d)\n\n", n)
	note.WriteString("Code-Review approved the diff these findings were raised against, and PR-Review found them afterwards.\n\n")
	fmt.Fprintf(&note, "Findings by severity: %s.\n\n", strings.Join(counts, ", "))
	for _, finding := range findings {
		fmt.Fprintf(&note, "- %s (%s): %s\n", finding.ID, finding.Severity, finding.Summary)
	}
	fmt.Fprintf(&note, "\nThe findings file: %s\n", findingsPath)
	return os.WriteFile(path, []byte(note.String()), 0o644)
}

// prFindingsVerb decides the findings set in the file at file, and returns the exit code.
// It reports a JSON envelope.
// The envelope's send_back is true with the stored file when the set goes back to Code-Review, and false otherwise.
// A set goes back when the send-back edge can fire and a finding is BLOCKING,
// or the findings plus the fixed-in-place count exceed the small maximum.
// Every finding counts, whatever its severity.
// A refusal or a failed read leaves the record unchanged.
// A record already holding the send-back mark answers the send-back again and writes nothing,
// so a re-run never sends twice.
// A fixed-in-place set is counted once, however often the verb runs over the same file.
// After any answer the verb commits the record, and a failed commit is an error envelope that a re-run completes.
func prFindingsVerb(out io.Writer, file string, deps prFindingsDeps) int {
	rerun := "run lym loom pr-findings " + file + " again"

	dirty, err := deps.dirty()
	if err != nil {
		return prFindingsReadFailure(out, file, "read the task worktree's uncommitted paths", err)
	}
	if len(dirty) > 0 {
		return output.Err(out, "loom: pr-findings: the task worktree has uncommitted paths: "+strings.Join(dirty, ", ")+"; way forward: commit or discard them, keeping the findings file under the task worktree's .scratch/ directory, which git ignores, then "+rerun)
	}

	content, err := os.ReadFile(file)
	var findings []burlerengine.Finding
	if err == nil {
		_, findings, err = burlerengine.ParseReview(content)
	}
	if err != nil {
		format := filepath.Join(deps.stencilsDir, filepath.FromSlash(prFindingsFormatStencil))
		return output.Err(out, "loom: pr-findings: "+file+" is not a findings file: "+err.Error()+"; way forward: write the findings in burler's review format, its verdict line and findings as "+format+" defines, then "+rerun)
	}

	rec, found, err := pullrequest.ReadRecord(deps.recordPath, deps.recordLockPath)
	if err != nil {
		return prFindingsReadFailure(out, file, "read the pull request record", err)
	}
	if !found {
		return output.Err(out, "loom: pr-findings: this run has no pull request record, so there is no review to send findings from; way forward: wait until the PR-Review row has opened the pull request, then "+rerun)
	}

	smallMax, err := deps.smallMax()
	if err != nil {
		return prFindingsReadFailure(out, file, "read pr_review_small_max", err)
	}
	canSendBack, reason, err := deps.edge()
	if err != nil {
		return prFindingsReadFailure(out, file, "read whether the send-back edge can fire", err)
	}

	answer := func(fields map[string]any) int {
		if err := deps.commit(); err != nil {
			return output.Err(out, "loom: pr-findings: commit the pull request record: "+err.Error()+"; way forward: "+rerun+", and tell the operator when it keeps failing")
		}
		return output.Ok(out, fields)
	}
	sendBackAnswer := func(stored string) int {
		return answer(map[string]any{
			"send_back": true,
			"file":      stored,
			"note":      "the findings go back to Code-Review; end your turn now while the run sends them",
		})
	}

	if rec.SendBack != "" {
		return sendBackAnswer(rec.SendBack)
	}

	sum := sha256.Sum256(content)
	hash := hex.EncodeToString(sum[:])
	counted := slices.Contains(rec.CountedSets, hash)
	total := rec.FixHereCount
	if !counted {
		total += len(findings)
	}
	blocking := slices.ContainsFunc(findings, func(finding burlerengine.Finding) bool { return finding.Severity == burlerengine.SeverityBlocking })

	if canSendBack && (blocking || total > smallMax) {
		n := rec.SendBacks + 1
		stored := prFindingsFile(deps.scratchDir, n)
		if err := os.MkdirAll(deps.scratchDir, 0o755); err != nil {
			return prFindingsReadFailure(out, file, "create the scratch directory", err)
		}
		if err := os.WriteFile(stored, content, 0o644); err != nil {
			return prFindingsReadFailure(out, file, "store the findings file", err)
		}
		if err := writeSendBackNote(deps.frictionDir, n, stored, findings); err != nil {
			return prFindingsReadFailure(out, file, "write the friction note", err)
		}
		err := pullrequest.UpdateRecord(deps.recordPath, deps.recordLockPath, func(current *pullrequest.Record) error {
			current.SendBack = stored
			current.SendBacks = n
			return nil
		})
		if err != nil {
			return prFindingsReadFailure(out, file, "write the pull request record", err)
		}
		return sendBackAnswer(stored)
	}

	if !counted {
		err := pullrequest.UpdateRecord(deps.recordPath, deps.recordLockPath, func(current *pullrequest.Record) error {
			current.FixHereCount += len(findings)
			current.CountedSets = append(current.CountedSets, hash)
			return nil
		})
		if err != nil {
			return prFindingsReadFailure(out, file, "write the pull request record", err)
		}
	}
	note := "fix these findings yourself, then go on with the review"
	if !canSendBack {
		note += "; they cannot go back to Code-Review because " + reason
	}
	return answer(map[string]any{
		"send_back":      false,
		"fix_here_count": total,
		"small_max":      smallMax,
		"note":           note,
	})
}

// prReviewSmallMax returns a reader of the run's pr_review_small_max over the pair's config layers.
// It reads at each call, so the task's run_settings, which land in the local layer, reach it.
func prReviewSmallMax(l *lymcwd.Location) func() (int, error) {
	return func() (int, error) {
		cfg, err := loomengine.LoadConfig(fabricengine.ConfigDirs(l), "loom")
		return cfg.PRReviewSmallMax, err
	}
}

// prReviewSendBackEdge reports whether the run's PR-Review row can send findings back.
// When it cannot, the reason comes with the answer.
// A recipe with no PR-Review row answers that it cannot.
// The landing seams are wired before the run's shed is built.
// The recipe's landing rows refuse to build without them.
func (c *loomCLI) prReviewSendBackEdge() (bool, string, error) {
	row, hasRow, err := loomrecipe.EngineRow(c.recipe, prReviewEngine)
	if err != nil {
		return false, "", err
	}
	if !hasRow {
		return false, "this run's recipe has no PR-Review row", nil
	}
	if err := c.wireLanding(); err != nil {
		return false, "", err
	}
	shed, err := c.newShed()
	if err != nil {
		return false, "", err
	}
	return sendBackEdge(shed.Producers, row)
}

// prFindingsCmd builds the `pr-findings` subcommand.
func (c *loomCLI) prFindingsCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "pr-findings <file>",
		Short:       "report the PR-Review findings in a file, so they go back to Code-Review or the agent fixes them",
		Annotations: map[string]string{clihelp.AudienceAnnotation: clihelp.AudienceRole},
		Long: `pr-findings decides what happens to the findings the PR-Review agent has
written in <file>, in burler's review format. Write the file under the task
worktree's .scratch/ directory, which git ignores, so it is never one of the
worktree's uncommitted paths. Run the verb on every findings set, after the
task worktree is committed.

When the set holds a BLOCKING finding, or the findings fixed in place since the
last send-back would pass pr_review_small_max, the verb stores the file,
marks the pull request record and answers send_back: true. End your turn then:
the run sends the findings back to Code-Review. Otherwise it answers
send_back: false and counts the findings, and you fix them yourself. It is
refused until the PR-Review row has opened the pull request, and while the task
worktree has uncommitted paths.

Example:
  lym loom pr-findings .scratch/pr-findings.md`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if clihelp.ShouldAbort(cmd.Context()) {
				return nil
			}
			code := prFindingsVerb(cmd.OutOrStdout(), args[0], prFindingsDeps{
				dirty:          func() ([]string, error) { return verifytree.DirtyPaths(c.location.WorktreePath()) },
				recordPath:     shedrun.PullRequestRecord(c.location, shedrun.SelfRunID),
				recordLockPath: shedrun.PullRequestRecordLock(c.location, shedrun.SelfRunID),
				scratchDir:     loomengine.LoomScratchDir(c.location),
				frictionDir:    c.frictionDir,
				stencilsDir:    c.runDeps.Geom.StencilsDir,
				smallMax:       prReviewSmallMax(c.location),
				edge:           c.prReviewSendBackEdge,
				commit:         pullRequestRecordCommitter(newPrePushCheck(c.location, shedrun.SelfRunID), c.location),
			})
			clihelp.SetExit(cmd.Context(), code)
			return nil
		},
	}
}
