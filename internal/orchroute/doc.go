// Package orchroute resolves which orch a worktree's agents reach.
// It provides the orch module's paths and the liveness of one worktree's orch.
// It also reads the strand name its orch state records and walks up a beam chain to the nearest live orch.
// It sits below internal/orchcli, which delegates to it.
// A package that orchcli imports, such as the creel CLI, resolves the same orch through it without importing orchcli.
//
// Asker applies one rule to every caller that needs the orch above a worktree.
// A post to the caller's parent and a hold notice both use it.
// From a pair, the walk starts at the pair's recorded parent.
// It ends at the nearest worktree whose orch is live, else the prime's.
// From a beam or the prime, the walk starts at that worktree itself.
package orchroute
