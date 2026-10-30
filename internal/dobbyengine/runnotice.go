// runnotice.go implements the run notices dobby sends for the running child of each run it drives: a dead driver, a usage limit, an API-error stall and a quiet child.
// It decides each condition on a probe tick, keeps the episode keys of the notices sent in the run's marker, and posts each notice through the Notice seam.

package dobbyengine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/Knatte18/loomyard/internal/logger"
	"github.com/Knatte18/loomyard/internal/pauserecord"
)

// runNoticePrefix is the literal every run notice begins with, so the orch can tell a notice from operator input.
const runNoticePrefix = "[dobby notice]"

// runNoticeErrorMax is the longest error text, in characters, a run notice carries.
const runNoticeErrorMax = 200

// runNoticeMaxAttempts is how many probe ticks try a run notice that was not posted before it is dropped.
const runNoticeMaxAttempts = 3

// apiErrorIdle is how long an agent run whose newest turn end is an API error must stay inactive before the api-error notice goes.
const apiErrorIdle = 2 * time.Minute

// apiErrorReviveStreak is how many api-error episodes in a row, each revived with no write of the child's status file between, make the orch noticed at the last of them.
const apiErrorReviveStreak = 3

// childStateRunning is the child's state while its run has not halted, paused, failed or finished.
const childStateRunning = "running"

// loomStartRemedy is the step the operator takes to bring a child's driver back, which `lym loom resume` does not: it never spawns an LLM driver.
const loomStartRemedy = `run "lym loom start" in the task worktree to bring a driver back`

// The conditions a run notice reports.
const (
	// conditionDriverDead is the running child's driver found dead.
	conditionDriverDead = "driver-dead"
	// conditionQuiet is every agent of the running child idle for the quiet window, with its driver alive and no wait live.
	// It is informational: an agent that keeps writing, or a verify or shuttle wait that hangs live, is never quiet.
	conditionQuiet = "quiet"
	// conditionAPIError is a live agent run whose newest turn end is an API error and which has not been active for apiErrorIdle since.
	// It takes precedence over conditionQuiet.
	conditionAPIError = "api-error"
	// conditionUsageLimit is a live agent run whose newest turn end is the provider's usage limit and which has not been active for apiErrorIdle since.
	// It takes precedence over conditionAPIError and conditionQuiet, sends no revive, and its key stands while any run still reads usage-limited.
	conditionUsageLimit = "usage-limit"
)

// ChildReading is what the run notices read of a run's child, the shed run in its task worktree.
type ChildReading struct {
	// State, CurrentProducer and Error are the child status's state, producer and reason.
	State           string
	CurrentProducer string
	Error           string
	// HistoryLen is the length of the child's history, and NewestHistory when its newest entry was written, zero when it has none or the time does not parse.
	HistoryLen    int
	NewestHistory time.Time
	// Modified is the modification time of the child's status file.
	Modified time.Time
	// PairDir is the task worktree's directory, which every path in a notice is rendered relative to.
	PairDir string
	// RespawnCap is the number of respawns with no write to the status file between that batten stopped at, zero while the running child is not capped.
	RespawnCap int
}

// AgentRun is one agent run of a child, as the run notices read it.
type AgentRun struct {
	// Producer labels the run, the strand name its agent works under.
	Producer string
	// LastActivity is when the run last wrote anything.
	LastActivity time.Time
	// APIError is true when the run's newest turn end is an API error, and APIErrorText is the error's text.
	APIError     bool
	APIErrorText string
	// UsageLimited is true when the API error is the provider's usage limit, and UsageLimitResetsAt is when it resets, zero when unknown.
	UsageLimited       bool
	UsageLimitResetsAt time.Time
	// SessionState, SessionCause and SessionSince are the run's session state, its cause and the RFC 3339 time it began, empty when unread.
	// They are only logged where a notice is decided, and no decision reads them.
	SessionState string
	SessionCause string
	SessionSince string
}

