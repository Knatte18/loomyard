# Discussion: Fix prowler: Reddit adapter blocked

```yaml
task: 'Fix prowler: Reddit adapter blocked'
slug: prowler-fix-reddit-block
status: discussing
parent: main
```

## Problem

prowler's Reddit adapter (`plugins/prowler/reddit.go`) fetches `old.reddit.com`'s server-rendered HTML because Reddit historically left that legacy host ungated while hard-blocking scraping of its modern `www` SPA.
That is no longer true.
Retested on 2026-08-25 from a Linux home dev machine on a confirmed residential Norwegian ISP IP, prowler returns a bot-challenge page instead of thread content;
the brief recorded the symptom as a generic "You've been blocked by network security" block page, with plain `curl` from the same machine still reaching Reddit (a 302 to login).

The brief left the cause unisolated between two confounded variables — OS/browser fingerprint (Windows Chrome vs Linux headless Chromium) and network context (corporate VPN vs home residential) — and named one un-run diagnostic: which Chrome binary `browser.go` discovers on Linux.
Live probing during this discussion settled it, and **neither confounded variable is the cause**.
Reddit changed policy on both hosts prowler depends on:

- `old.reddit.com/*` now answers anonymous requests with `302 -> /login/?reason=lor2` ("logged-out redirect"). Reproduced with bare `curl` **and** with prowler's exact header set — identical result, so headers/fingerprint are not the variable at the static tier.
- `www.reddit.com/*` HTML now serves an 8KB JavaScript bot-challenge interstitial ("Prove your humanity"), and `www.reddit.com/*.json` plus anonymous `oauth.reddit.com` return `403` carrying the exact "blocked by network security" string from the brief.

**Why now:** the adapter's core premise — that `old.reddit.com` is ungated for anonymous readers — expired.
No amount of fingerprint or transport tuning restores it, because the gate is an authentication check, not a bot check.

A second, independent defect surfaced while reproducing: prowler does not merely fail, it **answers with garbage**.
Running the built binary against a Reddit thread produced a successful-looking result whose entire body was `# Reddit - Prove your humanity` / "Complete the challenge below and let us know you're a real person."
A caller (a Claude session using the `prowler` skill) has no way to tell that from real content.

## Scope

**In:**

- `plugins/prowler/reddit.go` — replace the anonymous `old.reddit.com`-only strategy with a tiered strategy: authenticated Reddit OAuth API first when credentials are present, then the existing `old.reddit.com` HTML tier hardened with login-redirect detection, then a definitive error.
- A new file in `plugins/prowler/` owning the Reddit OAuth client: token acquisition, in-process token caching, and `oauth.reddit.com` thread retrieval + JSON-to-markdown formatting.
- `plugins/prowler/fetch.go` — stop following redirects blindly in `fetchOldRedditHTML`, and route Reddit URLs so they never reach the headless-browser tier.
- A shared block/challenge-page detector, used by both the generic cascade in `fetch.go` and by the Reddit adapter, so a bot wall is never returned as successful content.
- Unit tests (stubbed transport) for every new path, plus a `//go:build integration` live test gated on real credentials.
- `plugins/prowler/README.md` — document the new Reddit credentials prerequisite and the tier order.

**Manual operator prerequisite (not automatable, must be scheduled by the plan):** no Reddit application exists yet, and none can be created by the implementing agent — registering one requires an authenticated Reddit account and a web form.
The **operator** registers a Reddit "script"-type app at `https://www.reddit.com/prefs/apps` and exports the resulting client id and secret as `PROWLER_REDDIT_CLIENT_ID` and `PROWLER_REDDIT_CLIENT_SECRET` in the environment the live integration test runs in — the operator's own shell, not CI, since prowler is a locally-built CLI plugin and this repo runs no CI job for the nested `plugins/prowler` module.
The plan must place this as an explicit prerequisite step gating the live credentialed smoke test, not as a footnote: every unit-testable part of the task can be completed and reviewed before it, but the task cannot reach "done" until the operator has provisioned the app and the live test has been run and observed to pass.
The secrets are never committed, never written to a config file, and never echoed in test output or error messages — error messages name the variables only.

**Out:**

