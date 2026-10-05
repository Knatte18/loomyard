// notice.go — the orch notice queue: one file per one-line notice another module queues, delivered into the orch session only by the orch watcher.
//
// The queue lives under Paths.NoticesDir, told by the caller (Told-Geometry Invariant).
// Names sort by arrival, so listing the directory oldest first is a lexical sort, and concurrent writers never share a file.

package orchengine

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Knatte18/loomyard/internal/logger"
)

const (
	// maxNotices caps the queue; concurrent appenders may overshoot it transiently by at most one notice each.
	maxNotices = 50

	noticeSuffix = ".notice"
	noticeTmpTag = ".tmp-"
)

// noticeName returns the file name for a notice queued at now: UTC time to the nanosecond, then a random suffix.
func noticeName(now time.Time) (string, error) {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("orch: notice name: %w", err)
	}
	return now.UTC().Format("20060102T150405.000000000Z") + "-" + hex.EncodeToString(b[:]) + noticeSuffix, nil
}

// QueueNotice queues line for delivery into the orch session, reporting whether it was queued.
// A notice is typed as one line, so a line containing a newline or beginning with "/" is refused with an error and nothing is written.
// With no strand recorded in the orch state the notice is logged and not queued.
// After writing, the oldest notices beyond the cap are removed and each drop is logged.
func QueueNotice(p Paths, line string, now time.Time) (queued bool, err error) {
	if strings.ContainsAny(line, "\r\n") {
		return false, errors.New("orch: a notice is one line; it must not contain a newline")
	}
	if strings.HasPrefix(line, "/") {
		return false, errors.New("orch: a notice must not begin with \"/\", which the session would read as a command")
	}
	st, err := LoadState(p)
	if err != nil {
		return false, err
	}
	if st.Strand == "" {
		logger.Info("orch: no orch strand recorded; notice logged and not queued", "notice", line)
		return false, nil
	}
	if err := os.MkdirAll(p.NoticesDir, 0o755); err != nil {
		return false, fmt.Errorf("orch: create notices dir: %w", err)
	}
	name, err := noticeName(now)
	if err != nil {
		return false, err
	}
	tmp, err := os.CreateTemp(p.NoticesDir, noticeTmpTag+"*")
	if err != nil {
		return false, fmt.Errorf("orch: create notice: %w", err)
	}
	_, werr := tmp.WriteString(line)
	cerr := tmp.Close()
	if err := errors.Join(werr, cerr); err != nil {
		os.Remove(tmp.Name())
		return false, fmt.Errorf("orch: write notice: %w", err)
	}
	if err := os.Rename(tmp.Name(), filepath.Join(p.NoticesDir, name)); err != nil {
		os.Remove(tmp.Name())
		return false, fmt.Errorf("orch: queue notice: %w", err)
	}
	if err := enforceNoticeCap(p); err != nil {
		return true, err
	}
	return true, nil
}

// Notice is one queued notice.
type Notice struct {
	Path string // Absolute path of the notice file.
	Line string // The notice text.
}

// ListNotices returns the queued notices, oldest first; an absent queue is empty.
// A file that disappears between the listing and the read, as another delivery removes it, is skipped.
func ListNotices(p Paths) ([]Notice, error) {
	names, err := noticeNames(p)
	if err != nil {
		return nil, err
	}
	out := make([]Notice, 0, len(names))
	for _, name := range names {
		path := filepath.Join(p.NoticesDir, name)
		data, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("orch: read notice: %w", err)
		}
		out = append(out, Notice{Path: path, Line: string(data)})
	}
	return out, nil
}

// noticeNames returns the queued notice file names, oldest first.
func noticeNames(p Paths) ([]string, error) {
	entries, err := os.ReadDir(p.NoticesDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("orch: list notices: %w", err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), noticeSuffix) {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

// RemoveNotice removes one delivered notice; an already absent one is not an error.
func RemoveNotice(n Notice) error {
	if err := os.Remove(n.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("orch: remove notice: %w", err)
	}
	return nil
}

// DropNotices removes the whole queue; an absent queue is not an error.
func DropNotices(p Paths) error {
	if err := os.RemoveAll(p.NoticesDir); err != nil {
		return fmt.Errorf("orch: drop notices: %w", err)
	}
	return nil
}

// enforceNoticeCap removes the oldest notices beyond maxNotices, logging each drop.
func enforceNoticeCap(p Paths) error {
	names, err := noticeNames(p)
	if err != nil {
		return err
	}
	for len(names) > maxNotices {
		path := filepath.Join(p.NoticesDir, names[0])
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("orch: drop notice over the cap: %w", err)
		}
		logger.Warn("orch: notice queue over its cap; dropped the oldest", "notice", names[0], "cap", maxNotices)
		names = names[1:]
	}
	return nil
}
