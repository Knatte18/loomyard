// seed.go implements Seed, the production writer for loom's initial status file -- nothing else in
// production writes .lyx/loom/status.json today.

package loomshed

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Knatte18/loomyard/internal/loomengine"
	"github.com/Knatte18/loomyard/internal/shedengine"
	"github.com/Knatte18/loomyard/internal/state"
)

// ErrSeedExists marks Seed's refusal to overwrite an existing status file.
// A caller that is deliberately re-entrant -- the session bootstrap, which calls Seed
// unconditionally on every invocation -- can recognise exactly that one case with errors.Is and
// treat it as success, while every other Seed error still propagates.
var ErrSeedExists = errors.New("loomshed: status file already exists")

// Seed writes the initial status file at statusPath, guarded by statusLockPath: a shedengine.Status
// with CurrentProducer naming Preflight, State running, an empty non-nil History, PauseRequested
// false, and a Product carrying a marshalled loomengine.Status with the given slug and parent and a
// nil StartSha.
//
// Seeding happens before any Shed exists, so Seed takes the two told paths directly and couples to
// nothing that builds producers.
//
// Seed refuses when the file already exists, returning an error rather than overwriting, because
// overwriting silently destroys an in-flight run's history and the whole resume contract rests on
// that history; a deliberate re-seed is then an explicit operator act, never an accident.
// The refusal is reported as ErrSeedExists, for the session bootstrap's re-entrant Seed call to
// recognise and tolerate.
func Seed(statusPath, statusLockPath, slug, parent string) error {
	// internal/state creates the *status* file's parent but not the *lock* file's -- the same gap
	// internal/loomengine's own check-4 read already works around at its lock acquisition.
	if err := os.MkdirAll(filepath.Dir(statusLockPath), 0o755); err != nil {
		return fmt.Errorf("loomshed: create status lock parent dir: %w", err)
	}

	product, err := json.Marshal(loomengine.Status{Slug: slug, Parent: parent, StartSha: nil})
	if err != nil {
		return fmt.Errorf("loomshed: marshal seed product: %w", err)
	}

	// The refuse-if-exists decision is made under the held lock, via UpdateJSON's own found
	// argument -- never as a stat followed by a write, which would leave a TOCTOU window between
	// the check and the write.
	return state.UpdateJSON(statusPath, statusLockPath, func(_ shedengine.Status, found bool) (shedengine.Status, error) {
		if found {
			return shedengine.Status{}, fmt.Errorf("%w: %q already exists; Seed refuses to overwrite an in-flight run's history", ErrSeedExists, statusPath)
		}
		return shedengine.Status{
			CurrentProducer: NamePreflight,
			State:           shedengine.StateRunning,
			History:         []shedengine.HistoryEntry{},
			PauseRequested:  false,
			Product:         product,
		}, nil
	})
}