- Any stealth/anti-detection hardening of `browser.go` (masking `navigator.webdriver`, persistent profiles, fingerprint spoofing). Measured to fail *and* to actively harm — see the "no-browser-tier-for-Reddit" decision.
- `chrome.go` / `browser.go` discovery logic. Investigated and cleared: discovery works correctly on this machine.
- The Hacker News adapter (`hackernews.go`), `htmltext.go`, `outfile.go`, `main.go`, and `scripts/run.sh` — untouched.
- Generalizing an "authenticated adapter" seam across the `siteAdapter` interface. Only one site needs auth today.
- Third-party Reddit frontends (redlib/libreddit/safereddit mirrors). Probed and rejected — see the rejected alternatives.
- Any change to the root `github.com/Knatte18/loomyard` module. prowler is a nested module (`plugins/prowler/go.mod`) and this task stays inside it.

## Decisions

### reddit-access-via-oauth-api

- Decision: Reddit content is retrieved from the official OAuth API at `oauth.reddit.com`, authenticated with an app-only token, as the adapter's primary tier.
- Rationale: it is the only access path that is sanctioned, stable against the anti-scraping measures that just broke the adapter, and not dependent on evading a bot wall. Every anonymous HTML path Reddit exposes is now either login-gated or challenge-gated.
- Rejected:
  - *Session-cookie injection from the operator's logged-in browser* — cookies expire, must be re-harvested by hand on two machines, and are a secret-at-rest with no revocation story.
  - *Stealth-hardened headless Chrome* — measured to fail, and to cause collateral damage (next decision).
  - *Third-party frontends (redlib/libreddit/safereddit)* — probed live during discussion: `redlib.catsarch.com` returned `403`, `redlib.perennialte.ch` returned `503`, `safereddit.com` returned an Anubis proof-of-work "Verifying your browser…" wall, and two more instances did not resolve. Availability is not something this repo can depend on.
  - *Dropping the Reddit adapter entirely* — discards recoverable capability; the OAuth path exists precisely for this use.

### tier-order-and-missing-credentials

- Decision: the adapter runs three tiers in order — (1) OAuth API when both credential env vars are present and non-empty; (2) the existing `old.reddit.com` HTML fetch, hardened with login-redirect detection; (3) a definitive error result naming which tiers were attempted and why each failed, including the names of the missing credential env vars when tier 1 was skipped.
- Rationale: OAuth is deterministic where it is available, so it goes first rather than paying a wasted request. Tier 2 is one cheap request and still succeeds on networks Reddit has not gated — plausibly including the corporate VPN where this adapter last worked, which is a live possibility this task cannot rule out from one vantage point. Tier 3 exists because an honest failure is strictly more useful to the calling session than a challenge page.
- Rejected:
  - *`old.reddit` first, OAuth as fallback* — wastes a request on the common gated case and returns lower-fidelity text when the better source is available.
  - *OAuth only, hard error without credentials* — needlessly removes a tier that costs one request and may still work.
  - *Silent skip / fall through to the generic cascade* — that is exactly the current behaviour that produces garbage results.

### no-browser-tier-for-reddit

- Decision: Reddit URLs terminate in the adapter and are never routed to `fetchWithBrowser`. When all Reddit tiers fail, the adapter reports a definitive error rather than reporting `handled=false` and letting `fetchPage` fall through to the generic cascade and its headless-Chrome tier.
- Rationale: directly measured during discussion. A first headless request to a Reddit thread returned the solvable-looking "Prove your humanity" challenge; a **second** request from the same persistent Chrome profile returned the hard "blocked by network security" page. Repeated headless attempts escalate the reputation of the residential IP the brief went to the trouble of confirming clean. The browser tier cannot succeed here and its failure mode is cumulative and externally visible.
- Rejected: *keep the browser as a last resort* — it has no success path against this challenge (confirmed with `--headless=new`, `--disable-blink-features=AutomationControlled`, a real Linux Chrome UA, a 1920x1080 window, and a 25s virtual-time budget to let the challenge auto-solve; every combination returned the challenge), and it carries the escalation cost above.

### shared-block-page-detection

