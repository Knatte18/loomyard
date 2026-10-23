// publishfailure.go turns the Publish failure record into the extra steps the verify gate runs to confirm a fix.
// The record sits in the pair's own ignored directory, so every field is shape-checked before it reaches a command line.
// It also renders webster's run record as the Plan-Review rubric's note.

package loomshed

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
	"unicode"

	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/planparser"
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
	// tracedLocation is a relative path of letters, digits and `_.~@+-` segments, none starting with `-`, joined by `/`, then a colon and a line number.
	tracedLocation = regexp.MustCompile(`^[\p{L}\p{N}_.][\p{L}\p{N}_.~@+-]*(/[\p{L}\p{N}_.][\p{L}\p{N}_.~@+-]*)*:[0-9]+$`)
)

// failingGate resolves the gate and the failing tier a record names, reading the gate's tier list through tiers.
// A record of a retired kind maps onto the `landing` or `publish` gate with the highest tier of that gate's list, and so does a record whose tier is not on its gate's list.
// It is an error when tiers fails, when the gate's list is empty, and for a kind outside the closed set.
func failingGate(failure verifytree.PublishFailure, tiers func(gate string) ([]int, error)) (verifytree.FailureKind, int, error) {
	var kind verifytree.FailureKind
	retired := false
	switch failure.Kind {
	case verifytree.FailureKindLanding:
		kind = verifytree.FailureKindLanding
	case verifytree.FailureKindPublish:
		kind = verifytree.FailureKindPublish
	case verifytree.FailureKindPlanVerify:
		kind, retired = verifytree.FailureKindLanding, true
	case verifytree.FailureKindPublishVerify:
		kind, retired = verifytree.FailureKindPublish, true
	default:
		return "", 0, fmt.Errorf("loomshed: publish failure record has the unknown kind %q", failure.Kind)
	}
	list, err := tiers(string(kind))
	if err != nil {
		return "", 0, fmt.Errorf("loomshed: read the %s gate's tiers: %w", kind, err)
	}
	if len(list) == 0 {
		return "", 0, fmt.Errorf("loomshed: the %s gate lists no tier", kind)
	}
	if !retired && slices.Contains(list, failure.Tier) {
		return kind, failure.Tier, nil
	}
	return kind, slices.Max(list), nil
}

// checkedPublishFailure reads the Publish failure record at paths and returns it with every field that fails its shape check dropped, each drop logged.
// It returns false when there is no record, when it cannot be read, when its kind is not one of the closed set, or when tiers cannot give the gate's list.
// The returned record names the gate and tier failingGate resolves, so a record of a retired kind comes back as a `landing` or `publish` record.
//
// The merge-in commit is kept only as a full hex object name that resolves, through worktreeRoot's git objects, to a commit with two or more parents.
// A failing test is kept only when its path is a top-level Test identifier followed by optional subtest segments of safe characters and its package is an import path that starts with a letter or digit, so it never reads as a flag.
// The log path is kept only when it names a verify log directly inside the verify directory.
func checkedPublishFailure(paths verifytree.Paths, worktreeRoot string, tiers func(gate string) ([]int, error)) (verifytree.PublishFailure, bool) {
	failure, ok, err := verifytree.ReadPublishFailure(paths)
	if err != nil {
		logger.Warn("loomshed: publish failure record unreadable, round gate ignores it", "path", paths.PublishFailure, "cause", err)
		return verifytree.PublishFailure{}, false
	}
	if !ok {
		return verifytree.PublishFailure{}, false
	}
	kind, tier, err := failingGate(failure, tiers)
	if err != nil {
		logger.Warn("loomshed: publish failure record's gate and tier unresolved, round gate ignores it", "kind", failure.Kind, "cause", err)
		return verifytree.PublishFailure{}, false
	}
	failure.Kind, failure.Tier = kind, tier

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

	if failure.LogPath != "" && !verifytree.IsLogPath(paths, failure.LogPath) {
		logger.Warn("loomshed: publish failure record's log path dropped, it is not a verify log in the verify directory", "logPath", failure.LogPath)
		failure.LogPath = ""
	}

	failure.Cards = checkedTracedCards(failure.Cards)
	failure.LaterRange = checkedCommitRange(failure.LaterRange)
	failure.Untraced = checkedLocations(failure.Untraced)
	return failure, true
}

