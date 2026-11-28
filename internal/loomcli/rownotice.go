// rownotice.go wraps the producers of the rows loom.yaml's notify_parent_on names.
// lym mails the run's parent each time the run reaches one of them, so the parent can brief the row's agent.

package loomcli

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/shedrun"
	"github.com/Knatte18/loomyard/internal/state"
)

// rowNoticePaths locates the files a row notice reads and writes.
type rowNoticePaths struct {
	// status and statusLock are the run's status file and its lock, whose history counts the visit.
	status     string
	statusLock string
	// record is the run's row-notice record, holding the last visit told.
	record string
}

// watchRows wraps the producer of every row of shed that rows names,
// so the run reaching such a row mails notify a `[row reached]` line before the row's call.
// A name that is no row of shed is refused, naming recipe and its rows as the way forward.
func watchRows(shed *shedengine.Shed, rows []string, recipe string, paths rowNoticePaths, notify func(line string) error) error {
	names := make([]string, len(shed.Producers))
	for i, def := range shed.Producers {
		names[i] = def.Name
	}
	for _, row := range rows {
		if !slices.Contains(names, row) {
			return fmt.Errorf("loom config key %q names %q, which is no row of the %s recipe; way forward: set notify_parent_on in loom.yaml or the task's run_settings to rows of that recipe: %s", "notify_parent_on", row, recipe, strings.Join(names, ", "))
		}
	}
	for i, def := range shed.Producers {
		if slices.Contains(rows, def.Name) {
			shed.Producers[i].Producer = rowNoticeProducer{row: def.Name, inner: def.Producer, paths: paths, notify: notify}
		}
	}
	return nil
}

// rowNoticeProducer tells the run's parent that the run reached row, once per visit, then calls the row's own producer.
// A re-call after an escalation's answer or a resumed halt is a new visit, since it spawns a fresh agent.
type rowNoticeProducer struct {
	row    string
	inner  shedengine.ShedProducer
	paths  rowNoticePaths
	notify func(line string) error
}

// Call implements shedengine.ShedProducer: it tells the parent of a new visit, then returns the row's own call.
func (p rowNoticeProducer) Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error) {
	p.tell()
	return p.inner.Call(ctx)
}

// tell mails the row's notice unless the record already holds this visit.
// The visit is recorded before the mail goes out, so a failed post is never retried.
// A failed read, write or post only warns: a notice never stops the run.
func (p rowNoticeProducer) tell() {
	st, found, err := state.ReadJSON[shedengine.Status](p.paths.status, p.paths.statusLock)
	if err != nil || !found {
		logger.Warn("loomcli: the row notice could not read the run status; no notice sent", "row", p.row, "found", found, "error", err)
		return
	}
	visit := shedrun.RowNotice{Row: p.row, Visit: verdictCount(st.History)}
	told, err := shedrun.ReadRowNotice(p.paths.record)
	if err != nil {
		logger.Warn("loomcli: the row notice could not read its record; no notice sent", "row", p.row, "error", err)
		return
	}
	if told == visit {
		return
	}
	if err := shedrun.WriteRowNotice(p.paths.record, visit); err != nil {
		logger.Warn("loomcli: the row notice could not write its record; no notice sent", "row", p.row, "error", err)
		return
	}
	if err := p.notify(rowReachedLine(p.row)); err != nil {
		logger.Warn("loomcli: the row notice was not posted", "row", p.row, "error", err)
	}
}

// verdictCount returns the number of verdicts history records, each folded repeat counted.
// It grows with every recorded verdict and stays put across a yield or a pause, which record none.
// An escalation or a halt records its verdict before the row is called again, so that re-call counts as a new visit.
func verdictCount(history []shedengine.HistoryEntry) int {
	count := 0
	for _, entry := range history {
		count += 1 + entry.Repeats
	}
	return count
}

// rowReachedLine renders the notice that the run reached row and calls it now.
func rowReachedLine(row string) string {
	return fmt.Sprintf("[row reached] the run reached %s and calls it now; brief the row's agent with lym creel post once lym reed list shows its strand, ending your message with \"Answer briefly, then continue your task.\"", row)
}
