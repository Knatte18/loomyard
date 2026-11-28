// reinterpret.go applies each registered reinterpretation once to the board's base files.
// It mails the values each one rewrote or flagged to the hub's orch through the installed notifier.

package hubreconcile

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"sync/atomic"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/configreg"
	"github.com/Knatte18/loomyard/internal/configsync"
	"github.com/Knatte18/loomyard/internal/fsx"
	"github.com/Knatte18/loomyard/internal/logger"
)

// Notifier posts line to the orch of the hub whose Bolt checkout is boardDir and reports whether the mail was posted.
type Notifier func(boardDir, line string) (posted bool, err error)

var installedNotifier atomic.Pointer[Notifier]

// InstallNotifier stores n as the process's notifier, which a walk mails its reconcile notices through.
// It returns a function that reinstates the previous notifier.
func InstallNotifier(n Notifier) (restore func()) {
	previous := installedNotifier.Swap(&n)
	return func() { installedNotifier.Store(previous) }
}

// noticePrefix opens the mail a walk sends, so the orch tells it from operator input.
const noticePrefix = "[reconcile notice]"

// applyReinterpretations applies every registered reinterpretation whose board marker is absent.
// It applies each one to its module's hub base file at boardDir.
// It judges the file's values against the prime's root and anchored directory.
// It writes each marker after the file.
// An absent base file holds no value to carry across, so only its marker is written.
// It returns the board-relative paths it wrote and one line per value rewritten or flagged, naming the module's file.
func applyReinterpretations(boardDir, primeRoot, primeAnchor string) (written, lines []string, err error) {
	for _, module := range configreg.Modules() {
		for _, change := range module.Reinterpretations {
			marker := configengine.ReinterpretedMarker(boardDir, change.ID)
			if fileExists(marker) {
				continue
			}
			path := configengine.ConfigFile(boardDir, module.Name)
			fileError := func(err error) error { return &configsync.FileError{Module: module.Name, Path: path, Err: err} }
			existing, err := os.ReadFile(path)
			switch {
			case errors.Is(err, fs.ErrNotExist):
			case err != nil:
				return nil, nil, fileError(err)
			default:
				rewritten, rewrote, flagged, err := change.Rewrite(existing, primeRoot, primeAnchor)
				if err != nil {
					return nil, nil, fileError(fmt.Errorf("reinterpretation %s: %w", change.ID, err))
				}
				if !bytes.Equal(rewritten, existing) {
					if err := fsx.AtomicWriteBytes(path, rewritten); err != nil {
						return nil, nil, fileError(err)
					}
					written = append(written, configengine.ConfigFileRel(module.Name))
				}
				for _, line := range append(rewrote, flagged...) {
					lines = append(lines, module.Name+".yaml "+line)
				}
			}
			if err := fsx.AtomicWriteBytes(marker, nil); err != nil {
				return nil, nil, fmt.Errorf("write reinterpretation marker %s: %w", marker, err)
			}
			written = append(written, configengine.ReinterpretedMarkerRel(change.ID))
		}
	}
	return written, lines, nil
}

// notifyReinterpretations logs what the walk's reinterpretations rewrote or flagged.
// It mails that as one line to the hub's orch through the installed notifier.
// With no lines, it sends nothing; with no notifier installed, or a mail not posted, the log holds the line alone.
func notifyReinterpretations(boardDir, label string, lines []string) {
	if len(lines) == 0 {
		return
	}
	line := fmt.Sprintf("%s lym build %s changed what a value in the hub's config means, and the reconcile carried the hub's files across: %s", noticePrefix, label, strings.Join(lines, "; "))
	logger.Warn("hubreconcile: a reinterpretation rewrote or flagged a hub config value", "board", boardDir, "lines", lines)
	mailOrch(boardDir, line)
}

// mailOrch mails line to the orch of the hub whose Bolt checkout is boardDir through the installed notifier;
// with no notifier installed, a failed mail or no orch recorded, the log holds the line alone.
func mailOrch(boardDir, line string) {
	notifier := installedNotifier.Load()
	if notifier == nil {
		logger.Warn("hubreconcile: no notifier is installed, so the orch is not mailed", "line", line)
		return
	}
	posted, err := (*notifier)(boardDir, line)
	switch {
	case err != nil:
		logger.Warn("hubreconcile: mailing the orch failed", "line", line, "error", err)
	case !posted:
		logger.Warn("hubreconcile: the hub records no orch to mail", "line", line)
	}
}
