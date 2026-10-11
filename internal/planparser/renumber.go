// renumber.go implements InsertCard and RemoveCard, the plan's write paths that add or drop one card and renumber the cards after it, plus the plan scratch paths and the journal that makes an interrupted call restorable.

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

	"github.com/Knatte18/loomyard/internal/lyxdirs"
)

// renumberJournalFileName is the journal's file name under the plan scratch directory.
const renumberJournalFileName = "renumber-journal.json"

// renumberJournalTempSuffix is appended to the journal path to form the temporary name the journal is written under before it is renamed into place.
const renumberJournalTempSuffix = ".tmp"

// PlanScratchDir returns the absolute path to the plan directory's never-tracked mirror, `<anchor>/.lyx/plan`, for the worktree anchored at anchorPath.
// The caller supplies the anchor as for PlanDir.
func PlanScratchDir(anchorPath string) string {
	return filepath.Join(anchorPath, lyxdirs.DotLyxDirName, PlanDirName)
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

// RenumberResult is what InsertCard and RemoveCard changed.
// Created holds the inserted card's file name, Removed the removed card's, and ProseMentions the lines that refer to a moved card by its old number.
type RenumberResult struct {
	Moved         []MovedCard
	Created       []string
	Removed       []string
	ProseMentions []ProseMention
}

var (
	// ErrRenumberInput marks a position, slug or intent the renumber refuses.
	ErrRenumberInput = errors.New("planparser: invalid renumber input")
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
		return nil, fmt.Errorf("%w: position %d is outside the plan's cards %d to %d; choose a position from %d to %d", ErrRenumberInput, number, first, last, first, last+1)
	}
	if !cardSlugRe.MatchString(slug) {
		return nil, fmt.Errorf("%w: slug %q must be lowercase letters and digits joined by single hyphens", ErrRenumberInput, slug)
	}
	intent = strings.TrimSpace(intent)
	if intent == "" || strings.ContainsAny(intent, "\r\n") {
		return nil, fmt.Errorf("%w: the intent must be one non-empty line", ErrRenumberInput)
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
		return nil, fmt.Errorf("%w: the plan has one card, and a plan cannot be left without a card; replace the plan instead", ErrRenumberInput)
	}
	if number < first || number > last {
		return nil, fmt.Errorf("%w: position %d is outside the plan's cards %d to %d; choose a position from %d to %d", ErrRenumberInput, number, first, last, first, last)
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

	journal := renumberJournal{Files: map[string][]byte{}}
	newBytes := make(map[string][]byte, len(edit.moved))
	for _, m := range edit.moved {
		data, err := read(m.OldFile)
		if err != nil {
			return nil, err
		}
		journal.Files[m.OldFile] = data
		newBytes[m.NewFile] = moveCardBytes(data, m.OldNumber, m.NewNumber)
		journal.Created = append(journal.Created, m.NewFile)
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

	if err := writeRenumberJournal(ops, journalPath, journal); err != nil {
		return nil, err
	}

	result := &RenumberResult{Moved: edit.moved}
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
// A card's heading line and the Card Index lines are skipped, and no line is rewritten.
func proseMentions(ops RenumberIO, planDir string, plan *Plan, oldNumbers []int) ([]ProseMention, error) {
	old := make(map[int]bool, len(oldNumbers))
	for _, n := range oldNumbers {
		old[n] = true
	}

	names := []string{overviewFileName}
	for _, c := range sortedCards(plan) {
		names = append(names, cardFileName(c.Number, c.Slug))
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
			if namesOldNumber(line, old) {
				mentions = append(mentions, ProseMention{File: name, Line: i + 1, Text: strings.TrimSpace(line)})
			}
		}
	}
	return mentions, nil
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
