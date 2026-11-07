// Package githubclient owns GitHub token resolution, token caching, and construction of an authenticated *github.Client.
// This file is the package's design record; see PATTERN-github-auth for the machine-enforced half of the same contract.
//
// # Scope
//
// The package exposes no per-operation wrapper methods, only auth and client construction.
// Consumers call go-github's typed API directly on the returned *github.Client.
// They supply owner and repo as ordinary call parameters, as in c.Issues.Create(ctx, owner, repo, ...).
// Wrappers would reinvent a maintained library and create a surface that must track every consumer's needs.
// Credential resolution is what the package owns.
// A second copy of it would give a caller two token chains, two shell-outs and two timeouts to keep in sync.
// Adding a GitHub operation to a consumer therefore needs no change here.
//
// # Token resolution order and the cache
//
// resolveToken (token.go) tries its sources in fixed order.
// The first three are the GH_TOKEN environment variable, GITHUB_TOKEN, and the on-disk cache (cache.go).
// The last is a bounded `gh auth token` shell-out whose result is written back to the cache.
// Environment variables always outrank the cache, so an operator who sets GH_TOKEN gets that value on every call.
// Cached tokens are trusted for cacheFreshness, applied at read time with no expiry field stored on disk.
// Changing the TTL therefore never requires migrating existing cache files.
// The cache only avoids the `gh auth token` shell-out cost on every request and is never a source of truth.
// An unreadable, malformed, or foreign-shaped cache file is a miss, never a fatal error.
//
// The cache file lives at a machine-global location, outside every repository this process might operate on:
// %LOCALAPPDATA%\lym on Windows, $XDG_CONFIG_HOME/lym or $HOME/.config/lym elsewhere.
// A per-repository cache would multiply the places a token sits on disk with every checkout.
// It would also need its own gitignore discipline in each to avoid an accidental commit.
// The cache writes the operator's GitHub token to disk in plaintext in this one location.
// That is an accepted cost of the non-blocking property below.
// The file is readable only by its owner: 0600, plus an owner-only DACL on Windows, which ignores Go's permission bits.
// The DACL is set in cache_windows.go.
//
// The package also names that directory, through CredentialDir, for the Slack credential file.
// internal/slackengine alone reads and writes that file; this package resolves no Slack token itself.
//
// # Why WithAuthToken is banned
//
// Exactly one layer owns the Authorization header: authRT (transport.go).
// It is installed as the client's outermost http.RoundTripper.
// github.WithAuthToken is never used to construct a client here.
// It captures a fixed token in go-github's own transport wrapper.
// With authRT installed too, the header would have two owners.
// A 401 replay would then re-send the stale token from the WithAuthToken closure.
// That defeats the re-resolution authRT provides.
//
// authRT also holds every request while a pause covers the process.
// It waits at entry through pauserecord's network gate, before it resolves a token and outside the 401 replay.
// A replay is therefore not held twice.
// The hold only delays the request; with no gate installed it does nothing.
// A held call's caller deadline moves later by the time the gate held it.
// authRT applies its 30s timeout from the end of the hold, so a hold does not use up the call's budget.
//
// # The 401 replay, and why environment-sourced tokens skip it
//
// On a 401 response, authRT invalidates the token cache, re-resolves once, and rewinds the request body via req.GetBody.
// It then replays once with the freshly resolved token.
// A second consecutive 401 is returned to the caller unchanged.
// The GetBody rewind is required: issue creation is a POST with a JSON body.
// A replay without the rewind sends drained bytes and surfaces as a confusing GitHub validation error.
//
// A token resolved from GH_TOKEN or GITHUB_TOKEN never goes through this replay.
// Environment variables outrank the cache, so invalidating the cache and re-resolving reproduces the identical value.
// The replay would fail the same way.
// authRT discards the response and returns an error naming the rejected environment variable instead.
// That error wraps ErrTokenRejected.
// A caller can therefore tell a refused token from a transport failure inside the same *url.Error.
//
// # The two deadlines
//
// Two independent timeouts bound every path through this package.
// The `gh auth token` shell-out (token.go) is bounded at 5s via exec.CommandContext behind an injectable seam,
// so a hung or prompting `gh` process cannot stall an autonomous lym run.
// The returned *github.Client's transport, authRT, applies a 30s timeout itself, from the end of the pause hold.
// It is enforced through the request context, so it covers the original attempt and a 401 replay together:
// worst case 30s total across both HTTP round trips, not 30s per attempt.
// Both bounds are needed: http.DefaultTransport sets no response timeout of its own,
// and a credential path with no bound is indistinguishable, to an autonomous process, from a hang.
//
// # Never-block-on-credentials is the package's reason for existing
//
// `gh auth login` is never invoked anywhere in this package, and no code path leads to it.
// Every resolution step either returns quickly or returns ErrTokenUnresolvable immediately.
// The quick steps are an environment read, a cache read and a bounded shell-out; none prompts or waits indefinitely.
// lym runs autonomously, and a process blocked on a credential prompt is indistinguishable from a hang.
// This property is why the package is its own leaf rather than a helper inside whichever module first needed a token.
//
// # The GitHub surface consumers need
//
// Consumers need issue list, view, create and close (comment-then-close).
// They also need PR list, view and create, repo view, and an auth probe.
// All are standard authenticated REST calls that go-github already exposes.
// Supporting them needs nothing beyond auth and construction.
// `gh issue view` exits 0 on a closed issue.
// A consumer must therefore read the response's state field to tell open from closed.
// PR list results carry a state precedence of MERGED > OPEN > CLOSED.
// A consumer applies it to collapse several PRs touching one subject to a single state.
//
// `lym beam pr` opens, edits and reopens the pull request from a beam's branch into its parent, and never merges one.
// It reads the status through internal/pullrequest, so its token reaches it only through this package.
package githubclient
