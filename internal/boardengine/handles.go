// handles.go — an entry's short handle, such as T4 or B2: its letter, its parse, and its assignment across entries.
// It also holds the window title that opens with the handle.

package boardengine

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Knatte18/loomyard/internal/logger"
)

// Handle is an entry's short name, a letter and a number such as T4 or B2.
// A selector accepts it beside the entry's slug and numeric id.
type Handle string

// taskHandleLetter is the letter of every task's handle.
const taskHandleLetter = "T"

// projectHandleLetter is the letter of every project's handle.
const projectHandleLetter = "P"

// typeHandleLetter is the handle letter of a note carrying the type label typeLabel: the label's first letter, upper-cased.
func typeHandleLetter(typeLabel string) string {
	first, _ := utf8.DecodeRuneInString(typeLabel)
	return string(unicode.ToUpper(first))
}

// validateTypeLetters refuses type labels that would give two kinds of entry one handle letter:
// a label not starting with a letter, two labels sharing a first letter, or a label taking the task or project letter.
func validateTypeLetters(types []Label) error {
	owner := map[string]string{taskHandleLetter: "tasks", projectHandleLetter: "projects"}
	for _, typeLabel := range types {
		first, _ := utf8.DecodeRuneInString(typeLabel.Name)
		if !unicode.IsLetter(first) {
			return fmt.Errorf("board.yaml: type label %q does not begin with a letter, and a note's handle letter is the first letter of its type: rename the type label", typeLabel.Name)
		}
		letter := typeHandleLetter(typeLabel.Name)
		if taken, ok := owner[letter]; ok {
			return fmt.Errorf("board.yaml: type label %q takes handle letter %s, which %s already take, and each type needs a letter of its own: rename one of the type labels", typeLabel.Name, letter, taken)
		}
		owner[letter] = fmt.Sprintf("notes of type %q", typeLabel.Name)
	}
	return nil
}

// ParseHandle splits handle into its upper-cased letter and its number.
// It reports whether handle has the shape of one letter and a positive number, and the letter may be in either case.
func ParseHandle(handle string) (string, int, bool) {
	first, size := utf8.DecodeRuneInString(handle)
	if !unicode.IsLetter(first) {
		return "", 0, false
	}
	digits := handle[size:]
	if digits == "" || strings.TrimLeft(digits, "0123456789") != "" || digits[0] == '0' {
		return "", 0, false
	}
	number, err := strconv.Atoi(digits)
	if err != nil {
		return "", 0, false
	}
	return string(unicode.ToUpper(first)), number, true
}

// handleLetter returns the handle letter t takes and whether it can be decided.
// A task takes the task letter, and a project takes the project letter.
// A note takes the letter of its first type label,
// and it is undecided without a vocabulary or when it carries no configured type label.
func handleLetter(t Task, vocab *Vocabulary) (string, bool) {
	if t.Kind == KindTask {
		return taskHandleLetter, true
	}
	if t.Kind == KindProject {
		return projectHandleLetter, true
	}
	if vocab == nil {
		return "", false
	}
	for _, label := range t.Labels {
		if vocab.IsType(label) {
			return typeHandleLetter(label), true
		}
	}
	return "", false
}

// assignHandles gives a new handle to each entry of tasks that needs one.
// An entry needs one when its handle is missing, of another letter, or held by an entry of lower id.
// It gives the entry the lowest number of its letter not in use, in id order, and it changes tasks in place.
// Every other handle stays, so adding or removing an entry changes no other entry's handle,
// and a removed entry's number is given out again.
// An entry whose letter cannot be decided keeps its stored handle, which still counts as in use.
// The handle is cleared instead when an entry of lower id already holds it.
func assignHandles(tasks []Task, vocab *Vocabulary) {
	order := make([]int, len(tasks))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int { return tasks[a].ID - tasks[b].ID })

	inUse := map[string]map[int]bool{}
	claim := func(letter string, number int) bool {
		if inUse[letter] == nil {
			inUse[letter] = map[int]bool{}
		}
		if inUse[letter][number] {
			return false
		}
		inUse[letter][number] = true
		return true
	}

	var pending []int
	for _, i := range order {
		letter, decided := handleLetter(tasks[i], vocab)
		stored, number, parsed := ParseHandle(tasks[i].Handle)
		switch {
		case !decided:
			if parsed && !claim(stored, number) {
				tasks[i].Handle = ""
			}
		case parsed && stored == letter && claim(letter, number):
		default:
			pending = append(pending, i)
		}
	}
	for _, i := range pending {
		letter, _ := handleLetter(tasks[i], vocab)
		number := 1
		for !claim(letter, number) {
			number++
		}
		tasks[i].Handle = letter + strconv.Itoa(number)
	}
}

// handleTitleSeparator joins an entry's handle to the name it opens in a title.
const handleTitleSeparator = ": "

// TitleWithHandle returns name prefixed with the handle of the entry with slug on the board in boardDir.
// The result reads `<handle>: <name>`, so a window named for a run shows the handle the operator calls the run by.
// It returns name alone if the board holds no entry with slug, that entry has no handle, or the board cannot be read.
// An unreadable board is logged at Warn, since a title never fails its caller.
func TitleWithHandle(boardDir, slug, name string) string {
	tasks, err := ReadTasks(boardDir)
	if err != nil {
		logger.Warn("board: could not read the board for a title's handle; titling with the name alone", "slug", slug, "name", name, "cause", err)
		return name
	}
	for _, task := range tasks {
		if task.Slug == slug && task.Handle != "" {
			return task.Handle + handleTitleSeparator + name
		}
	}
	return name
}

// compareHandles orders two handles by letter, then by number.
// An unparsable or empty handle sorts after every parsable one.
func compareHandles(a, b string) int {
	letterA, numberA, okA := ParseHandle(a)
	letterB, numberB, okB := ParseHandle(b)
	switch {
	case okA != okB:
		if okA {
			return -1
		}
		return 1
	case !okA:
		return 0
	}
	if c := strings.Compare(letterA, letterB); c != 0 {
		return c
	}
	return numberA - numberB
}
