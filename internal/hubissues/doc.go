// Package hubissues reads, comments on and closes the GitHub issues of the repository a hub works on.
// It serves `lym board intake`.
//
// A Repo is bound to one repository.
// Open parses a remote URL into its owner and name and authenticates through githubclient.
// The hub's own repository comes from `fabricengine.HubRepoURL`, its recorded binding or else its origin remote.
// Each hub's intake therefore reads its own repository.
//
// The package never files an issue: filing LoomYard bugs belongs to selfreportengine, and intake never goes through it.
// Each request carries its own deadline, so a call that spans pages gives each page a fresh one.
// A failed call is logged and returned as an operator-facing error.
// The error tells an unresolvable token, an API rejection and an unreachable GitHub apart.
package hubissues
