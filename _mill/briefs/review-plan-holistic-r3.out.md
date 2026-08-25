MILL_REVIEW_BEGIN
# Review: Fix prowler: Reddit adapter blocked — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnetxhigh
reviewer_self_id: Claude Sonnet 5 (claude-sonnet-5)
reviewed_file: plan/
date: 2026-08-25
```

## Findings

### [BLOCKING:consistency] Stale `fetchOldRedditHTML` doc comment not scheduled for update
**Location:** batch 3 / card 8
**Issue:** Card 8 changes `fetchOldRedditHTML`'s signature to `(string, error)` and fixes the login-redirect defect, but its Requirements never touch the function's own doc comment in `fetch.go` ("fetches Reddit URLs from old.reddit.com's HTML, which avoids gating... Reports false if request fails or yields too little content"), which becomes false on both the "avoids gating" premise and the return-shape description — card 9 fixes the analogous stale comments in `reddit.go` (package comment, `Fetch` doc, `redditAdapter` type doc) but this one in `fetch.go` is missed.
**Fix:** Add a requirement to card 8 rewriting `fetchOldRedditHTML`'s doc comment to state it is now login-gated and returns `(string, error)`.

### [BLOCKING:design] "credentials absent" adapter test's tier-2 stub outcome unspecified
**Location:** batch 3 / card 9
**Issue:** The new `TestRedditAdapterFetch` sub-test "credentials absent, asserting the output names both variables" only produces that output if tier 2 (`fetchOldRedditHTML`) also fails and execution reaches tier 3 — if tier 2 succeeds, the returned string is tier 2's own content with no credential names in it. Requirements never states that tier 2 must be stubbed to fail here, nor that its raw `fetcher` literal needs `doNoRedirect` wired (per card 7's "unset field is a wiring bug" rule), risking a nil-pointer panic instead of a controlled failure when the implementer writes this sub-test.
**Fix:** State explicitly that this sub-test also stubs tier 2 to fail (e.g. a non-2xx response via `doNoRedirect`), matching the stub-by-stub rigor card 8 used for its own raw-literal sub-tests.

### [NIT:consistency] New files lack the module's per-file header-comment convention
**Location:** batch 1 / card 2, batch 2 / card 4
**Issue:** Every existing file in `plugins/prowler` (fetch.go, fetcher.go, headers.go, htmltext.go, chrome.go, hackernews.go, browser.go, adapter.go, reddit.go, main.go) opens with a descriptive header comment; Requirements for the new `blockdetect.go` and `redditoauth.go` never ask for one, breaking with an otherwise-universal repo convention this plan is careful about elsewhere.
**Fix:** Add a one-line requirement that each new file open with a header comment in the existing style.

### [NIT:scope] Batch 4's offline-suite/selftest re-run isn't in either card's Requirements
**Location:** batch 4 / card 11
**Issue:** "## Batch Tests" instructs re-running `go -C plugins/prowler test ./...` and `plugins/prowler/scripts/selftest.sh` once at the end of batch 4, but neither action appears in card 10 or card 11's Requirements, and it isn't part of the mechanically-scheduled `verify:` command either — easy to skip since nothing enforces it.
**Fix:** Fold that re-run instruction into card 11's Requirements list.

## Verdict

REQUEST_CHANGES
Two BLOCKING gaps (a stale doc comment, an underspecified test stub) plus two minor consistency NITs.
MILL_REVIEW_END
