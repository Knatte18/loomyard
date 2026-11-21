// Package githubclient owns GitHub token resolution, token caching, and construction of an authenticated *github.Client.
// This file is the package's design record; see PATTERN-github-auth for the machine-enforced half of the same contract.
//
// # Scope
//
// The package exposes no per-operation wrapper methods, only auth and client construction.
// Consumers call go-github's typed API directly on the returned *github.Client.
//
// # Token resolution order and the cache
//
// Sources are tried in fixed order: GH_TOKEN, GITHUB_TOKEN, the on-disk cache, then a bounded `gh auth token` shell-out.
// Environment variables always outrank the cache.
// The cache only avoids the shell-out and is never a source of truth;
// an unreadable, malformed, or foreign-shaped cache file is a miss, never a fatal error.
//
// The cache file lives at a machine-global location outside every repository, so a token never sits in a checkout.
// It holds the token in plaintext, readable only by its owner; that is the accepted cost of never blocking.
// CredentialDir names that directory for the Slack credential file, which `internal/slackengine` alone reads and writes.
//
// # Why WithAuthToken is banned
//
// Exactly one layer owns the Authorization header: the authenticating transport, installed outermost.
// github.WithAuthToken is never used to construct a client here.
// It would give the header a second owner, and a 401 replay would re-send its stale token.
//
// # The 401 replay
//
// On a 401 the transport invalidates the cache, re-resolves once, rewinds the body and replays once.
// A second consecutive 401 is returned unchanged.
// A token from GH_TOKEN or GITHUB_TOKEN is never replayed, since re-resolving reproduces the same value.
// The transport returns an error wrapping ErrTokenRejected instead,
// so a caller can tell a refused token from a transport failure.
//
// # The two deadlines
//
// The `gh auth token` shell-out is bounded at 5s, and the client's transport bounds a call, replay included, at 30s.
// Both bounds are needed: http.DefaultTransport sets no response timeout,
// and an unbounded credential path is indistinguishable from a hang to an autonomous process.
//
// # Never-block-on-credentials
//
// `gh auth login` is never invoked, and every resolution step returns quickly or returns ErrTokenUnresolvable.
// A process blocked on a credential prompt looks like a hang to an autonomous lym run.
// That property is why the package is its own leaf.
package githubclient
