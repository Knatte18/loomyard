// topology.go — the Topology holder: the entry point for fabric's hub-scoped worktree-topology
// verbs (add/remove/checkout/reconcile/status/prune/cleanup/ list, this batch and the next).
// It is a config-carrying holder whose methods take a *lymcwd.Location, keeping the topology
// surface mechanical and uniform across every verb.
//
// Topology is deliberately distinct from Fabric (fabric.go): Fabric is the per-pair cross-repo
// handle over two already-existing checkouts (Warp/Weft gitrepo.Repo fields, used by weft-git verbs
// like Commit);
// Topology is the hub-scoped holder that creates, removes, and reconciles the pairs themselves.
// A pair does not exist yet when Topology.Add runs, so Topology cannot hold a *Fabric — it only
// holds the Config needed to derive paths via lymcwd and branch names via RecordsBranchName (backed by
// internal/weftname).

package fabricengine

import "time"

// Topology is the high-level facade over hub-scoped worktree-topology operations.
// It holds the configuration needed by all topology methods.
type Topology struct {
	cfg Config
	// leftover is the seam behind Remove's finishing of a directory git deregistered but left; its zero value is production.
	leftover leftoverSeam
	// inFlight answers which other pairs' sessions are live when a pair is removed; nil answers none.
	inFlight InFlightProbe
	// teardownWait bounds how long Remove waits for a push of the pair to let go of the pair's teardown lock.
	teardownWait time.Duration
	// beforeWire runs in Add once the weft worktree exists and before the junctions are wired; nil in production.
	beforeWire func()
}

// SetInFlightProbe sets the probe Remove's store housekeeping consults before it runs gc.
func (t *Topology) SetInFlightProbe(probe InFlightProbe) {
	t.inFlight = probe
}

// NewTopology returns a Topology operating with the given config.
func NewTopology(cfg Config) *Topology {
	return &Topology{
		cfg:          cfg,
		teardownWait: StatusPushLockWait,
	}
}
