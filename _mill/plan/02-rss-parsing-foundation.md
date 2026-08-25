# Batch: rss-parsing-foundation

```yaml
task: "Add RSS-based Reddit read tier"
batch: "rss-parsing-foundation"
number: 2
cards: 4
verify: go -C plugins/prowler test .
depends-on: [1]
```

## Batch Scope

This batch delivers everything the RSS tier needs that involves no network and no shared state: the committed Atom fixtures, the canonical `.rss` URL builder, the Reddit-specific HTML-to-markdown converter, and the Atom parser plus its mapping onto batch 1's `redditPost`.
Every declaration here is a pure function or a plain type, so every card is a straightforward test-first unit.
Keeping the impure parts out means batch 2's tests need no limiter stub at all — nothing in it reaches the process-wide limiter, which batch 3 introduces.

The external interface batch 3 consumes is `redditRSSURL`, `redditAtomFeed`/`redditAtomEntry`, `parseRedditFeed`, `redditPostFromFeed`, and `formatRedditListing`.

Batch-local decision: `redditHTMLToMarkdown` runs its rewrites with `goquery` rather than regexes, because the trailer, the thumbnail `<table>`, and the anchor set it must handle are real HTML the module already has a parser for, and `goquery` is already a direct dependency of this module.

## Cards

### Card 4: Commit trimmed Atom fixtures from the existing live capture

- **Context:**
  - `plugins/prowler/testdata/reddit-thread.json`
  - `plugins/prowler/blockdetect_test.go`
- **Edits:** none
- **Creates:**
  - `plugins/prowler/testdata/reddit-thread.rss`
  - `plugins/prowler/testdata/reddit-listing.rss`
  - `plugins/prowler/testdata/reddit-rss-notfound.rss`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Produce the three Atom fixtures by trimming the live captures already present in this worktree at `.scratch/reddit-rss-capture/`.
  Do not issue any live Reddit request in this card — the captures exist precisely so no request is needed, and the endpoint allows about one per minute.

  The captures are single-line XML documents.
  Run exactly this script from the worktree root to produce all three fixtures:

```
python3 - <<'PY'
import pathlib, re

src = pathlib.Path(".scratch/reddit-rss-capture")
dst = pathlib.Path("plugins/prowler/testdata")

def trim(name, out, keep):
    text = src.joinpath(name).read_text(encoding="utf-8")
    head, sep, rest = text.partition("<entry>")
    entries = re.findall(r"<entry>.*?</entry>", sep + rest, re.S)
    kept = [entries[i] for i in keep]
    body = "\n".join([head.rstrip()] + kept + ["</feed>"])
    dst.joinpath(out).write_text(body + "\n", encoding="utf-8")

trim("thread.rss", "reddit-thread.rss", [0, 1, 2, 3, 4])
trim("subreddit.rss", "reddit-listing.rss", [0, 1, 2])
dst.joinpath("reddit-rss-notfound.rss").write_text(
    src.joinpath("notfound.rss").read_text(encoding="utf-8").rstrip() + "\n",
    encoding="utf-8",
)
PY
```

  The `head` slice keeps the XML declaration and every feed-level element (`<category>`, `<title>`, `<link>`, `<subtitle>`, `<id>`, `<updated>`, `<icon>`, `<logo>`) byte-for-byte;
  the only edit is dropping the entries that are not kept and re-adding the closing `</feed>`.
  Newlines are inserted between top-level children only, never inside an element, so no `<content>` payload is altered.

  Then confirm, before committing:
  - `plugins/prowler/testdata/reddit-thread.rss` has exactly 5 `<entry>` elements, its first entry's `<id>` is `t3_1vxc255`, and the remaining four ids all start with `t1_`.
  - `plugins/prowler/testdata/reddit-listing.rss` has exactly 3 `<entry>` elements, every id starts with `t3_`, and the entry with id `t3_1vx8uvc` is present and contains no `SC_OFF` marker — that is the marker-less link post several later tests depend on, and its `[link]` anchor points at `https://konradreiche.com/blog/excessive-nil-pointer-checks-in-go/` while its `[comments]` anchor points at the reddit permalink.
  - `plugins/prowler/testdata/reddit-rss-notfound.rss` has zero `<entry>` elements and a `<title>` of `announcements: page not found`.
  - All three parse as well-formed XML.

  If `.scratch/reddit-rss-capture/` is missing or any of the three source files is absent, stop and report it rather than fabricating a fixture or fetching a replacement — hand-authored Atom would defeat the whole point of these files, which is to carry Reddit's actual escaping and wrappers.
