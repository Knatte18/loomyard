// renumber.go implements InsertCard and RemoveCard, the plan's write paths that add or drop one card, renumber the cards after it and rewrite the card ids naming them.
// It also holds the plan scratch paths and the journal that makes an interrupted call restorable.

package planparser

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/Knatte18/loomyard/internal/lymdirs"
)

// renumberJournalFileName is the journal's file name under the plan scratch directory.
const renumberJournalFileName = "renumber-journal.json"

// renumberJournalTempSuffix is appended to the journal path to form the temporary name the journal is written under before it is renamed into place.
const renumberJournalTempSuffix = ".tmp"

// PlanScratchDir returns the absolute path to the plan directory's never-tracked mirror, `<anchor>/.lym/plan`, for the worktree anchored at anchorPath.
// The caller supplies the anchor as for PlanDir.
func PlanScratchDir(anchorPath string) string {
	return filepath.Join(anchorPath, lymdirs.DotLymDirName, PlanDirName)
}

// RenumberJournalPath returns the absolute path to the renumber journal under the plan scratch directory of the worktree anchored at anchorPath.
func RenumberJournalPath(anchorPath string) string {
	return filepath.Join(PlanScratchDir(anchorPath), renumberJournalFileName)
}

// RenumberIO is the raw file seam the renumber performs every file operation through.
// Each field has the signature of its `os` namesake; OSRenumberIO fills them from `os`, and a test substitutes a field to fail one operation on one file.
type RenumberIO struct {
	ReadFile  func(name string) ([]byte, error)
	WriteFile func(name string, data []byte, perm os.FileMode) error
	Rename    func(oldpath, newpath string) error
	Remove    func(name string) error
	MkdirAll  func(path string, perm os.FileMode) error
}

// OSRenumberIO returns a RenumberIO backed by the `os` package.
func OSRenumberIO() RenumberIO {
	return RenumberIO{
		ReadFile:  os.ReadFile,
		WriteFile: os.WriteFile,
		Rename:    os.Rename,
		Remove:    os.Remove,
		MkdirAll:  os.MkdirAll,
	}
}

// MovedCard is one card whose number, and so whose file name, a renumber changed.
type MovedCard struct {
	OldNumber int
	NewNumber int
	OldFile   string
	NewFile   string
}

// ProseMention is one line that refers to a card by an old number of a moved card and was left unchanged.
type ProseMention struct {
	File string
	Line int
	Text string
}

// RewrittenRef is one card-id reference to a moved card that a renumber rewrote to the card's new number.
// File is the plan file as the renumber leaves it, and From and To are the reference before and after.
type RewrittenRef struct {
	File string
	Line int
	From string
	To   string
}

// RenumberResult is what InsertCard and RemoveCard changed.
// Created holds the inserted card's file name, and Removed the removed card's.
// RewrittenRefs holds the card-id references rewritten to a moved card's new number, and ProseMentions the lines that refer to a moved card by its bare old number.
type RenumberResult struct {
	Moved         []MovedCard
	Created       []string
	Removed       []string
	RewrittenRefs []RewrittenRef
	ProseMentions []ProseMention
}

var (
	// ErrRenumberInput marks a position, slug or intent the renumber refuses.
	ErrRenumberInput = errors.New("planparser: invalid renumber input")
	// ErrRenumberPosition, ErrRenumberOnlyCard, ErrRenumberSlug and ErrRenumberIntent each name which input was refused, and each is an ErrRenumberInput.
	ErrRenumberPosition = fmt.Errorf("%w: position", ErrRenumberInput)
	ErrRenumberOnlyCard = fmt.Errorf("%w: only card", ErrRenumberInput)
	ErrRenumberSlug     = fmt.Errorf("%w: slug", ErrRenumberInput)
	ErrRenumberIntent   = fmt.Errorf("%w: intent", ErrRenumberInput)
	// ErrRenumberPlanInconsistent marks a plan whose Card Index, card files or card headings disagree.
	ErrRenumberPlanInconsistent = errors.New("planparser: plan is inconsistent")
	// ErrRenumberJournalExists marks a journal left by an earlier renumber that was never completed or restored.
	ErrRenumberJournalExists = errors.New("planparser: a renumber journal already exists")
	// ErrRenumberIOFailed marks a file operation that failed; the plan directory is back as it was.
	ErrRenumberIOFailed = errors.New("planparser: renumber file operation failed")
	// ErrRenumberReversalFailed marks a failed renumber whose undo also failed; the journal is kept on disk.
	ErrRenumberReversalFailed = errors.New("planparser: renumber reversal failed")
)

