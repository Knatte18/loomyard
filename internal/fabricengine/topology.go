// topology.go — the Topology holder: the entry point for fabric's hub-scoped
// worktree-topology verbs (add/remove/checkout/reconcile/status/prune/cleanup/
// list, this batch and the next). It mirrors warpengine.Worktree's shape
// exactly (a config-carrying holder, methods take a *hubgeometry.Layout) so
// the differential mapping between warp's operations and fabric's is
// one-to-one and the parity tests stay mechanical.
//
// Topology is deliberately distinct from Fabric (fabric.go): Fabric is the
// per-pair cross-repo handle over two already-existing checkouts (Warp/Weft
// gitrepo.Repo fields, used by weft-git verbs like SyncWeft); Topology is the
// hub-scoped holder that creates, removes, and reconciles the pairs
// themselves. A pair does not exist yet when Topology.Add runs, so Topology
// cannot hold a *Fabric — it only holds the Config needed to derive paths and
// branch names via hubgeometry and WeftBranchName.

package fabricengine

// Topology is the high-level facade over hub-scoped worktree-topology
// operations (add/remove/checkout/reconcile/status/prune/cleanup/list). It
// holds the configuration needed by all topology methods, mirroring
// warpengine.Worktree.
type Topology struct {
	cfg Config
}

// NewTopology returns a Topology operating with the given config.
func NewTopology(cfg Config) *Topology {
	return &Topology{
		cfg: cfg,
	}
}