// checkedTracedCards returns cards with every element that fails its shape check dropped, each drop logged.
// A card needs a positive number; a glyph must be non-empty and hold no whitespace, control character or backtick, a location must match tracedLocation, and a range keeps both ends only when each is a full hex object name.
// The glyph check is a prompt-safety character check, not a glyph grammar.
func checkedTracedCards(cards []verifytree.TracedCard) []verifytree.TracedCard {
	kept := cards[:0:0]
	for _, card := range cards {
		if card.Number <= 0 {
			logger.Warn("loomshed: publish failure record's traced card dropped, its number is not positive", "number", card.Number)
			continue
		}
		glyphs := card.Glyphs[:0:0]
		for _, glyph := range card.Glyphs {
			if !promptSafeGlyph(glyph) {
				logger.Warn("loomshed: publish failure record's traced glyph dropped", "card", card.Number, "glyph", glyph)
				continue
			}
			glyphs = append(glyphs, glyph)
		}
		card.Glyphs = glyphs
		card.Locations = checkedLocations(card.Locations)
		card.Range = checkedCommitRange(card.Range)
		kept = append(kept, card)
	}
	return kept
}

// promptSafeGlyph reports whether glyph is non-empty and holds no whitespace, control character or backtick, so it can sit in a code span of the prompt.
func promptSafeGlyph(glyph string) bool {
	if glyph == "" || strings.ContainsRune(glyph, '`') {
		return false
	}
	return !strings.ContainsFunc(glyph, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) })
}

// checkedLocations returns the entries of locations that match tracedLocation, each drop logged.
func checkedLocations(locations []string) []string {
	kept := locations[:0:0]
	for _, location := range locations {
		if !tracedLocation.MatchString(location) {
			logger.Warn("loomshed: publish failure record's traced location dropped", "location", location)
			continue
		}
		kept = append(kept, location)
	}
	return kept
}

// checkedCommitRange returns commits unchanged when both ends are full hex object names, and the empty range otherwise.
func checkedCommitRange(commits verifytree.CommitRange) verifytree.CommitRange {
	if commits == (verifytree.CommitRange{}) {
		return commits
	}
	if !fullObjectName.MatchString(commits.From) || !fullObjectName.MatchString(commits.To) {
		logger.Warn("loomshed: publish failure record's commit range dropped", "from", commits.From, "to", commits.To)
		return verifytree.CommitRange{}
	}
	return commits
}

// isMergeCommit reports whether sha is a full hex object name of a commit in worktreeRoot with two or more parents.
func isMergeCommit(worktreeRoot, sha string) bool {
	if !fullObjectName.MatchString(sha) {
		return false
	}
	parents, err := gitrepo.New(worktreeRoot).CommitParents(sha)
	return err == nil && len(parents) >= 2
}

// publishFailureSteps builds the gate's extra steps from a checked record: failedTests, run before the gate's own command, and packagePass, run after it, each joined by ` && ` or "" when there are none.
// failedTests is one `lyx gate test --wait --tier <failing tier> <package> -- -run '^<Test>$'` per failing top-level test, its package an explicit target.
// The subtests of one top-level test collapse into one step, since a whole test re-runs and a subtest filter can miss a failure the test's setup caused; steps keep the record's order of first appearance.
// packagePass is one `lyx gate test --wait --tier <failing tier>` over the packages of the failing tests; with no failing test it is "".
// The failing tier is the one failingGate resolves through tiers, and an error from it is returned.
// Nothing from the record is run as shell text: every argument is a checked identifier, and the `-run` name is quoted.
func publishFailureSteps(failure verifytree.PublishFailure, tiers func(gate string) ([]int, error)) (failedTests, packagePass string, err error) {
	_, tier, err := failingGate(failure, tiers)
	if err != nil {
		return "", "", err
	}
	var steps, packages []string
	seen := map[string]bool{}
	seenPackage := map[string]bool{}
	for _, test := range failure.Tests {
		pattern := anchoredRunPattern(test.Test)
		key := test.Package + " " + pattern
		if !seen[key] {
			seen[key] = true
			steps = append(steps, fmt.Sprintf("lyx gate test --wait --tier %d %s -- -run '%s'", tier, test.Package, pattern))
		}
		if !seenPackage[test.Package] {
			seenPackage[test.Package] = true
			packages = append(packages, test.Package)
		}
	}
	if len(packages) > 0 {
		packagePass = fmt.Sprintf("lyx gate test --wait --tier %d %s", tier, strings.Join(packages, " "))
	}
	return strings.Join(steps, " && "), packagePass, nil
}

