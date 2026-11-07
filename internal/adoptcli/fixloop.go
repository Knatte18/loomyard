// fixloop.go files the fix task of a failed adopt check through the `fronttask` package, up to the try limit.
// It also words that task's slug and body.

package adoptcli

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/Knatte18/loomyard/internal/arbiterstop"
	"github.com/Knatte18/loomyard/internal/boardengine"
	"github.com/Knatte18/loomyard/internal/dobbyengine"
	"github.com/Knatte18/loomyard/internal/fabricengine"
	"github.com/Knatte18/loomyard/internal/fronttask"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/machinename"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
)

const (
	// fixSlugPrefixHead opens the slug of every fix task.
	fixSlugPrefixHead = "adopt-fix-"
	// fixSlugNumberRoom is the room the `-t999` suffix of a fix slug keeps within `boardengine.MaxSlugLength`.
	fixSlugNumberRoom = len("-t999")
	// fixSlugHashDigits is the count of hex digits of the subject's SHA-256 a cut prefix carries.
	fixSlugHashDigits = 6
	// fixExcerptLeadLines and fixExcerptLines bound each excerpt: lead lines before the test's first line, and total lines.
	fixExcerptLeadLines = 2
	fixExcerptLines     = 12
	// fixRecipe and fixProfile are the darn recipe and run profile every fix task runs.
	fixRecipe  = "darn"
	fixProfile = "light"
	// fixLabelBug is the type label every fix task carries; fixLabelGate is added when the board's vocabulary knows it.
	fixLabelBug  = "bug"
	fixLabelGate = "gate"
)

// fixDeps holds the seams of one subject's failed end.
// Subject names the subject; Dir is its scratch directory; Project is its project, none for `root`;
// Machine is this machine's name, and TryLimit is the try limit.
// Tasks reads the board's entries.
// RunRecord reads the batten run record of a slug, reporting false when none exists.
// EscalationEligible reports whether a slug's awaiting run stands at a producer whose escalation the arbiter admits.
// RunRecordSlugs lists the names of the batten run directories.
// TaskBranchSlugs lists the slugs starting with a prefix whose task branch exists.
// Known reports whether the board's vocabulary knows a label.
// FrontDeps are the seams the front task's filing and removal read.
type fixDeps struct {
	Subject            string
	Dir                string
	Project            string
	Machine            string
	TryLimit           int
	Tasks              func() ([]boardengine.Task, error)
	RunRecord          func(slug string) (shedengine.Status, bool, error)
	EscalationEligible func(slug string) bool
	RunRecordSlugs     func() ([]string, error)
	TaskBranchSlugs    func(prefix string) ([]string, error)
	Known              func(label string) bool
	FrontDeps          fronttask.Deps
}

// fixRunHalted reports whether the batten run that status describes is halted:
// its state is blocked or failed, or awaiting with no admitted escalation.
func fixRunHalted(status shedengine.Status, escalationEligible bool) bool {
	switch status.State {
	case shedengine.StateBlocked, shedengine.StateFailed:
		return true
	case shedengine.StateAwaiting:
		return !escalationEligible
	}
	return false
}

// fixSlugPrefix returns the slug prefix of the subject's fix tasks:
// `adopt-fix-<subject>` when that and `-t999` fit `boardengine.MaxSlugLength`.
// Otherwise the prefix is `adopt-fix-`, the subject cut short, and a hyphen;
// the tail is the first `fixSlugHashDigits` hex digits of the full subject's SHA-256.
// The cut leaves room for the tail, so two long subjects sharing their cut get two prefixes.
func fixSlugPrefix(subject string) string {
	if full := fixSlugPrefixHead + subject; len(full)+fixSlugNumberRoom <= boardengine.MaxSlugLength {
		return full
	}
	sum := sha256.Sum256([]byte(subject))
	tail := "-" + hex.EncodeToString(sum[:])[:fixSlugHashDigits]
	room := boardengine.MaxSlugLength - fixSlugNumberRoom - len(fixSlugPrefixHead) - len(tail)
	return fixSlugPrefixHead + strings.TrimRight(subject[:room], "-") + tail
}

// nextFixSlug returns `<prefix>-t<N>` with N one more than the highest N among the `<prefix>-t<N>` slugs in taken.
// N is 1 when none of them exists, and any other slug is ignored.
func nextFixSlug(prefix string, taken []string) string {
	highest := 0
	for _, slug := range taken {
		number, found := strings.CutPrefix(slug, prefix+"-t")
		if !found {
			continue
		}
		n, err := strconv.Atoi(number)
		if err != nil || n < 0 || strconv.Itoa(n) != number {
			continue
		}
		highest = max(highest, n)
	}
	return prefix + "-t" + strconv.Itoa(highest+1)
}

