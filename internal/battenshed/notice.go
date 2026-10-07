// notice.go renders the run notices the Run-Shed row hands to InnerRunDeps.Notify, decides when a pending notice may go, and keeps the episode bookkeeping that holds each condition to one notice.
// A notice is informational: it never changes the row's outcome, and the orch acts on it only through "lyx batten status <slug>".

package battenshed

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/shedengine"
)

// noticePrefix is the literal every notice begins with, so the orch can tell a notice from operator input.
const noticePrefix = "[batten notice]"

// noticeErrorMax is the longest child error, in characters, a notice carries.
const noticeErrorMax = 200

// noticePathMax is the longest stop-report path, in characters, a notice carries.
const noticePathMax = 400

// noticeEpisodeFileSuffix is the fixed suffix of the marker recording the notices already sent, joined onto the producer's own name.
const noticeEpisodeFileSuffix = "-notice-episode"

// noticeReportWait is how long after the child's state changed a notice for a parked stop waits for the driver's stop report before it goes without one.
const noticeReportWait = 3 * time.Minute

// noticeRelayWait is how long after the stop report an awaiting child whose driver handed the run's parent a notice is left to that relay, with no decision record, before the notice goes anyway.
const noticeRelayWait = 10 * time.Minute

// noticeMaxAttempts is how many times a notice that was not queued is tried before it is dropped.
const noticeMaxAttempts = 3

// The conditions a notice reports.
const (
	// noticeStateChanged is the child's state changing away from running.
	noticeStateChanged = "state-changed"
	// noticeDriverDead is the child's driver strand found dead while the child is running.
	noticeDriverDead = "driver-dead"
	// noticeQuiet is the child's status file unchanged for the quiet window while the child is running and its driver strand is alive.
	noticeQuiet = "quiet"
)

// noticeEpisodeFile returns the path of the marker holding one episode key per line.
func noticeEpisodeFile(scratchDir, producer string) string {
	return filepath.Join(scratchDir, producer+noticeEpisodeFileSuffix)
}

// oneLine replaces control characters and line separators in s with spaces and cuts it to max characters.
func oneLine(s string, max int) string {
	runes := []rune(s)
	for i, r := range runes {
		if unicode.IsControl(r) || r == '\u2028' || r == '\u2029' {
			runes[i] = ' '
		}
	}
	if len(runes) > max {
		runes = runes[:max]
	}
	return string(runes)
}

// noticeLine is everything one notice line carries.
type noticeLine struct {
	slug      string
	condition string
	status    shedengine.Status
	// since is when the child's current state episode began.
	since time.Time
	// report is the report clause of a parked stop's state-changed notice, empty for every other notice.
	report    string
	attachDir string
}

// reportClause renders the report clause of a parked stop's notice: the stop report's path, or that none exists yet.
func reportClause(path string, found bool) string {
	if !found {
		return "report none yet"
	}
	return "report " + oneLine(path, noticePathMax)
}

// renderNotice renders one notice line: the prefix, the slug, the condition, the child's state and error if any, when the state began, the child's history length, the report clause if any, and the attach command.
func renderNotice(n noticeLine) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s: %s; child state %s", noticePrefix, n.slug, n.condition, n.status.State)
	if n.status.Error != "" {
		fmt.Fprintf(&b, ", error: %s", oneLine(n.status.Error, noticeErrorMax))
	}
	fmt.Fprintf(&b, "; since %s; history %d", n.since.UTC().Format(time.RFC3339), len(n.status.History))
	if n.report != "" {
		fmt.Fprintf(&b, "; %s", n.report)
	}
	fmt.Fprintf(&b, "; cd %s && lyx reed attach", n.attachDir)
	return b.String()
}

// noticeKey renders the episode key of one condition: the condition, the child's state and the stamp that tells its episodes apart.
// The stamp of a state-changed and of a driver-dead notice is the state episode's since; a quiet notice's is the status file's modification time.
func noticeKey(condition string, state shedengine.State, stampNanos int64) string {
	return condition + "|" + string(state) + "|" + strconv.FormatInt(stampNanos, 10)
}

// readNoticeKeys reads the episode keys the marker holds, empty when the marker is absent or unreadable.
func readNoticeKeys(path string) []string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var keys []string
	for _, line := range strings.Split(string(raw), "\n") {
		if line != "" {
			keys = append(keys, line)
		}
	}
	return keys
}

// keepEpisode drops the keys the current status no longer belongs to.
// A key ends with its episode when the child's state changed or the state episode's since moved;
// a quiet key ends when the status file's modification time moved.
func keepEpisode(keys []string, state shedengine.State, sinceNanos, modNanos int64) []string {
	var kept []string
	for _, k := range keys {
		parts := strings.Split(k, "|")
		if len(parts) != 3 || parts[1] != string(state) {
			continue
		}
		want := sinceNanos
		if parts[0] == noticeQuiet {
			want = modNanos
		}
		if parts[2] != strconv.FormatInt(want, 10) {
			continue
		}
		kept = append(kept, k)
	}
	return kept
}

