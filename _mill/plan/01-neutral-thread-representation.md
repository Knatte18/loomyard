# Batch: neutral-thread-representation

```yaml
task: "Add RSS-based Reddit read tier"
batch: "neutral-thread-representation"
number: 1
cards: 3
verify: go -C plugins/prowler test .
depends-on: []
```

## Batch Scope

This batch refactors Reddit markdown rendering onto a tier-neutral intermediate representation without adding any new tier.
It extracts `redditPost`/`redditComment` and the single `formatRedditThread` renderer into a new `plugins/prowler/redditformat.go`, re-signatures the formatter to take that representation, and adds the OAuth-listing-to-`redditPost` mapping so the existing OAuth path renders through the new seam.
The batch is bracketed by a golden-file regression: card 1 captures the current OAuth markdown byte-for-byte before anything changes, and that same golden file must still match at the end of card 2, which is what proves the refactor altered nothing on the credentialed path.
The external interface batch 2 consumes is `redditPost`, `redditComment`, and `formatRedditThread(post redditPost, sourceURL string) string`.

Batch-local decision, differing from nothing in `## Shared Decisions` but worth stating because the discussion left it to the planner: the neutral types and the shared formatter go in a **new** `plugins/prowler/redditformat.go` rather than staying in `redditoauth.go`.
`redditoauth.go`'s own file doc comment scopes it to the authenticated OAuth client, and the module's convention is one file per tier with a doc comment stating that role;
hosting a deliberately tier-neutral representation there would contradict both.
`maxTopComments` stays in `reddit.go`, per the discussion's `file-layout` decision.

## Cards

### Card 1: Capture the OAuth formatter's current output as a golden file

- **Context:**
  - `plugins/prowler/redditoauth.go`
  - `plugins/prowler/reddit.go`
  - `plugins/prowler/blockdetect_test.go`
  - `plugins/prowler/testdata/reddit-thread.json`
- **Edits:**
  - `plugins/prowler/redditoauth_test.go`
- **Creates:**
  - `plugins/prowler/testdata/reddit-thread-golden.md`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add a golden-file regression for `formatRedditThread`'s current output, before any refactor touches it, so card 2's rewrite is provably byte-neutral on the OAuth path.

  In `plugins/prowler/redditoauth_test.go`:
  - Add a package-level `var updateRedditGolden = flag.Bool("update-golden", false, ...)` with a doc comment explaining that it regenerates `testdata/reddit-thread-golden.md` from the current formatter and is never set in CI.
    Import `flag` and `os` as needed.
  - Add `TestFormatRedditThread_GoldenOAuthOutput(t *testing.T)`.
    It reads `testdata/reddit-thread.json` via the existing `readTestdataFile` helper, unmarshals it into `[]redditListing`, calls the formatter for `sourceURL` `"https://www.reddit.com/r/golang/comments/abc123/idiomatic_errors/"`, and compares the result against the contents of `testdata/reddit-thread-golden.md` read with `os.ReadFile`.
    When `*updateRedditGolden` is true it writes the produced output to that path with `os.WriteFile` and mode `0o644` instead of comparing, then returns.
  - The comparison is exact string equality on the full output — not `strings.Contains`.
    On mismatch, fail with a message naming the golden path and instructing the reader to re-run with `-args -update-golden` only after confirming the difference is intended.

  Generate the golden file by running, from the worktree root:
  `go -C plugins/prowler test -run TestFormatRedditThread_GoldenOAuthOutput . -args -update-golden`
  Then re-run `go -C plugins/prowler test .` with no flag and confirm the test passes against the committed file.
  Inspect the generated `plugins/prowler/testdata/reddit-thread-golden.md` before committing and confirm it contains the post title, a `Reddit | r/… | <n> points | by u/…` metadata line, a `Source:` line, and a `## Top Comments` heading — a golden file capturing empty or obviously wrong output is worse than none.

  Do not change `formatRedditThread` or any other production declaration in this card.
- **Commit:** `test(prowler): capture the OAuth reddit formatter's output as a golden file`

### Card 2: Extract the tier-neutral representation and re-signature the formatter

- **Context:**
  - `plugins/prowler/reddit.go`
  - `plugins/prowler/blockdetect_test.go`
  - `plugins/prowler/testdata/reddit-thread.json`
  - `plugins/prowler/testdata/reddit-thread-golden.md`
