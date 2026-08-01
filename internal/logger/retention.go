// retention.go implements the durable trace-file retention sweep: a
// standalone function over a directory path that enforces the age and count
// bounds discussion.md's `retention` decision describes. It is deliberately
// independent of the durable sink (batch 4), hubgeometry (batch 1), and
// trace identity (batch 2) — Sweep takes a directory path from its caller
// and needs only internal/proc's liveness probe plus the standard library.

package logger

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"time"

	"github.com/Knatte18/loomyard/internal/proc"
)

// retentionAgeBound is the maximum age, read from a candidate file's
// filename timestamp segment, before the sweep deletes it. 14 days is what
// discussion.md's `retention` decision fixes as "what an operator reasons
// in" (last week vs. older).
const retentionAgeBound = 14 * 24 * time.Hour

// retentionCountBound is the number of newest-by-filename-timestamp
// candidates the sweep keeps after the age pass, regardless of how many
// remain within the age bound. Protects a worktree having a "bad week" of
// many short-lived processes.
const retentionCountBound = 50

// traceFileTimestampLayout is the time.Parse layout matching the
// `<YYYYMMDDTHHMMSSZ>` segment of the trace-file grammar
// (trace-<timestamp>-<16-hex-id>-<pid>.log).
const traceFileTimestampLayout = "20060102T150405Z"

// traceFilePattern matches the trace-file grammar discussion.md's
// `one-file-per-process` decision fixes:
// trace-<YYYYMMDDTHHMMSSZ>-<16-hex-id>-<pid>.log. Capture groups extract the
// timestamp segment (age bound) and the pid segment (liveness rule). A
// filename that does not match this pattern is never a sweep candidate — it
// is neither deleted nor counted toward the count bound, per the
// `retention` decision's sweep-scope rule.
var traceFilePattern = regexp.MustCompile(`^trace-(\d{8}T\d{6}Z)-[0-9a-f]{16}-(\d+)\.log$`)

// retentionCandidate is one trace-file sweep candidate: its full path and
// the filename timestamp the age and count bounds both rank by (never
// mtime — see the `retention` decision's rationale for why mtime would let
// a long-running process's file never age out).
type retentionCandidate struct {
	path      string
	timestamp time.Time
}

// Sweep enforces the trace-file directory's retention policy: files older
// than retentionAgeBound are deleted, then only the newest
// retentionCountBound (by filename timestamp) of what remains are kept.
// Only filenames matching traceFilePattern are ever considered; any other
// file in dir is left untouched. A per-file delete failure is skipped, not
// propagated, so a locked file (Windows) or a sibling process racing this
// sweep never fails the call. An empty or absent directory is a no-op.
//
// Liveness rule: a candidate whose filename pid segment belongs to a
// currently-running process (per internal/proc.IsAlive), or to this process
// itself, is never deleted by either bound and is excluded from the count
// bound's ranking entirely — see the `retention` decision's live-skip
// rationale for why unlinking a live sibling's open file is unsafe, and why
// a live file must not consume one of the newest-50 slots.
func Sweep(dir string) error {
	// A missing directory is nothing to sweep, not an error: the sink that
	// calls Sweep on open may run before the directory has ever been
	// created, and a caller with nothing written yet has nothing to retain.
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var candidates []retentionCandidate
	now := time.Now()
	selfPID := os.Getpid()
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		match := traceFilePattern.FindStringSubmatch(entry.Name())
		if match == nil {
			// Out-of-grammar files (e.g. a different tool's log dropped in
			// the same directory) are never candidates for either bound.
			continue
		}
		ts, err := time.Parse(traceFileTimestampLayout, match[1])
		if err != nil {
			// A filename that matched the regex's digit shape but is not a
			// valid calendar timestamp (e.g. month 13) is not a real
			// trace file; treat it the same as a grammar mismatch.
			continue
		}
		pid, err := strconv.Atoi(match[2])
		if err != nil {
			// The grammar's \d+ group always parses as an int; this branch
			// exists only to satisfy strconv's error return and is
			// unreachable in practice.
			continue
		}
		if pid == selfPID || proc.IsAlive(pid) {
			// Liveness rule: a live process's file (including this one's
			// own) is unconditionally kept and never enters the count
			// bound's candidate set, so it cannot consume one of the
			// newest-50 slots.
			continue
		}

		path := filepath.Join(dir, entry.Name())
		if now.Sub(ts) > retentionAgeBound {
			// Age bound: delete unconditionally now rather than deferring to
			// the count pass below, since an over-age file must go
			// regardless of how many other candidates exist.
			_ = os.Remove(path)
			continue
		}
		candidates = append(candidates, retentionCandidate{path: path, timestamp: ts})
	}

	// Count bound: rank the age-surviving candidates newest-first by
	// filename timestamp and delete everything beyond the newest
	// retentionCountBound.
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].timestamp.After(candidates[j].timestamp)
	})
	for _, candidate := range candidates[minInt(retentionCountBound, len(candidates)):] {
		_ = os.Remove(candidate.path)
	}
	return nil
}

// minInt returns the smaller of a and b. A tiny local helper rather than a
// stdlib import: Go's builtin min was added in 1.21, but this keeps the
// sweep readable without depending on the toolchain version in use.
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
