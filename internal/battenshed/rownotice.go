// rownotice.go implements the failure notice the rows after Run-Shed send the orch.
// It also owns the per-run markers, which batten clears where it seeds a fresh status, and the hub notice log every notice is appended to.

package battenshed

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
)

// rowNoticeFileSuffix is the fixed suffix of the marker recording the notices a row already sent, joined onto the row's own name.
const rowNoticeFileSuffix = "-row-notice"

// rowNoticeRetrySpacing is the pause between two delivery attempts of one row notice.
const rowNoticeRetrySpacing = 30 * time.Second

// rowNoticeFailurePrefix leads the marker line recording the history length of the last failure episode noticed.
const rowNoticeFailurePrefix = "failure "

// runMarkerSuffixes names every marker that must last exactly one batten run, by file-name suffix.
// ClearRunMarkers removes them all where batten seeds a fresh status.
var runMarkerSuffixes = []string{
	rowNoticeFileSuffix,
	readySnapshotFileSuffix,
	publishRetryFileSuffix,
}

// ClearRunMarkers removes every file in scratchDir whose name ends in a suffix of runMarkerSuffixes.
// An absent file or directory is not an error.
func ClearRunMarkers(scratchDir string) error {
	entries, err := os.ReadDir(scratchDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("battenshed: read scratch directory %s: %w", scratchDir, err)
	}
	for _, entry := range entries {
		for _, suffix := range runMarkerSuffixes {
			if !strings.HasSuffix(entry.Name(), suffix) {
				continue
			}
			path := filepath.Join(scratchDir, entry.Name())
			if err := os.RemoveAll(path); err != nil {
				return fmt.Errorf("battenshed: remove run marker %s: %w", path, err)
			}
			break
		}
	}
	return nil
}

// NoticeLog names the hub file every notice this process composes is appended to, and what the notice's line carries beside the text.
type NoticeLog struct {
	// Path is the log file; empty switches the log off.
	Path string
	// AttachCommand is the command line that attaches to the run the notice is about.
	AttachCommand string
	// Now returns the time stamped on a line; nil reads time.Now.
	Now func() time.Time
}

// appendNoticeLog appends one line, `<UTC time, RFC 3339>\t<notice text>\t<attach command>\n`, to log.Path in a single write, so concurrent batten processes never interleave inside a line.
// A failure to open or write is a Warn naming the path; it never blocks or changes the notice.
func appendNoticeLog(log NoticeLog, line string) {
	if log.Path == "" {
		return
	}
	now := time.Now
	if log.Now != nil {
		now = log.Now
	}
	entry := now().UTC().Format(time.RFC3339) + "\t" + line + "\t" + log.AttachCommand + "\n"

	file, err := os.OpenFile(log.Path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		logger.Warn("battenshed: open the notice log failed", "path", log.Path, "error", err)
		return
	}
	defer file.Close()
	if _, err := file.WriteString(entry); err != nil {
		logger.Warn("battenshed: write the notice log failed", "path", log.Path, "error", err)
	}
}

// RowNoticeDeps carries the seams a row's notices go through.
type RowNoticeDeps struct {
	// Notify hands one notice line to whoever tells the orch, and reports whether the line was queued.
	// A nil Notify switches the row's notices off.
	Notify func(ctx context.Context, line string) (queued bool, err error)
	// OrchStrandRecorded reports whether the orch the notices go to has a strand recorded to receive them.
	// A nil OrchStrandRecorded reads as one recorded.
	OrchStrandRecorded func() (bool, error)
	// BattenHistoryLen returns the length of batten's own status history, which tells one failure episode of a row from the next.
	BattenHistoryLen func() (int, error)
	// Sleep is the pause between two delivery attempts; a test replaces it to keep the spacing out of real time.
	// A nil Sleep resolves to waitOrCancel.
	Sleep func(ctx context.Context, d time.Duration)
	// NoticeLog receives every notice the row composes, once per composition.
	NoticeLog NoticeLog
}

// rowNotifier sends the notices of one row of the lifecycle recipe.
type rowNotifier struct {
	producer   string
	slug       string
	scratchDir string
	deps       RowNoticeDeps
}

// newRowNotifier returns the notifier of the row named producer for the task worktree slug, keeping its marker under scratchDir.
func newRowNotifier(producer, slug, scratchDir string, deps RowNoticeDeps) rowNotifier {
	if deps.Sleep == nil {
		deps.Sleep = waitOrCancel
	}
	return rowNotifier{producer: producer, slug: slug, scratchDir: scratchDir, deps: deps}
}