// RunNoticeDeps is every seam the run notices read, told by the hub wiring.
// A nil ReadChild sends no run notice.
type RunNoticeDeps struct {
	// ReadChild reads the child of the run slug; found is false while the child has no status file.
	ReadChild func(slug string) (child ChildReading, found bool, err error)
	// DriverAlive reports whether the child has a live driver: a live driver strand, or a held driver or run lock.
	DriverAlive func(ctx context.Context, slug string) (bool, error)
	// Agents reads the child's agent runs, and whether a Go-side wait is live, such as a verify, a parent lock wait or a pull request's review.
	Agents func(ctx context.Context, slug string) (runs []AgentRun, waitLive bool, err error)
	// Pause reads the pause record's coverage of the slug, with its paused intervals.
	Pause func(slug string) (pauserecord.Coverage, error)
	// ReviveAPIError sends the revive line to the child's sessions stalled on an API error.
	// A nil ReviveAPIError revives nothing, and an api-error notice then goes at its first sight.
	ReviveAPIError func(ctx context.Context, slug string) error
	// MarkerPath returns the path of the marker holding the episode keys of the notices sent for slug, one per line.
	MarkerPath func(slug string) string
	// Quiet is how long every agent of a running child may stay idle before the quiet notice; zero sends none.
	Quiet time.Duration
}

// RunNoticeState is dobby's memory of one run's child for the run notices, kept in the run's entry of the state record so a re-exec keeps it.
type RunNoticeState struct {
	// ChildState is the child's state at the last sight, and Since the child status's modification time at the first sight of it in that state.
	ChildState string    `json:"child_state,omitempty"`
	Since      time.Time `json:"since,omitzero"`
	// APIReviveKey is the api-error episode key last revived, APIRevives how many episodes in a row were revived and APIReviveStatusTime the child status's modification time at the last revive.
	APIReviveKey        string    `json:"api_revive_key,omitempty"`
	APIRevives          int       `json:"api_revives,omitempty"`
	APIReviveStatusTime time.Time `json:"api_revive_status_time,omitzero"`
}

// noticeDelivery is the record of the attempts to post one run notice.
type noticeDelivery struct {
	attempts int
	dropped  bool
}

// noticeFinding is the condition the child is in, the stamp of its episode key and the words that follow the child's state in the notice.
// A zero condition means the child is in none.
type noticeFinding struct {
	condition string
	stamp     int64
	text      string
}

// activityStamp is the newest agent activity as an episode stamp, known only when the agents were read.
// usageLimited is true when any run read stands on the provider's usage limit.
type activityStamp struct {
	nanos        int64
	known        bool
	usageLimited bool
}

// runNotices runs the run notices of every listed run whose batten state is running with no cancel marker, on a probe tick only.
// A failure is logged once per episode and never fails the tick.
func (d *daemon) runNotices(ctx context.Context, now time.Time) {
	if d.deps.RunNotices.ReadChild == nil || !d.probeDue {
		return
	}
	current, _, err := ReadState(d.geom.StatePath)
	if err != nil {
		d.noticeFailures.Fail("state", "dobbyengine: the state record is unreadable; no run notice this tick", "error", err)
		return
	}
	d.noticeFailures.Recover("state", "dobbyengine: the state record reads again for the run notices")
	listed := map[string]bool{}
	for _, run := range d.runs {
		if run.State != runStateRunning || run.CancelMarker {
			continue
		}
		listed[run.Slug] = true
		d.runNotice(ctx, now, run.Slug, current.Runs[run.Slug].Notice, current.Sleeps)
	}
	for key := range d.deliveries {
		if slug, _, _ := strings.Cut(key, "|"); !listed[slug] {
			delete(d.deliveries, key)
		}
	}
}