// cardSlugRe is the slug shape a new card's file name admits, which keeps a path separator and `..` out of the name.
var cardSlugRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// renumberJournal records what a renumber is about to change so that the call can be undone.
// Files holds the pre-call bytes of every file the call rewrites, renames or deletes, and Created every file name the call creates that did not exist before it; both are keyed by base name.
type renumberJournal struct {
	Files   map[string][]byte `json:"files"`
	Created []string          `json:"created"`
}

// InsertCard adds an empty card numbered number with the given slug, moves every card from number up one place, and adds the card's line to the overview's Card Index.
// number runs from the plan's first card to one past its last; intent is the new card's Card Index text.
// journalPath receives the journal for the duration of the call, and a call that fails is undone before it returns.
func InsertCard(ops RenumberIO, planDir, journalPath string, number int, slug, intent string) (*RenumberResult, error) {
	plan, err := renumberPrecheck(ops, planDir, journalPath)
	if err != nil {
		return nil, err
	}

	first, last := cardRange(plan)
	if number < first || number > last+1 {
		return nil, fmt.Errorf("%w: %d is outside the plan's cards %d to %d; choose a position from %d to %d", ErrRenumberPosition, number, first, last, first, last+1)
	}
	if !cardSlugRe.MatchString(slug) {
		return nil, fmt.Errorf("%w: %q must be lowercase letters and digits joined by single hyphens", ErrRenumberSlug, slug)
	}
	intent = strings.TrimSpace(intent)
	if intent == "" || strings.ContainsAny(intent, "\r\n") {
		return nil, fmt.Errorf("%w: the intent must be one non-empty line", ErrRenumberIntent)
	}

	var moved []MovedCard
	for _, c := range sortedCards(plan) {
		if c.Number >= number {
			moved = append(moved, MovedCard{OldNumber: c.Number, NewNumber: c.Number + 1, OldFile: cardFileName(c.Number, c.Slug), NewFile: cardFileName(c.Number+1, c.Slug)})
		}
	}
	sort.Slice(moved, func(i, j int) bool { return moved[i].OldNumber > moved[j].OldNumber })

	return applyRenumber(ops, planDir, journalPath, plan, renumberEdit{
		moved:       moved,
		created:     cardFileName(number, slug),
		createdSlug: slug,
		createdNew:  fmt.Sprintf("# Card %d — %s\n", number, slug),
		indexLine:   fmt.Sprintf("%d — %s — %s", number, slug, intent),
		position:    number,
	})
}

// RemoveCard deletes the card numbered number, moves every later card down one place, and drops the card's line from the overview's Card Index.
// number runs from the plan's first card to its last, and a plan with one card cannot lose it.
// journalPath receives the journal for the duration of the call, and a call that fails is undone before it returns.
func RemoveCard(ops RenumberIO, planDir, journalPath string, number int) (*RenumberResult, error) {
	plan, err := renumberPrecheck(ops, planDir, journalPath)
	if err != nil {
		return nil, err
	}

	first, last := cardRange(plan)
	if len(plan.Cards) == 1 {
		return nil, fmt.Errorf("%w: the plan has one card, and a plan cannot be left without a card; insert the replacement card first, then remove this one", ErrRenumberOnlyCard)
	}
	if number < first || number > last {
		return nil, fmt.Errorf("%w: %d is outside the plan's cards %d to %d; choose a position from %d to %d", ErrRenumberPosition, number, first, last, first, last)
	}

	var removed string
	var moved []MovedCard
	for _, c := range sortedCards(plan) {
		switch {
		case c.Number == number:
			removed = cardFileName(c.Number, c.Slug)
		case c.Number > number:
			moved = append(moved, MovedCard{OldNumber: c.Number, NewNumber: c.Number - 1, OldFile: cardFileName(c.Number, c.Slug), NewFile: cardFileName(c.Number-1, c.Slug)})
		}
	}

	return applyRenumber(ops, planDir, journalPath, plan, renumberEdit{
		moved:    moved,
		removed:  removed,
		position: number,
	})
}

