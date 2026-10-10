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
	// noticeQuiet is every agent of the running child idle for the quiet window, with its driver strand alive and no wait marker live.
	// It is informational: an agent that keeps writing, or a verify or shuttle wait that hangs live, is never quiet, and a dead pid is not an agent.
	noticeQuiet = "quiet"
	// noticeAPIError is a live agent run of the running child whose newest turn end is an API error and which has not been active for noticeAPIErrorIdle since.
	// It takes precedence over noticeQuiet and is informational.
	noticeAPIError = "api-error"
)

// noticeAPIErrorIdle is how long an agent run whose newest turn end is an API error must stay inactive before the api-error notice goes.
const noticeAPIErrorIdle = 2 * time.Minute

// sleepGapTolerance is how far the wall clock may run ahead of the awake clock between two checks before the excess counts as the machine having slept.
// A shorter lag is clock drift or a scheduling delay and still counts as idle time.
const sleepGapTolerance = time.Minute

// sleepGap returns how long the machine slept between two checks: the excess of the wall delta over the awake delta when it exceeds sleepGapTolerance, else zero.
// The wall readings are UnixNano values, so they carry no monotonic part that would hide a sleep.
func sleepGap(prevWallNanos, wallNanos int64, prevAwake, awake time.Duration) time.Duration {
	excess := time.Duration(wallNanos-prevWallNanos) - (awake - prevAwake)
	if excess <= sleepGapTolerance {
		return 0
	}
	return excess
}

// observeClocks folds one check's wall and awake readings into the accumulated suspended time.
// The first check only records the readings and, when known, the activity stamp.
// Later, a known stamp that differs from the recorded one drops the accumulation without adding the gap, since the activity it was counted against has ended; any other check adds the gap.
func (w *childWait) observeClocks(wallNanos int64, awake time.Duration, stamp activityStamp) {
	if !w.clocksSeen {
		w.clocksSeen, w.prevWallNanos, w.prevAwake = true, wallNanos, awake
		if stamp.known {
			w.asleepStamp = stamp
		}
		return
	}
	gap := sleepGap(w.prevWallNanos, wallNanos, w.prevAwake, awake)
	w.prevWallNanos, w.prevAwake = wallNanos, awake
	if stamp.known && w.asleepStamp.known && stamp.nanos != w.asleepStamp.nanos {
		w.asleep, w.asleepStamp = 0, stamp
		return
	}
	if stamp.known {
		w.asleepStamp = stamp
	}
	w.asleep += gap
}

// awakeSince returns how long ago at was as of now, less the suspended time observed since the newest agent activity moved.
func (w *childWait) awakeSince(now, at time.Time) time.Duration {
	return now.Sub(at) - w.asleep
}

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
// The stamp of a state-changed and of a driver-dead notice is the state episode's since; a quiet and an api-error notice's is the newest agent activity.
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
// a quiet or api-error key ends when the newest agent activity moved, and stays while the activity cannot be read.
func keepEpisode(keys []string, state shedengine.State, sinceNanos int64, activity activityStamp) []string {
	var kept []string
	for _, k := range keys {
		parts := strings.Split(k, "|")
		if len(parts) != 3 || parts[1] != string(state) {
			continue
		}
		if parts[0] == noticeQuiet || parts[0] == noticeAPIError {
			if activity.known && parts[2] != strconv.FormatInt(activity.nanos, 10) {
				continue
			}
		} else if parts[2] != strconv.FormatInt(sinceNanos, 10) {
			continue
		}
		kept = append(kept, k)
	}
	return kept
}

// activityStamp is the newest agent activity as an episode stamp, known only when the agents were read.
type activityStamp struct {
	nanos int64
	known bool
}

// noticeFinding is the condition the child is in, the stamp of its episode key, and the words that replace the condition's fixed text when it carries any.
// A zero condition means the child is in none.
type noticeFinding struct {
	condition string
	stamp     int64
	text      string
}

// agentReading is one reading of the child's agent runs.
type agentReading struct {
	runs     []AgentActivity
	waitLive bool
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
	status := w.status
	old := readNoticeKeys(noticeEpisodeFile(p.scratchDir, p.name))
	finding, activity := p.noticeCondition(ctx, w)
	kept := keepEpisode(old, status.State, p.episode.since.UnixNano(), activity)

	condition := finding.condition
	if condition == "" {
		p.saveEpisode(kept, old)
		return
	}
	key := noticeKey(condition, status.State, finding.stamp)
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
	line := renderNotice(noticeLine{slug: p.slug, condition: p.conditionText(finding), status: status, since: p.episode.since, report: report, attachDir: attachDir})
	if p.deliver(ctx, key, condition, line, final) {
		kept = append(kept, key)
	}
	p.saveEpisode(kept, old)
}

