// haltnote.go renders and tolerantly parses the Go-authored loom halt note: the friction note a `blocked` or `failed` loom halt leaves.

package haltnote

import (
	"bytes"
	"errors"
	"strconv"
	"strings"
)

// NoteID is the friction note stem a halt note is written under.
const NoteID = "loom-halt"

const (
	titlePrefix     = "loom halted "
	fence           = "```"
	historyHeader   = "history:"
	noHistoryLine   = "no history entries"
	parentHeader    = "parent fabric status:"
	rowSeparator    = " / "
	noteFileName    = NoteID + ".md"
	numberedPrefix  = NoteID + "-"
	noteFileSuffix  = ".md"
	fenceThenFields = "\n" + fence + "\n\nproducer: "
)

// Note is one halt note. Every field is a string or an int, so the leaf needs no producer-state types.
type Note struct {
	Producer string
	State    string
	Reason   string
	// Anomaly is the anomaly kind the halt classifies as; empty omits the `anomaly:` line.
	Anomaly string
	// HistoryEntries is the value of the `history_entries:` line.
	HistoryEntries int
	TraceFile      string
	History        []Row
	// ParentStatus is the rendered dirty state of the parent pair; empty writes no section.
	ParentStatus string
}

// Row is one history row of a halt note.
type Row struct {
	Producer string
	Outcome  string
	At       string
}

// Render returns the note's bytes.
func Render(n Note) []byte {
	var b strings.Builder
	b.WriteString(titlePrefix + n.State + " at " + n.Producer + "\n\n")
	b.WriteString(fence + "\n" + n.Reason + "\n" + fence + "\n\n")
	b.WriteString("producer: " + n.Producer + "\n")
	b.WriteString("state: " + n.State + "\n")
	if n.Anomaly != "" {
		b.WriteString("anomaly: " + n.Anomaly + "\n")
	}
	b.WriteString("history_entries: " + strconv.Itoa(n.HistoryEntries) + "\n")
	b.WriteString("trace_file: " + n.TraceFile + "\n")
	b.WriteString("\n" + historyHeader + "\n")
	if len(n.History) == 0 {
		b.WriteString(noHistoryLine + "\n")
	}
	for _, r := range n.History {
		b.WriteString("- " + r.Producer + rowSeparator + r.Outcome + rowSeparator + r.At + "\n")
	}
	if n.ParentStatus != "" {
		b.WriteString("\n" + parentHeader + "\n" + n.ParentStatus + "\n")
	}
	return []byte(b.String())
}

// Parse reads a halt note back, skipping every line it does not know.
// It errors only when the `producer:` or `state:` line is missing.
func Parse(data []byte) (Note, error) {
	var n Note
	var sawProducer, sawState bool

	rest := data
	if i := bytes.Index(data, []byte(fenceThenFields)); i >= 0 {
		head := string(data[:i])
		if open := strings.Index(head, fence+"\n"); open >= 0 {
			n.Reason = head[open+len(fence)+1:]
		}
		rest = data[i+len("\n"+fence+"\n\n"):]
	}

	section := ""
	for _, line := range strings.Split(string(rest), "\n") {
		switch {
		case line == historyHeader:
			section = historyHeader
			continue
		case line == parentHeader:
			section = parentHeader
			continue
		}
		switch section {
		case historyHeader:
			if row, ok := parseRow(line); ok {
				n.History = append(n.History, row)
			}
		case parentHeader:
			if n.ParentStatus != "" {
				n.ParentStatus += "\n"
			}
			n.ParentStatus += line
		default:
			key, value, ok := strings.Cut(line, ": ")
			if !ok {
				continue
			}
			switch key {
			case "producer":
				n.Producer, sawProducer = value, true
			case "state":
				n.State, sawState = value, true
			case "anomaly":
				n.Anomaly = value
			case "history_entries":
				n.HistoryEntries, _ = strconv.Atoi(value)
			case "trace_file":
				n.TraceFile = value
			}
		}
	}
	n.ParentStatus = strings.TrimRight(n.ParentStatus, "\n")

	if !sawProducer || !sawState {
		return Note{}, errors.New("haltnote: note lacks its producer or state line")
	}
	return n, nil
}

func parseRow(line string) (Row, bool) {
	body, ok := strings.CutPrefix(line, "- ")
	if !ok {
		return Row{}, false
	}
	parts := strings.SplitN(body, rowSeparator, 3)
	if len(parts) != 3 {
		return Row{}, false
	}
	return Row{Producer: parts[0], Outcome: parts[1], At: parts[2]}, true
}

// IsNoteName reports whether a friction directory file name is a halt note: `loom-halt.md` or `loom-halt-<N>.md`.
func IsNoteName(name string) bool {
	if name == noteFileName {
		return true
	}
	number, ok := strings.CutPrefix(name, numberedPrefix)
	if !ok {
		return false
	}
	number, ok = strings.CutSuffix(number, noteFileSuffix)
	if !ok || number == "" {
		return false
	}
	for _, r := range number {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