// markerPath returns the path of the row's marker.
func (n rowNotifier) markerPath() string {
	return filepath.Join(n.scratchDir, n.producer+rowNoticeFileSuffix)
}

// readMarker returns the marker's lines, none when it is absent or unreadable.
func (n rowNotifier) readMarker() []string {
	data, err := os.ReadFile(n.markerPath())
	if err != nil {
		if !os.IsNotExist(err) {
			logger.Warn("battenshed: read row notice marker failed", "producer", n.producer, "slug", n.slug, "error", err)
		}
		return nil
	}
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

// setMarkerLine writes line into the marker in place of any line sharing its prefix.
// A write failure is a Warn: a lost marker costs at most one repeated notice.
func (n rowNotifier) setMarkerLine(prefix, line string) {
	kept := []string{}
	for _, existing := range n.readMarker() {
		if existing != "" && !strings.HasPrefix(existing, prefix) {
			kept = append(kept, existing)
		}
	}
	kept = append(kept, line)
	if err := os.MkdirAll(n.scratchDir, 0o755); err != nil {
		logger.Warn("battenshed: create scratch directory for row notice marker failed", "producer", n.producer, "slug", n.slug, "scratchDir", n.scratchDir, "error", err)
		return
	}
	if err := os.WriteFile(n.markerPath(), []byte(strings.Join(kept, "\n")+"\n"), 0o644); err != nil {
		logger.Warn("battenshed: write row notice marker failed", "producer", n.producer, "slug", n.slug, "error", err)
	}
}

// failed queues one notice per failure episode of the row, naming the failed step and the way forward.
// An episode is told apart by batten's history length: a failure at the length the marker recorded is the same episode and sends nothing.
// A history that cannot be read reads as a new episode, since a repeated notice costs less than a lost one.
// A cancelled context sends nothing, and a notice never changes the row's outcome.
func (n rowNotifier) failed(ctx context.Context, step, reason string) {
	if n.deps.Notify == nil || ctx.Err() != nil {
		return
	}
	historyLen := -1
	if n.deps.BattenHistoryLen != nil {
		length, err := n.deps.BattenHistoryLen()
		if err != nil {
			logger.Warn("battenshed: batten history unreadable for the row notice; reading it as a new episode", "producer", n.producer, "slug", n.slug, "error", err)
		} else {
			historyLen = length
		}
	}
	marker := rowNoticeFailurePrefix + strconv.Itoa(historyLen)
	if historyLen >= 0 {
		for _, line := range n.readMarker() {
			if line == marker {
				return
			}
		}
	}
	line := fmt.Sprintf("%s %s: %s failed at %s: %s; way forward: fix the cause, then run \"lyx batten run %s\" from the prime",
		noticePrefix, n.slug, n.producer, step, oneLine(reason, noticeErrorMax), n.slug)
	if n.send(ctx, line) && historyLen >= 0 {
		n.setMarkerLine(rowNoticeFailurePrefix, marker)
	}
}

// send appends line to the notice log, then makes at most noticeMaxAttempts attempts, rowNoticeRetrySpacing apart, to queue it, and reports whether it was queued.
// With no orch strand recorded it does not call Notify and warns once.
// A line still unqueued after the attempts is dropped with a Warn.
func (n rowNotifier) send(ctx context.Context, line string) bool {
	appendNoticeLog(n.deps.NoticeLog, line)
	if n.deps.OrchStrandRecorded != nil {
		recorded, err := n.deps.OrchStrandRecorded()
		if err != nil {
			logger.Warn("battenshed: row notice held back; the orch state is unreadable", "producer", n.producer, "slug", n.slug, "error", err)
			return false
		}
		if !recorded {
			logger.Warn("battenshed: row notice held back; no orch strand is recorded to receive it", "producer", n.producer, "slug", n.slug)
			return false
		}
	}
	for attempt := 1; attempt <= noticeMaxAttempts; attempt++ {
		queued, err := n.deps.Notify(ctx, line)
		if err == nil && queued {
			return true
		}
		logger.Warn("battenshed: row notice not delivered", "producer", n.producer, "slug", n.slug, "queued", queued, "attempt", attempt, "error", err)
		if attempt == noticeMaxAttempts {
			break
		}
		n.deps.Sleep(ctx, rowNoticeRetrySpacing)
		if ctx.Err() != nil {
			return false
		}
	}
	logger.Warn("battenshed: row notice dropped", "producer", n.producer, "slug", n.slug, "attempts", noticeMaxAttempts)
	return false
}
