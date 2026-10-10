// publishfailure.go turns the Publish failure record into the extra steps the Webster-Burler round gate runs to confirm a fix.
// The record sits in the pair's own ignored directory, so every field is shape-checked before it reaches a command line.
// It also renders webster's run record as the Plan-Review rubric's note.

package loomshed

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/verifytree"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

var (
	// fullObjectName is a full SHA-1 or SHA-256 hex object name.
	fullObjectName = regexp.MustCompile(`^([0-9a-f]{40}|[0-9a-f]{64})$`)
	// failedTestPath is a top-level Test identifier followed by `/`-separated subtest segments of letters, digits and `_.=,:+@-`.
	failedTestPath = regexp.MustCompile(`^Test[\p{L}\p{N}_]*(/[\p{L}\p{N}_.=,:+@-]+)*$`)
	// failedTestPackage is an import path that starts with a letter or digit: segments of letters, digits and `_.~-` joined by `/`.
	failedTestPackage = regexp.MustCompile(`^[\p{L}\p{N}][\p{L}\p{N}_.~-]*(/[\p{L}\p{N}_.~-]+)*$`)
)

// checkedPublishFailure reads the Publish failure record at paths and returns it with every field that fails its shape check dropped, each drop logged.
// It returns false when there is no record, when it cannot be read, or when its kind is not one of the closed set.
//
// The merge-in commit is kept only as a full hex object name that resolves, through worktreeRoot's git objects, to a commit with two or more parents.
// A failing test is kept only when its path is a top-level Test identifier followed by optional subtest segments of safe characters and its package is an import path that starts with a letter or digit, so it never reads as a flag.
// The log path is kept only when it is the verify directory's failure log copy.
func checkedPublishFailure(paths verifytree.Paths, worktreeRoot string) (verifytree.PublishFailure, bool) {
	failure, ok, err := verifytree.ReadPublishFailure(paths)
	if err != nil {
		logger.Warn("loomshed: publish failure record unreadable, round gate ignores it", "path", paths.PublishFailure, "cause", err)
		return verifytree.PublishFailure{}, false
	}
	if !ok {
		return verifytree.PublishFailure{}, false
	}
	if failure.Kind != verifytree.FailureKindPlanVerify && failure.Kind != verifytree.FailureKindPublishVerify {
		logger.Warn("loomshed: publish failure record has an unknown kind, round gate ignores it", "kind", failure.Kind)
		return verifytree.PublishFailure{}, false
	}

	if failure.MergeCommit != "" && !isMergeCommit(worktreeRoot, failure.MergeCommit) {
		logger.Warn("loomshed: publish failure record's merge commit dropped", "mergeCommit", failure.MergeCommit)
		failure.MergeCommit = ""
	}

	kept := failure.Tests[:0:0]
	for _, test := range failure.Tests {
		if !failedTestPackage.MatchString(test.Package) || !failedTestPath.MatchString(test.Test) {
			logger.Warn("loomshed: publish failure record's test dropped", "package", test.Package, "test", test.Test)
			continue
		}
		kept = append(kept, test)
	}
	failure.Tests = kept

	if failure.LogPath != "" && filepath.Clean(failure.LogPath) != paths.PublishFailureLog {
		logger.Warn("loomshed: publish failure record's log path dropped, it is not the verify directory's failure log copy", "logPath", failure.LogPath)
		failure.LogPath = ""
	}
	return failure, true
}

// isMergeCommit reports whether sha is a full hex object name of a commit in worktreeRoot with two or more parents.
func isMergeCommit(worktreeRoot, sha string) bool {
	if !fullObjectName.MatchString(sha) {
		return false
	}
	parents, err := gitrepo.New(worktreeRoot).CommitParents(sha)
	return err == nil && len(parents) >= 2
}

// publishFailureCommand builds the round gate's extra steps from a checked record, joined by ` && `, or "" when there are none.
// Each failing test runs by name in its package under the failing verify's tier: `integration` for the plan verify, `tmux` for `publish_verify`.
// For `publish_verify` the `tmux` tier also runs over packages, the impacted set; with no packages that pass is skipped.
// Nothing from the record is run as shell text: every argument is a checked identifier, and each `-run` segment is quoted.
func publishFailureCommand(failure verifytree.PublishFailure, packages []string) string {
	tag := "integration"
	if failure.Kind == verifytree.FailureKindPublishVerify {
		tag = "tmux"
	}
	var steps []string
	for _, test := range failure.Tests {
		steps = append(steps, fmt.Sprintf("go test -tags %s -run '%s' %s", tag, anchoredRunPattern(test.Test), test.Package))
	}
	if failure.Kind == verifytree.FailureKindPublishVerify && len(packages) > 0 {
		steps = append(steps, "go test -tags tmux "+strings.Join(packages, " "))
	}
	return strings.Join(steps, " && ")
}

