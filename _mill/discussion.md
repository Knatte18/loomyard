# Discussion: prowler: installable Claude Code plugin (Go), hosted in LoomYard

```yaml
task: 'prowler: installable Claude Code plugin (Go), hosted in LoomYard'
slug: prowler
status: discussing
parent: main
```

## Problem

Millhouse's `weblens` skill fetches web pages that Claude Code's built-in `WebFetch` cannot read — bot-blocked sites, paywalls, JS-rendered content, Reddit — by driving a real headless browser (Puppeteer/headless Chrome) plus Mozilla Readability extraction. It is a TypeScript/npm stack (`puppeteer-core`, `@mozilla/readability`, `linkedom`) that pulls in thousands of `node_modules` files, and it is used across **all** of the operator's Claude Code sessions today, not scoped to one repo.

**Why now:** the operator wants a Go-native replacement with the same cross-session reach, shipped fast. The lever is a proper, installable Claude Code plugin — not a repo-scoped project skill — so it keeps weblens' any-repo/any-session reach after install. Its source is to live in the **LoomYard** repo (explicit, repeated operator decision — *not* Millhouse), sourced from LoomYard's own repo instead of Millhouse's plugin repo.

## Scope

**In:**

- A new Claude Code plugin **`prowler`** whose source lives in the LoomYard repo at `plugins/prowler/1.0.0/`.
- A root **`.claude-plugin/marketplace.json`** in LoomYard (marketplace name `loomyard`) listing `prowler` as its first plugin — this makes LoomYard installable via `/plugin marketplace add` + `/plugin install prowler`.
- The plugin's own `.claude-plugin/plugin.json`, a `prowler` skill (`SKILL.md` + `INDEX.md`), a `settings.json` permissions block, a single wrapper `run.sh` (invoked via `bash` on all platforms, git-bash on Windows — no separate PowerShell wrapper), and the Go source for the fetch binary.
- A **separate nested Go module** (`plugins/prowler/1.0.0/go.mod`, module path `github.com/Knatte18/loomyard/plugins/prowler`) containing the fetcher: static-first fetch → Readability extraction → headless-Chrome fallback → body-text fallback, plus the Reddit `.json` special-case, multi-URL support, and cross-platform Chrome discovery.
- **Build-on-first-run**: the wrapper builds the Go binary into the plugin dir on first use if absent, under a lock, and reuses it thereafter.
- A `plugins/prowler/README.md` and Apache-2.0 `LICENSE` metadata.

**Out:**

