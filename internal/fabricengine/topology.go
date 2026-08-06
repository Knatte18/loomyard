// topology.go — the Topology holder: the entry point for fabric's hub-scoped worktree-topology verbs (add/remove/checkout/reconcile/status/prune/cleanup/ list, this batch and the next).
// It is a config-carrying holder whose methods take a *lyxcwd.Location, keeping the topology surface mechanical and uniform across every verb.
//
// Topology is deliberately distinct from Fabric (fabric.go): Fabric is the per-pair cross-repo handle over two already-existing checkouts (Warp/Weft gitrepo.Repo fields, used by weft-git verbs like Commit);
// Topology is the hub-scoped holder that creates, removes, and reconciles the pairs themselves.
// A pair does not exist yet when Topology.Add runs, so Topology cannot hold a *Fabric — it only holds the Config needed to derive paths via lyxcwd and branch names via WeftBranchName (backed by internal/weftname).

package fabricengine

// Topology is the high-level facade over hub-scoped worktree-topology operations.
// It holds the configuration needed by all topology methods.
type Topology struct {
	cfg Config
}

// NewTopology returns a Topology operating with the given config.
func NewTopology(cfg Config) *Topology {
	return &Topology{
		cfg: cfg,
	}
}