// failedTiers returns the failed tiers of record as `tier<N>` words and its failed tests as `<package> <test>` lines.
// The lines are capped at `maxListedFailures`, and more counts the failed tests past that cap.
func failedTiers(record CheckRecord) (tiers, tests []string, more int) {
	for _, tier := range record.Tiers {
		if tier.Outcome != checkFailed {
			continue
		}
		tiers = append(tiers, "tier"+strconv.Itoa(tier.Tier))
		for _, failure := range tier.Failures {
			if len(tests) == maxListedFailures {
				more++
				continue
			}
			tests = append(tests, failure.Package+" "+failure.Test)
		}
	}
	return tiers, tests, more
}

// fixBody renders the body of the subject's fix task.
// It names the failing tiers and failed tests, quoting an excerpt of suiteLog around each failed test's first line.
// It tells the agent to diagnose the failure first, and gives the stop condition for a machine failure.
// An excerpt starts a few lines before the test's first line in suiteLog and holds a bounded number of lines.
// A test whose first line, or its name when it has none, appears in no line of suiteLog gets no excerpt.
func fixBody(subject string, record CheckRecord, suiteLog string) string {
	tiers, _, _ := failedTiers(record)
	logLines := strings.Split(suiteLog, "\n")

	var body strings.Builder
	fmt.Fprintf(&body, "The adopt check of %s failed on this machine: %s.\n\n", subject, strings.Join(tiers, ", "))

	var listed, excerpts []string
	remaining := 0
	for _, tier := range record.Tiers {
		if tier.Outcome != checkFailed {
			continue
		}
		for _, failure := range tier.Failures {
			if len(listed) == maxListedFailures {
				remaining++
				continue
			}
			line := "- " + failure.Package + " " + failure.Test
			if failure.FirstLine != "" {
				line += ": " + failure.FirstLine
			}
			listed = append(listed, line)

			needle := failure.FirstLine
			if needle == "" {
				needle = failure.Test
			}
			index := slices.IndexFunc(logLines, func(line string) bool { return strings.Contains(line, needle) })
			if index < 0 {
				continue
			}
			start := max(0, index-fixExcerptLeadLines)
			end := min(len(logLines), start+fixExcerptLines)
			excerpts = append(excerpts, "```\n"+strings.Join(logLines[start:end], "\n")+"\n```")
		}
	}
	if len(listed) > 0 {
		body.WriteString("Failed tests:\n" + strings.Join(listed, "\n") + "\n")
		if remaining > 0 {
			fmt.Fprintf(&body, "and %d more\n", remaining)
		}
		body.WriteString("\n")
	}
	if len(excerpts) > 0 {
		body.WriteString("Excerpts of the check's suite.log:\n\n" + strings.Join(excerpts, "\n\n") + "\n\n")
	}

	body.WriteString("Diagnose the failure first. A code failure you fix and land as usual. A machine failure, such as a missing tool, an old version or timing like a security agent holding new binaries, has nothing to commit.\n\n")
	fmt.Fprintf(&body, "Stop condition: the failure is a machine failure. Write a stop report naming the exact steps the operator must take on this machine and, as its way forward, a re-run of lym adopt %s once the machine is settled, never lym loom resume.\n", subject)
	return body.String()
}