- **Edits:**
  - `plugins/prowler/redditoauth.go`
  - `plugins/prowler/redditoauth_test.go`
- **Creates:**
  - `plugins/prowler/redditformat.go`
  - `plugins/prowler/redditformat_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Introduce the tier-neutral representation and route the OAuth path through it, in one card, because the formatter's signature change and its callers must land together.

  Create `plugins/prowler/redditformat.go` with a file-level doc comment stating that it holds the tier-neutral thread representation and the single markdown renderer both Reddit tiers share, so the two tiers match by construction rather than by convention.
  It declares:

  - `type redditPost struct` with fields `Title string`, `Subreddit string`, `Author string`, `Score *int`, `Selftext string`, `URL string`, `Flat bool`, `Comments []redditComment`.
    `Author` holds a bare username with no `u/` prefix.
    `Score` is a pointer so the renderer can tell "zero points" apart from "this source has no scores".
    `Flat` is set explicitly by each tier and states that the source cannot express reply structure.
  - `type redditComment struct` with fields `Author string`, `Score *int`, `Body string`, `Replies []redditComment`.
    Document that only one level of `Replies` is ever rendered.
  - `func formatRedditThread(post redditPost, sourceURL string) string` — moved out of `redditoauth.go` and re-signatured.
    It no longer returns an error: the "response carried no post" validation moves to `redditPostFromListings` below.
    It renders, in order: `# <Title>`;
    a metadata line;
    `Source: <sourceURL>`;
    `post.Selftext` when non-empty, else `Link: <post.URL>` when `post.URL` is non-empty;
    and, only when `post.Comments` is non-empty, a comments heading followed by the comments.
    Preserve the existing trailing-newline trimming (`strings.TrimRight(b.String(), "\n")`).
  - `func redditPostFromListings(listings []redditListing) (redditPost, error)` — the OAuth mapping.
    It returns an error when `listings` is empty or when `listings[0]` has no `"t3"` child, carrying the same two messages `formatRedditThread` returns today so `fetchRedditOAuthThread`'s failure text is unchanged.
    It maps the post's `Title`, `Subreddit`, `Author`, `Selftext`, `URL`, sets `Score` to a pointer to the post's `int` score, and sets `Flat: false`.
    It walks `listings[1]`'s children when present, skipping any child whose `Kind` is not `"t1"`, and appends one `redditComment` per remaining child with `Score` pointing at that comment's score and `Replies` built the same way from `redditReplies(child.Data.Replies)`, again skipping non-`"t1"` children.
    It **never truncates** — every comment and every reply it parsed is handed over, so the caps live in exactly one place.

  Rendering rules that must hold exactly:
  - Metadata line with a non-nil `Score`: `Reddit | r/%s | %d points | by u/%s`.
    With a nil `Score` the points segment is omitted entirely, giving `Reddit | r/%s | by u/%s` — not `0 points`, not `? points`.
  - Comments heading: `## Top Comments` when `post.Flat` is false, `## Comments` when it is true.
    The heading is chosen from `Flat` alone.
    Do not derive it from whether every comment's `Replies` is nil — a genuinely reply-less OAuth thread is common and must still render `## Top Comments`.
  - Comment header with a non-nil `Score`: `**u/%s** (%d points):` followed by a newline and the body.
    With a nil `Score`: `**u/%s**:` followed by a newline and the body.
  - Reply line: `> **u/%s**: %s`, one level deep only, exactly as today.
  - `maxTopComments` is applied by the formatter in two places: at most `maxTopComments` entries of `post.Comments` are rendered, and at most `maxTopComments` entries of each rendered comment's `Replies`.
  - Every other byte of the OAuth-path output stays as it is today — spacing, blank lines, and the blank line emitted after a comment that rendered at least one reply.

  In `plugins/prowler/redditoauth.go`:
  - Delete the `formatRedditThread` declaration and its doc comment;
    keep `redditListing`, `redditChild`, `redditThing`, and `redditReplies` here, since they model the OAuth JSON wire format and belong with the OAuth client.
  - In `fetchRedditOAuthThread`, replace the closing `return formatRedditThread(listings, rawURL)` with a call to `redditPostFromListings(listings)`, returning its error unchanged on failure and otherwise returning `formatRedditThread(post, rawURL), nil`.
    `rawURL` — the caller's original URL, never the rewritten `oauth.reddit.com` URL — stays the `sourceURL` argument.
  - Update the file's opening doc comment: it no longer performs JSON-to-markdown formatting itself, it maps decoded listings onto the shared representation in `redditformat.go`.

  In `plugins/prowler/redditoauth_test.go`:
  - Move `TestFormatRedditThread` and `TestFormatRedditThread_GoldenOAuthOutput`, plus the `updateRedditGolden` flag var from card 1, into the new `plugins/prowler/redditformat_test.go`, giving that file a doc comment stating it exercises the shared representation and renderer.
    Adapt both to the new call shape — decode the fixture, call `redditPostFromListings`, then `formatRedditThread(post, sourceURL)`.
    Keep every existing assertion in `TestFormatRedditThread` intact, including the maxTopComments cap counts, the "no nested reply's own reply" assertion, and the quiet-thread "no Top Comments heading with zero comments" subtest.
  - Leave the rest of `redditoauth_test.go` — credentials, User-Agent, token, URL, and `fetchRedditOAuthThread` tests — where it is.

  The golden test must pass unchanged against the file committed in card 1.
  Do not regenerate the golden file in this card;
  if it fails, the refactor changed the output and the refactor is what is wrong.
