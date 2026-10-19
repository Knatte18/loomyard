// portals.go declares the name of the retired `_portals` hub container.
// No verb creates or removes a portal link any more; the name stays reserved and `lyx fabric reconcile` sweeps what a hub still holds.

package fabricengine

// portalsDirName is the directory name of the retired hub-level portals container (<hub>/_portals).
// It stays declared here, and reserved by HubReservedNames, so no pair can take the name of a link a hub may still hold.
// Declared here rather than in internal/lyxcwd: it is not a resolution primitive lyxcwd needs to expose.
const portalsDirName = "_portals"
