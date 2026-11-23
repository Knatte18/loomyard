// oomnotice.go tells a worktree's parent orch when the kernel's OOM killer ends a process in one of its strand panes.
// Each entered session's watch goroutine reads its strand panes' OOM kill counts and posts a notice for each new kill.

package reedcli

import (
	"context"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/lymcwd"
	"github.com/Knatte18/loomyard/internal/orchcli"
	"github.com/Knatte18/loomyard/internal/proc"
	"github.com/Knatte18/loomyard/internal/reedengine"
)

// reedProgram is the program name the watchdog signs its notices with.
const reedProgram = "REED"

// paneOOMKills maps the cgroup of each capped strand pane a session showed at its last read to a count.
// The count is the OOM kills in that cgroup that notices have told of.
type paneOOMKills map[string]int

// paneOOMNotice is one capped strand pane whose cgroup counts OOM kills no notice has told of yet.
type paneOOMNotice struct {
	strand string
	pane   string
	cgroup string
	// told is the count of kills in cgroup that earlier notices told of.
	told int
	// newKills is the count of kills in cgroup past told.
	newKills int
}

// paneOOMBoundaries holds what a session's OOM watch reads and writes outside the process.
type paneOOMBoundaries struct {
	// strands reads the session's strands with their pane shell pids.
	strands func() ([]reedengine.StrandStatus, error)
	// paneKills reads a pane shell's cgroup and the count of OOM kills in it.
	paneKills func(pid int) (cgroup string, kills int, err error)
	// journalKills reads the kills the kernel journal records for a cgroup, oldest first.
	journalKills func(cgroup string) ([]proc.OOMKill, error)
	// post sends a notice line to the worktree's parent orch.
	post func(line string) error
}

// engineOOMBoundaries returns the boundaries of the session that eng drives in the worktree at location.
func engineOOMBoundaries(eng *reedengine.Engine, location *lymcwd.Location) paneOOMBoundaries {
	return paneOOMBoundaries{
		strands: func() ([]reedengine.StrandStatus, error) {
			status, err := eng.Status()
			return status.Strands, err
		},
		paneKills:    proc.ProcessOOMKills,
		journalKills: proc.OOMKillsInCgroup,
		post: func(line string) error {
			_, err := orchcli.NotifyAsker(location, reedProgram, line)
			return err
		},
	}
}

// planPaneOOMNotices returns a notice for each live capped strand pane.
// A pane gets one when its cgroup counts more OOM kills than seen holds.
// readKills reads a pane shell's cgroup and its kill count; a pane it cannot read is skipped.
// A pane whose cgroup is not a pane scope, so one the memory cap never reached, is skipped:
// that cgroup may hold other panes or the tmux server, and its kills are none of this strand's.
// seen is not advanced; it loses each cgroup no live pane is in any more.
// A cgroup first seen counts from 0, so a kill before the daemon first looked is still told.
func planPaneOOMNotices(strands []reedengine.StrandStatus, readKills func(pid int) (string, int, error), seen paneOOMKills) []paneOOMNotice {
	var notices []paneOOMNotice
	current := make(map[string]bool, len(strands))
	for _, s := range strands {
		if s.PanePID <= 0 {
			continue
		}
		cgroup, kills, err := readKills(s.PanePID)
		if cgroup != "" {
			current[cgroup] = true
		}
		if err != nil || !strings.HasPrefix(path.Base(cgroup), reedengine.PaneScopeUnitPrefix) {
			continue
		}
		if told := seen[cgroup]; kills > told {
			notices = append(notices, paneOOMNotice{strand: s.Name, pane: s.PaneID, cgroup: cgroup, told: told, newKills: kills - told})
		}
	}
	for cgroup := range seen {
		if !current[cgroup] {
			delete(seen, cgroup)
		}
	}
	return notices
}

// paneOOMNoticeLine returns the notice line for notice in the worktree session.
// killed is what the kernel journal records for the pane's cgroup, oldest first.
// The journal is cumulative and the kernel's line can land after the kill is counted,
// so the line names the notice.newKills entries that follow the notice.told it already told of.
// When the journal holds fewer, or could not be read, the line counts the kills and gives journalErr.
func paneOOMNoticeLine(session string, notice paneOOMNotice, killed []proc.OOMKill, journalErr error) string {
	what := fmt.Sprintf("%d process(es) the kernel journal does not name", notice.newKills)
	if journalErr != nil {
		what += fmt.Sprintf(" (%v)", journalErr)
	} else if end := notice.told + notice.newKills; len(killed) >= end {
		names := make([]string, 0, notice.newKills)
		for _, kill := range killed[notice.told:end] {
			names = append(names, fmt.Sprintf("%s (pid %d)", kill.Name, kill.PID))
		}
		what = strings.Join(names, ", ")
	}
	return fmt.Sprintf("reed: the kernel's OOM killer ended %s at the memory cap of strand %s, pane %s, in worktree %s; the pane lives on",
		what, notice.strand, notice.pane, session)
}

// reportPaneOOMKills posts a notice to the parent orch for each capped strand pane with a new OOM kill.
// It reports whether it posted any.
// A kill counts as told in seen only once its post succeeds, so a failed post is retried at the next read.
// A failure to read the session's strands or to post is logged and never stops the watch.
func reportPaneOOMKills(hub, session string, b paneOOMBoundaries, seen paneOOMKills) (posted bool) {
	strands, err := b.strands()
	if err != nil {
		logger.Debug("reed: watchdog could not read the strands to check their OOM kills", "hub", hub, "session", session, "err", err)
		return false
	}
	for _, notice := range planPaneOOMNotices(strands, b.paneKills, seen) {
		logger.Debug("reed: watchdog reading the kernel journal through journalctl for a pane's OOM kills", "hub", hub, "session", session, "cgroup", notice.cgroup)
		killed, journalErr := b.journalKills(notice.cgroup)
		line := paneOOMNoticeLine(session, notice, killed, journalErr)
		if err := b.post(line); err != nil {
			logger.Warn("reed: watchdog could not post a pane's OOM kill to its parent", "hub", hub, "session", session, "err", err)
			continue
		}
		logger.Warn(line, "hub", hub, "cgroup", notice.cgroup)
		seen[notice.cgroup] = notice.told + notice.newKills
		posted = true
	}
	return posted
}

// watchPaneOOMKills reports every new OOM kill in a capped strand pane of session until ctx is done.
// It reads at once, then waits timing.DiscoveryCycle between reads.
// The wait doubles up to timing.DiscoveryCeiling across reads that post nothing.
// It runs in its own goroutine because reading the strands waits on the worktree's reed operation lock.
// The journal read and the post take time of their own.
func watchPaneOOMKills(ctx context.Context, hub, session string, b paneOOMBoundaries, timing watchdogTiming) {
	seen := paneOOMKills{}
	cadence := timing.DiscoveryCycle
	for {
		posted := reportPaneOOMKills(hub, session, b, seen)
		cadence = reedengine.NextWakeCadence(cadence, timing.DiscoveryCycle, timing.DiscoveryCeiling, posted)
		timer := time.NewTimer(cadence)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