// failedEnd handles a failed adopt check that recorded a failed tier, and returns the lines of its notice.
// In order, each case ends the call with its lines:
//  1. The subject's own fix task stands open and healthy:
//     released, starting, running, paused, or awaiting with an admitted escalation.
//     Nothing is filed or removed.
//  2. Another front task stands open in the subject's worktree.
//     Nothing is filed or removed.
//  3. The try count is at the limit.
//     Nothing is filed or removed.
//  4. The own fix task is a draft or halted, and it is removed.
//     A failed removal files nothing.
//  5. A fix task is filed with the next free slug.
//     The try count rises by one between the entry's creation and its release.
//
// It never starts a pass of the task: its parent's dobby starts it from the empty status.
func failedEnd(deps fixDeps, record CheckRecord) []string {
	tasks, err := deps.Tasks()
	if err != nil {
		return []string{fixReadError(deps, "the board", err)}
	}
	tries, err := readTries(deps.Dir)
	if err != nil {
		return []string{fixReadError(deps, "the fix-try count", err)}
	}

	own, hasOwn := fronttask.FrontOf(tasks, deps.Project, fronttask.ActionAdoptCheck)
	ownHalted := false
	if hasOwn {
		var state, handStep string
		state, handStep, ownHalted, err = judgeOwnFix(deps, own)
		if err != nil {
			return []string{fixReadError(deps, "the run record of "+own.Slug, err)}
		}
		if !ownHalted {
			return []string{fmt.Sprintf("the fix task %s of %s stands %s, so no new one was filed: its landing starts the next check; %s", own.Slug, deps.Subject, state, handStep)}
		}
	}

	others := slices.DeleteFunc(slices.Clone(tasks), func(task boardengine.Task) bool { return hasOwn && task.Slug == own.Slug })
	if other, found := fronttask.OpenFront(others, deps.Project); found {
		return []string{fmt.Sprintf("the front task %s stands open in the worktree of %s, so no fix task was filed: a front task of another action starts no check; re-run lym adopt %s once %s lands", other.Slug, deps.Subject, deps.Subject, other.Slug)}
	}

	failedTierWords, failedTests, more := failedTiers(record)
	if tries >= deps.TryLimit {
		line := fmt.Sprintf("the fix tries of %s ran out (%d of %d), so no fix task was filed: %s failed", deps.Subject, tries, deps.TryLimit, strings.Join(failedTierWords, ", "))
		if len(failedTests) > 0 {
			line += " in " + strings.Join(failedTests, ", ")
			if more > 0 {
				line += fmt.Sprintf(" and %d more", more)
			}
		}
		line += "; suite.log in " + deps.Dir + " holds the output"
		if hasOwn {
			line += fmt.Sprintf("; the fix task %s stands halted", own.Slug)
		}
		return []string{line + fmt.Sprintf("; re-run lym adopt %s once you have settled it, whose check removes that task", deps.Subject)}
	}

	// taken is read before the removal, so the slug of a removed task is not given out again.
	taken, err := takenFixSlugs(deps, tasks)
	if err != nil {
		return []string{fixFilingError(deps, err)}
	}
	if hasOwn {
		if err := fronttask.Remove(deps.FrontDeps, own.Slug); err != nil {
			return []string{fixRemovalError(deps, own.Slug, err)}
		}
	}

	slug := nextFixSlug(fixSlugPrefix(deps.Subject), taken)
	suiteLog, _ := os.ReadFile(filepath.Join(deps.Dir, suiteLogName))
	labels := []string{fixLabelBug}
	if deps.Known(fixLabelGate) {
		labels = append(labels, fixLabelGate)
	}
	request := fronttask.Request{
		Slug:    slug,
		Title:   fmt.Sprintf("Fix the adopt check of %s on %s", deps.Subject, deps.Machine),
		Body:    fixBody(deps.Subject, record, string(suiteLog)),
		Labels:  labels,
		Recipe:  fixRecipe,
		Profile: fixProfile,
		Project: deps.Project,
		OnLand:  fronttask.ActionAdoptCheck,
	}
	if err := fronttask.File(deps.FrontDeps, request, func() error { return writeTries(deps.Dir, tries+1) }); err != nil {
		return []string{fixFilingError(deps, err)}
	}
	return []string{fmt.Sprintf("filed the fix task %s for the failed adopt check of %s (try %d of %d); it is the only run that starts: its parent's dobby starts it ahead of the worktree's other tasks, and its landing checks again", slug, deps.Subject, tries+1, deps.TryLimit)}
}

// judgeOwnFix reads the state of the subject's own fix task.
// halted is true for a task to remove: a draft or other closed status, or a run the batten run record reads halted.
// Otherwise state describes the open task.
// handStep names what the operator takes to move or drop it.
func judgeOwnFix(deps fixDeps, own boardengine.Task) (state, handStep string, halted bool, err error) {
	drop := fmt.Sprintf("to drop it run `lym batten cancel %s --remove`, then `%s`, whose removal runs its adopt-check action", own.Slug, boardengine.RemoveCommand(own.Slug))
	if boardengine.IsEmptyStatus(own.Status) {
		return "released and waits for its dobby to start it", drop, false, nil
	}
	if !boardengine.IsRunStatus(own.Status) {
		return "", "", true, nil
	}

	status, found, err := deps.RunRecord(own.Slug)
	if err != nil {
		return "", "", false, err
	}
	if !found {
		return "starting", drop, false, nil
	}
	eligible := status.State == shedengine.StateAwaiting && deps.EscalationEligible(own.Slug)
	if fixRunHalted(status, eligible) {
		return "", "", true, nil
	}
	if status.State == shedengine.StatePaused {
		return "paused", fmt.Sprintf("`lym batten run %s` from the prime resumes it at the row it halted at, and %s", own.Slug, drop), false, nil
	}
	return string(status.State), drop, false, nil
}