// renumberEdit is one renumber computed from the plan before any file is touched.
// A non-empty removed drops that card's file and its Card Index line at position; a non-empty created adds the file with createdNew's bytes and indexLine at position.
type renumberEdit struct {
	moved       []MovedCard
	removed     string
	created     string
	createdSlug string
	createdNew  string
	indexLine   string
	position    int
}

// cardRange returns the lowest and highest card number of a consistent plan.
func cardRange(plan *Plan) (first, last int) {
	return plan.FirstCard, plan.FirstCard + len(plan.Cards) - 1
}

// sortedCards returns the plan's cards in ascending number order.
func sortedCards(plan *Plan) []Card {
	cards := append([]Card(nil), plan.Cards...)
	sort.Slice(cards, func(i, j int) bool { return cards[i].Number < cards[j].Number })
	return cards
}

// renumberPrecheck refuses a renumber that must not start: a journal already on disk, a plan that does not parse, or a plan whose Card Index, files and headings disagree.
// It writes nothing.
func renumberPrecheck(ops RenumberIO, planDir, journalPath string) (*Plan, error) {
	if _, err := ops.ReadFile(journalPath); err == nil {
		return nil, fmt.Errorf("%w: %s", ErrRenumberJournalExists, journalPath)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%w: read journal %s: %w", ErrRenumberIOFailed, journalPath, err)
	}

	plan, err := ParsePlanFrom(planDir, func(name string) ([]byte, error) {
		return ops.ReadFile(filepath.Join(planDir, name))
	})
	if err != nil {
		return nil, err
	}

	findings := append(checkIndexFileConsistency(plan), checkCardNumbering(plan)...)
	if len(findings) > 0 {
		details := make([]string, len(findings))
		for i, f := range findings {
			details[i] = f.Error()
		}
		return nil, fmt.Errorf("%w: %s", ErrRenumberPlanInconsistent, strings.Join(details, "; "))
	}
	return plan, nil
}