// noticeDelivery is the in-process record of the attempts to send one notice.
type noticeDelivery struct {
	attempts int
	lastTry  time.Time
	// strandWarned is true once the missing orch strand was warned about for this notice.
	strandWarned bool
	// dropped is true once the notice was given up on, after its attempts or its last chance.
	dropped bool
}

// noticeEpisode is the producer's in-process record of the child's current state episode and of the delivery of its notices.
// It outlives a Call, so a pause or an error return does not restart it, and ends when the child's state changes.
type noticeEpisode struct {
	known bool
	state shedengine.State
	// since is the status file's modification time at the first sight of the child in this state.
	// A rewrite of the file in the same state does not move it; only a batten start reads it from the file.
	since      time.Time
	deliveries map[string]*noticeDelivery
}

// observeState records a sight of the child in state, with the status file's modification time at that sight.
// A state the producer has not seen yet starts a new episode whose since is that time; a sight of the same state moves nothing.
func (p *innerRunProducer) observeState(state shedengine.State, at time.Time) {
	if p.episode.known && p.episode.state == state {
		return
	}
	if at.IsZero() {
		at = p.deps.Now()
	}
	p.episode = noticeEpisode{known: true, state: state, since: at, deliveries: map[string]*noticeDelivery{}}
}

// noticeStep sends the notice the child's current status calls for, once per episode, when it may go.
// The row's outcome never depends on it: every failure here is a Warn.
// It does nothing when no Notify was wired.
// final is true for the attempt the Done return makes: it ignores the retry spacing, and a notice that still cannot be queued is logged as lost.
func (p *innerRunProducer) noticeStep(ctx context.Context, w *childWait, final bool) {
	if !p.notices {
		return
	}
	info, err := os.Stat(w.statusPath)
	if err != nil {
		logger.Warn("battenshed: notice skipped; status file unreadable", "producer", p.name, "slug", p.slug, "path", w.statusPath, "error", err)
		return
	}
	status := w.status
	old := readNoticeKeys(noticeEpisodeFile(p.scratchDir, p.name))
	kept := keepEpisode(old, status.State, p.episode.since.UnixNano(), info.ModTime().UnixNano())

	condition, stamp := p.noticeCondition(ctx, w, info.ModTime())
	if condition == "" {
		p.saveEpisode(kept, old)
		return
	}
	key := noticeKey(condition, status.State, stamp)
	for _, k := range kept {
		if k == key {
			p.saveEpisode(kept, old)
			return
		}
	}

	var report string
	if condition == noticeStateChanged {
		ready, clause := p.stateChangeReady(ctx, w)
		if !ready {
			p.saveEpisode(kept, old)
			return
		}
		report = clause
	}
	attachDir, err := p.deps.AttachDir()
	if err != nil {
		logger.Warn("battenshed: notice skipped; attach directory unresolved", "producer", p.name, "slug", p.slug, "error", err)
		return
	}
	line := renderNotice(noticeLine{slug: p.slug, condition: p.conditionText(condition), status: status, since: p.episode.since, report: report, attachDir: attachDir})
	if p.deliver(ctx, key, condition, line, final) {
		kept = append(kept, key)
	}
	p.saveEpisode(kept, old)
}

// noticeCondition returns the condition the child is in and the stamp of its episode key, or an empty condition when it is in none.
// A dead driver strand is checked before the quiet window, so a quiet status file with a live driver is the only quiet case.
func (p *innerRunProducer) noticeCondition(ctx context.Context, w *childWait, modTime time.Time) (string, int64) {
	if w.status.State != shedengine.StateRunning {
		return noticeStateChanged, p.episode.since.UnixNano()
	}
	alive, known := p.driverAlive(ctx, w)
	if !known {
		return "", 0
	}
	if !alive {
		return noticeDriverDead, p.episode.since.UnixNano()
	}
	if p.noticeQuiet > 0 && p.deps.Now().Sub(modTime) >= p.noticeQuiet {
		return noticeQuiet, modTime.UnixNano()
	}
	return "", 0
}

// stateChangeReady decides whether the state-changed notice may go now, and returns its report clause.
// A done child's notice goes at once and carries no report.
// A parked stop's goes once a stop report written at or after the episode's since exists, or the driver strand has ended, or noticeReportWait has passed since;
// a report older than since never qualifies, and a notice that goes without one says `report none yet`.
// An awaiting child that carries a parent notice, while this batten holds the watched marker, is left to its driver's own relay:
// its notice goes when the driver strand has ended, or noticeReportWait passed with no report, or noticeRelayWait passed since the report with no decision record.
func (p *innerRunProducer) stateChangeReady(ctx context.Context, w *childWait) (ready bool, clause string) {
	status := w.status
	if status.State == shedengine.StateDone {
		return true, ""
	}
	now := p.deps.Now()
	path, at, found := p.stopReport()
	qualifies := found && !at.Before(p.episode.since)
	clause = reportClause(path, qualifies)
	alive, known := p.driverAlive(ctx, w)
	ended := known && !alive
	waited := now.Sub(p.episode.since) >= noticeReportWait

	if status.State == shedengine.StateAwaiting && status.ParentNotice != "" && p.watched {
		switch {
		case ended:
			return true, clause
		case !qualifies:
			return waited, clause
		case now.Sub(at) >= noticeRelayWait:
			return p.undecided(), clause
		default:
			return false, clause
		}
	}
	return qualifies || ended || waited, clause
}