// noticeCondition returns the condition the child is in, with the stamp of its episode key, and the newest agent activity the check could read.
// A dead driver strand is checked before the agents, so an agent-based condition needs a live driver.
func (p *innerRunProducer) noticeCondition(ctx context.Context, w *childWait) (noticeFinding, activityStamp) {
	if w.status.State != shedengine.StateRunning {
		return noticeFinding{condition: noticeStateChanged, stamp: p.episode.since.UnixNano()}, activityStamp{}
	}
	alive, known := p.driverAlive(ctx, w)
	if !known {
		return noticeFinding{}, activityStamp{}
	}
	if !alive {
		return noticeFinding{condition: noticeDriverDead, stamp: p.episode.since.UnixNano()}, activityStamp{}
	}
	return p.judgeAgents(ctx, w)
}

// agentActivity reads deps.Activity at most once per probe and reuses the answer on the checks between.
// ok is false when the read failed, which is warned about once per read.
func (p *innerRunProducer) agentActivity(ctx context.Context, w *childWait) (reading agentReading, ok bool) {
	if w.activityStale {
		w.activityStale = false
		runs, waitLive, err := p.deps.Activity(ctx)
		w.activity, w.activityOK = agentReading{runs: runs, waitLive: waitLive}, err == nil
		if err != nil && ctx.Err() == nil {
			logger.Warn("battenshed: agent activity read failed; no activity notice", "producer", p.name, "slug", p.slug, "error", err)
		}
	}
	return w.activity, w.activityOK
}

// judgeAgents returns the api-error or quiet finding of a running child with a live driver, and the newest agent activity.
// With several live runs, api-error needs any one and quiet needs every run idle.
// With no live run found, quiet falls back to the later of the child's newest history entry and the state episode's since.
// Both idle clocks measure awake time: suspended time observed between two checks is subtracted until the newest activity moves, so it can delay a notice and never raise one earlier.
func (p *innerRunProducer) judgeAgents(ctx context.Context, w *childWait) (noticeFinding, activityStamp) {
	reading, ok := p.agentActivity(ctx, w)
	now := p.deps.Now()
	awake := p.deps.Awake()
	if !ok {
		w.observeClocks(now.UnixNano(), awake, activityStamp{})
		return noticeFinding{}, activityStamp{}
	}

	if len(reading.runs) == 0 {
		newest := p.episode.since
		if n := len(w.status.History); n > 0 {
			if at, err := time.Parse(time.RFC3339, w.status.History[n-1].At); err == nil && at.After(newest) {
				newest = at
			}
		}
		stamp := activityStamp{nanos: newest.UnixNano(), known: true}
		w.observeClocks(now.UnixNano(), awake, stamp)
		if idle := w.awakeSince(now, newest); p.noticeQuiet > 0 && !reading.waitLive && idle >= p.noticeQuiet {
			text := fmt.Sprintf("no agent activity readable for %s while the child is running", idle.Round(time.Second))
			return noticeFinding{condition: noticeQuiet, stamp: stamp.nanos, text: text}, stamp
		}
		return noticeFinding{}, stamp
	}

	newest := reading.runs[0].LastActivity
	for _, run := range reading.runs[1:] {
		if run.LastActivity.After(newest) {
			newest = run.LastActivity
		}
	}
	stamp := activityStamp{nanos: newest.UnixNano(), known: true}
	w.observeClocks(now.UnixNano(), awake, stamp)
	for _, run := range reading.runs {
		if run.APIError && w.awakeSince(now, run.LastActivity) >= noticeAPIErrorIdle {
			text := fmt.Sprintf("agent %s hit an API error: %s", run.Producer, oneLine(run.APIErrorText, noticeErrorMax))
			p.logSessionStatesAtNotice(reading.runs, noticeAPIError)
			return noticeFinding{condition: noticeAPIError, stamp: stamp.nanos, text: text}, stamp
		}
	}
	if idle := w.awakeSince(now, newest); p.noticeQuiet > 0 && !reading.waitLive && idle >= p.noticeQuiet {
		text := fmt.Sprintf("agents idle for %s while the child is running", idle.Round(time.Second))
		p.logSessionStatesAtNotice(reading.runs, noticeQuiet)
		return noticeFinding{condition: noticeQuiet, stamp: stamp.nanos, text: text}, stamp
	}
	return noticeFinding{}, stamp
}

// logSessionStatesAtNotice logs at Info the session state of every run in runs beside the condition a finding returned for them, so each notice can be checked against the state.
// It is shadow logging: no notice, stamp or finding depends on the state fields.
func (p *innerRunProducer) logSessionStatesAtNotice(runs []AgentActivity, condition string) {
	for _, run := range runs {
		logger.Info("battenshed: session state at notice", "producer", run.Producer, "slug", p.slug, "condition", condition, "state", run.SessionState, "cause", run.SessionCause, "since", run.SessionSince)
	}
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
func (p *innerRunProducer) conditionText(finding noticeFinding) string {
	if finding.text != "" {
		return finding.text
	}
	switch finding.condition {
	case noticeDriverDead:
		return "driver strand is dead while the child is running"
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
