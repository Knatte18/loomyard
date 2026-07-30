# Batch: fetch-binary

```yaml
task: 'prowler: installable Claude Code plugin (Go), hosted in LoomYard'
batch: fetch-binary
number: 1
cards: 7
verify: cd plugins/prowler && go test ./...
depends-on: []
```

## Batch Scope

This batch delivers the complete prowler fetch binary as a self-contained nested Go module at `plugins/prowler/` (`package main`, `module github.com/Knatte18/loomyard/plugins/prowler`, `go 1.26`). It ports weblens' fetch pipeline: Reddit `.json` special-case → static fetch + Readability → body-text fallback → headless-Chrome (chromedp) fallback, multi-URL joined with `\n\n---\n\n`, output written to a uniquely-named file under the scratch output directory, whose absolute path is the binary's sole stdout line. Deps: `github.com/chromedp/chromedp`, `github.com/go-shiori/go-readability`, `github.com/PuerkitoBio/goquery`. Cards are ordered bottom-up (pure helpers → cascade → browser+main). Dependencies enter `go.mod`/`go.sum` incrementally: each card that first imports an external dep runs `go get <dep>` (which records the require directive and its `go.sum` hashes), and Card 7 runs a final `go mod tidy`; a card that imports only the standard library adds nothing. Verification is at the batch boundary: the whole module compiles and its unit tests pass at the batch `verify:` (`cd plugins/prowler && go test ./...`). The external interface batch 2 consumes: a built binary at module root invoked as `prowler <url1> [url2...]` that prints one output-file path to stdout.

