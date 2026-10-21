// Package hubissues reads, comments on and closes the GitHub issues of the repository a hub works on, for `lyx board intake`.
//
// A Repo is bound to one repository: Open parses a remote URL into its owner and name and authenticates through githubclient.
// The hub's own repository comes from `fabricengine.HubRepoURL`, its warp binding or else its warp origin, so the loomyard hub's intake reads `Knatte18/loomyard` and any other hub's intake reads its own repository.
//
// The package never files an issue: filing LoomYard bugs is selfreport's alone, and intake never goes through selfreport.
// Each request carries its own deadline, so a call that spans pages gives each page a fresh one.
// A failed call is logged and returned as an operator-facing error that tells an unresolvable token, an API rejection and an unreachable GitHub apart.
package hubissues
