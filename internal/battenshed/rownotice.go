// rownotice.go implements the notice the driver sends the orch when it hands back a stop at a row after Run-Shed.
// It also owns the per-run markers, which batten clears where it seeds a fresh status.

package battenshed

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
)

// rowNoticeFileSuffix is the fixed suffix of the marker recording the notices a row has already sent.
// The marker's file name is the row's name followed by this suffix.
const rowNoticeFileSuffix = "-row-notice"

// rowNoticeRetrySpacing is the pause between two delivery attempts of one row notice.
const rowNoticeRetrySpacing = 30 * time.Second

// rowNoticeHandBackPrefix leads the marker line that records the history length of the last stop handed back.
const rowNoticeHandBackPrefix = "handback "

// runMarkerSuffixes lists the file-name suffixes of every marker that must last exactly one batten run.
// Batten removes these markers where it seeds a fresh status.
var runMarkerSuffixes = []string{
	rowNoticeFileSuffix,
	innerRunMemoryFileSuffix,
}

// ClearRunMarkers removes the run markers from scratchDir, so a fresh batten run starts without them.
// An absent scratch directory is not an error.
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

// RowNoticeDeps carries the functions a row's notices use to reach the orch, read batten's history and pause.
type RowNoticeDeps struct {
	// Notify posts one notice line for the orch, and reports whether the line was posted.
	// A nil Notify switches the row's notices off.
	Notify func(ctx context.Context, line string) (queued bool, err error)
	// OrchStrandRecorded reports whether the orch the notices go to has a strand recorded to receive them.
	// A nil OrchStrandRecorded reads as one recorded.
	OrchStrandRecorded func() (bool, error)
	// BattenHistoryLen returns the length of batten's own status history.
	// The length tells one failure episode of a row from the next.
	BattenHistoryLen func() (int, error)
	// Sleep is the pause between two delivery attempts; a test replaces it to keep the spacing out of real time.
	// A nil Sleep resolves to waitOrCancel.
	Sleep func(ctx context.Context, d time.Duration)
}

// rowNotifier sends the notices of one row of the lifecycle recipe.
type rowNotifier struct {
	producer   string
	slug       string
	scratchDir string
	deps       RowNoticeDeps
}

// newRowNotifier returns the notifier of the row named producer in the task whose worktree slug is slug.
// Its marker is kept under scratchDir.
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

// RowStopNotice sends the notice of a stop at the row producer after Run-Shed, and reports whether it was posted.
// The stop was handed back with the report at reportPath, and historyLen is the history length of its stop key.
// The line names the row, the reason cut to one line, the report and the way forward.
// A stop is noticed once: a second notice for the same historyLen sends nothing.
func RowStopNotice(ctx context.Context, deps RowNoticeDeps, slug, scratchDir, producer, reason, reportPath string, historyLen int) bool {
	line := fmt.Sprintf("%s %s: %s failed: %s; report %s; way forward: fix the cause, then run \"lym batten run %s\"",
		noticePrefix, slug, producer, oneLine(reason, noticeErrorMax), reportPath, slug)
	return HandBackNotice(ctx, deps, slug, scratchDir, producer, historyLen, line)
}

// HandBackNotice sends line for the stop at the batten row producer whose stop key's history length is historyLen.
// It reports whether the line was posted.
// It sends nothing for a stop already handed back: the marker records `handback <historyLen>`.
// The line is retried on failure, and a posted line records `handback <historyLen>` in the marker.
// A nil deps.Notify or a cancelled context sends nothing.
func HandBackNotice(ctx context.Context, deps RowNoticeDeps, slug, scratchDir, producer string, historyLen int, line string) bool {
	if deps.Notify == nil || ctx.Err() != nil {
		return false
	}
	notifier := newRowNotifier(producer, slug, scratchDir, deps)
	handBackMarker := rowNoticeHandBackPrefix + strconv.Itoa(historyLen)
	if slices.Contains(notifier.readMarker(), handBackMarker) {
		return false
	}
	if !notifier.send(ctx, line) {
		return false
	}
	notifier.setMarkerLine(rowNoticeHandBackPrefix, handBackMarker)
	return true
}

// send posts line and reports whether it was posted.
// With no orch strand recorded, or an unreadable orch state, it does not call Notify and warns once.
func (n rowNotifier) send(ctx context.Context, line string) bool {
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
	return n.deliver(ctx, line, n.deps.Notify)
}

// deliver posts line through notify and reports whether it was posted.
// It makes at most noticeMaxAttempts attempts, rowNoticeRetrySpacing apart.
// A line still unposted after the attempts is dropped with a Warn.
func (n rowNotifier) deliver(ctx context.Context, line string, notify func(ctx context.Context, line string) (bool, error)) bool {
	for attempt := 1; attempt <= noticeMaxAttempts; attempt++ {
		queued, err := notify(ctx, line)
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
