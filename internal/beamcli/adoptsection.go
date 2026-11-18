// adoptsection.go renders the adopt hold the hub's Bolt records as the `adopt` section of `beam status`.

package beamcli

import (
	"fmt"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/boardengine"
)

// adoptSection is the `adopt` section of `beam status`: the adopt hold that stands over the hub.
// Kind is the hold's kind, Machine the machine that wrote it and At when.
// FixTask and Tries, each subject's fix-try count, are set only under an adopt-fix hold.
// Rerun is the `lym adopt <subjects>` command, set only for this machine's own hold.
// Lines is the section rendered as text, opening with `adopt hold`.
type adoptSection struct {
	Kind     string         `json:"kind"`
	Machine  string         `json:"machine"`
	At       time.Time      `json:"at"`
	Subjects []string       `json:"subjects"`
	FixTask  string         `json:"fix_task,omitempty"`
	Tries    map[string]int `json:"tries,omitempty"`
	Rerun    string         `json:"rerun,omitempty"`
	Lines    []string       `json:"lines"`
}

// adoptSectionOf returns the section for hold; own reports whether the hold's machine reads as this machine.
func adoptSectionOf(hold boardengine.Hold, own bool) *adoptSection {
	subjects := make([]string, len(hold.Subjects))
	for i, held := range hold.Subjects {
		subjects[i] = held.Subject
	}
	section := &adoptSection{
		Kind:     hold.Kind,
		Machine:  hold.Machine,
		At:       hold.At,
		Subjects: subjects,
		Lines: []string{
			fmt.Sprintf("adopt hold (%s) by %s since %s over %s", hold.Kind, hold.Machine, hold.At.Format(time.RFC3339), strings.Join(subjects, ", ")),
		},
	}
	if hold.Kind == boardengine.HoldKindAdoptFix {
		section.FixTask = hold.FixTask
		section.Tries = make(map[string]int, len(hold.Subjects))
		tries := make([]string, len(hold.Subjects))
		for i, held := range hold.Subjects {
			section.Tries[held.Subject] = held.Tries
			tries[i] = fmt.Sprintf("%s %d", held.Subject, held.Tries)
		}
		section.Lines = append(section.Lines, fmt.Sprintf("fix task %s, tries %s", hold.FixTask, strings.Join(tries, ", ")))
	}
	if own {
		section.Rerun = "lym adopt " + strings.Join(subjects, " ")
		section.Lines = append(section.Lines, "way forward: "+section.Rerun)
	}
	return section
}
