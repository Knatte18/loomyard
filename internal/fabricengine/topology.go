// topology.go holds Topology, the entry point for the hub-scoped worktree-topology verbs.

package fabricengine

import "time"

// Topology is the high-level facade over hub-scoped worktree-topology operations.
// It holds the configuration needed by all topology methods.
// A pair does not exist until a topology method creates it, so Topology cannot hold a per-pair handle.
type Topology struct {
	cfg Config
	// leftover is the seam Remove uses to finish a directory git deregistered but left; its zero value is production.
	leftover leftoverSeam
	// inFlight answers which other pairs' sessions are live when a pair is removed; nil answers none.
	inFlight InFlightProbe
	// gcAutoThreshold is the loose-object estimate above which a removal's foreground gc packs a store.
	gcAutoThreshold int
	// anchor stands in for the failures of an anchor change's weft move; its zero value is production.
	anchor anchorSeams
	// teardownWait bounds how long a mutation of a pair waits for the pair's teardown lock.
	teardownWait time.Duration
	// archiveSleep sleeps between the attempts of an archive-tag push; nil sleeps for real.
	archiveSleep func(time.Duration)
	// beforeWire runs in Add once the weft worktree exists and before the junctions are wired; nil in production.
	beforeWire func()
	// cleanupAfterEnumerate runs in Cleanup once per orphan right before the slug's teardown lock is taken; nil in production.
	cleanupAfterEnumerate func()
}

// SetInFlightProbe sets the probe that a removal's store housekeeping consults before it runs gc.
func (t *Topology) SetInFlightProbe(probe InFlightProbe) {
	t.inFlight = probe
}

// NewTopology returns a Topology operating with the given config.
func NewTopology(cfg Config) *Topology {
	return &Topology{
		cfg:             cfg,
		teardownWait:    StatusPushLockWait,
		gcAutoThreshold: defaultGCAutoThreshold,
	}
}
