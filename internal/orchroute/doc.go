// Package orchroute resolves which orch a worktree's agents reach: the orch module's paths, the liveness of one worktree's orch, the strand name its orch state records, and the walk up a beam chain to the nearest live orch.
// It sits below internal/orchcli, which delegates to it, so a package orchcli imports, such as the creel CLI, resolves the same orch without importing orchcli.
//
// Asker is the one rule both a post to the caller's parent and a hold notice share.
// From a pair, the walk starts at the pair's recorded parent and ends at the nearest worktree whose orch is live, else the prime's.
// From a beam or the prime, the walk starts at that worktree itself.
package orchroute