// PublishFailureNote renders the checked Publish failure record of the verify directory as markdown for the Webster-Review rubric, or `none` when there is no record.
// The note names the failing verify, the failing tests, the log path, and the merge-in commit with the command that reads what it brought in.
// A field that failed its shape check is absent, so no free text from the record reaches the prompt.
func PublishFailureNote(worktreeRoot, verifyDir string) string {
	failure, ok := checkedPublishFailure(verifytree.NewPaths(worktreeRoot, verifyDir), worktreeRoot)
	if !ok {
		return "none"
	}
	var b strings.Builder
	switch failure.Kind {
	case verifytree.FailureKindPlanVerify:
		b.WriteString("Publish failed on the plan's `## verify:` command.\n")
	case verifytree.FailureKindPublishVerify:
		b.WriteString("Publish failed on landing config's `publish_verify` command.\n")
	}
	if len(failure.Tests) > 0 {
		b.WriteString("\nFailing tests:\n\n")
		for _, test := range failure.Tests {
			fmt.Fprintf(&b, "- `%s` in `%s`\n", test.Test, test.Package)
		}
	}
	if failure.LogPath != "" {
		fmt.Fprintf(&b, "\nLog of the failing run: %s\n", failure.LogPath)
	}
	if failure.MergeCommit != "" {
		fmt.Fprintf(&b, "\nMerge-in commit Publish made: %s\nRead what it brought in with `git diff %s^1 %s`.\n", failure.MergeCommit, failure.MergeCommit, failure.MergeCommit)
	}
	return strings.TrimRight(b.String(), "\n")
}

// anchoredRunPattern spells a test path as a `-run` pattern matching exactly that test: each `/`-separated segment is quoted for regexp and anchored.
func anchoredRunPattern(testPath string) string {
	segments := strings.Split(testPath, "/")
	for i, segment := range segments {
		segments[i] = "^" + regexp.QuoteMeta(segment) + "$"
	}
	return strings.Join(segments, "/")
}

// WebsterRecordNote renders webster's run record under anchorPath as markdown for the Plan-Review rubric, or `none` when there is no record or it holds no begun batch.
// The note names each begun batch with its status and card ids, the done cards, and the rule that a done card is frozen.
// A record that cannot be read is logged at Warn and renders a fixed note saying the frozen cards are unknown, so the reviewer is never told nothing is frozen and a rubric read never fails on it.
func WebsterRecordNote(anchorPath string) string {
	st, err := websterengine.LoadState(websterengine.Dir(anchorPath), websterengine.ScratchDir(anchorPath))
	if err != nil {
		logger.Warn("loomshed: webster's run record could not be read for the plan review", "anchorPath", anchorPath, "error", err)
		return "Webster's run record could not be read, so which cards are frozen is unknown; the plan gate reports the read error."
	}
	if st == nil || len(st.Batches) == 0 {
		return "none"
	}

	numbers := make([]int, 0, len(st.Batches))
	for number, batch := range st.Batches {
		if batch != nil {
			numbers = append(numbers, number)
		}
	}
	sort.Ints(numbers)

	var begun, done []string
	for _, number := range numbers {
		batch := st.Batches[number]
		cards := batch.Cards
		if len(cards) == 0 {
			cards = []string{fmt.Sprintf("%02d-%s", number, batch.Slug)}
		}
		status := "in flight"
		if batch.Terminal {
			status = batch.Status
		}
		begun = append(begun, fmt.Sprintf("- batch %d (%s): %s", number, status, strings.Join(cards, ", ")))
		if batch.Terminal && batch.Status == websterengine.DigestStatusDone {
			done = append(done, cards...)
		}
	}

	var b strings.Builder
	b.WriteString("Webster has begun these batches:\n\n")
	b.WriteString(strings.Join(begun, "\n"))
	if len(done) > 0 {
		fmt.Fprintf(&b, "\n\nDone cards: %s", strings.Join(done, ", "))
	}
	b.WriteString("\n\nA done card is frozen: its work has landed, and the plan gate refuses an edit to it.\nA finding against a done card lands as a follow-up card after the last begun batch, with its Card Index line in `00-overview.md`.")
	return b.String()
}