// runNotice sends the notice the child of slug calls for, once per episode, and records the episode keys and the memory.
// A child that is not running is in no condition, and its keys are dropped.
func (d *daemon) runNotice(ctx context.Context, now time.Time, slug string, memory RunNoticeState, sleeps []pauserecord.Interval) {
	seams := d.deps.RunNotices
	child, found, err := seams.ReadChild(slug)
	if err != nil {
		d.noticeFailures.Fail(slug+"|child", "dobbyengine: the child is unreadable; no run notice", "slug", slug, "error", err)
		return
	}
	d.noticeFailures.Recover(slug+"|child", "dobbyengine: the child reads again for the run notices", "slug", slug)
	if !found {
		return
	}
	recorded := memory
	if memory.ChildState != child.State || memory.Since.IsZero() {
		since := child.Modified
		if since.IsZero() {
			since = now
		}
		memory.ChildState, memory.Since = child.State, since
	}

	markerPath := seams.MarkerPath(slug)
	old := readNoticeKeys(markerPath)
	finding, activity := d.noticeCondition(ctx, now, slug, child, memory, sleeps)
	kept := keepEpisode(old, child.State, memory.Since.UnixNano(), activity)
	defer func() {
		saveNoticeKeys(markerPath, slug, kept, old)
		d.saveNoticeMemory(slug, memory, recorded)
	}()
	if finding.condition == "" {
		return
	}
	key := noticeKey(finding.condition, child.State, finding.stamp)
	for _, k := range kept {
		if k == key {
			return
		}
	}
	if finding.condition == conditionAPIError && seams.ReviveAPIError != nil && !d.apiErrorNoticeDue(ctx, slug, key, child.Modified, &memory) {
		return
	}
	line := renderRunNotice(slug, conditionText(finding, child), child, memory.Since)
	if d.deliverRunNotice(ctx, slug, key, finding.condition, line) {
		kept = append(kept, key)
	}
}

// noticeCondition returns the condition the child is in, with the stamp of its episode key, and the newest agent activity it could read.
// Only a running child that no pause covers is in a condition, and a dead driver is checked before the agents, so an agent-based condition needs a live driver.
// A dead driver respawned the cap of times names the cap and `lym loom start`.
func (d *daemon) noticeCondition(ctx context.Context, now time.Time, slug string, child ChildReading, memory RunNoticeState, sleeps []pauserecord.Interval) (noticeFinding, activityStamp) {
	if child.State != childStateRunning {
		return noticeFinding{}, activityStamp{}
	}
	seams := d.deps.RunNotices
	pause, err := seams.Pause(slug)
	if err != nil {
		d.noticeFailures.Fail(slug+"|pause", "dobbyengine: the pause record is unreadable; no run notice", "slug", slug, "error", err)
		return noticeFinding{}, activityStamp{}
	}
	d.noticeFailures.Recover(slug+"|pause", "dobbyengine: the pause record reads again for the run notices", "slug", slug)
	// A running child under a pause has no agents at work and no driver to wait for, so none of its conditions is an event.
	if pause.Covered {
		return noticeFinding{}, activityStamp{}
	}
	alive, err := seams.DriverAlive(ctx, slug)
	if err != nil {
		d.noticeFailures.Fail(slug+"|driver", "dobbyengine: the child's driver liveness is unreadable; no run notice", "slug", slug, "error", err)
		return noticeFinding{}, activityStamp{}
	}
	d.noticeFailures.Recover(slug+"|driver", "dobbyengine: the child's driver liveness reads again", "slug", slug)
	if !alive {
		finding := noticeFinding{condition: conditionDriverDead, stamp: memory.Since.UnixNano(), text: "driver strand is dead"}
		if child.RespawnCap > 0 {
			finding.text = fmt.Sprintf("driver strand is dead; respawned %d times without progress; %s", child.RespawnCap, loomStartRemedy)
		}
		return finding, activityStamp{}
	}
	runs, waitLive, err := seams.Agents(ctx, slug)
	if err != nil {
		d.noticeFailures.Fail(slug+"|agents", "dobbyengine: the child's agent activity is unreadable; no activity notice", "slug", slug, "error", err)
		return noticeFinding{}, activityStamp{}
	}
	d.noticeFailures.Recover(slug+"|agents", "dobbyengine: the child's agent activity reads again", "slug", slug)
	return judgeAgents(now, slug, child, memory.Since, runs, waitLive, seams.Quiet, sleeps, pause.Intervals)
}