- Decision: add a shared block/challenge-page detector and apply it at both sites — the generic `fetchPage` cascade and the Reddit adapter. A response that the detector identifies as a bot wall, login wall, or challenge interstitial is never returned as successful content; the generic cascade treats it as extraction failure, and the adapter as tier failure.
- Rationale: prowler returning `# Reddit - Prove your humanity` as a successful fetch is a correctness bug in the cascade, not a Reddit quirk. Any bot-walled site hits it, because the cascade's only success criterion is `len(text) >= minUsableTextLen` and challenge pages comfortably clear 100 characters. Fixing it only inside the Reddit adapter would leave the general defect live.
- Rejected:
  - *Reddit-only detection inside the adapter* — leaves the generic bug in place.
  - *No detection* — the caller cannot distinguish garbage from content, which is the worse of the two failure modes.

### oauth-credential-shape

- Decision: app-only `client_credentials` grant against `https://www.reddit.com/api/v1/access_token`, using HTTP Basic auth with credentials from a Reddit "script" app, read from `PROWLER_REDDIT_CLIENT_ID` and `PROWLER_REDDIT_CLIENT_SECRET`. Both must be present and non-empty for tier 1 to run.
- Rationale: fewest secrets to hold and rotate; no user password or long-lived refresh token in the environment. Env vars match how `CHROME_PATH` is already consumed by `chrome.go`, so prowler gains no new configuration mechanism.
- Rejected:
  - *Password grant* — adds a username and account password to the environment for no capability this task needs.
  - *Pre-authorized refresh-token flow* — requires an interactive authorization round-trip and a place to persist the refresh token; disproportionate for read-only public-thread access.
- **Open risk, must be closed during implementation:** this exact grant was **not** verified end-to-end, because no Reddit app credentials exist yet. The only probe possible during discussion was anonymous (`grant_type=client_credentials` with no Basic auth), which returned the expected `401`. If a real credentialed run shows `client_credentials` is insufficient for the endpoints this adapter needs, the fallback within this same decision is the installed-client grant (`grant_type=https://oauth.reddit.com/grants/installed_client` with a device id), which needs no additional secret. Escalating to the password grant is a scope change and must be raised, not taken unilaterally.

### api-user-agent

- Decision: OAuth API requests send a descriptive, non-impersonating User-Agent (`prowler/1.0`), overridable via an env var. `browserUA` from `headers.go` is never sent to `oauth.reddit.com` or the token endpoint.
- Rationale: Reddit's API rules require a descriptive User-Agent and specifically penalize generic/impersonating ones with aggressive rate limiting. `browserUA` claims to be Chrome 131 on Windows, which is exactly the shape that gets throttled on the API. The existing `browserUA` stays in use for the tier-2 HTML fetch, where presenting as a browser is still correct.
- Rejected: *reuse `browserUA` everywhere* — invites API-side rate limiting and misrepresents the client on an authenticated endpoint where there is nothing to gain by it.

### token-caching

- Decision: cache the access token in process memory only, honouring the expiry Reddit returns, and re-acquire when absent or expired. No disk cache.
- Rationale: prowler is a short-lived CLI — one invocation fetches a handful of URLs concurrently (`runAll`) and exits. A disk cache would add a secret at rest, a lock (the module already carries lock complexity in `scripts/run.sh`), and cross-process invalidation, for a saving of one token request per invocation.
- Rejected:
  - *Disk cache under `bin/`* — cost and risk out of proportion to the benefit; `bin/` is gitignored build output, not a state directory.
  - *Fetch a token per request* — `runAll` fetches URLs concurrently, so this multiplies token requests against a rate-limited endpoint within a single invocation.

### output-format

- Decision: OAuth-fetched threads are formatted as markdown — title, then the post's selftext, then top-level comments capped at the existing `maxTopComments` constant (20), each with one level of replies.
- Rationale: mirrors the shape `hackernews.go` already produces, so downstream consumers see consistent output across adapters, and reuses `maxTopComments`, which already lives in `reddit.go` and is currently only read by the Hacker News adapter. One reply level keeps the useful context of a threaded exchange without letting a large thread dominate the output file.
- Rejected:
  - *Flat comment list* — loses reply attribution, which is often where the answer is.
  - *Full recursive tree* — unbounded output from deep threads, and the deep tail is rarely the valuable part.

## Technical context

`plugins/prowler/` is a **nested Go module** (`module github.com/Knatte18/loomyard/plugins/prowler`, Go 1.26), independent of the repo-root `github.com/Knatte18/loomyard` module.
Build and test from inside `plugins/prowler/`.
Existing direct dependencies: `chromedp`, `go-readability`, `goquery`, `brotli`.
The task needs no new dependency — token acquisition and JSON decoding are stdlib (`net/http`, `encoding/json`).