// applyRenumber computes every new file in memory, journals the pre-call bytes, applies the edit and removes the journal.
// A failure after the journal is written undoes the edit and returns ErrRenumberIOFailed.
func applyRenumber(ops RenumberIO, planDir, journalPath string, plan *Plan, edit renumberEdit) (*RenumberResult, error) {
	overviewPath := filepath.Join(planDir, overviewFileName)
	read := func(name string) ([]byte, error) {
		data, err := ops.ReadFile(filepath.Join(planDir, name))
		if err != nil {
			return nil, fmt.Errorf("%w: read %s: %w", ErrRenumberIOFailed, name, err)
		}
		return data, nil
	}

	moves := movedSlugs(plan, edit.moved)
	var rewritten []RewrittenRef
	journal := renumberJournal{Files: map[string][]byte{}}
	newBytes := make(map[string][]byte, len(edit.moved))
	for _, m := range edit.moved {
		data, err := read(m.OldFile)
		if err != nil {
			return nil, err
		}
		journal.Files[m.OldFile] = data
		moved, refs := rewriteCardIDs(string(moveCardBytes(data, m.OldNumber, m.NewNumber)), m.NewFile, moves)
		newBytes[m.NewFile] = []byte(moved)
		rewritten = append(rewritten, refs...)
		journal.Created = append(journal.Created, m.NewFile)
	}

	// A card that keeps its number is rewritten in place, and journaled only when a reference in it changes.
	var stayed []string
	for _, c := range sortedCards(plan) {
		name := cardFileName(c.Number, c.Slug)
		if _, moving := moves[c.Number]; moving || name == edit.removed {
			continue
		}
		data, err := read(name)
		if err != nil {
			return nil, err
		}
		after, refs := rewriteCardIDs(string(data), name, moves)
		if len(refs) == 0 {
			continue
		}
		journal.Files[name] = data
		newBytes[name] = []byte(after)
		rewritten = append(rewritten, refs...)
		stayed = append(stayed, name)
	}
	if edit.removed != "" {
		data, err := read(edit.removed)
		if err != nil {
			return nil, err
		}
		journal.Files[edit.removed] = data
	}
	if edit.created != "" {
		journal.Created = append(journal.Created, edit.created)
	}

	overview, err := read(overviewFileName)
	if err != nil {
		return nil, err
	}
	journal.Files[overviewFileName] = overview
	newOverview, err := renumberCardIndex(string(overview), edit)
	if err != nil {
		return nil, err
	}
	newOverview, overviewRefs := rewriteCardIDs(newOverview, overviewFileName, moves)
	rewritten = append(rewritten, overviewRefs...)
	sort.SliceStable(rewritten, func(i, j int) bool {
		if rewritten[i].File != rewritten[j].File {
			return rewritten[i].File < rewritten[j].File
		}
		return rewritten[i].Line < rewritten[j].Line
	})

	if err := writeRenumberJournal(ops, journalPath, journal); err != nil {
		return nil, err
	}

	result := &RenumberResult{Moved: edit.moved, RewrittenRefs: rewritten}
	var steps []func() error
	if edit.removed != "" {
		steps = append(steps, func() error { return ops.Remove(filepath.Join(planDir, edit.removed)) })
		result.Removed = []string{edit.removed}
	}
	for _, m := range edit.moved {
		steps = append(steps,
			func() error { return ops.WriteFile(filepath.Join(planDir, m.NewFile), newBytes[m.NewFile], 0o644) },
			func() error { return ops.Remove(filepath.Join(planDir, m.OldFile)) },
		)
	}
	for _, name := range stayed {
		steps = append(steps, func() error { return ops.WriteFile(filepath.Join(planDir, name), newBytes[name], 0o644) })
	}
	if edit.created != "" {
		steps = append(steps, func() error {
			return ops.WriteFile(filepath.Join(planDir, edit.created), []byte(edit.createdNew), 0o644)
		})
		result.Created = []string{edit.created}
	}
	steps = append(steps,
		func() error { return ops.WriteFile(overviewPath, []byte(newOverview), 0o644) },
		func() error {
			mentions, err := proseMentions(ops, planDir, planAfterRenumber(plan, edit), movedOldNumbers(edit.moved))
			result.ProseMentions = mentions
			return err
		},
		func() error { return ops.Remove(journalPath) },
	)

	for _, step := range steps {
		if err := step(); err != nil {
			if reverseErr := reverseRenumber(ops, planDir, journal); reverseErr != nil {
				return nil, fmt.Errorf("%w: %w; the apply failed first: %w", ErrRenumberReversalFailed, reverseErr, err)
			}
			if err := ops.Remove(journalPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return nil, fmt.Errorf("%w: %w", ErrRenumberReversalFailed, err)
			}
			return nil, fmt.Errorf("%w: %w", ErrRenumberIOFailed, err)
		}
	}
	return result, nil
}

// cardIDMove is the slug of a moved card and the number it moves to, keyed by its old number.
type cardIDMove struct {
	slug string
	to   int
}

// movedSlugs indexes every moved card of plan by its old number.
func movedSlugs(plan *Plan, moved []MovedCard) map[int]cardIDMove {
	newNumbers := make(map[int]int, len(moved))
	for _, m := range moved {
		newNumbers[m.OldNumber] = m.NewNumber
	}
	moves := make(map[int]cardIDMove, len(moved))
	for _, c := range plan.Cards {
		if to, ok := newNumbers[c.Number]; ok {
			moves[c.Number] = cardIDMove{slug: c.Slug, to: to}
		}
	}
	return moves
}

// cardIDNumberRe matches the number and hyphen a card-id token opens with.
var cardIDNumberRe = regexp.MustCompile(`\d+-`)

// cardIDToken is one card-id token of a line: its byte span, the number it names, and the width its number is padded to, zero when unpadded.
type cardIDToken struct {
	start, end int
	number     int
	width      int
}