// takenFixSlugs returns the slugs a new fix task of the subject must not reuse:
// the board's, the batten run directories' and the task branches' of its prefix.
func takenFixSlugs(deps fixDeps, tasks []boardengine.Task) ([]string, error) {
	taken := make([]string, 0, len(tasks))
	for _, task := range tasks {
		taken = append(taken, task.Slug)
	}
	runs, err := deps.RunRecordSlugs()
	if err != nil {
		return nil, fmt.Errorf("list the batten run records: %w", err)
	}
	branches, err := deps.TaskBranchSlugs(fixSlugPrefix(deps.Subject))
	if err != nil {
		return nil, fmt.Errorf("list the task branches: %w", err)
	}
	return append(append(taken, runs...), branches...), nil
}

// fixReadError words a failed read of the fix loop, which files nothing.
func fixReadError(deps fixDeps, what string, err error) string {
	return fmt.Sprintf("%s was not read (%v), so no fix task of %s was filed; way forward: fix the cause it names, then re-run lym adopt %s, which resets the count and checks again, the holds standing meanwhile", what, err, deps.Subject, deps.Subject)
}

// fixFilingError words a filing that failed, which files nothing more.
func fixFilingError(deps fixDeps, err error) string {
	return fmt.Sprintf("the fix task of %s was not filed (%v); way forward: fix the cause it names, then re-run lym adopt %s, which resets the count and checks again, the holds standing meanwhile", deps.Subject, err, deps.Subject)
}

// fixRemovalError words a failed removal of the halted or draft fix task slug.
// A step before the record's drop leaves the record standing, so the hand removal alone is the way forward;
// a failure of the entry's removal adds the re-run.
func fixRemovalError(deps fixDeps, slug string, err error) string {
	line := fmt.Sprintf("the fix task %s of %s was not removed, so no new one was filed: %v", slug, deps.Subject, err)
	var removal *fronttask.RemoveError
	if errors.As(err, &removal) && removal.Step == "entry" {
		line += fmt.Sprintf("; then re-run lym adopt %s", deps.Subject)
	}
	return line
}

// hubFixDeps fills fixDeps for the failed end of the check of subject over the hub of prime.
// The try limit is `adopt_fix_tries` of the config stack of the subject's worktree,
// and the board's vocabulary is the prime's.
func hubFixDeps(prime *lymcwd.Location, subject string) (fixDeps, error) {
	worktree, err := resolveSubjectWorktree(prime, subject)
	if err != nil {
		return fixDeps{}, err
	}
	beam, err := dobbyengine.LoadConfig(fabricengine.ConfigDirs(worktree))
	if err != nil {
		return fixDeps{}, err
	}
	board, err := boardengine.LoadConfig(fabricengine.ConfigDirs(prime), "board")
	if err != nil {
		return fixDeps{}, err
	}
	machine, err := machinename.Name()
	if err != nil {
		return fixDeps{}, err
	}
	frontDeps, err := fronttask.HubDeps(prime)
	if err != nil {
		return fixDeps{}, err
	}
	boardDir := fabricengine.BoardDir(prime.HubPath)
	project := ""
	if subject != boardengine.RootSubject {
		project = subject
	}
	return fixDeps{
		Subject:  subject,
		Dir:      subjectDir(prime.HubPath, subject),
		Project:  project,
		Machine:  machine,
		TryLimit: beam.AdoptFixTries,
		Tasks:    func() ([]boardengine.Task, error) { return boardengine.ReadTasks(boardDir) },
		RunRecord: func(slug string) (shedengine.Status, bool, error) {
			return readStatus(shedrun.StatusFile(prime, slug))
		},
		EscalationEligible: hubEscalationEligible(prime),
		RunRecordSlugs: func() ([]string, error) {
			entries, err := os.ReadDir(filepath.Dir(shedrun.RunDir(prime, "-")))
			if errors.Is(err, os.ErrNotExist) {
				return nil, nil
			}
			if err != nil {
				return nil, err
			}
			var slugs []string
			for _, entry := range entries {
				if entry.IsDir() {
					slugs = append(slugs, entry.Name())
				}
			}
			return slugs, nil
		},
		TaskBranchSlugs: func(prefix string) ([]string, error) { return fabricengine.TaskBranchSlugs(prime, prefix) },
		Known:           board.Vocabulary().Known,
		FrontDeps:       frontDeps,
	}, nil
}

// hubEscalationEligible returns the escalation read of the hub of prime:
// whether the batten run of a slug stands at a producer whose escalation the arbiter admits.
// The read goes through the readers of the slug's pair.
// A run record or a pair that does not read counts as not eligible.
func hubEscalationEligible(prime *lymcwd.Location) func(slug string) bool {
	return func(slug string) bool {
		status, found, err := readStatus(shedrun.StatusFile(prime, slug))
		if err != nil || !found {
			return false
		}
		pair, err := lymcwd.ResolveWorktree(fabricengine.WorktreePath(prime, slug))
		if err != nil {
			return false
		}
		return arbiterstop.EscalationEligible(status.CurrentProducer, arbiterstop.NewReaders(pair))
	}
}