Files that matter:

- `reddit.go` (43 lines) — `redditAdapter` implements `siteAdapter`; `Matches` uses `redditHostPattern` (bare/`www.`/`old.` hosts); `Fetch` delegates entirely to `fetchOldRedditHTML`. `toOldRedditURL` rewrites via `redditHostReplace`. `maxTopComments = 20` lives here but is consumed by `hackernews.go`.
- `fetch.go` (200 lines) — `fetchPage` is the cascade: adapters first, then static HTTP + Readability, then body-text fallback, then `f.browser`. Note the control flow: when a matching adapter returns `handled=false`, the loop `break`s and execution **continues into the generic cascade**, which is how a failed Reddit adapter currently reaches headless Chrome. `fetchOldRedditHTML` also lives here.
- `fetcher.go` (26 lines) — the `fetcher` struct is the injection seam: `do func(*http.Request) (*http.Response, error)`, `browser func(ctx, url) (string, bool)`, `adapters []siteAdapter`. All new code must be written against this seam so it is unit-testable with a stubbed transport, exactly as the existing tests do.
- `headers.go` (32 lines) — `browserUA`, `defaultHeaders()`, and the shared `httpClient` (60s timeout). **`httpClient` uses Go's default redirect policy, which follows up to 10 redirects.** This is the mechanism by which `fetchOldRedditHTML` silently lands on the login page and never learns it was redirected. Suppressing or detecting the redirect is the tier-2 fix; note that `httpClient` is shared with the generic cascade, so changing its `CheckRedirect` globally would alter unrelated behaviour — prefer a request-scoped or Reddit-scoped mechanism.
- `htmltext.go` (62 lines) — `stripToBodyText` removes `script/style/noscript/nav/header/footer` then normalizes whitespace. It is what turns a challenge page into >100 characters of plausible-looking "content".
- `chrome.go` / `browser.go` — investigated and cleared, not modified. `findChromeExecutable` checks `CHROME_PATH` then `chromeCandidates`; on this machine `/usr/bin/google-chrome` (real Google Chrome) is the first Linux candidate and is found correctly.
- `main.go` — `newFetcher()` is the sole production construction site for `fetcher`; `runAll` fetches all URLs **concurrently** in goroutines, which is why token acquisition must be concurrency-safe.
- `README.md` — has a "Runtime prerequisite: Chrome/Chromium" section and a "Site adapters" section; both need updating, and a Reddit-credentials prerequisite needs adding.

Reproduction commands and their observed results, for the implementer to re-confirm:

| Command | Observed 2026-08-25 |
| --- | --- |
| `curl -sI https://old.reddit.com/r/golang/` | `302`, `location: https://old.reddit.com/login/?reason=lor2&dest=...` |
| same with prowler's full header set | identical `302` |
| `curl -sL https://old.reddit.com/r/golang/` | `200`, ~320KB, `<title>Welcome to Reddit</title>` |
| `curl -s https://www.reddit.com/r/golang/` | `200`, 8402 bytes, `<title>Reddit</title>`, JS auto-submit challenge form |
| `curl -s https://www.reddit.com/r/golang/.json` | `403`, ~190KB, contains "blocked by network security" |
| `curl -s https://oauth.reddit.com/r/golang/` | `403`, same block page |
| built binary vs a thread URL | writes a 229-byte file whose body is `# Reddit - Prove your humanity` |
| `google-chrome --headless=new --dump-dom <thread>` | "Prove your humanity"; unchanged by UA/window/AutomationControlled/25s virtual-time flags |
| second `--dump-dom` with the same `--user-data-dir` | escalates to "blocked by network security" |

Gotchas:

- The 8KB `www` interstitial has `<title>Reddit</title>` and the challenge string only appears after JS runs — a detector keyed on the *static* `www` body must not rely on the rendered title.
- Reddit's block page is ~190KB and the login page ~320KB; both clear `minUsableTextLen` (100) by orders of magnitude. Size thresholds are not a usable detection signal.
- `fetchOldRedditHTML` currently swallows every failure into a bare `return "", false`, giving the caller no reason for the failure. Tier 3's error message needs those reasons, so the tier functions must return something more informative than a bool to the adapter's `Fetch`.
- Do not re-run live Reddit probes in a loop during development. Each headless attempt degrades this IP's standing (measured above), and the integration test must be explicitly opt-in for the same reason.

