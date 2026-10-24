// publishfailure.go renders webster's run record as the Plan-Review rubric's note.

package loomshed

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/websterengine"
)

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
