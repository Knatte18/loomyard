// handles.go — an entry's short handle, such as T4 or B2: its letter, its parse and its assignment over the whole entry list.

package boardengine

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Handle selects an entry by its handle, beside a slug string and a numeric id, wherever a Board or Store method takes a selector.
type Handle string

// taskHandleLetter is the letter of every task's handle.
const taskHandleLetter = "T"

// typeHandleLetter is the handle letter of a note carrying the type label typeLabel: the label's first letter, upper-cased.
func typeHandleLetter(typeLabel string) string {
	first, _ := utf8.DecodeRuneInString(typeLabel)
	return string(unicode.ToUpper(first))
}

// validateTypeLetters refuses type labels that would give two kinds of entry one handle letter: a label not beginning with a letter, two labels sharing a first letter, or a label whose letter is the task letter.
func validateTypeLetters(types []Label) error {
	owner := map[string]string{taskHandleLetter: "tasks"}
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

// ParseHandle splits handle into its upper-cased letter and its number, matching a letter case-insensitively, and reports whether handle has the shape of one letter and a positive number.
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

// handleLetter returns the handle letter t takes and whether it can be decided: the task letter for a task, and for a note the letter of its first type label;
// a note's letter is undecided without a vocabulary or when it carries no configured type label.
func handleLetter(t Task, vocab *Vocabulary) (string, bool) {
	if t.Kind == KindTask {
		return taskHandleLetter, true
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

// assignHandles gives each entry of tasks whose handle is missing, of another letter or held by an entry of lower id the lowest number of its letter not in use, in id order, in place.
// Every other handle stays, so adding or removing an entry changes no other entry's handle and a removed entry's number is given out again.
// An entry whose letter handleLetter cannot decide keeps its stored handle, which still counts as in use, unless an entry of lower id already holds it: then the handle is cleared.
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

// compareHandles orders two handles by letter, then by number, with an unparsable or empty handle after every parsable one.
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
