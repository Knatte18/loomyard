# Batch: block-detection

```yaml
task: 'Fix prowler: Reddit adapter blocked'
batch: 'block-detection'
number: 1
cards: 3
verify: go -C plugins/prowler test -run 'TestLooksLikeBlockPage|TestFetchPage|TestBrowserFallback' .
depends-on: []
```

## Batch Scope

This batch delivers the shared block/challenge-page detector and fixes the generic fetch cascade's silent-garbage defect, independently of anything Reddit-specific.
It is one batch because the detector and its single consumer (the generic cascade) are meaningless apart: shipping the detector without wiring it leaves the reported bug live, and wiring it without real captured fixtures leaves the detector unvalidated against the pages it exists to recognise.

The external interface batches 2 and 3 consume is one function, `looksLikeBlockPage(text string) (reason string, blocked bool)`, declared in the new `plugins/prowler/blockdetect.go`, plus the fixture files under `plugins/prowler/testdata/`.

Batch-local decision beyond `## Shared Decisions`: the detector takes a plain `string` (not `[]byte`, not an `*http.Response`) so the same function serves three different callers — raw decoded HTML in the static tier, extracted plain text from the browser tier, and a JSON-endpoint error body in batch 2 — without any of them needing to reconstruct a response.

## Cards

### Card 1: Capture real block/challenge/login page fixtures

- **Context:**
  - `_mill/discussion.md`
