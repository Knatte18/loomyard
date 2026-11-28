// Package orchroute resolves which orch a worktree's agents reach.
// It provides the orch module's paths and the liveness of one worktree's orch.
// It also reads the strand name its orch state records and walks up a beam chain to the orch a notice goes to.
// It sits below internal/orchcli, which delegates to it.
// A package that orchcli imports, such as the creel CLI, resolves the same orch through it without importing orchcli.
//
// Asker applies one rule to every caller that needs the orch above a worktree.
// A post to the caller's parent and a hold notice both use it.
// From a pair, the walk starts at the pair's recorded parent.
// From a beam or the prime, the walk starts at that worktree itself.
//
// Destination is the walk's one rule.
// The worktree it starts at takes the notice while reed still holds the strand its orch state records, live or not.
// So a member's mail stays with its beam's orch when the liveness probe reads that orch as down.
// Creel delivers it once the orch is live.
// A start whose orch never started or was stopped passes the notice to the nearest live orch above it, else the prime's.
package orchroute