// judgeAgents returns the usage-limit, api-error or quiet finding of a running child with a live driver, and the newest agent activity.
// With several runs read, usage-limit and api-error need any one and quiet needs every run idle.
// With no run read, quiet falls back to the later of the child's newest history entry and the state episode's since.
// Both idle clocks measure awake unpaused time: every recorded sleep and every paused interval after an activity is subtracted from the time since it, so a sleep or a pause can delay a notice and never raise one earlier.
func judgeAgents(now time.Time, slug string, child ChildReading, since time.Time, runs []AgentRun, waitLive bool, quiet time.Duration, sleeps, paused []pauserecord.Interval) (noticeFinding, activityStamp) {
	awakeSince := func(at time.Time) time.Duration {
		return pauserecord.AwakeSince(at, now, sleeps, paused)
	}

	if len(runs) == 0 {
		newest := since
		if child.NewestHistory.After(newest) {
			newest = child.NewestHistory
		}
		stamp := activityStamp{nanos: newest.UnixNano(), known: true}
		if idle := awakeSince(newest); quiet > 0 && !waitLive && idle >= quiet {
			text := fmt.Sprintf("no agent activity readable for %s", idle.Round(time.Second))
			return noticeFinding{condition: conditionQuiet, stamp: stamp.nanos, text: text}, stamp
		}
		return noticeFinding{}, stamp
	}

	newest := runs[0].LastActivity
	for _, run := range runs[1:] {
		if run.LastActivity.After(newest) {
			newest = run.LastActivity
		}
	}
	stamp := activityStamp{nanos: newest.UnixNano(), known: true}
	for _, run := range runs {
		stamp.usageLimited = stamp.usageLimited || run.UsageLimited
	}
	for _, run := range runs {
		if run.UsageLimited && awakeSince(run.LastActivity) >= apiErrorIdle {
			text := fmt.Sprintf("agent %s hit a usage limit, %s: %s", run.Producer, resetText(run.UsageLimitResetsAt), oneLineCut(run.APIErrorText, runNoticeErrorMax))
			logSessionStatesAtNotice(slug, runs, conditionUsageLimit)
			return noticeFinding{condition: conditionUsageLimit, stamp: resetStamp(run.UsageLimitResetsAt), text: text}, stamp
		}
	}
	for _, run := range runs {
		if run.APIError && awakeSince(run.LastActivity) >= apiErrorIdle {
			text := fmt.Sprintf("agent %s hit an API error: %s", run.Producer, oneLineCut(run.APIErrorText, runNoticeErrorMax))
			logSessionStatesAtNotice(slug, runs, conditionAPIError)
			return noticeFinding{condition: conditionAPIError, stamp: stamp.nanos, text: text}, stamp
		}
	}
	if idle := awakeSince(newest); quiet > 0 && !waitLive && idle >= quiet {
		text := fmt.Sprintf("agents idle for %s", idle.Round(time.Second))
		logSessionStatesAtNotice(slug, runs, conditionQuiet)
		return noticeFinding{condition: conditionQuiet, stamp: stamp.nanos, text: text}, stamp
	}
	return noticeFinding{}, stamp
}

// logSessionStatesAtNotice logs at Debug the session state of every run in runs beside the condition a finding returned for them, so each notice can be checked against the state.
// It is shadow logging: no notice, stamp or finding depends on the state fields.
func logSessionStatesAtNotice(slug string, runs []AgentRun, condition string) {
	for _, run := range runs {
		logger.Debug("dobbyengine: session state at notice", "producer", run.Producer, "slug", slug, "condition", condition, "state", run.SessionState, "cause", run.SessionCause, "since", run.SessionSince)
	}
}

