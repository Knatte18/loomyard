// seed.go implements Seed, the production writer for loom's initial status file -- nothing else in
// production writes _lyx/loom/status.json today.

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
	err = state.UpdateJSON(statusPath, statusLockPath, func(_ shedengine.Status, found bool) (shedengine.Status, error) {
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
	if err != nil && errors.Is(err, state.ErrDecode) {
		// A present-but-undecodable status file (malformed JSON, an unknown top-level field) is
		// still PRESENT: Seed's whole contract is never to overwrite an existing status file, and a
		// corrupt one is exactly what must not be destroyed -- it may be the only forensic record of
		// what an in-flight run was doing when it broke. So this is the same refusal as a
		// cleanly-decoding file, reported as ErrSeedExists for the session bootstrap to tolerate.
		// UpdateJSON aborts before its mutate on a decode failure, so no write happened and the file
		// is left untouched.
		//
		// This defers the decode diagnosis to the spawned driver's own Shed.Run step-1 read gate,
		// exactly as the unknown-field shape already did (UpdateJSON's lenient read tolerates an
		// unknown field, so that shape decodes here and takes the found branch above). Without this
		// mapping the malformed-JSON shape made `lyx loom run` refuse on the envelope before ever
		// spawning a driver -- the very state manifest/designs/loom.md's crash-recovery section
		// promises a poisoned status file never presents as ("A poisoned status file must never look
		// like it belongs to bootstrap's own gate"), reproduced live in crucible round
		// fable5-high-r5 (F3). `lyx loom drive` was already correct, since it never calls Seed.
		return fmt.Errorf("%w: %q already exists but does not decode; Seed refuses to overwrite it, deferring the decode diagnosis to the driver's own preflight read", ErrSeedExists, statusPath)
	}
	return err
}
