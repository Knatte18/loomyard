// stops.go reads what stopped a run: its batten halts matched to the halt notes loom left, each stop classed by the note's anomaly and its reason.

package reportengine

import (
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/Knatte18/loomyard/internal/gitrepo"
	"github.com/Knatte18/loomyard/internal/haltnote"
	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// SourceHaltNotes is the name of the source holding a run's halt notes.
const SourceHaltNotes = "halt-notes"

// The stop classes that no halt note decides.
const (
	// StopClassAwaiting is a halt that waits on a person or on the gate; it is never a failure.
	StopClassAwaiting = "awaiting"
	// StopClassNoNote is a blocked or failed halt for which the run holds no note.
	StopClassNoNote = "no-note"
	// stopClassUnclassified is the class of a halt note that names no anomaly.
	stopClassUnclassified = "unclassified"
)

// Stop is one halt of a run.
// Change is true when batten recorded the halt as a child state change; Note is the tree path of the halt note, empty when the stop has none.
// Failure is the failure class of the note's reason.
type Stop struct {
	Class    string `json:"class"`
	State    string `json:"state"`
	Producer string `json:"producer"`
	Failure  string `json:"failure,omitempty"`
	Change   bool   `json:"change"`
	Note     string `json:"note,omitempty"`
}

// haltNote is a parsed halt note and the tree path it was read from.
type haltNote struct {
	note haltnote.Note
	file string
}

var (
	quotedString = regexp.MustCompile("(^|\\s)(\"[^\"]*\"|`[^`]*`|'[^']*')")
	digitRun     = regexp.MustCompile(`\d+`)
)

// failureClass names the failure a halt belongs to: the producer, `: `, and the leading clause of the reason, up to the first `:`, `;` or `(`.
// Quoted strings, paths and digit runs in the clause are replaced by `<str>`, `<path>` and `<n>`, so halts that differ only in them share a class.
func failureClass(producer, reason string) string {
	clause := quotedString.ReplaceAllString(reason, "${1}<str>")
	if end := strings.IndexAny(clause, ":;("); end >= 0 {
		clause = clause[:end]
	}
	words := strings.Fields(clause)
	for i, word := range words {
		if strings.Contains(word, "/") {
			words[i] = "<path>"
		}
	}
	clause = digitRun.ReplaceAllString(strings.Join(words, " "), "<n>")
	return producer + ": " + clause
}

// noteOrder is the number in a halt note's file name, 0 for the unnumbered first note.
func noteOrder(name string) int {
	digits := strings.Map(func(r rune) rune {
		if unicode.IsDigit(r) {
			return r
		}
		return -1
	}, name)
	number, err := strconv.Atoi(digits)
	if err != nil {
		return 0
	}
	return number
}

// readHaltNotes reads the halt notes of the tree at tag: the files of each archived friction directory by name, then the live friction directory, each directory's notes in suffix order.
// present is false when the tree holds neither the friction directory nor an archived sibling of it.
// A note that does not parse is a finding.
func readHaltNotes(records Reader, tag gitrepo.TagRef) (notes []haltNote, findings []Finding, present bool, err error) {
	durable := filepath.ToSlash(loomengine.LoomDurableDirRel())
	siblings, err := records.SubdirsAtRevision(tag.Commit, durable)
	if err != nil {
		return nil, nil, false, err
	}
	var dirs []string
	for _, sibling := range siblings {
		if loomengine.IsLoomFrictionArchive(sibling) {
			dirs = append(dirs, path.Join(durable, sibling))
		}
	}
	present = len(dirs) > 0
	live := filepath.ToSlash(loomengine.LoomFrictionDirRel())
	liveFiles, err := records.FilesInDirAtRevision(tag.Commit, live)
	if err != nil {
		return nil, nil, false, err
	}
	if len(liveFiles) > 0 {
		present = true
	}
	dirs = append(dirs, live)

	for _, dir := range dirs {
		names, err := records.FilesInDirAtRevision(tag.Commit, dir)
		if err != nil {
			return nil, nil, false, err
		}
		var noteNames []string
		for _, name := range names {
			if haltnote.IsNoteName(name) {
				noteNames = append(noteNames, name)
			}
		}
		sort.SliceStable(noteNames, func(i, j int) bool { return noteOrder(noteNames[i]) < noteOrder(noteNames[j]) })
		for _, name := range noteNames {
			file := path.Join(dir, name)
			data, err := records.FileAtRevision(tag.Commit, file)
			if err != nil {
				return nil, nil, false, err
			}
			note, err := haltnote.Parse(data)
			if err != nil {
				findings = append(findings, Finding{Tag: tag.Name, File: file, Error: err.Error()})
				continue
			}
			notes = append(notes, haltNote{note: note, file: file})
		}
	}
	return notes, findings, present, nil
}

// noteStop is the stop a halt note makes; change says whether batten recorded the halt too.
func noteStop(held haltNote, change bool) Stop {
	class := held.note.Anomaly
	if class == "" {
		class = stopClassUnclassified
	}
	return Stop{
		Class:    class,
		State:    held.note.State,
		Producer: held.note.Producer,
		Failure:  failureClass(held.note.Producer, held.note.Reason),
		Change:   change,
		Note:     held.file,
	}
}

// readStops matches the run's recorded halts to its halt notes and fills Stops and Failures.
// Both stay absent when the tree holds no friction directory and no archived sibling of it, whatever the run's batten changes.
func (row *RunRow) readStops(records Reader, tag gitrepo.TagRef) ([]Finding, error) {
	notes, findings, present, err := readHaltNotes(records, tag)
	if err != nil {
		return nil, err
	}
	row.Sources[SourceHaltNotes] = present
	if !present {
		return nil, nil
	}

	row.Stops = []Stop{}
	matched := make([]bool, len(notes))
	for _, change := range row.changes {
		switch change.To {
		case shedengine.StateAwaiting:
			row.Stops = append(row.Stops, Stop{Class: StopClassAwaiting, State: string(change.To), Producer: change.Producer, Change: true})
		case shedengine.StateBlocked, shedengine.StateFailed:
			index := -1
			for i, held := range notes {
				if !matched[i] && held.note.Producer == change.Producer && held.note.State == string(change.To) {
					index = i
					break
				}
			}
			if index < 0 {
				row.Stops = append(row.Stops, Stop{Class: StopClassNoNote, State: string(change.To), Producer: change.Producer, Change: true})
				continue
			}
			matched[index] = true
			row.Stops = append(row.Stops, noteStop(notes[index], true))
		}
	}
	for i, held := range notes {
		if !matched[i] {
			row.Stops = append(row.Stops, noteStop(held, false))
		}
	}

	row.Failures = map[string]int{}
	for _, stop := range row.Stops {
		if stop.Failure != "" {
			row.Failures[stop.Failure]++
		}
	}
	return findings, nil
}
