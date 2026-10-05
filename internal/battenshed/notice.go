// notice.go renders the run notices the Run-Shed row hands to InnerRunDeps.Notify and keeps the episode bookkeeping that holds each condition to one notice.
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

// noticeEpisodeFileSuffix is the fixed suffix of the marker recording the notices already sent, joined onto the producer's own name.
const noticeEpisodeFileSuffix = "-notice-episode"

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
		if unicode.IsControl(r) || r == ' ' || r == ' ' {
			runes[i] = ' '
		}
	}
	if len(runes) > max {
		runes = runes[:max]
	}
	return string(runes)
}

// renderNotice renders one notice line: the prefix, the slug, the condition, the child's state and error if any, and the attach command.
func renderNotice(slug, condition string, status shedengine.Status, attachDir string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s: %s; child state %s", noticePrefix, slug, condition, status.State)
	if status.Error != "" {
		fmt.Fprintf(&b, ", error: %s", oneLine(status.Error, noticeErrorMax))
	}
	fmt.Fprintf(&b, "; cd %s && lyx reed attach", attachDir)
	return b.String()
}

// noticeKey renders the episode key of one condition: the condition, the child's state and the status file's modification time.
func noticeKey(condition string, state shedengine.State, modNanos int64) string {
	return condition + "|" + string(state) + "|" + strconv.FormatInt(modNanos, 10)
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
// A key ends with its episode when the status file's modification time moved, and a state-changed key also ends when the child returns to running.
func keepEpisode(keys []string, modNanos int64, running bool) []string {
	suffix := "|" + strconv.FormatInt(modNanos, 10)
	var kept []string
	for _, k := range keys {
		if !strings.HasSuffix(k, suffix) {
			continue
		}
		if running && strings.HasPrefix(k, noticeStateChanged+"|") {
			continue
		}
		kept = append(kept, k)
	}
	return kept
}

// noticeStep sends the notice the child's current status calls for, once per episode.
// The row's outcome never depends on it: every failure here is a Warn.
// It does nothing when no Notify was wired.
func (p *innerRunProducer) noticeStep(ctx context.Context, statusPath string, status shedengine.Status) {
	if !p.notices {
		return
	}
	info, err := os.Stat(statusPath)
	if err != nil {
		logger.Warn("battenshed: notice skipped; status file unreadable", "producer", p.name, "slug", p.slug, "path", statusPath, "error", err)
		return
	}
	modNanos := info.ModTime().UnixNano()
	running := status.State == shedengine.StateRunning

	old := readNoticeKeys(noticeEpisodeFile(p.scratchDir, p.name))
	kept := keepEpisode(old, modNanos, running)

	condition := noticeStateChanged
	if running {
		condition = p.runningCondition(ctx, info.ModTime())
		if condition == "" {
			p.saveEpisode(kept, old)
			return
		}
	}

	key := noticeKey(condition, status.State, modNanos)
	for _, k := range kept {
		if k == key {
			p.saveEpisode(kept, old)
			return
		}
	}

	attachDir, err := p.deps.AttachDir()
	if err != nil {
		logger.Warn("battenshed: notice skipped; attach directory unresolved", "producer", p.name, "slug", p.slug, "error", err)
		return
	}
	if err := p.deps.Notify(ctx, renderNotice(p.slug, p.conditionText(condition), status, attachDir)); err != nil {
		logger.Warn("battenshed: notice not delivered", "producer", p.name, "slug", p.slug, "condition", condition, "error", err)
	}
	p.saveEpisode(append(kept, key), old)
}

// runningCondition returns the condition a running child is in, or empty when it is in none.
// A dead driver strand is checked before the quiet window, so a quiet status file with a live driver is the only quiet case.
func (p *innerRunProducer) runningCondition(ctx context.Context, modTime time.Time) string {
	alive, err := p.deps.DriverAlive(ctx)
	if err != nil {
		logger.Warn("battenshed: notice skipped; driver liveness read failed", "producer", p.name, "slug", p.slug, "error", err)
		return ""
	}
	if !alive {
		return noticeDriverDead
	}
	if p.noticeQuiet > 0 && p.deps.Now().Sub(modTime) >= p.noticeQuiet {
		return noticeQuiet
	}
	return ""
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