- **Commit:** `refactor(prowler): render both reddit tiers through one tier-neutral formatter`

### Card 3: Cover the representation's new rendering rules

- **Context:**
  - `plugins/prowler/redditformat.go`
  - `plugins/prowler/reddit.go`
  - `plugins/prowler/redditoauth.go`
- **Edits:**
  - `plugins/prowler/redditformat_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add the cases the moved-over OAuth tests cannot reach, all against hand-built `redditPost` values so they need no fixture.

  - Nil-score rendering: a `redditPost` with `Score == nil` renders a metadata line with no points segment;
    a `redditComment` with `Score == nil` renders `**u/name**:` with no points segment.
    A `Score` pointing at `0` still renders `0 points` at both post and comment level — this is the assertion that fails if anyone reaches for a zero-value sentinel instead of a pointer.
  - Heading discriminator: `Flat: false` renders `## Top Comments`, `Flat: true` renders `## Comments`, and — the case that matters — a `Flat: false` post whose comments all have nil `Replies` still renders `## Top Comments`.
  - Cap placement: a synthetic `redditPost` carrying `maxTopComments + 5` comments renders exactly `maxTopComments` of them;
    a comment carrying `maxTopComments + 5` replies renders exactly `maxTopComments` of them.
    Separately, `redditPostFromListings` applied to a synthetic `[]redditListing` holding `maxTopComments + 5` `"t1"` children returns all of them untruncated, proving the mapping does not cap.
    `testdata/reddit-thread.json` cannot detect a misplaced cap on its own, which is why these are synthetic.
  - Link rendering: a post with an empty `Selftext` and a non-empty `URL` renders a `Link: <url>` line;
    a post with a non-empty `Selftext` renders the selftext and no `Link:` line even when `URL` is also set.
  - Mapping kind-filtering: `redditPostFromListings` drops `"more"` pagination placeholders at both top level and reply level, and returns its documented error for an empty `listings` slice and for a first listing with no `"t3"` child.
- **Commit:** `test(prowler): cover nil scores, flat headings, and cap placement`

## Batch Tests

`verify: go -C plugins/prowler test .` runs the whole `plugins/prowler` package, which is the module's only Go package.
The files this batch's cards create or change and that the command covers are `plugins/prowler/redditformat_test.go` (the moved `TestFormatRedditThread`, the golden regression, and card 3's new cases) and `plugins/prowler/redditoauth_test.go` (unchanged credential/token/URL/fetch coverage, which must keep passing across the formatter re-signature).
Every other existing test in the package — `reddit_test.go`, `fetch_test.go`, `hackernews_test.go`, `htmltext_test.go` — is a regression guard here: none of them should need editing in this batch, and one of them failing means the refactor leaked outside the OAuth path.

The decisive gate is the golden file `plugins/prowler/testdata/reddit-thread-golden.md`, captured in card 1 from the pre-refactor formatter and asserted byte-for-byte in card 2.
It is the only thing that proves the neutral-representation refactor changed nothing observable on the credentialed path.

The command runs fully offline in well under a second;
no card in this batch touches a `//go:build integration` file, so no tag flag is needed.