// apiErrorNoticeDue revives the stalled sessions of slug when the api-error episode key has not been revived yet, and reports whether the orch is noticed now.
// The first sight of an episode revives and notices nothing; the same episode standing at a later probe tick after its revive is noticed.
// So is the third revived episode in a row with no write of the child's status file between them, which has its revive sent too.
// statusTime is the child status's modification time; a write since the last revive resets the count of episodes in a row.
// A revive error is logged once per episode and still counts as the revive.
func (d *daemon) apiErrorNoticeDue(ctx context.Context, slug, key string, statusTime time.Time, memory *RunNoticeState) bool {
	if memory.APIReviveKey == key {
		return true
	}
	if statusTime.After(memory.APIReviveStatusTime) {
		memory.APIRevives = 0
	}
	if err := d.deps.RunNotices.ReviveAPIError(ctx, slug); err != nil {
		if ctx.Err() == nil {
			d.noticeFailures.Fail(slug+"|revive", "dobbyengine: could not revive the session stalled on an API error", "slug", slug, "error", err)
		}
	} else {
		d.noticeFailures.Recover(slug+"|revive", "dobbyengine: the API-error revive succeeds again", "slug", slug)
	}
	memory.APIReviveKey, memory.APIReviveStatusTime = key, statusTime
	memory.APIRevives++
	return memory.APIRevives >= apiErrorReviveStreak
}

// deliverRunNotice makes one attempt to post line through the Notice seam, and reports whether it was posted.
// A notice not posted after runNoticeMaxAttempts probe ticks is dropped with a Warn.
func (d *daemon) deliverRunNotice(ctx context.Context, slug, key, condition, line string) bool {
	deliveryKey := slug + "|" + key
	if d.deliveries == nil {
		d.deliveries = map[string]*noticeDelivery{}
	}
	delivery := d.deliveries[deliveryKey]
	if delivery == nil {
		delivery = &noticeDelivery{}
		d.deliveries[deliveryKey] = delivery
	}
	if delivery.dropped || d.deps.Notice == nil {
		return false
	}
	err := d.deps.Notice(ctx, line)
	if err == nil {
		delete(d.deliveries, deliveryKey)
		return true
	}
	delivery.attempts++
	logger.Warn("dobbyengine: run notice not delivered", "slug", slug, "condition", condition, "attempt", delivery.attempts, "error", err)
	if delivery.attempts >= runNoticeMaxAttempts {
		delivery.dropped = true
		logger.Warn("dobbyengine: run notice dropped", "slug", slug, "condition", condition, "attempts", delivery.attempts)
	}
	return false
}

// saveNoticeMemory writes memory into the run's entry of the state record unless it equals old.
// A write failure is a Warn: a lost memory costs at most one repeated notice.
func (d *daemon) saveNoticeMemory(slug string, memory, old RunNoticeState) {
	if memory == old {
		return
	}
	err := UpdateState(d.geom.StatePath, func(s *State) error {
		if s.Runs == nil {
			s.Runs = map[string]RunState{}
		}
		rs := s.Runs[slug]
		rs.Notice = memory
		s.Runs[slug] = rs
		return nil
	})
	if err != nil {
		logger.Warn("dobbyengine: the run notice memory could not be recorded", "slug", slug, "error", err)
	}
}

// renderRunNotice renders one run notice line: the prefix, the slug, the condition, the child's reason if any, when its state began and its history length.
// Every path under the pair directory is rendered relative to it, since the slug already names the pair.
func renderRunNotice(slug, condition string, child ChildReading, since time.Time) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s: %s", runNoticePrefix, slug, pairRelative(condition, child.PairDir))
	if child.Error != "" {
		fmt.Fprintf(&b, "; waiting: %s", oneLineCut(pairRelative(child.Error, child.PairDir), runNoticeErrorMax))
	}
	fmt.Fprintf(&b, "; since %s; history %d", since.UTC().Format(time.RFC3339), child.HistoryLen)
	return b.String()
}