- **Edits:** none
- **Creates:**
  - `plugins/prowler/testdata/reddit-block-page.html`
  - `plugins/prowler/testdata/reddit-login-page.html`
  - `plugins/prowler/testdata/reddit-www-interstitial.html`
  - `plugins/prowler/testdata/good-article.html`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Capture four fixture files under a new `plugins/prowler/testdata/` directory, using exactly one live request per fixture — never a loop, never a retry loop, and never a headless browser (the discussion records that repeated headless attempts escalate this IP's standing).
  Capture with `curl` only.
  Sources, one request each:
  (a) `plugins/prowler/testdata/reddit-block-page.html` from `curl -s https://www.reddit.com/r/golang/.json` — the 403 body containing the "blocked by network security" string;
  (b) `plugins/prowler/testdata/reddit-login-page.html` from `curl -sL https://old.reddit.com/r/golang/` — the followed-redirect login page whose title is "Welcome to Reddit";
  (c) `plugins/prowler/testdata/reddit-www-interstitial.html` from `curl -s https://www.reddit.com/r/golang/` — the ~8KB JavaScript challenge interstitial;
  (d) `plugins/prowler/testdata/good-article.html` from `curl -s https://example.com/` — a known-good page that must never be flagged.
  Trim (a) and (b) to at most 8KB each by keeping the document's opening `<html>`/`<head>`/`<title>` region plus the contiguous region containing the distinguishing marker text, and inserting a single HTML comment line reading `<!-- trimmed for fixture use -->` at each elision point;
  do not otherwise rewrite, reformat, or normalise the captured bytes, because the fixtures' whole value is that they are the genuine articles.
  Leave (c) and (d) untrimmed — both are already small.
  If a capture returns something materially different from what the discussion's reproduction table records (for example, if Reddit has changed again and no marker string is present), stop and report it rather than hand-writing a substitute: a hand-written approximation would make the detector's tests prove nothing.
  Add no `.gitignore` entry and run no `git add -f` — these are ordinary tracked files.
- **Commit:** `test(prowler): capture real Reddit block, login, and challenge page fixtures`

### Card 2: Shared block/challenge-page detector

- **Context:**
  - `_mill/discussion.md`
  - `plugins/prowler/htmltext.go`
  - `plugins/prowler/fetch.go`
  - `plugins/prowler/fetch_test.go`
  - `plugins/prowler/testdata/reddit-block-page.html`
  - `plugins/prowler/testdata/reddit-login-page.html`
  - `plugins/prowler/testdata/reddit-www-interstitial.html`
  - `plugins/prowler/testdata/good-article.html`
- **Edits:** none
- **Creates:**
  - `plugins/prowler/blockdetect.go`
  - `plugins/prowler/blockdetect_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Write `plugins/prowler/blockdetect_test.go` first, then `plugins/prowler/blockdetect.go`, and commit both together so the tree is never left with a failing build.
  Open each of the two new files with a descriptive header comment above the `package main` line, in the style every existing file in this module uses (see `plugins/prowler/htmltext.go`): what the file is for and why it exists, not a restatement of its identifiers.
  In `plugins/prowler/blockdetect.go`, package `main`, declare:
  a `blockSignature` struct with two string fields, `name` (a short human-readable reason such as `"bot challenge"`, `"network-security block"`, `"login wall"`) and `marker` (an already-lowercased substring);
  a package-level `var blockSignatures []blockSignature` holding the table;
  and `func looksLikeBlockPage(text string) (reason string, blocked bool)`, which lowercases `text` once, returns the `name` of the first matching signature together with `true`, and returns `"", false` when nothing matches.
  Seed `blockSignatures` with these markers, all lowercase, exactly as written: `"blocked by network security"`, `"prove your humanity"`, `"complete the challenge below"`, `"checking your browser before accessing"`, `"verifying you are human"`, `"enable javascript and cookies to continue"`, `"attention required! | cloudflare"`, `"verifying your browser"`.
  Then add two further signatures whose markers are *discovered from the captured fixtures* rather than assumed, because neither page's distinguishing text is known in advance:
  a `login wall` signature whose `marker` is a distinctive substring of `plugins/prowler/testdata/reddit-login-page.html`;
  and a `bot challenge` signature whose `marker` is a distinctive substring of the *static* bytes of `plugins/prowler/testdata/reddit-www-interstitial.html`.
  The interstitial needs its own discovered marker because `_mill/discussion.md`'s Gotchas record that its "Prove your humanity" text appears only after JavaScript runs, so none of the eight seeded markers above can be assumed present in the bytes `curl` captured;
  read the fixture and pick something the static document actually contains, such as the auto-submitting challenge form's `action` value or one of its hidden field names.
  Both discovered markers must be absent from `plugins/prowler/testdata/good-article.html`, from `redditLikeHTMLWithComments`, and from `readableArticleHTML`;
  the test named below is what proves each choice is discriminating.
  If either fixture turns out to contain nothing distinctive enough to key on, stop and report it rather than weakening the marker into something a real article could contain — a false positive here silently discards genuine content.
  Do not add a size threshold, a status-code check, or a title-only heuristic: the discussion records that the block page is ~190KB and the login page ~320KB, so size is not a usable signal, and the `www` interstitial's challenge text appears only after JavaScript runs, so the static title is not one either.
  In `plugins/prowler/blockdetect_test.go`, package `main`, write `TestLooksLikeBlockPage` as a table-driven test reading each of the four fixtures with `os.ReadFile`, asserting `blocked` is `true` for the three wall fixtures and `false` for `plugins/prowler/testdata/good-article.html`, and additionally asserting `blocked` is `false` for the existing `redditLikeHTMLWithComments` and `readableArticleHTML` constants, both already declared in `plugins/prowler/fetch_test.go` and usable directly because every test file in this module is in package `main`, so a signature that would swallow real Reddit or article content fails the test.
  The test makes no network call and spawns no process.
- **Commit:** `feat(prowler): add shared block/challenge-page detector`

### Card 3: Never return a wall as content from the generic cascade

- **Context:**
  - `plugins/prowler/blockdetect.go`
  - `plugins/prowler/htmltext.go`
  - `plugins/prowler/fetcher.go`
  - `plugins/prowler/testdata/reddit-block-page.html`
- **Edits:**
  - `plugins/prowler/fetch.go`
  - `plugins/prowler/fetch_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `plugins/prowler/fetch.go`, add `func browserFallback(ctx context.Context, f fetcher, url string) (string, bool)`, which calls `f.browser(ctx, url)` and, when that reports success with non-empty text, additionally runs `looksLikeBlockPage` over the returned text and reports `("", false)` when it is flagged, so a challenge page rendered by headless Chrome is never promoted to a successful result.
  Replace all three existing `f.browser(ctx, url)` call sites inside `fetchPage` with `browserFallback(ctx, f, url)`;
  `fetchPage`'s own signature and its existing return strings are otherwise unchanged.
  Then, in `fetchPage`, immediately after `decodeContentEncoding` returns successfully and before the `scriptStyleNoscriptBlock` strip, run `looksLikeBlockPage` over `string(rawHTML)`;
  when it reports blocked, skip Readability and the body-text fallback entirely and go straight to `browserFallback` (a real browser can still legitimately clear a challenge on a non-Reddit site), and when that also fails return `errorResult(url, "blocked: "+reason)` rather than any "Could not extract readable content" text — the caller must be able to tell a wall from a page prowler merely failed to parse.
  In `plugins/prowler/fetch_test.go`, add `TestFetchPage_ChallengePageIsNotReturnedAsContent`: a non-Reddit URL whose stubbed response body is the contents of `plugins/prowler/testdata/reddit-block-page.html`, with the stubbed browser returning that same body as its text, asserting the result starts with `"# Error fetching "` and contains `"blocked:"`, and specifically asserting the result does not contain the block page's marker text.
  Add a second sub-test asserting that when the stubbed browser instead returns genuine article text, `browserFallback` passes it through unchanged.
  Do not change the existing assertions in `plugins/prowler/fetch_test.go`;
  only add to it.
- **Commit:** `fix(prowler): reject bot walls instead of returning them as content`

## Batch Tests

`verify:` runs `go -C plugins/prowler test -run 'TestLooksLikeBlockPage|TestFetchPage|TestBrowserFallback' .` from the worktree root.
The `-run` filter covers `TestLooksLikeBlockPage` (card 2's detector table, including the negative cases over the existing in-package HTML constants) and every `TestFetchPage*` function in `plugins/prowler/fetch_test.go` and `plugins/prowler/adapter_test.go` — the full existing regression surface for the cascade this batch modifies, plus card 3's two new sub-tests.
`TestBrowserFallback` is included in the filter so a directly-named unit test for the new helper is picked up if the implementer adds one;
it is not required.
The run is offline and needs no Chrome: every path is driven through stubbed `fetcher.do` and `fetcher.browser` fields.
Card 1 contributes no test of its own — its fixtures are consumed by cards 2 and 3, which is where their correctness is asserted.
