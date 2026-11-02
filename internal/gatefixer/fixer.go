// fixer.go declares the spawn-and-wait seam a caller drives one cold fixer through, and its production implementation over a strand.

package gatefixer

import "time"

// Fixer is the seam a caller spawns, waits on and stops one cold fixer through, so a test drives the caller over a scripted fixer.
type Fixer interface {
	// SpawnOrAttach attaches to the live record store holds, and otherwise spawns a strand under spec and records it in store.
	// It reports whether this call spawned the fixer.
	SpawnOrAttach(store Store, spec Spec) (*Record, bool, error)
	// Await blocks up to wait for rec's fixer to reach a terminal classification, bounding its run by timeout.
	Await(rec *Record, timeout, wait time.Duration) (*Result, error)
	// Stop ends rec's strand, and is a no-op for one that is already gone.
	Stop(rec *Record) error
}

// strandFixer is the production Fixer over reed strands.
type strandFixer struct {
	deps Deps
}

// NewStrandFixer returns the Fixer that spawns, waits on and stops real fixer strands through deps.
func NewStrandFixer(deps Deps) Fixer {
	return strandFixer{deps: deps}
}

func (f strandFixer) SpawnOrAttach(store Store, spec Spec) (*Record, bool, error) {
	return SpawnOrAttach(f.deps, store, spec)
}

func (f strandFixer) Await(rec *Record, timeout, wait time.Duration) (*Result, error) {
	return Await(f.deps, rec, timeout, wait)
}

func (f strandFixer) Stop(rec *Record) error {
	return f.deps.Stopper.StopStrand(rec.StrandGUID)
}