// cardIDTokens returns every token of line that spells a card id `<N>-<slug>`, zero-padded or not, for a card slugOf knows.
// A token is bounded on both sides, so a number inside a longer word or a slug that runs on into a longer one is no token.
func cardIDTokens(line string, slugOf func(number int) (string, bool)) []cardIDToken {
	var tokens []cardIDToken
	for _, m := range cardIDNumberRe.FindAllStringIndex(line, -1) {
		if m[0] > 0 && isCardIDByte(line[m[0]-1]) {
			continue
		}
		digits := line[m[0] : m[1]-1]
		number, err := strconv.Atoi(digits)
		if err != nil {
			continue
		}
		slug, ok := slugOf(number)
		if !ok || !strings.HasPrefix(line[m[1]:], slug) {
			continue
		}
		end := m[1] + len(slug)
		if end < len(line) && isCardIDByte(line[end]) && !(line[end] == '-' && (end+1 == len(line) || !isCardIDByte(line[end+1]))) {
			continue
		}
		// A leading zero keeps its width; a card file name is the zero-padded file form whatever its number.
		width := 0
		switch {
		case len(digits) > 1 && digits[0] == '0':
			width = len(digits)
		case strings.HasPrefix(line[end:], ".md"):
			width = 2
		}
		tokens = append(tokens, cardIDToken{start: m[0], end: end, number: number, width: width})
	}
	return tokens
}

// isCardIDByte reports whether b can continue a card number or slug, which a card-id token may not touch.
func isCardIDByte(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '-' || b == '_'
}

// rewriteCardIDs rewrites every card-id token in text that names a moved card by its old number to the card's new number, padded as the token is.
// Every token is read against the pre-renumber numbering in one pass, so a reference is never moved twice.
// It returns the rewritten text and one RewrittenRef per token, located in file.
func rewriteCardIDs(text, file string, moves map[int]cardIDMove) (string, []RewrittenRef) {
	slugOf := func(number int) (string, bool) {
		m, ok := moves[number]
		return m.slug, ok
	}
	lines := strings.Split(text, "\n")
	var refs []RewrittenRef
	for i, line := range lines {
		tokens := cardIDTokens(line, slugOf)
		if len(tokens) == 0 {
			continue
		}
		var b strings.Builder
		last := 0
		for _, tok := range tokens {
			m := moves[tok.number]
			to := fmt.Sprintf("%0*d-%s", tok.width, m.to, m.slug)
			b.WriteString(line[last:tok.start])
			b.WriteString(to)
			last = tok.end
			refs = append(refs, RewrittenRef{File: file, Line: i + 1, From: line[tok.start:tok.end], To: to})
		}
		b.WriteString(line[last:])
		lines[i] = b.String()
	}
	return strings.Join(lines, "\n"), refs
}

// movedOldNumbers returns the old number of every moved card.
func movedOldNumbers(moved []MovedCard) []int {
	numbers := make([]int, len(moved))
	for i, m := range moved {
		numbers[i] = m.OldNumber
	}
	return numbers
}

// planAfterRenumber returns the plan's card list as the edit leaves it, which is the file set the prose scan reads.
func planAfterRenumber(plan *Plan, edit renumberEdit) *Plan {
	newNumbers := make(map[int]int, len(edit.moved))
	for _, m := range edit.moved {
		newNumbers[m.OldNumber] = m.NewNumber
	}

	after := &Plan{Dir: plan.Dir}
	for _, c := range plan.Cards {
		if edit.removed != "" && c.Number == edit.position {
			continue
		}
		if to, ok := newNumbers[c.Number]; ok {
			c.Number = to
		}
		after.Cards = append(after.Cards, c)
	}
	if edit.created != "" {
		after.Cards = append(after.Cards, Card{Number: edit.position, Slug: edit.createdSlug})
	}
	return after
}

// writeRenumberJournal encodes j and puts it at journalPath through a temporary file, so a journal on disk is always complete.
// A failure removes the temporary file and leaves the plan directory untouched.
func writeRenumberJournal(ops RenumberIO, journalPath string, j renumberJournal) error {
	data, err := json.Marshal(j)
	if err != nil {
		return fmt.Errorf("%w: encode journal: %w", ErrRenumberIOFailed, err)
	}
	if err := ops.MkdirAll(filepath.Dir(journalPath), 0o755); err != nil {
		return fmt.Errorf("%w: create journal directory: %w", ErrRenumberIOFailed, err)
	}

	temp := journalPath + renumberJournalTempSuffix
	if err := ops.WriteFile(temp, data, 0o644); err != nil {
		_ = ops.Remove(temp)
		return fmt.Errorf("%w: write journal: %w", ErrRenumberIOFailed, err)
	}
	if err := ops.Rename(temp, journalPath); err != nil {
		_ = ops.Remove(temp)
		return fmt.Errorf("%w: place journal: %w", ErrRenumberIOFailed, err)
	}
	return nil
}

