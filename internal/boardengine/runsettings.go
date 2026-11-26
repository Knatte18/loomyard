// runsettings.go rewrites the run_settings of every board entry in one board write.

package boardengine

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/configengine"
	"github.com/Knatte18/loomyard/internal/fsx"
)

// RewriteRunSettings hands the run_settings of every entry of the hub board at boardDir to rewrite, in one board write.
// It opens the board from the board config under boardDir.
// rewrite returns an entry's new settings and one line per change, and an entry with no line is left as it is.
// With no line from any entry, nothing is saved, rendered or synced.
// afterSave, when non-nil, runs under the write lock after the save and before the render.
// A failure after the save puts board.json back as it was, so a later call finds the same settings.
// It returns every line, each prefixed with its entry's slug.
func RewriteRunSettings(boardDir string, rewrite func(slug string, settings map[string]any) (map[string]any, []string, error), afterSave func() error) ([]string, error) {
	cfg, err := LoadConfig(configengine.Dirs{Board: boardDir}, "board")
	if err != nil {
		return nil, err
	}
	cfg.Path = boardDir
	board := New(ApplySkipEnv(cfg))

	path := filepath.Join(boardDir, boardFile)
	var before []byte
	var lines []string
	_, err = board.boardCriticalSectionRollingBack(func(store *Store) (any, error) {
		data, err := os.ReadFile(path)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		before = data
		for i := range store.tasks {
			entry := &store.tasks[i]
			if len(entry.RunSettings) == 0 {
				continue
			}
			settings, changes, err := rewrite(entry.Slug, entry.RunSettings)
			if err != nil {
				return nil, fmt.Errorf("run_settings of entry %q: %w", entry.Slug, err)
			}
			if len(changes) == 0 {
				continue
			}
			entry.RunSettings = settings
			for _, change := range changes {
				lines = append(lines, entry.Slug+" "+change)
			}
		}
		if len(lines) == 0 {
			return noWrite{}, nil
		}
		return nil, nil
	}, afterSave, func() error { return fsx.AtomicWriteBytes(path, before) })
	if err != nil {
		return nil, err
	}
	return lines, nil
}