- **Commit:** `test(prowler): commit trimmed reddit atom fixtures from the live capture`

### Card 5: Canonical .rss URL construction

- **Context:**
  - `plugins/prowler/redditoauth.go`
  - `plugins/prowler/reddit.go`
- **Edits:** none
- **Creates:**
  - `plugins/prowler/redditrss.go`
  - `plugins/prowler/redditrss_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `plugins/prowler/redditrss.go` with a file-level doc comment stating that it implements Reddit's unauthenticated `.rss` read tier — URL canonicalisation, per-IP pacing, Atom parsing, and markdown rendering — and that it is the tier that needs no credentials and no app registration.
  In this card the file contains only the URL builder;
  later cards in this batch and in batch 3 add to it.

  Add `func redditRSSURL(rawURL string) (string, error)`, built on `net/url` and mirroring `redditOAuthURL`'s structure in `redditoauth.go`.
  Its steps, in this exact order:

  1. `url.Parse(rawURL)`;
     on failure return an error of the form `parse reddit URL %q: %w`.
  2. Return an error `reddit URL %q has no path` when the parsed path is empty.
  3. Force `Scheme` to `https`.
  4. Force `Host` to `www.reddit.com`.
  5. Clear `RawQuery` and `Fragment`.
  6. Strip a trailing `.rss` from the path when present.
  7. Ensure the path ends with exactly one `/`.
  8. Append `.rss`.

  Step 6 is what makes the function idempotent: `redditRSSURL` applied to its own output returns that output unchanged.
  Without it, normalising the trailing slash first would turn an already-`.rss` path into `/.rss/.rss`.
  Document that reasoning on the declaration.
  Normalising to `www.reddit.com` — rather than preserving `old.` — is deliberate: `Matches` in `reddit.go` accepts bare, `www.`, and `old.` hosts, and collapsing them onto one host keeps error strings and fixtures singular.

  Create `plugins/prowler/redditrss_test.go` with a doc comment stating it exercises the RSS tier offline, no network.
  Add a table-driven `TestRedditRSSURL` covering: a bare `reddit.com` host;
  a `www.` host;
  an `old.` host;
  an `http` scheme;
  a path with a trailing slash and one without;
  a URL carrying a query string and one carrying a fragment, both dropped;
  a path already ending in `.rss` with a trailing slash and one without, both returning unchanged output rather than `/.rss/.rss`;
  a subreddit path such as `https://www.reddit.com/r/golang/`;
  a URL with an empty path, expecting an error;
  and an unparseable URL, expecting an error.
  Add a separate idempotence subtest that feeds each success case's own output back through `redditRSSURL` and asserts the second result equals the first.
- **Commit:** `feat(prowler): add canonical reddit .rss URL construction`

### Card 6: Reddit HTML fragment to markdown

- **Context:**
  - `plugins/prowler/htmltext.go`
  - `plugins/prowler/testdata/reddit-thread.rss`
- **Edits:**
  - `plugins/prowler/redditrss.go`
  - `plugins/prowler/redditrss_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add `func redditHTMLToMarkdown(fragment string) string` to `plugins/prowler/redditrss.go`.
  It converts one Reddit Atom `<content>` payload — which is HTML, unlike the OAuth tier's Reddit-markdown bodies — into markdown, then delegates whitespace normalisation and tag stripping to the existing `htmlToText`.

  Do not modify `htmlToText` or anything else in `plugins/prowler/htmltext.go`.
  It is built on goquery's `.Text()`, so it discards every `href` and every block boundary;
  the generic fetch cascade and the Hacker News adapter both depend on exactly that behaviour, and links are the substance of the use case this tier exists for.
  Wrapping it is what preserves both.

  Implement three rewrites over the fragment with `goquery` before the `htmlToText` call:

  1. **Anchors become markdown links.**
     Replace each `<a href="X">Y</a>` with the literal text `[Y](X)`, where `Y` is the anchor's trimmed text.
     A relative `href` — Reddit emits `/r/golang` and `/u/name` — is resolved against `https://www.reddit.com` first, so the rendered link is openable.
     An anchor whose trimmed text already equals its resolved href collapses to the bare URL rather than `[url](url)`.
     An anchor with an empty `href` renders as its text alone.
  2. **Block boundaries become newlines.**
     `</p>`, `<br>`, and `</blockquote>` each become a blank-line break;
     each `<li>` is prefixed with `- ` and terminated by a single newline.
  3. Hand the rewritten fragment to `htmlToText`, whose `normalizeWhitespace` collapses the runs steps 1 and 2 leave behind.

  Document on the declaration why a bare `htmlToText` call is not enough, naming the concrete failure it prevents: a comment written `[the docs](https://example.com)` would otherwise arrive as the bare words `the docs` with the URL gone, and a five-paragraph post would arrive as one run-on line.

  Extend `plugins/prowler/redditrss_test.go` with a table-driven `TestRedditHTMLToMarkdown` covering: an absolute-href anchor becoming `[text](href)`;
  a relative `/r/golang` href absolutized to `https://www.reddit.com/r/golang`;
  an anchor whose text equals its href rendering as the bare URL and not `[url](url)`;
  `</p>` and `<br>` producing blank-line breaks;
  `<li>` items rendering as `- ` bullets;
  and nested markup inside an anchor's text.
  Add one regression subtest that reads `plugins/prowler/testdata/reddit-thread.rss`, pulls a real comment body containing an external link out of it, and asserts the converted output still contains that URL.
  Add one subtest asserting `htmlToText`'s own behaviour on an anchor-bearing fragment is unchanged — the URL is still dropped — so the shared helper is provably untouched.