// PublishFailureNote renders the checked Publish failure record of the verify directory as markdown for the Webster-Review rubric, or `none` when there is no record.
// The note names the failing gate and tier, the failing tests, the log path, and the merge-in commit with the command that reads what it brought in.
// A field that failed its shape check is absent, so no free text from the record reaches the prompt.
// tiers reads a gate's tier list; a record it cannot place on a gate renders `none`.
//
// A record that carries a trace of the failure also renders each traced card with its number, its slug and first intent line read from the plan in planDir at render time, its matched members and locations, and the commit range to diff when starting the fix;
// then the later commits, and the locations no card holds.
// A card number the plan no longer holds renders its number alone, and a plan that does not parse renders the numbers alone and logs a warning, so the note never fails.
func PublishFailureNote(worktreeRoot, verifyDir, planDir string, tiers func(gate string) ([]int, error)) string {
	failure, ok := checkedPublishFailure(verifytree.NewPaths(worktreeRoot, verifyDir), worktreeRoot, tiers)
	if !ok {
		return "none"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Publish failed on tier %d of the `%s` gate.\n", failure.Tier, failure.Kind)
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
	writeFailureTrace(&b, failure, planDir)
	return strings.TrimRight(b.String(), "\n")
}

// writeFailureTrace renders the trace fields of a checked record into b: the traced cards, the later range and the untraced locations.
// It writes nothing for a record without them.
func writeFailureTrace(b *strings.Builder, failure verifytree.PublishFailure, planDir string) {
	if len(failure.Cards) > 0 {
		b.WriteString("\nPlan cards the failing locations trace to:\n\n")
		livePlan := parseTracePlan(planDir)
		for _, traced := range failure.Cards {
			fmt.Fprintf(b, "- card %d", traced.Number)
			if live, found := liveCard(livePlan, traced.Number); found {
				fmt.Fprintf(b, " `%s`", live.Slug)
				if line := firstIntentLine(live.Intent); line != "" {
					fmt.Fprintf(b, ": %s", line)
				}
			}
			b.WriteString("\n")
			if len(traced.Glyphs) > 0 {
				fmt.Fprintf(b, "  - members: %s\n", codeSpans(traced.Glyphs))
			}
			if len(traced.Locations) > 0 {
				fmt.Fprintf(b, "  - locations: %s\n", codeSpans(traced.Locations))
			}
			if traced.Range == (verifytree.CommitRange{}) {
				b.WriteString("  - Webster holds no commit range for this card.\n")
			} else {
				fmt.Fprintf(b, "  - range to diff when starting the fix: `git diff %s %s`\n", traced.Range.From, traced.Range.To)
			}
		}
	}
	if failure.LaterRange != (verifytree.CommitRange{}) {
		fmt.Fprintf(b, "\nLater commits touching the same members: `git diff %s %s`\n", failure.LaterRange.From, failure.LaterRange.To)
	}
	if len(failure.Untraced) > 0 {
		fmt.Fprintf(b, "\nLocations no plan card holds: %s\n", codeSpans(failure.Untraced))
	}
}

// parseTracePlan parses the plan in planDir for a note's card lines, and returns nil when it does not parse.
// The failure is logged at Warn, since the note renders the card numbers alone.
func parseTracePlan(planDir string) *planparser.Plan {
	plan, err := planparser.ParsePlan(planDir)
	if err != nil {
		logger.Warn("loomshed: the plan could not be read for the publish failure note, so it names card numbers alone", "planDir", planDir, "cause", err)
		return nil
	}
	return plan
}

// liveCard returns the card of plan numbered number, and false when plan is nil or holds none.
func liveCard(plan *planparser.Plan, number int) (planparser.Card, bool) {
	if plan == nil {
		return planparser.Card{}, false
	}
	for _, card := range plan.Cards {
		if card.Number == number {
			return card, true
		}
	}
	return planparser.Card{}, false
}

// firstIntentLine returns the first non-blank line of a card's intent, trimmed.
func firstIntentLine(intent string) string {
	for _, line := range strings.Split(intent, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// codeSpans renders each of items as a markdown code span, joined by commas.
func codeSpans(items []string) string {
	spans := make([]string, len(items))
	for i, item := range items {
		spans[i] = "`" + item + "`"
	}
	return strings.Join(spans, ", ")
}

// anchoredRunPattern spells a test path as a `-run` pattern matching exactly its top-level test: the first `/`-separated segment is quoted for regexp and anchored, and the subtest path is dropped.
func anchoredRunPattern(testPath string) string {
	topLevel, _, _ := strings.Cut(testPath, "/")
	return "^" + regexp.QuoteMeta(topLevel) + "$"
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