// conditionText renders the condition as the words a notice carries: the child's state and row, then `; `, then the finding's own text.
func conditionText(finding noticeFinding, child ChildReading) string {
	state := child.State
	if child.CurrentProducer != "" {
		state += " at " + child.CurrentProducer
	}
	return state + "; " + finding.text
}

// pairRelative strips the pair directory prefix from every path in text that lies under it.
// The prefix is stripped only where it starts a path, so a longer path that merely contains the pair directory is left whole.
func pairRelative(text, pairDir string) string {
	if pairDir == "" {
		return text
	}
	pathUnderPair := regexp.MustCompile(`(^|[^\w./\\~-])` + regexp.QuoteMeta(pairDir+string(filepath.Separator)))
	return pathUnderPair.ReplaceAllString(text, "${1}")
}

// oneLineCut replaces control characters and line separators in s with spaces and cuts it to max characters.
func oneLineCut(s string, max int) string {
	runes := []rune(s)
	for i, r := range runes {
		if unicode.IsControl(r) || r == ' ' || r == ' ' {
			runes[i] = ' '
		}
	}
	if len(runes) > max {
		runes = runes[:max]
	}
	return string(runes)
}

// resetText renders when a usage limit resets, in UTC as a notice's since is, or that its reset time is unknown.
func resetText(resetsAt time.Time) string {
	if resetsAt.IsZero() {
		return "reset time unknown"
	}
	return "resets " + resetsAt.UTC().Format(time.RFC3339)
}

// resetStamp returns the episode stamp of a usage limit that resets at resetsAt: its Unix seconds, or 0 for an unknown reset time.
func resetStamp(resetsAt time.Time) int64 {
	if resetsAt.IsZero() {
		return 0
	}
	return resetsAt.Unix()
}

// noticeKey renders the episode key of one condition: the condition, the child's state and the stamp that tells its episodes apart.
// The stamp of a driver-dead notice is the state episode's since; a quiet and an api-error notice's is the newest agent activity, and a usage-limit notice's the limit's reset time.
func noticeKey(condition, state string, stampNanos int64) string {
	return condition + "|" + state + "|" + strconv.FormatInt(stampNanos, 10)
}

// keepEpisode drops the keys the child's current reading no longer belongs to.
// A key ends with its episode when the child's state changed or the state episode's since moved;
// a quiet or api-error key ends when the newest agent activity moved, and a usage-limit key when no run reads usage-limited any more; each stays while the activity cannot be read.
func keepEpisode(keys []string, state string, sinceNanos int64, activity activityStamp) []string {
	var kept []string
	for _, k := range keys {
		parts := strings.Split(k, "|")
		if len(parts) != 3 || parts[1] != state {
			continue
		}
		switch parts[0] {
		case conditionUsageLimit:
			if activity.known && !activity.usageLimited {
				continue
			}
		case conditionQuiet, conditionAPIError:
			if activity.known && parts[2] != strconv.FormatInt(activity.nanos, 10) {
				continue
			}
		default:
			if parts[2] != strconv.FormatInt(sinceNanos, 10) {
				continue
			}
		}
		kept = append(kept, k)
	}
	return kept
}

// readNoticeKeys reads the episode keys the marker at path holds, none when it is absent or unreadable.
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

// saveNoticeKeys writes keys to the marker at path unless they equal old.
// A write failure is a Warn: a lost marker costs at most one repeated notice.
func saveNoticeKeys(path, slug string, keys, old []string) {
	if strings.Join(keys, "\n") == strings.Join(old, "\n") {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		logger.Warn("dobbyengine: create the run notice marker's directory failed", "slug", slug, "path", path, "error", err)
		return
	}
	content := ""
	if len(keys) > 0 {
		content = strings.Join(keys, "\n") + "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		logger.Warn("dobbyengine: write the run notice marker failed", "slug", slug, "path", path, "error", err)
	}
}
