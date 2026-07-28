// Package githubclient owns GitHub token resolution, token caching, and
// construction of an authenticated *github.Client -- nothing else. This file
// is the package's durable design record; see the GitHub Auth Invariant in
// CONSTRAINTS.md for the machine-enforced half of the same contract.
//
// # What this package deliberately is not
//
// githubclient exposes no per-operation wrapper methods -- there is no
// CreateIssue, no ListPullRequests, nothing beyond auth and client
// construction. Consumers call go-github's typed API directly against the
// *github.Client this package returns (c.Issues.Create(ctx, owner, repo,
// ...), c.PullRequests.List(ctx, owner, repo, ...)), supplying owner and repo
// themselves as ordinary call parameters. Hand-writing wrappers would
// reinvent a typed, maintained library and create a surface that must track
// every consumer's needs forever. The one thing that genuinely cannot be
// duplicated this way is non-blocking credential resolution -- duplicating
// that gets a caller two token chains, two shell-outs, two timeouts to keep
// in sync -- and that is exactly, and only, what this package owns. Under
// this shape, adding a GitHub operation to any consumer costs zero package
// work: it is a go-github call, not a new method here. selfreportengine is
// explicitly not extended to wrap this package either; it calls go-github
// directly like every other consumer.
//
// # Token resolution order and the cache
//
// resolveToken (token.go) tries, in fixed order: the GH_TOKEN environment
// variable, then GITHUB_TOKEN, then the on-disk cache (cache.go), then a
// bounded `gh auth token` shell-out whose result is written back to the
// cache. Environment variables always outrank the cache -- an operator who
// sets GH_TOKEN gets that value on every call, never a stale cached one.
// Cached tokens are trusted for cacheFreshness (12 hours), applied entirely
// at read time with no expiry field stored on disk, so changing the TTL
// never requires migrating existing cache files. The cache exists purely to
// avoid paying the `gh auth token` shell-out cost on every request; it is an
// optimization, never a source of truth -- any unreadable, malformed, or
// foreign-shaped cache file is treated as a miss, never a fatal error.
//
// The cache file lives at a machine-global location -- %LOCALAPPDATA%\lyx on
// Windows, $XDG_CONFIG_HOME/lyx or $HOME/.config/lyx elsewhere -- outside
// every repository this process might operate on. This is deliberate: a
// per-repository cache would multiply the number of places a token sits on
// disk with every checkout lyx touches, and would need its own gitignore
// discipline in every one of them to avoid an accidental commit. State
// plainly: this cache writes the operator's GitHub token to disk in
// plaintext, in this one additional location, readable only by its owner
// (0600 plus an owner-only DACL on Windows, since Windows otherwise ignores
// Go's permission bits -- see cache_windows.go). That is a real, accepted
// cost of the non-blocking property below, not an oversight.
//
// # Why WithAuthToken is banned
//
// Exactly one layer owns the Authorization header: authRT (transport.go),
// installed as the client's outermost http.RoundTripper. github.WithAuthToken
// is never used to construct a client here, because it captures a fixed
// token inside go-github's own transport wrapper -- with authRT installed
// too, the header would have two owners, and a 401 replay would re-send the
// stale token from WithAuthToken's closure, defeating the entire
// re-resolution authRT exists to provide. One owner removes the ordering
// question rather than leaving it to be answered by whichever transport
// happens to run last.
//
// # The 401 replay, and why environment-sourced tokens skip it
//
// On a 401 response, authRT invalidates the token cache, re-resolves once,
// rewinds the request body via req.GetBody, and replays exactly once with
// the freshly resolved token; a second consecutive 401 is returned to the
// caller unchanged, never a loop. The GetBody rewind is not incidental:
// issue creation is a POST with a JSON body, and a naive replay without it
// sends already-drained bytes, surfacing as a confusing GitHub validation
// error rather than as an obviously missing rewind.
//
// A token resolved from GH_TOKEN or GITHUB_TOKEN never goes through this
// replay. Environment variables outrank the cache by rule, so invalidating
// the cache and re-resolving is guaranteed to reproduce the identical
// environment value -- a replay in that case is a guaranteed-identical
// second failure, not a chance at recovery, so authRT discards the response
// and returns an error naming the rejected environment variable instead.
//
// # The two deadlines
//
// Two independent timeouts bound every path through this package. The `gh
// auth token` shell-out (token.go) is bounded at 5s via exec.CommandContext
// behind an injectable seam, so a hung or unexpectedly prompting `gh`
// process can never stall an autonomous lyx run. Separately, the returned
// *github.Client's underlying http.Client carries a 30s Timeout. That
// Client.Timeout is enforced through the request context, so it covers the
// original attempt and a 401 replay TOGETHER -- worst case 30s total across
// both HTTP round trips, not 30s per attempt. Neither deadline is
// decoration: http.DefaultTransport sets no response timeout of its own, and
// a credential path with no bound at all is indistinguishable, to an
// autonomous process, from a hang.
//
// # Never-block-on-credentials is the package's reason for existing
//
// `gh auth login` is never invoked anywhere in this package, and there is no
// code path from here to it. Every resolution step either returns quickly
// (an environment read, a cache read, a bounded shell-out) or returns
// ErrTokenUnresolvable as a typed error immediately -- never a prompt, never
// an indefinite wait. lyx runs autonomously, and a process blocked forever
// on a credential prompt is indistinguishable from a hang; this property,
// more than any particular resolution step, is why this package exists as
// its own leaf rather than as a thin helper inside whichever module first
// needed a GitHub token.
//
// # The GitHub surface consumers need
//
// The set of GitHub operations lyx's consumers need is derived from the
// sibling millhouse toolchain's actual usage, not assumed: issue list,
// issue view, issue create, issue close (comment-then-close), PR list, PR
// view, PR create, repo view, and an auth probe. All of these are standard
// authenticated REST calls that go-github already exposes, so "supporting"
// them costs this package nothing beyond auth and construction being
// sufficient -- which they are. Two consumption details are worth recording
// here so a future consumer does not rediscover them the hard way: `gh
// issue view` exits 0 on a closed issue, so the response's state field must
// be read explicitly to distinguish open from closed -- a consumer that
// checks only for a successful response will treat every closed issue as
// open. And PR list results carry a state precedence of MERGED > OPEN >
// CLOSED when a consumer needs to collapse multiple PRs touching the same
// subject down to one representative state.
package githubclient