// reverseRenumber puts the plan directory back as j found it: it removes every created name, then writes every recorded file back.
// A created name that does not exist is skipped, since a failure before the last create leaves some unwritten.
// Every step is attempted, and the errors are joined.
func reverseRenumber(ops RenumberIO, planDir string, j renumberJournal) error {
	var errs []error
	for _, name := range j.Created {
		if err := ops.Remove(filepath.Join(planDir, name)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			errs = append(errs, fmt.Errorf("remove %s: %w", name, err))
		}
	}

	names := make([]string, 0, len(j.Files))
	for name := range j.Files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if err := ops.WriteFile(filepath.Join(planDir, name), j.Files[name], 0o644); err != nil {
			errs = append(errs, fmt.Errorf("restore %s: %w", name, err))
		}
	}
	return errors.Join(errs...)
}

// moveCardBytes rewrites the two things a card moving from number from to number to carries: the number in its heading, and the `from: ` prefix of an explicit `**Commit:**` value, inside its backticks when it has them.
// Every other byte is kept.
func moveCardBytes(data []byte, from, to int) []byte {
	lines := strings.Split(string(data), "\n")

	trimmed := strings.TrimSpace(lines[0])
	if m := cardHeadingRe.FindStringSubmatchIndex(trimmed); m != nil {
		offset := strings.Index(lines[0], trimmed)
		lines[0] = lines[0][:offset+m[2]] + strconv.Itoa(to) + lines[0][offset+m[3]:]
	}

	oldPrefix := strconv.Itoa(from) + ": "
	for i, line := range lines {
		t := strings.TrimSpace(line)
		if !strings.HasPrefix(t, commitLabel) {
			continue
		}
		valueStart := strings.Index(line, commitLabel) + len(commitLabel)
		rest := line[valueStart:]
		value := strings.TrimLeft(rest, " \t")
		skipped := len(rest) - len(value)
		quote := 0
		if strings.HasPrefix(value, "`") {
			quote = 1
		}
		if strings.HasPrefix(value[quote:], oldPrefix) {
			at := valueStart + skipped + quote
			lines[i] = line[:at] + strconv.Itoa(to) + ": " + line[at+len(oldPrefix):]
		}
	}
	return []byte(strings.Join(lines, "\n"))
}

// indexNumberRe captures a Card Index line's leading bullet marker and its number.
var indexNumberRe = regexp.MustCompile(`^(\s*(?:-\s+)?)(\d+)`)

// renumberCardIndex rewrites the overview's Card Index section for edit and keeps every other byte.
// Each index line of a moved card has its number replaced, the removed card's line is dropped, and the new card's line goes before the first line numbered at or above its position, or after the last index line when there is none.
func renumberCardIndex(overview string, edit renumberEdit) (string, error) {
	_, body, _, err := splitFrontmatter(overview)
	if err != nil {
		return "", fmt.Errorf("planparser: plan overview: %w", err)
	}
	prefix := overview[:len(overview)-len(body)]

	lines := strings.Split(body, "\n")
	headingIdx, end, err := cardIndexBounds(lines)
	if err != nil {
		return "", fmt.Errorf("planparser: plan overview: %w", err)
	}

	newNumbers := make(map[int]int, len(edit.moved))
	for _, m := range edit.moved {
		newNumbers[m.OldNumber] = m.NewNumber
	}

	var section []string
	inserted := edit.indexLine == ""
	lastIndexLine := -1
	for _, line := range lines[headingIdx+1 : end] {
		entry := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "- "))
		if !cardIndexLineRe.MatchString(entry) {
			section = append(section, line)
			continue
		}

		m := indexNumberRe.FindStringSubmatch(line)
		number, _ := strconv.Atoi(m[2])
		if edit.removed != "" && number == edit.position {
			continue
		}
		if !inserted && number >= edit.position {
			section = append(section, edit.indexLine)
			inserted = true
		}
		if to, ok := newNumbers[number]; ok {
			line = m[1] + strconv.Itoa(to) + line[len(m[0]):]
		}
		section = append(section, line)
		lastIndexLine = len(section) - 1
	}
	if !inserted {
		section = append(section[:lastIndexLine+1], append([]string{edit.indexLine}, section[lastIndexLine+1:]...)...)
	}

	kept := append(append(append([]string(nil), lines[:headingIdx+1]...), section...), lines[end:]...)
	return prefix + strings.Join(kept, "\n"), nil
}

