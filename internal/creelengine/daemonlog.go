// daemonlog.go declares the daemon log's line format and the excerpt a client appends after giving up on connecting.

package creelengine

import (
	"fmt"
	"os"
	"strings"
	"time"
)

const (
	// daemonStartHeaderPrefix opens the start header line of every daemon.
	daemonStartHeaderPrefix = "creel daemon start pid="
	// daemonLogPidPrefix opens every line the daemon writes through its logger.
	daemonLogPidPrefix = "pid="
	// maxExcerptLines bounds the lines one excerpt quotes, keeping the last.
	maxExcerptLines = 20
)

// DaemonStartHeader returns the start header line of the daemon with pid started at the given time, without a newline.
func DaemonStartHeader(pid int, at time.Time) string {
	return fmt.Sprintf("%s%d at=%s", daemonStartHeaderPrefix, pid, at.UTC().Format(time.RFC3339))
}

// DaemonLogPrefix returns the prefix that opens every line the daemon with pid writes through its logger.
func DaemonLogPrefix(pid int) string {
	return fmt.Sprintf("%s%d ", daemonLogPidPrefix, pid)
}

// daemonLogExcerpt describes what the daemon log at path holds from the start of the daemon with pid.
// It is the excerpt a client appends to its error after giving up on connecting.
// It first opens the file for append without creating it, so a missing log is reported, not created.
// It says the log could not be opened when the open, the close, or the read fails.
// It quotes the lines that open with the daemon's pid prefix.
// While no other start header follows the daemon's own, it also quotes the lines after its header with no pid prefix.
// When another start header follows, it says the file also holds lines it cannot attribute.
func daemonLogExcerpt(path string, pid int) string {
	probe, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		return fmt.Sprintf("the daemon log could not be opened: %v", err)
	}
	if err := probe.Close(); err != nil {
		return fmt.Sprintf("the daemon log could not be opened: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Sprintf("the daemon log could not be opened: %v", err)
	}

	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	ownHeader := fmt.Sprintf("%s%d ", daemonStartHeaderPrefix, pid)
	headerAt := -1
	foreignHeaderAfter := false
	for index, line := range lines {
		switch {
		case strings.HasPrefix(line, ownHeader):
			headerAt, foreignHeaderAfter = index, false
		case strings.HasPrefix(line, daemonStartHeaderPrefix):
			foreignHeaderAfter = headerAt >= 0
		}
	}

	prefix := DaemonLogPrefix(pid)
	var quoted []string
	for index, line := range lines {
		tagged := strings.HasPrefix(line, prefix)
		followsHeader := headerAt >= 0 && index >= headerAt && !foreignHeaderAfter
		anonymous := line != "" && !strings.HasPrefix(line, daemonLogPidPrefix) && !strings.HasPrefix(line, daemonStartHeaderPrefix)
		if tagged || (followsHeader && (index == headerAt || anonymous)) {
			quoted = append(quoted, line)
		}
	}
	if len(quoted) == 0 {
		return "the daemon log holds no line from that start"
	}
	if len(quoted) > maxExcerptLines {
		quoted = quoted[len(quoted)-maxExcerptLines:]
	}
	excerpt := "the daemon log holds, from that start:\n  " + strings.Join(quoted, "\n  ")
	if foreignHeaderAfter {
		excerpt += "\nthe file also holds lines it cannot attribute to that start"
	}
	return excerpt
}