## Constraints

From `CONSTRAINTS.md` (repo root) — most invariants there govern the root `lyx` module and its `internal/` packages and are **not** applicable to this nested plugin module. The ones that bear on this task:

- **Documentation Lifecycle** — and this repo's `CLAUDE.md` rule that a task changing observable CLI behaviour updates its docs *in the same commit*. Changing prowler's Reddit behaviour and adding a credentials prerequisite requires the `plugins/prowler/README.md` update to land with the code.
- **Never Force-Add Invariant** — no `git add -f`. `plugins/prowler/bin/` is gitignored build output and must stay uncommitted.
- **Test Tier Purity Invariant** — enforced by `cmd/lyx/tierpurity_test.go`, which scans the *root* module and therefore does not mechanically scan `plugins/prowler`. The spirit still applies and the module already honours it: untagged tests stay offline and fast. Anything touching the network or spawning a browser carries `//go:build integration`, matching the existing `browser_integration_test.go` header.
- **GitHub Auth Invariant** — not applicable (Reddit, not GitHub; and prowler is outside the root module), but its shape is the local precedent: credential resolution belongs in one place. Keep all Reddit token/credential handling inside the single new OAuth file rather than scattering `os.Getenv` calls across the adapter.

Discovered during discussion:

- No new module dependency. Everything needed is stdlib on top of the existing `go.mod`.
- Credentials are read from the environment only — never a config file, never a committed default, never logged. Error messages may name the *variables*, never their values.
- `runAll` runs per-URL fetches concurrently, so token acquisition and the token cache must be safe under concurrent use within one process.

## Testing

TDD candidates — write the test first for each:

- **Block/challenge-page detection.** Pure function over response bytes/status. Table-driven: the real Reddit block page, the real login page, the `www` JS interstitial, and known-good article HTML that must *not* be flagged. Capture trimmed fixtures of the real pages rather than hand-writing approximations, since the whole point is recognizing the genuine articles. This is the highest-value TDD target — it is a pure function with a sharp contract and a real false-positive risk.
- **`old.reddit.com` login-redirect detection.** Stubbed transport returning `302` with a `location` of `https://old.reddit.com/login/?reason=lor2&dest=...`; assert tier 2 reports failure with a reason, and specifically that it does **not** follow through to a `200` login page and report success.
- **OAuth token acquisition.** Stubbed transport: assert the request targets the token endpoint, carries HTTP Basic auth built from the two env vars, sends the descriptive User-Agent and not `browserUA`, and that a malformed or `401` response yields a tier failure rather than a panic or an empty-token request.
- **Token caching.** Two fetches through one client issue exactly one token request; an expired token triggers re-acquisition. Include a concurrent case, since `runAll` is concurrent.
- **Thread JSON to markdown.** Fixture of a real `oauth.reddit.com` thread response. Assert title and selftext present, top-level comments capped at 20, exactly one reply level rendered, and that a thread with zero comments still produces usable output rather than being judged too short.
- **Missing-credentials path.** Neither env var set, and each set alone: tier 1 is skipped without issuing any request, and the eventual tier-3 error names the missing variables.
- **Tier ordering and termination.** Both tiers stubbed to fail: assert the result is a definitive error and that the browser function on the injected `fetcher` was **never called** — this is the regression guard for the IP-escalation decision, and it is the single most important behavioural test in the task.
- **Generic cascade regression.** A non-Reddit URL whose response is a challenge page: assert `fetchPage` does not return it as successful content.

Scenarios that must be covered but are not unit-testable:

- **Live credentialed smoke test**, `//go:build integration`, skipped when the credential env vars are absent. This is what closes the open risk in the `oauth-credential-shape` decision, and the task is not done until it has been run for real against a live Reddit app and observed to pass. A plan that only reasons about the grant working is insufficient. It depends on the **manual operator prerequisite** in Scope — the operator registering the Reddit script app and exporting the two env vars — which the plan must schedule as its own gating step before this test can run.
- The existing `//go:build integration` browser test must still pass unchanged — this task does not alter the browser tier for non-Reddit URLs.
- `scripts/selftest.sh` must still pass; it is offline and build-focused and should be unaffected, but it is the module's existing harness and is cheap to re-run.