// cardMentionRe matches a `card` or `cards` reference followed by one number or a list or range of numbers.
var cardMentionRe = regexp.MustCompile(`\b[Cc]ards?\s+\d+(?:(?:\s*,\s*(?:and\s+)?|\s+and\s+|\s*[–—-]\s*)\d+)*`)

// mentionNumberRe splits a card reference into its numbers and range dashes.
var mentionNumberRe = regexp.MustCompile(`\d+|[–—-]`)

// proseMentions scans the overview and every card file of plan for a card reference that names one of oldNumbers, a range's interior included.
// A card's heading line, the Card Index lines and every card id of plan are skipped, and no line is rewritten.
func proseMentions(ops RenumberIO, planDir string, plan *Plan, oldNumbers []int) ([]ProseMention, error) {
	old := make(map[int]bool, len(oldNumbers))
	for _, n := range oldNumbers {
		old[n] = true
	}

	names := []string{overviewFileName}
	slugs := make(map[int]string, len(plan.Cards))
	for _, c := range sortedCards(plan) {
		names = append(names, cardFileName(c.Number, c.Slug))
		slugs[c.Number] = c.Slug
	}
	slugOf := func(number int) (string, bool) {
		slug, ok := slugs[number]
		return slug, ok
	}

	var mentions []ProseMention
	for _, name := range names {
		data, err := ops.ReadFile(filepath.Join(planDir, name))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
		lines := strings.Split(string(data), "\n")

		skipFrom, skipTo := 0, 0
		if name == overviewFileName {
			_, body, _, _ := splitFrontmatter(string(data))
			if headingIdx, end, err := cardIndexBounds(strings.Split(body, "\n")); err == nil {
				shift := len(lines) - len(strings.Split(body, "\n"))
				skipFrom, skipTo = shift+headingIdx+1, shift+end
			}
		}

		for i, line := range lines {
			if name != overviewFileName && i == 0 {
				continue
			}
			if i >= skipFrom && i < skipTo && cardIndexLineRe.MatchString(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "- "))) {
				continue
			}
			if namesOldNumber(maskCardIDs(line, slugOf), old) {
				mentions = append(mentions, ProseMention{File: name, Line: i + 1, Text: strings.TrimSpace(line)})
			}
		}
	}
	return mentions, nil
}

// maskCardIDs blanks every card-id token of line for a card slugOf knows, so a card id is never read as a bare card number.
func maskCardIDs(line string, slugOf func(number int) (string, bool)) string {
	tokens := cardIDTokens(line, slugOf)
	if len(tokens) == 0 {
		return line
	}
	masked := []byte(line)
	for _, tok := range tokens {
		for i := tok.start; i < tok.end; i++ {
			masked[i] = ' '
		}
	}
	return string(masked)
}

// namesOldNumber reports whether line holds a card reference naming a number in old.
func namesOldNumber(line string, old map[int]bool) bool {
	for _, ref := range cardMentionRe.FindAllString(line, -1) {
		previous, inRange := 0, false
		for _, token := range mentionNumberRe.FindAllString(ref, -1) {
			n, err := strconv.Atoi(token)
			if err != nil {
				inRange = true
				continue
			}
			if old[n] {
				return true
			}
			if inRange {
				for k := previous + 1; k < n; k++ {
					if old[k] {
						return true
					}
				}
				inRange = false
			}
			previous = n
		}
	}
	return false
}