Batch-local decisions (differ from / refine `## Shared Decisions`):
- **Injection seam for testability:** the `fetcher` struct (defined in Card 1 so no later card forward-references it) carries two function fields — `do func(*http.Request) (*http.Response, error)` (the raw HTTP transport, so callers apply their own headers) and `browser func(ctx context.Context, url string) (string, bool)`. Both the static path and the Reddit path issue their requests through `f.do`, so the whole cascade — including the Reddit branch — is unit-tested with a stubbed `do` (no network) and a stubbed `browser` (no Chrome), with no `exec.Command` in any test file. Production wires the real transport and the chromedp browser via `newFetcher()`, defined in Card 7 alongside `browser.go`.
- **Readability output:** use go-readability's `Article.TextContent` (already plain text) and `Article.Title` directly; the `≥100`-char usability threshold is applied to `TextContent`.
- **Concurrency:** `runAll` (Card 7) fetches multiple URLs concurrently (like weblens' `Promise.all`) or sequentially — either is acceptable. Per-URL errors are captured inline as content, never fatal to siblings.

## Cards

### Card 1: Nested module init + shared HTTP headers + fetcher seam type

- **Context:**
  - `_mill/discussion.md`
- **Edits:** none
- **Creates:**
  - `plugins/prowler/go.mod`
  - `plugins/prowler/go.sum`
  - `plugins/prowler/headers.go`
  - `plugins/prowler/fetcher.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `go.mod` with exactly `module github.com/Knatte18/loomyard/plugins/prowler` and `go 1.26` and NO `require` lines yet — external deps are added by the cards that first import them via `go get` (see the Batch Scope incremental-deps note), keeping every intermediate commit buildable with the stdlib-only cards. Create `go.sum` as an empty file (it is populated by the `go get` calls in Cards 6 and 7 and the final `go mod tidy`; commit the final version). Create `headers.go` (`package main`) exposing: a `browserUA` string constant equal to `Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36`; a `defaultHeaders()` helper returning the exact weblens header set (`User-Agent: browserUA`, `Accept: text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8`, `Accept-Language: en-US,en;q=0.9,nb;q=0.8`, `Accept-Encoding: gzip, deflate, br`, `Cache-Control: no-cache`, `DNT: 1`); and a package-level `*http.Client` (`httpClient`) that follows redirects (stdlib default) with a ~60s `Timeout`. Create `fetcher.go` (`package main`) defining the injection seam so no later card forward-references it: `type fetcher struct { do func(*http.Request) (*http.Response, error); browser func(ctx context.Context, url string) (string, bool) }`. All of `headers.go`/`fetcher.go` are stdlib-only and compile with the empty `go.sum`. This card contains no `*_test.go`.
- **Commit:** `feat(prowler): init nested go module, http headers, and fetcher seam`

### Card 2: Output-file writer

- **Context:**
  - `_mill/discussion.md`
- **Edits:** none
- **Creates:**
  - `plugins/prowler/outfile.go`
  - `plugins/prowler/outfile_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `outfile.go` (`package main`): add `slugForURL(raw string) string` deriving a descriptive slug from a URL — parse with `net/url`, take host + the first non-empty path segment, lowercase, replace every run of non-`[a-z0-9]` characters with a single `-`, trim leading/trailing `-`, truncate to ~40 chars, and fall back to `page` when the result is empty or the URL is unparseable. Add `writeOutput(firstURL, content string) (string, error)` that `os.MkdirAll(".scratch", 0o755)`, creates the file via `os.CreateTemp(".scratch", "prowler-"+slugForURL(firstURL)+"-*.md")`, writes `content`, closes it, and returns `filepath.Abs` of the created name. `os.CreateTemp`'s `*` placeholder supplies the collision-free random suffix (safe for parallel agents and same-agent repeat calls). In `outfile_test.go` (`package main`, pure — no banned substrings per the guard-cleanliness decision): table-test `slugForURL` (host+path slug, non-alnum→hyphen collapse, truncation, empty/unparseable→`page`); assert `writeOutput` returns distinct absolute paths across many concurrent goroutine calls, that each file exists and contains exactly the passed content, and that names match `prowler-<slug>-*.md`. Use `t.Chdir(t.TempDir())` so the scratch output directory is created under a temp cwd.
- **Commit:** `feat(prowler): unique .scratch output-file writer`

### Card 3: HTML-to-text normalization

- **Context:**
  - `_mill/discussion.md`
- **Edits:**
  - `plugins/prowler/go.mod`
  - `plugins/prowler/go.sum`
- **Creates:**
  - `plugins/prowler/htmltext.go`
  - `plugins/prowler/htmltext_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** First run `go get github.com/PuerkitoBio/goquery` — this card is the first to import an external dep (goquery), so the command adds its require directive and `go.sum` hashes; commit the updated `go.mod`/`go.sum` with this card. In `htmltext.go` (`package main`): add `htmlToText(fragment string) string` using goquery (`goquery.NewDocumentFromReader` over `<div>`+fragment+`</div>`): remove all `script, style, noscript` elements, take the div's text, then normalize whitespace exactly as weblens does — collapse `[ \t]+` to a single space, collapse `\n[ \t]+` to `\n`, collapse 3-or-more newlines to `\n\n`, and trim. Add `stripToBodyText(fullHTML string) string`: parse the full document with goquery, remove `script, style, noscript, nav, header, footer`, and return `htmlToText` of the `body`'s inner HTML (empty string when there is no body). In `htmltext_test.go` (`package main`, pure): fixture-driven tests for script/style/noscript removal, whitespace collapse, blank-line collapse, and the extra `nav/header/footer` stripping in `stripToBodyText`.
- **Commit:** `feat(prowler): html-to-text normalization helpers`

### Card 4: Chrome executable discovery

- **Context:**
  - `_mill/discussion.md`
- **Edits:** none
- **Creates:**
  - `plugins/prowler/chrome.go`
  - `plugins/prowler/chrome_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `chrome.go` (`package main`): add `findChromeExecutable() string` that checks candidates in order and returns the first that exists (via `os.Stat`), else `""`. Candidate order mirrors weblens exactly: the `CHROME_PATH` env var (only if set and non-empty) first, then these literal filesystem locations in order — "C:/Program Files/Google/Chrome/Application/chrome.exe", "C:/Program Files (x86)/Google/Chrome/Application/chrome.exe", "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome", "/usr/bin/google-chrome", "/usr/bin/chromium-browser". In `chrome_test.go` (`package main`, pure — uses `os.Stat`/`t.Setenv`/`os.WriteFile`, none of which are banned substrings): create fake executables in `t.TempDir()`, assert `CHROME_PATH` wins when set to an existing file, assert candidate-list ordering, assert `""` when nothing exists. Do not shell out.
- **Commit:** `feat(prowler): cross-platform chrome discovery`

### Card 5: Reddit `.json` path

- **Context:**
  - `_mill/discussion.md`
  - `plugins/prowler/fetcher.go`
- **Edits:** none
- **Creates:**
  - `plugins/prowler/reddit.go`
  - `plugins/prowler/reddit_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `reddit.go` (`package main`): add `isRedditUrl(url string) bool` matching `^https?://(www\.|old\.)?reddit\.com` (compiled `regexp`). Add `toRedditJsonUrl(url string) string` that strips a single trailing slash and appends a `.json` suffix. Define JSON structs for Reddit's API: a thing wrapper `{Kind string; Data ...}` and a data struct with fields `Title, Subreddit string; Score, NumComments int (json:"num_comments"); Selftext, URL, Author, Body string`. Add `fetchReddit(ctx context.Context, f fetcher, url string) (out string, handled bool)` — it takes the `fetcher` (defined in Card 1) and issues its request through `f.do` so it shares the cascade's injectable transport (this is what makes the Reddit branch testable with a stubbed `do`): build an `http.NewRequestWithContext(ctx, GET, toRedditJsonUrl(url), nil)` — if that construction returns an error (e.g. a malformed URL), treat it identically to a transport error: return (`"# Error fetching "+url+"\n\n"+err`, `true`) without calling `f.do`; otherwise set only `User-Agent: prowler-reader/1.0` on it (NOT the browser `defaultHeaders`), and call `f.do(req)`; on transport error return (`"# Error fetching "+url+"\n\n"+err`, `true`); on non-2xx return (`"# Error fetching "+url+"\n\nHTTP <status>"`, `true`); read the `Content-Type` — if it does not contain `json`, return `("", false)` (fall through to HTML extraction, matching weblens' behavior on Reddit search/HTML responses); read the body and, if it fails to JSON-parse, return `("", false)`; if the top-level JSON is an array, format via `formatRedditPost`, else via `formatRedditSubreddit`; when the formatter yields empty, return a `"# "+url+"\n\nCould not parse Reddit ..."` string with `handled=true`. `formatRedditPost(raw json.RawMessage) string`: reproduce weblens — `# <title>`, then `r/<subreddit> | <score> points | <num_comments> comments`, then `selftext` (or `Link: <url>` when no selftext), then a `## Top Comments` section listing up to the first 20 `t1`-kind comments as `**<author>** (<score> pts):\n<body>`. `formatRedditSubreddit(raw json.RawMessage) string`: reproduce weblens — `# r/<subreddit>`, then one `- **<title>** (<score> pts, <num_comments> comments)` bullet per post with an optional 200-char `selftext` snippet (newlines→spaces, ellipsis when truncated). In `reddit_test.go` (`package main`, pure — no banned substrings): table-test `isRedditUrl` (www/old/plain reddit hosts true, non-reddit false), `toRedditJsonUrl` (trailing-slash and no-slash), `formatRedditPost` (title/subreddit/score/comments, selftext vs link, empty-children→empty), and `formatRedditSubreddit` (listing render, empty→empty) — feed the formatters in-memory JSON fixture bytes; no network. Additionally test `fetchReddit` directly by constructing a `fetcher` with a stubbed `do` (no network), covering each of its branches: a `do` transport error → (`# Error fetching …`, `handled=true`); a non-2xx status → (`# Error fetching … HTTP <status>`, `handled=true`); a `Content-Type` without `json` → (`""`, `handled=false`); a `json` Content-Type but unparseable body → (`""`, `handled=false`); and a parseable-but-empty payload (no post/no children) → (`# … Could not parse Reddit …`, `handled=true`).
- **Commit:** `feat(prowler): reddit .json fetch and formatting`

### Card 6: Static fetch + extraction cascade

- **Context:**
  - `_mill/discussion.md`
  - `plugins/prowler/headers.go`
  - `plugins/prowler/fetcher.go`
  - `plugins/prowler/htmltext.go`
  - `plugins/prowler/reddit.go`
- **Edits:**
  - `plugins/prowler/go.mod`
  - `plugins/prowler/go.sum`
- **Creates:**
  - `plugins/prowler/fetch.go`
  - `plugins/prowler/fetch_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** First run `go get github.com/go-shiori/go-readability` (this card newly imports go-readability; goquery is already a dependency from Card 3, and go-readability also pulls it transitively). In `fetch.go` (`package main`): the `fetcher` struct is already defined in `fetcher.go` (Card 1) — do NOT redefine it here. Add `fetchPage(ctx context.Context, f fetcher, url string) string` reproducing weblens' `fetchPage` cascade: (1) if `isRedditUrl(url)`, call `fetchReddit(ctx, f, url)`; if `handled` is true, return its output, else fall through; (2) build an `http.NewRequestWithContext(ctx, GET, url, nil)` — if that construction returns an error (e.g. a malformed URL), treat it identically to a transport error and return `"# Error fetching "+url+"\n\n"+err` without calling `f.do`; otherwise apply `defaultHeaders()` to it and call `f.do(req)` — on transport error return `"# Error fetching "+url+"\n\n"+err`, on non-2xx return `"# Error fetching "+url+"\n\nHTTP <status>"`; (3) read the body, strip `<style>/<script>/<noscript>` blocks (regexp, as weblens does), run go-readability (`readability.FromReader(bytes.NewReader(cleaned), parsedURL)`); (4) if Readability succeeds and `article.TextContent` is `≥100` chars, return `"# "+article.Title+"\n\nSource: "+url+"\n\n"+article.TextContent`; but if it succeeds with `TextContent < 100`, attempt `f.browser(ctx, url)` and return that when non-empty, else return the short readability result anyway (mirror weblens lines 247–252); (5) if Readability fails, try `stripToBodyText` on the raw HTML and return `"# "+url+"\n\n"+text` when `>100`; else try `f.browser(ctx, url)` and return it when non-empty; else return `"# "+url+"\n\nCould not extract readable content from this page."`. Do NOT define `newFetcher` here — it lives in Card 7 (it references `fetchWithBrowser`). In `fetch_test.go` (`package main`, pure — inject a stub `do` and stub `browser`, no network, no Chrome, no banned substrings): construct a `fetcher` whose `do` returns canned `*http.Response`s keyed by request URL and assert each cascade branch — readability-usable page returns the titled/sourced form; a `<100`-char readability page triggers the browser stub; a non-article page with `>100` body text returns the body-text form; a page with neither triggers the browser stub; a transport error and a non-2xx both return the `# Error fetching` form without invoking the browser; a Reddit URL routes through the Reddit path (the stub `do` returns a `Content-Type: application/json` Reddit `.json` body, which reaches `fetchReddit` because it shares `f.do`). Also cover the double-fallback-failure cases where the stubbed `browser` itself returns `("", false)`: for the `<100`-char-readability branch, assert `fetchPage` then returns the short readability text anyway (weblens lines 247–252); for the no-extraction branch, assert it returns the final `"# "+url+"\n\nCould not extract readable content from this page."` message. The test constructs its own `fetcher` — it never calls `newFetcher`.
- **Commit:** `feat(prowler): static fetch and extraction cascade`

### Card 7: Headless-Chrome fallback + main entrypoint

- **Context:**
  - `_mill/discussion.md`
  - `plugins/prowler/headers.go`
  - `plugins/prowler/fetcher.go`
  - `plugins/prowler/chrome.go`
  - `plugins/prowler/htmltext.go`
  - `plugins/prowler/fetch.go`
  - `plugins/prowler/outfile.go`
- **Edits:**
  - `plugins/prowler/go.mod`
  - `plugins/prowler/go.sum`
- **Creates:**
  - `plugins/prowler/browser.go`
  - `plugins/prowler/main.go`
  - `plugins/prowler/main_test.go`
  - `plugins/prowler/browser_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** First run `go get github.com/chromedp/chromedp` (first import of chromedp). In `browser.go` (`package main`): add `fetchWithBrowser(ctx context.Context, url string) (string, bool)`. Resolve Chrome via `findChromeExecutable()`; when empty, log `prowler: Chrome not found, skipping browser fallback` to stderr and return `("", false)`. Otherwise log `prowler: falling back to headless Chrome for <url>` to stderr and drive chromedp: build a `chromedp.NewExecAllocator` with `chromedp.ExecPath(chromePath)` plus the flags `headless`, `no-sandbox`, `disable-gpu`, `disable-dev-shm-usage` (compose from `chromedp.DefaultExecAllocatorOptions`); wrap with an overall `context.WithTimeout` of ~60s and a per-navigation deadline of ~30s; `chromedp.Run` a `Navigate(url)`, then a render-settle step **before** capturing HTML — `chromedp.WaitReady("body", chromedp.ByQuery)` followed by `chromedp.Sleep(~2s)` — as a deliberate parity substitute for weblens/Puppeteer's `networkidle0` (chromedp has no built-in network-idle action; `Navigate` returns at the `load` event, before JS-driven content renders, which is exactly the content the browser fallback exists to capture). Then capture the rendered HTML via `chromedp.OuterHTML("html", &html, chromedp.ByQuery)`; on any error log to stderr and return `("", false)`; on success run go-readability on the rendered HTML and return `"# "+article.Title+"\n\nSource: "+url+"\n\n"+article.TextContent` when an article parses, else `stripToBodyText` and return `"# "+url+"\n\n"+text` when `>100`, else `("", false)`; always release the allocator/context (defer cancels). No `LookPath("gh")` or `exec.Command(…, "gh")` appears here (guard-cleanliness). In `main.go` (`package main`): add `newFetcher() fetcher` wiring the real seam — `do: httpClient.Do`, `browser: fetchWithBrowser`. Add `runAll(ctx context.Context, f fetcher, urls []string) string` (the testable orchestration extracted out of `main`): fetch every URL via `fetchPage(ctx, f, url)` (concurrently or sequentially) and join the per-URL results with `"\n\n---\n\n"`; a per-URL failure is already captured inline as its `# Error fetching …` result string, so a bad URL never aborts its siblings. `main()` reads `os.Args[1:]`; when empty, print `Usage: prowler <url> [url2] [url3]...` to stderr and `os.Exit(1)`; otherwise `joined := runAll(context.Background(), newFetcher(), os.Args[1:])`, `writeOutput(os.Args[1], joined)`, and on success print the returned absolute path to **stdout** as the only stdout output (all diagnostics stay on stderr); on a `writeOutput` error, log to stderr and `os.Exit(1)`. In `main_test.go` (`package main`, pure — inject a stub `fetcher`, no banned substrings): drive `runAll` with a stub `do` returning canned bodies for several URLs and assert the results are joined with exactly `"\n\n---\n\n"` in input order, and that one URL whose stub `do` returns a transport error / non-2xx yields its inline `# Error fetching …` segment without dropping or aborting the sibling URLs' segments. Finally run `go mod tidy` and commit the finalized `go.mod`/`go.sum`. In `browser_integration_test.go` add a leading `//go:build integration` line (first non-empty line) with a test that drives `fetchWithBrowser` against a known-readable page guarded by `CHROME_PATH`/discovery — it must contain no `exec.Command`/`exec.CommandContext`/`gitexec.RunGit`/`lyxtest.` substring (chromedp's process spawn lives inside the library).
- **Commit:** `feat(prowler): headless-chrome fallback and main entrypoint`

## Batch Tests

`verify: cd plugins/prowler && go test ./...` — a Go project, so no `PYTHONPATH=` prefix; the `cd` scopes the run to the nested module (the parent `go test ./...` deliberately excludes it). This runs only the untagged unit tests (`outfile_test.go`, `htmltext_test.go`, `chrome_test.go`, `reddit_test.go`, `fetch_test.go`, `main_test.go`), which are pure — no network, no Chrome, no subprocess spawn — so the command is deterministic and offline. The `//go:build integration` browser/network tests are excluded (run manually with `-tags integration` and a local Chrome). Separately, the repo-wide done-gate (`go test ./...` from the repo root, configured in `mill-config.yaml`) exercises the three parent-module grep guards against prowler's committed files and is the real enforcement of the guard-cleanliness decision — every prowler `*_test.go` here is written to contain none of the banned substrings and every production `.go` contains no `gh` shell-out, so the done-gate stays green.