- **Commit:** `feat(prowler): convert reddit atom HTML bodies to markdown, links intact`

### Card 7: Atom parsing and mapping onto the neutral representation

- **Context:**
  - `plugins/prowler/redditformat.go`
  - `plugins/prowler/redditoauth.go`
  - `plugins/prowler/reddit.go`
  - `plugins/prowler/blockdetect_test.go`
  - `plugins/prowler/testdata/reddit-thread.rss`
  - `plugins/prowler/testdata/reddit-listing.rss`
  - `plugins/prowler/testdata/reddit-rss-notfound.rss`
- **Edits:**
  - `plugins/prowler/redditrss.go`
  - `plugins/prowler/redditrss_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Add the Atom types, the parser, and both rendering mappings to `plugins/prowler/redditrss.go`, using stdlib `encoding/xml` only.

  Types:
  - `type redditAtomFeed struct` decoding the feed-level `<title>` and the feed-level `<category term=…>` attribute (the subreddit name), plus `Entries []redditAtomEntry` from `xml:"entry"`.
  - `type redditAtomEntry struct` decoding `<title>`, `<id>`, the `<author><name>` text, the `<content>` text, and the `<link href=…>` attribute.
    Document that `<id>` carries Reddit's fullname with its kind prefix — `t3_` for a post, `t1_` for a comment — and that this is the tier's only kind discriminator, mirroring `redditChild.Kind` on the OAuth side.
    Unmapped elements such as `media:thumbnail`, `<updated>`, and `<published>` are ignored by `encoding/xml` and need no fields.

  Parsing and helpers:
  - `func parseRedditFeed(body []byte) (redditAtomFeed, error)` — unmarshals with `encoding/xml`, wrapping a decode failure as `decode reddit atom feed: %w`.
  - `func redditRSSAuthor(name string) string` — trims a leading `/u/` or `u/` from an Atom author name, because the feed emits `/u/username` while the formatter emits its own `u/` prefix.
  - `func redditRSSBody(content string) string` — returns `redditHTMLToMarkdown` applied to the span strictly between the literal markers `<!-- SC_OFF -->` and `<!-- SC_ON -->`, and the empty string when either marker is absent.
    Never fall back to the whole `<content>`: a link post carries no markers and its entire content is a thumbnail `<table>` plus a `submitted by … [link] … [comments]` trailer, so a whole-content fallback would render that scaffolding as the post's body.
    4 of the 25 entries in the source capture take exactly this branch, so it is the common case, not an edge case.
  - `func redditRSSLinkURL(content, permalink string) string` — parses `content` with `goquery` and returns the `href` of the anchor whose trimmed text is `[link]`, but only when that href differs from `permalink`;
    otherwise returns the empty string.
    The permalink comparison is what stops a self-post's `[comments]`-only trailer from rendering as a `Link:` pointing back at the page the reader is already on.

  Mappings:
  - `func redditPostFromFeed(feed redditAtomFeed, sourceURL string) (redditPost, error)` — the thread mapping.
    Returns an error when `feed.Entries` is empty, and an error naming the offending id when the first entry's `<id>` does not start with `t3_`.
    Otherwise it maps the first entry to `redditPost`: `Title` from the entry's `<title>`, `Subreddit` from the feed's `<category term>`, `Author` via `redditRSSAuthor`, `Score: nil`, `Selftext` via `redditRSSBody`, `URL` via `redditRSSLinkURL` against the entry's own `<link href>`, and `Flat: true`.
    It appends one `redditComment` per remaining entry whose `<id>` starts with `t1_`, each with `Score: nil`, `Replies: nil`, `Author` via `redditRSSAuthor`, and `Body` via `redditRSSBody`.
    An entry whose body comes back empty is skipped rather than rendered as a blank comment.
    It never truncates — the `maxTopComments` caps live in `formatRedditThread` alone.
    `Flat` is set to `true` explicitly here, never inferred: the feed carries every comment as a sibling with no parent reference of any kind, so depth is genuinely unrecoverable and `## Top Comments` would be a false claim about entries that are not necessarily top-level.
  - `func formatRedditListing(feed redditAtomFeed, sourceURL string) string` — the non-thread mapping, for subreddit and user feeds.
    Renders `# <feed title>`, a blank line, `Source: <sourceURL>`, a blank line, then one `- ` bullet per entry giving the entry's title, its author via `redditRSSAuthor`, and its `<link href>`.
    Trims trailing newlines the same way `formatRedditThread` does.
    `sourceURL` is the caller's original URL, never the derived `.rss` URL, on this branch exactly as on the thread branch.

  Extend `plugins/prowler/redditrss_test.go`:
  - `TestParseRedditFeed` against `plugins/prowler/testdata/reddit-thread.rss`: 5 entries, first id `t3_1vxc255`, feed category term `golang`, remaining ids all `t1_`.
  - `TestRedditPostFromFeed` against the same fixture: the post title, the subreddit, the author with its `/u/` prefix stripped, a `Selftext` that contains the post's real body text and contains none of `submitted by`, `[link]`, or `[comments]`, four comments with their authors and bodies, `Score` nil at post and every comment level, `Replies` nil throughout, and `Flat` true.
  - Marker-absent link post: locate the entry with id `t3_1vx8uvc` in `plugins/prowler/testdata/reddit-listing.rss` — by id, not by index — build a single-entry `redditAtomFeed` from it, and assert `redditPostFromFeed` yields an empty `Selftext`, a `URL` equal to the external `[link]` href and not the reddit permalink, and that `formatRedditThread` on the result emits a `Link:` line and none of the trailer text.
  - Self-post `URL` suppression: the `t3_1vxc255` post's `[link]` anchor points at its own permalink, so its mapped `URL` is empty and `formatRedditThread` emits its selftext with no `Link:` line.
  - Zero entries: `redditPostFromFeed` on the parsed `plugins/prowler/testdata/reddit-rss-notfound.rss` returns an error.
  - Non-`t3_` first entry: a hand-built feed whose first entry's id is `t1_something` returns an error naming that id.
  - `TestFormatRedditListing` against `plugins/prowler/testdata/reddit-listing.rss`: an H1 from the feed title, a `Source:` line carrying the caller's original non-`.rss` URL, and one bullet per entry with title, author, and link.
- **Commit:** `feat(prowler): parse reddit atom feeds into the neutral thread representation`

## Batch Tests

`verify: go -C plugins/prowler test .` runs the module's single Go package.
The new coverage this batch adds all lives in `plugins/prowler/redditrss_test.go`: `TestRedditRSSURL` and its idempotence subtest (card 5), `TestRedditHTMLToMarkdown` including the untouched-`htmlToText` assertion (card 6), and `TestParseRedditFeed`, `TestRedditPostFromFeed`, `TestFormatRedditListing` and the marker-absent/self-post/zero-entry/non-`t3_` cases (card 7).
Card 4 contributes no test of its own;
its fixtures are what every card-7 test reads, and its own correctness is asserted by those tests failing loudly if a fixture was trimmed wrong.

No test in this batch reaches the process-wide limiter, because nothing in this batch issues a request — that is why `stubRedditRSSLimiter(t)` does not exist yet and is not called here.
Batch 3 introduces both.

The `plugins/prowler/redditformat_test.go` suite from batch 1 keeps running as a regression guard: card 7 adds a second producer of `redditPost` values, and the OAuth golden file must still match byte-for-byte afterwards.
The command is fully offline and no card in this batch touches a `//go:build integration` file.