Rate-limiting note for whoever runs the live checks: prefer one thread URL, run once. Do not loop live Reddit requests.

## Q&A log

- **Q:** Primary Reddit access strategy, given `old.reddit.com` now requires login and `www` serves a bot challenge? **A:** [auto-pick] Reddit OAuth API (`oauth.reddit.com`) with credentials from env. **Why:** the only sanctioned, stable path; cookie injection and stealth headless are measured-failing or IP-burning, and dropping the adapter discards capability OAuth can recover.
- **Q:** Cascade order and behaviour when credentials are absent? **A:** [auto-pick] OAuth tier when creds present, then `old.reddit.com` with login-redirect detection, then an explicit error naming the attempted tiers and any missing env vars. **Why:** OAuth is deterministic when available; the cheap tier-2 request may still succeed on networks Reddit has not gated, plausibly including the corporate VPN where the adapter last worked.
- **Q:** Should block/challenge detection be Reddit-specific or shared with the generic cascade? **A:** [auto-pick] Shared, applied at both sites. **Why:** returning a challenge page as successful content is a general cascade defect — its only success test is a 100-character length threshold, which every challenge page clears.
- **Q:** Should Reddit URLs still be able to reach the headless-browser tier as a last resort? **A:** [auto-pick] No — Reddit terminates in the adapter with a definitive result. **Why:** measured escalation; a second headless request from the same profile turned a "Prove your humanity" challenge into a hard "blocked by network security" block on a residential IP the brief confirmed was clean.
- **Q:** Scope — how much of prowler changes? **A:** [auto-pick] `reddit.go` plus a new OAuth file, `fetch.go` block detection, README, and tests. **Why:** a `reddit.go`-only fix leaves the generic silent-garbage bug live, while generalizing an authenticated-adapter seam is speculative generality for a single site.
- **Q:** Which OAuth grant and credential shape? **A:** [auto-pick] `client_credentials` app-only token from a script app, via `PROWLER_REDDIT_CLIENT_ID` / `PROWLER_REDDIT_CLIENT_SECRET`. **Why:** fewest secrets; no password or refresh token in the environment. **Unverified** — no credentials existed to test with; the installed-client grant is the in-scope fallback, the password grant is not.
- **Q:** Output format for OAuth-fetched threads? **A:** [auto-pick] Markdown — title, selftext, top-level comments capped at the existing `maxTopComments` (20), one reply level each. **Why:** mirrors `hackernews.go`'s shape and reuses a constant already declared in `reddit.go`.
- **Q:** Token caching strategy? **A:** [auto-pick] In-memory for the process lifetime, honouring Reddit's expiry, no disk cache. **Why:** prowler is a short-lived CLI; a disk cache adds a secret at rest and a lock for the saving of one request per invocation.
- **Q:** User-Agent on API calls? **A:** [auto-pick] Descriptive `prowler/1.0`, env-overridable, never `browserUA`. **Why:** Reddit's API rules require a descriptive UA and throttle impersonating ones; `browserUA` stays correct for the tier-2 HTML fetch only.
- **Q:** Testing approach? **A:** [auto-pick] Unit tests against the `fetcher` stub seam for every new path, plus a `//go:build integration` live test gated on real credentials. **Why:** matches the module's existing convention (`browser_integration_test.go`), and the live test is what closes the unverified-grant risk — the task is not done on reasoning alone.
- **Q:** Was the brief's floated diagnostic (which Chrome binary `browser.go` discovers on Linux) the cause? **A:** [auto-pick] No — investigated and cleared, `chrome.go`/`browser.go` stay out of scope. **Why:** `/usr/bin/google-chrome` is real Google Chrome and is the first Linux candidate matched; discovery works correctly, and the same challenge appears when driving that binary directly.
- **Q:** Are the brief's two confounded variables (Windows-vs-Linux fingerprint, VPN-vs-residential network) the cause? **A:** [auto-pick] Neither — the cause is a Reddit policy change on both hosts. **Why:** bare `curl` and prowler's exact header set produce an identical `302` to `/login/?reason=lor2`, so the gate is an authentication check rather than a fingerprint or IP-reputation check.