// stopReport reads the driver's stop report through deps.StopReport; an error is a Warn and reads as none.
func (p *innerRunProducer) stopReport() (path string, at time.Time, found bool) {
	if p.deps.StopReport == nil {
		return "", time.Time{}, false
	}
	path, at, found, err := p.deps.StopReport()
	if err != nil {
		logger.Warn("battenshed: stop report unreadable; reading it as none", "producer", p.name, "slug", p.slug, "error", err)
		return "", time.Time{}, false
	}
	return path, at, found
}

// undecided reports whether the awaiting child has no decision record yet.
// A read error is a Warn and reads as decided, so the notice waits rather than going on a guess.
func (p *innerRunProducer) undecided() bool {
	_, found, err := p.deps.ReadDecision()
	if err != nil {
		logger.Warn("battenshed: decision unreadable for the notice step; reading it as decided", "producer", p.name, "slug", p.slug, "error", err)
		return false
	}
	return !found
}

// deliver makes one attempt to send line, honouring the retry spacing, the missing orch strand and the attempt cap, and reports whether the notice was queued.
// The attempts are spaced by noticeProbe unless final is set; a notice not queued after noticeMaxAttempts is dropped with a Warn, and a final attempt that does not queue it logs it as lost.
func (p *innerRunProducer) deliver(ctx context.Context, key, condition, line string, final bool) bool {
	d := p.episode.deliveries[key]
	if d == nil {
		d = &noticeDelivery{}
		p.episode.deliveries[key] = d
	}
	if d.dropped {
		return false
	}
	now := p.deps.Now()
	if !final && !d.lastTry.IsZero() && now.Sub(d.lastTry) < p.noticeProbe {
		return false
	}
	d.lastTry = now

	recorded, err := p.deps.OrchStrandRecorded()
	if err != nil {
		logger.Warn("battenshed: notice held back; the orch state is unreadable", "producer", p.name, "slug", p.slug, "condition", condition, "error", err)
		return false
	}
	if !recorded {
		if !d.strandWarned {
			d.strandWarned = true
			logger.Warn("battenshed: notice held back; no orch strand is recorded to receive it", "producer", p.name, "slug", p.slug, "condition", condition)
		}
		if final {
			d.dropped = true
			logger.Warn("battenshed: notice lost; no orch strand was recorded when the run finished", "producer", p.name, "slug", p.slug, "condition", condition)
		}
		return false
	}

	queued, err := p.deps.Notify(ctx, line)
	if err == nil && queued {
		return true
	}
	d.attempts++
	logger.Warn("battenshed: notice not delivered", "producer", p.name, "slug", p.slug, "condition", condition, "queued", queued, "attempt", d.attempts, "error", err)
	if d.attempts >= noticeMaxAttempts || final {
		d.dropped = true
		logger.Warn("battenshed: notice dropped", "producer", p.name, "slug", p.slug, "condition", condition, "attempts", d.attempts)
	}
	return false
}

// conditionText renders the condition as the words a notice carries.
func (p *innerRunProducer) conditionText(condition string) string {
	switch condition {
	case noticeDriverDead:
		return "driver strand is dead while the child is running"
	case noticeQuiet:
		return fmt.Sprintf("status file unchanged for %s while the child is running", p.noticeQuiet)
	default:
		return "child left running"
	}
}

// saveEpisode writes keys to the marker unless they equal old.
// A write failure is a Warn: a lost marker costs at most one repeated notice.
func (p *innerRunProducer) saveEpisode(keys, old []string) {
	if strings.Join(keys, "\n") == strings.Join(old, "\n") {
		return
	}
	if err := os.MkdirAll(p.scratchDir, 0o755); err != nil {
		logger.Warn("battenshed: create scratch directory for notice marker failed", "producer", p.name, "slug", p.slug, "scratchDir", p.scratchDir, "error", err)
		return
	}
	content := ""
	if len(keys) > 0 {
		content = strings.Join(keys, "\n") + "\n"
	}
	if err := os.WriteFile(noticeEpisodeFile(p.scratchDir, p.name), []byte(content), 0o644); err != nil {
		logger.Warn("battenshed: write notice marker failed", "producer", p.name, "slug", p.slug, "error", err)
	}
}