- **Not** a `lyx` Cobra CLI module. No `internal/prowlerengine`/`internal/prowlercli`, no Cobra registration, no `Short`/`Long`, no help-tree/drift/registration/longlist tests, no `docs/overview.md` module-table entry, no `manifest/designs/prowler.md`. A future `lyx prowler` is a **separate, later, optional** task and must not be conflated with this one.
- No changes to LoomYard's `go.mod`/`go.sum` or the `internal/`/`cmd/` tree — prowler's deps stay isolated in the nested module.
- No changes to Millhouse's repo of any kind.
- No bundling of a Chrome/Chromium binary — a local browser remains a run-time prerequisite (same as weblens today; not a TypeScript-specific problem).
- No committed compiled binaries (LoomYard's `.gitignore` bans committing binaries; prowler honors this via build-on-first-run).

## Decisions

### plugin-placement-and-marketplace

- Decision: prowler's plugin source lives at `plugins/prowler/1.0.0/` under LoomYard's repo root (this dir holds `.claude-plugin/plugin.json`, `skills/`, `scripts/`, `settings.json`, the Go source, and the nested `go.mod`), with a new root `.claude-plugin/marketplace.json` (marketplace name `loomyard`, owner Knatte18) listing `prowler` v1.0.0 as its sole plugin (extensible later). The marketplace entry's `plugins[].source` is **`./plugins/prowler/1.0.0`** — the exact directory containing `.claude-plugin/plugin.json` (Claude Code resolves `source` to that path and reads the plugin manifest from `<source>/.claude-plugin/plugin.json`). Plan must confirm this path resolves correctly at install (`/plugin install prowler@loomyard`) before shipping.
- Rationale: `plugins/` does not collide with the lyx Go module's root layout (`cmd/`, `internal/`, `manifest/`, `docs/`). The root marketplace manifest is the mechanism that gives prowler cross-session reach (`/plugin marketplace add <loomyard-repo>` → `/plugin install prowler`).
- Note on the `<version>/` subdir: this is a **deliberate divergence** from Millhouse's convention, not a mirror of it. Millhouse's *source* repo is flat — `plugins/weblens/`, `plugins/mill/`, … with marketplace `source: ./plugins/<name>` and **no** `<version>/` subdir (verified at `/home/knatte/Code/millhouse/wts/millhouse/plugins/`). The `<version>/` path segment exists only under the install *cache* (`~/.claude/plugins/cache/<marketplace>/<name>/<version>/`), which Claude Code creates itself. prowler adds the `<version>/` subdir in-repo on purpose, to leave room for future side-by-side versions; the trade-off is that `source` must name the versioned path (`./plugins/prowler/1.0.0`), unlike Millhouse's flat `./plugins/<name>`.
- Rejected: a flat `plugins/prowler/` with no version subdir (diverges from Millhouse convention for no gain); a project-scoped `.claude/skills/prowler/` (auto-active only in LoomYard-rooted sessions — does **not** replace weblens' reach, the operator explicitly rejected this).

### separate-nested-go-module

- Decision: prowler's Go code is a **separate nested Go module** at `plugins/prowler/1.0.0/go.mod`, module path `github.com/Knatte18/loomyard/plugins/prowler`, `go 1.26`. It is NOT part of the main lyx module.
- Rationale: isolates the browser-automation dependency stack (chromedp, go-readability, goquery) from lyx's `go.mod`/`go.sum`. A nested module (a dir with its own `go.mod`) is excluded from the parent module's **`go build ./...` / `go test ./...`** (the `go` tool treats a nested `go.mod` as a separate module), so prowler's deps never enter lyx's dependency graph and its Go tests never run under lyx's `go test`.
- **Important correction — the nested module is NOT invisible to the two disk-walking grep guards.** `cmd/lyx/tierpurity_test.go` (`TestTierPurity_UntaggedTestsSpawnNothing`) and `cmd/lyx/hermeticenv_test.go` (`TestHermeticGitEnv_GitSpawningPackagesHaveTestMain`) resolve their walk root via `go env GOMOD` = the **parent** repo root and `filepath.WalkDir` it, skipping only `.git`, `_lyx`, `_mill`, `.scratch`, `.wiki`, `_raddle` (verified in `tierPuritySkipDirs`). They **descend into `plugins/prowler/1.0.0/` and read its `*_test.go` files**, because they scan the filesystem, not the Go package graph — the `go env GOMOD` exclusion applies to `go build/test`, not to these guards. Consequence for the plan (binding): **prowler's `*_test.go` files must contain NONE of the banned substrings** — `exec.Command`, `exec.CommandContext`, `gitexec.RunGit`, or any `lyxtest.*` (`lyxtest.Copy*`/`MustRun`/`SeedConfig`). The Hermetic guard is the tighter constraint: it scans **every** test file regardless of build tag and would demand a `HermeticGitEnv` call in any file it classifies as git-spawning — a call prowler cannot make (it can't import the parent module's `internal/lyxtest`). This is satisfiable at no cost because prowler's Go unit tests are pure (see Testing): none need to spawn a process or git. The build-lock/concurrency behavior (which would need a real shell-out) is verified by a **shell-script harness, not a Go `*_test.go`** — a non-`*_test.go` file is not scanned by either guard. We do **not** add `plugins/` to the guards' skip sets: that would edit `cmd/lyx` enforcement tests, contradicting this task's "no changes to internal/cmd tree" scope and weakening the guards for everyone.
- Rejected: placing the code under `internal/` in the main module (pulls chromedp/readability into lyx's dependency graph and under every enforcement sweep; contradicts the task's explicit "not a lyx module for now"); adding `plugins/` to the tierpurity/hermetic guards' skip sets (edits cmd/lyx, out of scope, weakens the guards).

### build-on-first-run

- Decision: a **single** wrapper `run.sh` (invoked via `bash` on all platforms — see the skill-contract decision) runs the prebuilt binary at `${CLAUDE_PLUGIN_ROOT}/bin/prowler[.exe]` if it exists; otherwise it `go build`s the nested module into that path **under a lock**, then runs it. If `go` is not on PATH, it exits non-zero with a clear message ("prowler: Go toolchain not found — install Go or add it to PATH"). A fresh plugin version installs into a fresh dir, so the binary auto-rebuilds on version bump.
- **stdout/stderr discipline (resolves the path-capture gap):** the wrapper emits **only** the binary's single stdout line (the output-file path — see the output decision) on stdout. Every diagnostic — build progress, `go build` errors, the "Go toolchain not found" message, lock-wait notices — goes to **stderr**. The skill must **check the wrapper's exit code before reading the path**: on non-zero exit, treat the run as failed (surface the stderr diagnostic), do not attempt to read a path. This prevents build output from being captured as a bogus "path" by `path=$(bash run.sh …)`.
- **Lock mechanism (portable, resolves the lock gap):** use an **atomic `mkdir` lock-directory** at `${CLAUDE_PLUGIN_ROOT}/bin/.build.lock` — `mkdir` is atomic on Linux, macOS, and Windows git-bash alike (unlike `flock`, which is absent on macOS/git-bash). Acquisition: loop `mkdir` with a short sleep, up to a bounded timeout (~120 s); on timeout, exit non-zero with a stderr message. Stale-lock recovery: if the lock dir exists but the binary is present and non-empty, another builder already finished — proceed to run. On a failed `go build`, remove the lock dir and the partial binary and exit non-zero (never leave a half-written binary that a later run would execute). `trap` cleanup removes the lock dir on any exit path.
- Rationale: LoomYard's `.gitignore` bans committing compiled binaries ("built with go build, never committed"). Build-on-first-run honors that norm while giving a compiled binary at run time. The operator's machines (Linux + Windows) are Go dev boxes, so the toolchain is present. The mkdir-lock prevents parallel sessions from racing the same build with no platform-specific dependency. This deliberately trades the proposal's "no toolchain at run time" wish for the repo's no-committed-binaries invariant — accepted by the operator.
- Rejected: committing per-platform binaries (violates the `.gitignore` norm, ~15–30 MB each in git, must rebuild+recommit on every change); `go run` at run time (still needs the toolchain, slower per-invocation, no persistent artifact); an OS-cache dir keyed by source hash (more moving parts than needed — a fresh version dir already forces a rebuild); `flock` (not portable to macOS/git-bash).

### full-weblens-parity

- Decision: v1 replicates weblens' behavior fully. Fetch pipeline per URL: (1) if the URL is Reddit, hit the `.json` endpoint and format post + top ~20 comments (or a subreddit listing); on non-JSON response fall through; (2) static `http.Get` with a real-browser User-Agent + headers; strip `<script>/<style>/<noscript>`; run Readability; (3) if Readability yields usable text (≥100 chars) return it; (4) else strip `script/style/noscript/nav/header/footer` and return body text if >100 chars; (5) else fall back to headless Chrome via chromedp (same nav, re-run Readability, then body-text fallback); (6) else return `# <url>\n\nCould not extract readable content`. Multi-URL input, results joined with `\n\n---\n\n`.
- Rationale: drop-in replacement — the operator relies on all of these paths today (Reddit especially, since Reddit hard-blocks HTML but leaves `.json` open).
- Rejected: dropping the Reddit special-case (loses clean Reddit fetches); browser-only (slower, heavier, loses the fast static path).

### output-to-unique-scratch-file

- Decision: the binary writes the extracted markdown to a **uniquely-named file it creates itself** via `os.CreateTemp` in `.scratch/` (relative to cwd, created if absent), with filename pattern `prowler-<descriptive-slug>-<random>.md`. The `<descriptive-slug>` is derived from the fetched target (first URL's host + leading path segment, lowercased, non-alphanumerics collapsed to hyphens, truncated to ~40 chars); the `<random>` component is supplied by `os.CreateTemp`'s `*` placeholder. The binary prints **only** the created file's absolute path to stdout (single line). The skill captures that path (`path=$(run.sh <urls>)`) and reads the file.
- **cwd `.scratch/` pollution (accepted parity + cleanup):** in LoomYard `.scratch/` is gitignored (`**/.scratch/`), but prowler runs in *any* repo, where `.scratch/` may not be — so a fetched `.md` would show as untracked clutter (weblens has the identical trait with `_millhouse/scratch/`). Accepted as parity, mitigated by cleanup: **the skill deletes the output file after reading it** (read → answer/summarize → `rm` the file). No file survives the invocation, so no repo accumulates clutter. Not swept by the binary itself (the binary's job ends at printing the path; the skill owns the read/answer/delete lifecycle).
- Rationale: multiple agents in parallel — and the **same** agent across multiple prowler subcommand invocations — must never write to or read the same file. Letting the binary create the file with `os.CreateTemp` guarantees OS-level uniqueness even for same-PID, same-instant concurrent calls; no caller-side filename logic can. The descriptive slug makes a `.scratch/` listing human-readable at a glance (useful if a run is interrupted before cleanup); the random suffix guarantees collision-freedom. Printing the path (not the content) to stdout keeps large pages out of the transcript and lets the skill read the file deterministically, then delete it.
- Rejected: caller-computed filenames from PID+timestamp (collide for same-agent, same-instant calls); a single fixed `.scratch/prowler-output.md` (parallel/repeat calls clobber each other); stdout-only content (bloats the transcript, the operator chose a file in Q5).

### dependency-stack

- Decision: nested-module deps are `github.com/chromedp/chromedp` (headless Chrome over the DevTools protocol, pure Go — replaces puppeteer-core), `github.com/go-shiori/go-readability` (a Go port of Mozilla Readability; its `Article.TextContent`/`.Title` cover the primary extraction directly — replaces `@mozilla/readability`), and `github.com/PuerkitoBio/goquery` (jQuery-like DOM traversal over stdlib `net/html`, for the strip-and-extract body-text fallback — replaces `linkedom`; go-readability pulls goquery in transitively regardless). Reddit `.json` and static fetch use stdlib `net/http` + `encoding/json`. No linkedom analog is otherwise needed.
- Rationale: this is the minimal Go stack that matches every weblens code path. go-readability giving `TextContent` directly removes most of the manual DOM-to-text work weblens did with linkedom; goquery covers only the fallback path (remove `script/style/noscript/nav/header/footer`, take body text).
- Rejected: stdlib `net/html` only, dropping goquery (goquery arrives transitively via go-readability anyway, and hand-rolling the traversal is more verbose for no dependency saving); a different browser driver (chromedp is the pure-Go DevTools client that matches Puppeteer's capability class — real browser fingerprint + JS execution, which is what defeats bot-blocking).

### chrome-discovery-and-failure-behavior

- Decision: discover the Chrome/Chromium executable via `CHROME_PATH` env first, then a platform candidate list mirroring weblens (`C:/Program Files/Google/Chrome/Application/chrome.exe`, `C:/Program Files (x86)/...`, `/Applications/Google Chrome.app/Contents/MacOS/Google Chrome`, `/usr/bin/google-chrome`, `/usr/bin/chromium-browser`). Timeouts mirror weblens: ~30 s page-load nav, ~60 s overall. Same browser UA/headers as weblens. If Chrome is not found, the browser fallback is **skipped** (degrade to whatever static extraction produced, with a note) — never a hard failure of the whole run. Per-URL fetch errors are captured inline as `# Error fetching <url>\n\n<detail>`, not fatal to sibling URLs.
- Rationale: exact parity with weblens' resilient degrade-don't-die behavior; a missing browser or one bad URL should not sink a multi-URL batch.
- Rejected: hard-failing when Chrome is missing (regresses on weblens' graceful degradation); tuning the timeout/header values (no reason to diverge from a known-good baseline).

### skill-contract

- Decision: the `prowler` skill mirrors weblens' contract — `argument-hint: "<url> [url2...] [question]"`; guidance to use it only when built-in `WebFetch` fails or returns unusable content. **Single cross-platform invocation line**, pinned exactly (matches how weblens already invokes its own `run.sh` on Windows): ``path=$(bash "${CLAUDE_PLUGIN_ROOT}/scripts/run.sh" <url1> [url2...])``. There is **one** wrapper, `run.sh`, used on Linux, macOS, and Windows alike — on Windows it runs under git-bash, which the operator already relies on for weblens (weblens' SKILL invokes `bash …/run.sh`). No `run.ps1`/`run.cmd` is created. Steps: run the wrapper; **check its exit code — on non-zero, report the failure (from stderr) and stop, do not read a path**; on success, read the file at the captured path, answer the user's question (or give a 3–5 sentence per-source summary if none), **delete the output file after reading** (so no untracked clutter is left in the invoking repo's `.scratch/`), and **never** dump raw fetched content to the user. `settings.json` permissions allow `Skill(prowler:*)` and the `bash`/binary invocation.
- Rationale: preserves the operator's existing weblens muscle memory, keeps transcripts clean, and a single git-bash-invoked wrapper avoids maintaining a parallel PowerShell script; the exit-code check keeps build/toolchain diagnostics from corrupting the path capture.
- Rejected: a divergent invocation surface (needless retraining for a drop-in replacement); a separate Windows PowerShell wrapper (git-bash is already a weblens prerequisite on the operator's Windows box, so a second wrapper is redundant maintenance).

## Technical context

Reference implementation (behavior to match) — Millhouse weblens, source at `/home/knatte/Code/millhouse/wts/millhouse/plugins/weblens/` and installed cache at `~/.claude/plugins/cache/millhouse/weblens/1.0.0/`:

- `scripts/fetch-worker.mjs` — the full fetch pipeline: `HEADERS` (browser UA), `REDDIT_RE`/`isRedditUrl`/`toRedditJsonUrl`/`fetchReddit`/`formatRedditPost`/`formatRedditSubreddit` (Reddit `.json` path, top-20 comments), `htmlToText` (whitespace normalization), `findChromeExecutable` (candidate list), `fetchWithBrowser` (puppeteer launch args `--no-sandbox --disable-gpu --disable-dev-shm-usage`, `networkidle0`, 30 s timeout), `fetchPage` (static→readability→bodytext→browser cascade, ≥100-char thresholds), `run` (multi-URL, `\n\n---\n\n` join).
- `scripts/fetch.mjs` — Windows CA-bundle export (re-exec worker with `NODE_EXTRA_CA_CERTS`). Go equivalent: rely on the system cert pool / `crypto/x509` — Go on Windows reads the system root store natively, so weblens' PowerShell CA-export workaround is **not needed** in Go. Note this in the plan; do not port the CA-bundle code.
- `scripts/run.sh` — PATH-repair wrapper. prowler's `run.sh` instead does the build-on-first-run + run.
- `skills/weblens/SKILL.md`, `skills/INDEX.md`, `settings.json`, `.claude-plugin/plugin.json` — shape templates for prowler's equivalents.

Millhouse marketplace shape (template for LoomYard's root manifest): `/home/knatte/Code/millhouse/wts/millhouse/.claude-plugin/marketplace.json` — `$schema`, `name`, `version`, `description`, `owner`, `plugins[]` with `name`/`description`/`version`/`author`/`source: ./plugins/<name>`/`category`.

LoomYard specifics:

- Repo root: `/home/knatte/Code/loomyard/wts/prowler` (this task worktree; branch `prowler`, parent `main`). No existing `plugins/` dir or root `.claude-plugin/` — both are new. `.claude/` exists (agents only).
- `.gitignore` bans committed binaries (`/lyx`, `lyx.exe`) and ignores `**/.scratch/`, `.vscode/settings.json`, and mill-managed paths. Add prowler build artifacts (`plugins/prowler/1.0.0/bin/`) to `.gitignore` in the same commit.
- Go 1.26 on PATH (`go1.26.0 linux/amd64`); chromedp/go-readability/goquery are **not** currently in any go.sum — they enter only the nested module.
- Nested-module caveat: because the module lives under the parent repo, confirm the plan adds `plugins/prowler/1.0.0/bin/` (and any `go` build cache scratch) to `.gitignore`, and that the nested `go.mod`/`go.sum` **are** committed (they are source, not artifacts).

Cross-platform note: prowler must run on both Linux and Windows (the operator uses both). A **single** `run.sh` handles all platforms — on Windows it runs under git-bash (already a weblens prerequisite there), invoked as `bash "${CLAUDE_PLUGIN_ROOT}/scripts/run.sh" …`; there is no separate PowerShell/cmd wrapper. The build-on-first-run + invoke path, the mkdir-atomic build lock, `os.CreateTemp`, chromedp, and Chrome discovery are all cross-platform.

## Constraints

From `CONSTRAINTS.md`. The *Go-package-graph* invariants don't apply (prowler is a separate module, outside `go build/test ./...`), but the *disk-walking grep guards* DO reach prowler's test files — see the correction under the separate-nested-go-module decision. Stated per-invariant so the plan neither trips a guard nor places code in the main module:

- **CLI/Cobra Invariant** — does not apply: prowler is not a Cobra module and adds no `cmd/lyx` command. Do not register anything in `newRoot()`.
- **Hub Geometry Invariant** — does not apply to the Go package graph, but keep it true by construction: prowler constructs no `_lyx`/geometry paths and imports no `hubgeometry`; it writes only to cwd `.scratch/`. (The geometry-literal guard scans the parent module's production `.go`, not prowler's separate module, but there is no reason to use those tokens anyway.)
- **Test Tier Purity Invariant** — **DOES reach prowler's `*_test.go`** (disk walk from parent root). Prowler test files must be free of `exec.Command`/`exec.CommandContext`, `gitexec.RunGit`, and `lyxtest.Copy*`, OR be `//go:build integration`/`smoke`-tagged (tagged files are skipped by this guard). Prefer the former: keep prowler unit tests pure and tag nothing.
- **Hermetic Git Test Environment Invariant** — **DOES reach prowler's `*_test.go`, regardless of build tag.** Any prowler test file it classifies as git-spawning (contains `exec.Command`/`exec.CommandContext`/`gitexec.RunGit`/`lyxtest.Copy*`/`lyxtest.MustRun`/`lyxtest.SeedConfig`) would require a `HermeticGitEnv` call prowler cannot provide → the only safe path is **prowler test files contain none of those substrings**. Enforced by construction (pure unit tests + shell-harness for shell-outs), not by editing the guard.
- **Sandbox Coverage Invariant** — does not apply: prowler registers no lyx cobra module, so it is not in `newRoot().Commands()` and needs neither a suite scenario nor an allowlist entry.
- **Documentation Lifecycle** — prowler is not a lyx module, so no `manifest/designs/` doc and no `docs/overview.md` module-table entry. Plugin docs live in `plugins/prowler/README.md`.

Discovered constraint: **no committed binaries** (LoomYard `.gitignore` norm) — enforced here by build-on-first-run + gitignoring `plugins/prowler/1.0.0/bin/`.

New cross-cutting invariant? **No** — prowler is self-contained and imported by nothing in lyx, so nothing in `CONSTRAINTS.md` needs a new entry for this task.

## Testing

Nested-module Go tests (`plugins/prowler/1.0.0/`), runnable via `go test ./...` from inside that module dir:

- **URL classification & Reddit transforms (TDD candidates, pure functions):** `isRedditUrl` (www/old/plain reddit hosts, non-reddit), `toRedditJsonUrl` (trailing-slash handling), `formatRedditPost` (title/subreddit/score/comments, missing-post nil, selftext vs link), `formatRedditSubreddit` (listing, empty). Table-driven, no network.
- **HTML→text normalization (TDD candidate):** the `htmlToText` equivalent — whitespace collapse, blank-line collapse, script/style stripping — over fixed HTML fixtures.
- **Extraction cascade decision logic:** given a stubbed fetch result, assert the correct branch is taken (readability-usable vs <100-char → fallback vs body-text vs browser). Inject the HTTP fetch and browser fetch behind an interface so the cascade is testable without network/Chrome.
- **Chrome discovery ordering:** `CHROME_PATH` wins; candidate list order; nil when none exist. Use a temp dir + fake executables + env override.
- **Output-file naming:** descriptive-slug derivation from a URL (host+path slug, truncation, non-alnum→hyphen), and that `os.CreateTemp` produces distinct paths across concurrent calls; assert the binary prints the created path and the file contains the markdown.
- **Multi-URL join:** results joined with `\n\n---\n\n`; per-URL error captured inline without aborting siblings.

**Hard guard constraint (binding on every prowler `*_test.go`):** because the parent repo's `tierpurity_test.go` and `hermeticenv_test.go` walk into `plugins/prowler/1.0.0/` (see the separate-nested-go-module decision), **no prowler test file may contain** `exec.Command`, `exec.CommandContext`, `gitexec.RunGit`, or any `lyxtest.*` substring. All the unit tests above satisfy this naturally — they are pure functions over in-memory fixtures and need no process/git spawn. chromedp's own process spawning lives inside the library, not in prowler's test source, so a `//go:build integration`-tagged browser test that *drives* chromedp is fine (it puts no banned substring in the test file); if any integration test ever needs a literal `exec.Command`, it must live in a non-`*_test.go` harness instead.

Integration / manual (require network + a local Chrome, so `//go:build integration`-tagged or manual — not part of the fast unit run, and still subject to the guard constraint above):

- End-to-end static fetch of a known-readable page.
- Browser fallback against a JS-rendered or bot-blocked page (needs Chrome; `CHROME_PATH` set) — drives chromedp, no `exec.Command` in the test file.
- Reddit `.json` fetch against a live post.

**Build-on-first-run & lock concurrency — shell harness, NOT a Go test.** Verified by a non-`*_test.go` shell script (e.g. `plugins/prowler/1.0.0/scripts/selftest.sh`) or a manual checklist, precisely because a Go test exercising it would need `exec.Command`/shell-out and trip both guards. Checks: from a clean `bin/`, the wrapper builds then runs; a second run reuses the binary; two concurrent invocations don't corrupt the binary (mkdir-lock); `go`-missing exits non-zero with the stderr message and no path on stdout; a failed build removes the lock + partial binary and exits non-zero.

The plan keeps the pure-function/cascade tests in the always-run set, the network/Chrome tests behind `//go:build integration`, and the build/lock behavior in the shell harness.

## Q&A log

- **Q:** Plugin directory placement in LoomYard? **A:** `plugins/prowler/1.0.0/` + root `.claude-plugin/marketplace.json` (marketplace `loomyard`), mirroring Millhouse.
- **Q:** Go module boundary? **A:** Separate nested Go module (`plugins/prowler/1.0.0/go.mod`), isolated from lyx's module — explicitly confirmed ("Egen go-modul, ja").
- **Q:** Binary distribution? **A:** Build-on-first-run (wrapper builds into the plugin `bin/` under a lock if absent), honoring LoomYard's no-committed-binaries norm; Go toolchain required at run time (operator's machines have it).
- **Q:** Feature parity with weblens? **A:** Full parity, including the Reddit `.json` special-case (Reddit blocks HTML but leaves `.json` open → clean post + top-20 comments).
- **Q:** Skill contract & output? **A:** Mirror weblens' skill; write to an own scratch file, read it, answer/summarize, never dump raw content.
- **Q:** Build-on-first-run mechanics? **A:** Build into `${CLAUDE_PLUGIN_ROOT}/bin/`, lockfile to prevent races, clear error if `go` missing, auto-rebuild on version bump.
- **Q:** Output filename — parallel agents *and* the same agent across multiple subcommands must not collide? **A:** The binary creates the file itself via `os.CreateTemp` in `.scratch/`, pattern `prowler-<descriptive-slug>-<random>.md` — descriptive slug from the fetched URL for at-a-glance readability, random suffix + OS-level temp creation for guaranteed uniqueness; binary prints the path, skill reads it.
- **Q:** Dependency stack? **A:** chromedp + go-shiori/go-readability + PuerkitoBio/goquery; Reddit/static via stdlib `net/http`+`encoding/json`. (Operator delegated the choice.)
- **Q:** Timeouts / headers / Chrome-not-found? **A:** Mirror weblens exactly — same UA/headers, 30 s nav / 60 s overall, degrade to static-only if Chrome missing, per-URL errors inline.
- **Q:** Docs & metadata? **A:** Plugin README + marketplace.json only; no `docs/overview.md`/`manifest`/`CONSTRAINTS` churn (prowler is not a lyx module). Marketplace `loomyard`, plugin `prowler` v1.0.0, author Knatte18, Apache-2.0.
- **Q:** Windows CA-bundle port (weblens `fetch.mjs`)? **A:** Not needed in Go — Go reads the Windows system root store natively; skip the PowerShell CA-export workaround.
- **Q:** [review r1 gap] What is the marketplace `plugins[].source` value given the `1.0.0/` subdir? **A:** [auto] `./plugins/prowler/1.0.0` — the exact dir holding `.claude-plugin/plugin.json`; plan confirms it resolves at install. **Why:** a flat `./plugins/prowler` would miss the versioned dir and break install.
- **Q:** [review r1 gap] How is the wrapper's `go build` output kept from corrupting the skill's `path=$(run.sh …)` capture? **A:** [auto] All wrapper diagnostics (build progress/errors, toolchain-missing) go to stderr; only the output-file path goes to stdout; the skill checks the wrapper exit code before reading the path. **Why:** stdout must carry only the single path line for the capture to be reliable.
- **Q:** [review r2 gap] The nested module is NOT invisible to the disk-walking grep guards (`tierpurity_test.go`, `hermeticenv_test.go` walk into `plugins/prowler/`). How to stay green? **A:** [auto] Prowler `*_test.go` files contain none of `exec.Command`/`exec.CommandContext`/`gitexec.RunGit`/`lyxtest.*`; unit tests stay pure; the build-lock/shell-out check moves to a non-`*_test.go` shell harness; do NOT edit the guards' skip sets. **Why:** the Hermetic guard scans every test file regardless of tag and would demand a `HermeticGitEnv` call prowler can't make; editing cmd/lyx is out of scope and weakens the guards.
