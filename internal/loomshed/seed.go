// seed.go implements Seed, the production writer of loom's initial status file at _lym/loom/status.json.

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
// A caller that runs Seed on every invocation can match this one case with errors.Is.
// It can then treat the case as success, while every other Seed error still propagates.
var ErrSeedExists = errors.New("loomshed: status file already exists")

// Seed writes the initial status file at statusPath, guarded by statusLockPath.
// The file holds a shedengine.Status with an empty CurrentProducer, State running, and an empty non-nil History.
// PauseRequested is false.
// Product is a marshalled loomengine.Status with the given slug and parent and a nil StartSha.
//
// The empty CurrentProducer says only that the run starts at its start, which is the recipe's first row.
// One seed therefore serves every loom recipe and never names a row a recipe lacks.
//
// Seeding happens before any Shed exists, so Seed takes its two paths directly,
// and it depends on nothing that builds producers.
//
// Seed refuses when the file already exists and returns an error rather than overwriting it.
// Overwriting silently destroys an in-flight run's history, and the resume contract rests on that history;
// so a deliberate re-seed is an explicit operator act, never an accident.
// The refusal is reported as ErrSeedExists so that a caller re-running Seed can tolerate it.
func Seed(statusPath, statusLockPath, slug, parent string) error {
	// internal/state creates the status file's parent directory but not the lock file's,
	// so the lock file's parent is created here.
	if err := os.MkdirAll(filepath.Dir(statusLockPath), 0o755); err != nil {
		return fmt.Errorf("loomshed: create status lock parent dir: %w", err)
	}

	product, err := json.Marshal(loomengine.Status{Slug: slug, Parent: parent, StartSha: nil})
	if err != nil {
		return fmt.Errorf("loomshed: marshal seed product: %w", err)
	}

	// The refuse-if-exists decision is made under the held lock, via UpdateJSON's found argument.
	// It is never a stat followed by a write, which would leave a TOCTOU window between the check and the write.
	err = state.UpdateJSON(statusPath, statusLockPath, func(_ shedengine.Status, found bool) (shedengine.Status, error) {
		if found {
			return shedengine.Status{}, fmt.Errorf("%w: %q already exists; Seed refuses to overwrite an in-flight run's history", ErrSeedExists, statusPath)
		}
		return shedengine.Status{
			State:          shedengine.StateRunning,
			History:        []shedengine.HistoryEntry{},
			PauseRequested: false,
			Product:        product,
		}, nil
	})
	if err != nil && errors.Is(err, state.ErrDecode) {
		// A present but undecodable status file (malformed JSON) still counts as present.
		// Seed never overwrites an existing status file.
		// A corrupt one may be the only record of what an in-flight run was doing when it broke,
		// so it is refused like a cleanly decoding file.
		// Reporting it as ErrSeedExists defers the decode diagnosis to the spawned driver's own read.
		// Refusing at bootstrap instead would fail before any driver spawns.
		//
		// UpdateJSON aborts before its mutate on a decode failure, so no write happens and the file is untouched.
		return fmt.Errorf("%w: %q already exists but does not decode; Seed refuses to overwrite it, deferring the decode diagnosis to the driver's own preflight read", ErrSeedExists, statusPath)
	}
	return err
}
