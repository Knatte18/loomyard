# Batch: plugin-packaging

```yaml
task: 'prowler: installable Claude Code plugin (Go), hosted in LoomYard'
batch: plugin-packaging
number: 2
cards: 7
verify: bash plugins/prowler/scripts/selftest.sh
depends-on: [1]
```

## Batch Scope

This batch wraps the batch-1 binary into a real, installable Claude Code plugin: the build-on-first-run wrapper (`run.sh`), an offline shell harness for the build/lock mechanic (`selftest.sh`), the plugin manifest (`plugin.json`), the permissions block (`settings.json`), the `prowler` skill (`SKILL.md` + `INDEX.md`), the root marketplace manifest that makes LoomYard installable (`.claude-plugin/marketplace.json`), the plugin README, and the `.gitignore` entry that keeps the built binary out of git. It depends on batch 1 because `run.sh`/`selftest.sh` build the batch-1 Go module. No Go source is touched here.

Batch-local decisions (refine `## Shared Decisions`):
- **Single cross-platform wrapper:** one `run.sh` invoked via `bash` on Linux/macOS/Windows (git-bash on Windows, already a weblens prerequisite). No `run.ps1`/`run.cmd`. `run.sh` self-locates everything from `$0` — no dependency on `CLAUDE_PLUGIN_ROOT`/`CLAUDE_SKILL_DIR` inside the wrapper.
- **`PROWLER_BUILD_ONLY` escape:** when that env var is non-empty, `run.sh` ensures the binary is built and exits 0 without executing it. This lets `selftest.sh` exercise build/reuse/lock offline (no network, no Chrome), so `verify` is deterministic.
- **Flat marketplace `source` (proven shape):** the plugin lives at `plugins/prowler/` and the marketplace entry uses `source: ./plugins/prowler`, matching every real local-source example (weblens' `./plugins/weblens`; all 276 official-marketplace plugins use flat local sources, several with an explicit sibling `version` field — version and source-path are decoupled). This supersedes the discussion's *versioned-subdir default*: the discussion's own binding rule was "do not ship the versioned form unverified" and named flat as the sanctioned fallback, and since the autonomous mill-go pipeline cannot run the interactive `/plugin install` verification, flat is the only form that can ship verified. The plugin `version` (`1.0.0`) is carried in the `plugin.json`/marketplace `version` field, so no versioning affordance is lost. A post-merge `/plugin install prowler@loomyard` smoke-check is still listed in `## Batch Tests` (it cannot be a `verify:` command), but it is a confirmation, not a gate with a fallback.

## Cards

### Card 8: Build-on-first-run wrapper `run.sh`

- **Context:**
  - `_mill/discussion.md`
- **Edits:** none
- **Creates:**
  - `plugins/prowler/scripts/run.sh`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create an executable (`chmod +x`, committed exec bit) bash script. `PROWLER_BUILD_ONLY` selects ONLY the final action (exit-0 vs exec); it must NOT affect the reuse decision, so a second `PROWLER_BUILD_ONLY=1` run reuses the existing binary and does not rebuild (this is what selftest.sh's unchanged-mtime reuse assertion depends on). The lock uses an **owner token** so a slow-but-live builder is never "stolen from": the exit-trap and the staleness reclaim both verify ownership before removing the lock dir, and the build itself writes to a unique temp path then atomically renames, so even a mis-timed concurrent build can never corrupt or half-write the final binary. Steps: (1) self-locate `SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"` and set `PLUGIN_ROOT="$SCRIPT_DIR/.."` (the nested Go module root and parent of `bin/`); (2) set `BIN="$PLUGIN_ROOT/bin/prowler"`, appending `.exe` when `uname -s` matches `MINGW*|MSYS*|CYGWIN*`; compute a per-invocation `MYTOKEN="$$-$RANDOM"`; (3) define a shell function `run_or_exit` that FIRST releases the lock if this process owns it — `if [ "$(cat "$LOCK/owner" 2>/dev/null)" = "$MYTOKEN" ]; then rm -rf "$LOCK"; fi` — and THEN performs the final action: if `PROWLER_BUILD_ONLY` is non-empty, `exit 0`; else `exec "$BIN" "$@"`. The explicit release is mandatory because `exec` replaces the shell process image and the `EXIT` trap (step 6) never fires on the `exec` branch; without this release the lock dir would be orphaned on every real run and the next genuine rebuild would stall the full staleness window. The token check makes the release a safe no-op on the reuse path (step 4), where no lock was acquired; (4) if `BIN` already exists and is non-empty (reuse — independent of `PROWLER_BUILD_ONLY`), call `run_or_exit` immediately (no rebuild, mtime untouched); (5) otherwise acquire the lock dir `LOCK="$PLUGIN_ROOT/bin/.build.lock"` (after `mkdir -p "$PLUGIN_ROOT/bin"`): loop `mkdir "$LOCK"` with `sleep 1`, up to a generous `~300s` deadline (comfortably above a cold first build that fetches chromedp/goquery/go-readability, including the ~4× AV-scanning overhead on the operator's Windows box); the moment `mkdir` succeeds, write the token with `echo "$MYTOKEN" > "$LOCK/owner"` and set the exit-trap in the same step (step 6); inside the wait loop, if `BIN` becomes present+non-empty another builder finished → stop waiting and call `run_or_exit`, and if `LOCK` still exists with an mtime older than `~300s` treat it as orphaned (SIGKILL case), `rm -rf "$LOCK"`, and retry — read mtime portably via `stat -c %Y "$LOCK" 2>/dev/null || stat -f %m "$LOCK"`; on deadline exceeded, print a lock-timeout message to stderr and exit non-zero; (6) install a token-checked exit-trap immediately after acquiring: `trap 'if [ "$(cat "$LOCK/owner" 2>/dev/null)" = "$MYTOKEN" ]; then rm -rf "$LOCK"; fi' EXIT` — it removes the lock ONLY if this process still owns it, so if a waiter wrongly reclaimed our lock our later exit cannot delete the new holder's lock; (7) verify the toolchain with `command -v go` — if absent, print `prowler: Go toolchain not found — install Go or add it to PATH` to stderr, release the lock (the trap will, on exit), exit non-zero; (8) build to a unique temp path then atomically rename: `TMP="$BIN.tmp.$MYTOKEN"`; run `go build -o "$TMP" "$PLUGIN_ROOT"` sending build stdout+stderr to stderr (`>&2`); on failure print a build-failed message to stderr, `rm -f "$TMP"`, exit non-zero (trap releases the lock); on success `mv -f "$TMP" "$BIN"` (atomic on the same filesystem, so no reader ever sees a partial `$BIN`, and concurrent builders' renames are last-writer-wins with a complete binary either way); (9) call `run_or_exit`, which releases the lock (token-checked) and then execs/exits. The `EXIT` trap from step 6 remains as a backstop for the error-exit paths (toolchain-missing, build-failure) that exit normally without going through `run_or_exit`; the normal success path releases the lock explicitly in `run_or_exit` because `exec` never triggers the trap. Only the executed binary's stdout may reach the wrapper's stdout — every wrapper diagnostic and all `go build` output goes to stderr.
- **Commit:** `feat(prowler): build-on-first-run wrapper with mkdir lock`

### Card 9: Offline build/lock shell harness `selftest.sh`

- **Context:**
  - `_mill/discussion.md`
  - `plugins/prowler/scripts/run.sh`
- **Edits:** none
- **Creates:**
  - `plugins/prowler/scripts/selftest.sh`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create an executable bash script (a plain shell harness, NOT a `*_test.go` file — this is where the subprocess/build behavior lives, per the guard-cleanliness decision) that self-locates `PLUGIN_ROOT` the same way as `run.sh` and exercises build-on-first-run offline via `PROWLER_BUILD_ONLY=1`: (1) `rm -rf "$PLUGIN_ROOT/bin"`, run `PROWLER_BUILD_ONLY=1 bash "$PLUGIN_ROOT/scripts/run.sh"`, assert exit 0 and that `bin/prowler` (or `bin/prowler.exe`) exists and is non-empty (clean-build path); (2) record the binary mtime, run the same command again, assert exit 0 and unchanged mtime (reuse path — no rebuild); (3) `rm -rf "$PLUGIN_ROOT/bin"`, launch two `PROWLER_BUILD_ONLY=1 bash run.sh &` in parallel, `wait`, assert both exit 0 and the resulting binary is present+non-empty (mkdir-lock does not corrupt a concurrent build); (4) lock-age staleness (the SIGKILL-orphaned-lock recovery, no live owner): `rm -rf "$PLUGIN_ROOT/bin"`, `mkdir -p "$PLUGIN_ROOT/bin"`, create `"$PLUGIN_ROOT/bin/.build.lock"` (optionally with a stale `owner` file whose token belongs to no live process) and no binary present, backdate its mtime to well past the ~300s threshold (`touch -d '400 seconds ago' "$PLUGIN_ROOT/bin/.build.lock"` on GNU touch — the operator's Linux and git-bash platforms; if the platform's `touch` rejects `-d`, log a skip note for this one case rather than failing), then run `PROWLER_BUILD_ONLY=1 bash run.sh` and assert it completes promptly (well under the full ~300s deadline) with exit 0 and a present+non-empty binary — proving the orphaned lock was reclaimed rather than waited out. (The live-owner race — a slow-but-alive builder — is guarded structurally by the owner-token check and atomic rename in `run.sh`, not by a shell-harness timing test, since a live-owner race is not deterministically reproducible in a harness; test 3's concurrent-build assertion already exercises that the atomic rename yields a valid binary.) (5) exec-branch lock release (the real, non-`PROWLER_BUILD_ONLY` path): `rm -rf "$PLUGIN_ROOT/bin"`, then run `bash "$PLUGIN_ROOT/scripts/run.sh"` with NO `PROWLER_BUILD_ONLY` and NO URL arguments (so after building, the wrapper execs the binary, which prints its `Usage:` line to stderr and exits 1), and assert both that the binary now exists and — critically — that `"$PLUGIN_ROOT/bin/.build.lock"` is **absent** afterward (proving `run_or_exit` released the lock before `exec`, since the `EXIT` trap cannot fire across `exec`); the non-zero exit from the usage message is expected and not a failure of this assertion; (6) print a clear PASS/FAIL summary and exit non-zero on any failed assertion. The go-missing path and the failed-build path are documented in a comment as manual checks (stripping `go` from PATH without also losing coreutils is not portable enough to assert here), keeping `verify` deterministic. The harness requires `go` on PATH and the batch-1 module deps to be fetchable or already cached.
- **Commit:** `test(prowler): offline build/lock self-test harness`

### Card 10: Plugin manifest `plugin.json`

- **Context:**
  - `_mill/discussion.md`
- **Edits:** none
- **Creates:**
  - `plugins/prowler/.claude-plugin/plugin.json`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create the plugin manifest mirroring weblens' shape: JSON object with `name: "prowler"`, `description: "Fetch blocked/restricted/JS-rendered web pages and output readable markdown"`, `version: "1.0.0"`, `license: "Apache-2.0"`, and `author: { "name": "Knatte18" }`. Must be valid JSON.
- **Commit:** `feat(prowler): plugin manifest`

### Card 11: Permissions `settings.json`

- **Context:**
  - `_mill/discussion.md`
- **Edits:** none
- **Creates:**
  - `plugins/prowler/settings.json`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create the permissions block: JSON `{ "permissions": { "allow": ["Skill(prowler:*)", "Bash(bash *)", "Bash(go *)"] } }` — three entries, mirroring the **verified** shape of weblens' own `settings.json`, which grants `Skill(weblens:*)` + `Bash(bash *)` + `Bash(node *)` (it explicitly gates the `node` child that its `run.sh` spawns via `exec node`, the exact relationship prowler's `go build` child has to its wrapper). `Skill(prowler:*)` permits the skill; `Bash(bash *)` covers the wrapper invocation `bash …/run.sh …` and the `bash -c 'rm -f …'` cleanup; `Bash(go *)` explicitly grants the child `go build` the wrapper spawns rather than assuming the grandchild spawn is covered transitively by `Bash(bash *)`. This deliberately supersedes the discussion's skill-contract "exactly two entries" claim, which was based on a mischaracterization of weblens' shape (weblens is three entries, not two) — see the overview Shared Decision below. Must be valid JSON.
- **Commit:** `feat(prowler): plugin permissions`

### Card 12: The `prowler` skill

- **Context:**
  - `_mill/discussion.md`
  - `plugins/prowler/scripts/run.sh`
- **Edits:** none
- **Creates:**
  - `plugins/prowler/skills/prowler/SKILL.md`
  - `plugins/prowler/skills/INDEX.md`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `SKILL.md` mirroring weblens' contract, with YAML frontmatter `name: prowler`, `description: "Fetch blocked/restricted web pages and answer questions about their content"`, `argument-hint: "<url> [url2...] [question]"`. Body guidance: use only when the built-in `WebFetch` fails or returns unusable content. Steps: (1) capture the output path with ``path=$(bash "${CLAUDE_SKILL_DIR}/../../scripts/run.sh" <url1> [url2...])`` — the `${CLAUDE_SKILL_DIR}/../../scripts/run.sh` path-resolution hop (from `skills/prowler/` to the plugin's `scripts/` dir) is identical to weblens' verified `SKILL.md`; the `path=$(...)` capture itself is prowler's own addition (weblens redirects run.sh's stdout to one fixed shared file — `> _millhouse/scratch/weblens-output.md`), required by prowler's unique-output-file design, not claimed as weblens-identical; (2) **check the wrapper exit code first** — on non-zero, report the failure (surfaced on stderr) and stop, do NOT read a path; (3) read the file at `$path`; (4) answer the user's question, or give a 3–5-sentence per-source summary when none was asked; (5) delete the output file with ``bash -c 'rm -f "<path>"'`` (issued as `bash -c` so the pinned `Bash(bash *)` permission covers it — never a bare `rm`); (6) never dump raw fetched content to the user. Create `INDEX.md` with a one-row table linking `[prowler](prowler/SKILL.md)` with its description (weblens `INDEX.md` shape).
- **Commit:** `feat(prowler): prowler skill contract`

### Card 13: Root marketplace manifest

- **Context:**
  - `_mill/discussion.md`
- **Edits:** none
- **Creates:**
  - `.claude-plugin/marketplace.json`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create LoomYard's root marketplace manifest so `/plugin marketplace add` + `/plugin install prowler@loomyard` work. JSON object with `$schema: "https://anthropic.com/claude-code/marketplace.schema.json"`, `name: "loomyard"`, `version: "1.0.0"`, `description` (short, e.g. "LoomYard Claude Code plugin marketplace"), `owner: { "name": "Knatte18" }`, and a `plugins` array with one entry: `{ "name": "prowler", "description": "Fetch blocked/restricted/JS-rendered web pages and output readable markdown", "version": "1.0.0", "author": { "name": "Knatte18" }, "source": "./plugins/prowler", "category": "productivity" }`. The `source` is the flat path — the exact dir holding `.claude-plugin/plugin.json` — matching every real local-source example (weblens and the 276 official-marketplace plugins all use flat `./plugins/<name>`); the `version` field carries `1.0.0` independently. Must be valid JSON.
- **Commit:** `feat(prowler): root loomyard marketplace manifest`

### Card 14: README + gitignore/gitattributes housekeeping

- **Context:**
  - `_mill/discussion.md`
  - `plugins/prowler/scripts/run.sh`
- **Edits:**
  - `.gitignore`
  - `.gitattributes`
- **Creates:**
  - `plugins/prowler/README.md`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Append `plugins/prowler/bin/` to `.gitignore` **after the `# === end mill-managed ===` line** (outside the mill-managed fence, so a future `mill-setup` regeneration of that fenced block does not drop the entry) — this keeps the built binary and the `.build.lock` dir out of git; the nested `go.mod`/`go.sum` remain committed as source. Add a line `plugins/prowler/scripts/*.sh text eol=lf` to `.gitattributes` (mirroring the existing per-script pins such as `internal/fabricengine/post-checkout.sh text eol=lf`), so a Windows checkout's `core.autocrlf` cannot rewrite `run.sh`/`selftest.sh` to CRLF and break `bash run.sh` on the operator's git-bash box. Create `plugins/prowler/README.md` covering: what prowler is (a Go replacement for weblens — fetch bot-blocked/paywalled/JS-rendered pages + Reddit as readable markdown); install (`/plugin marketplace add <loomyard repo>` then `/plugin install prowler@loomyard`); build-on-first-run behavior (the wrapper builds the Go binary into `bin/` on first use under a lock; requires a Go toolchain on PATH; manual lock remedy `rm -rf plugins/prowler/bin/.build.lock`); the Chrome/Chromium runtime prerequisite (discovered via `CHROME_PATH` or platform defaults; the browser fallback is skipped, not fatal, when absent); and Apache-2.0 licensing (referencing the repo-root `LICENSE`).
- **Commit:** `docs(prowler): readme and gitignore build artifacts`

## Batch Tests

`verify: bash plugins/prowler/scripts/selftest.sh` — the offline shell harness from Card 9. It exercises build-on-first-run, binary reuse, and the concurrent mkdir-lock via `PROWLER_BUILD_ONLY=1` with no network or Chrome, so it is deterministic given `go` on PATH and the batch-1 module deps cached (batch 1 already fetched them). It is a plain shell script, not a Go test, so none of the parent grep guards see it.

Manual / non-automatable verifications (stated here because they cannot be `verify:` commands):
- **Install smoke-check (post-merge confirmation):** run a real `/plugin marketplace add` of this LoomYard repo, then `/plugin install prowler@loomyard`, and confirm the plugin loads and the `prowler` skill runs. The flat `source: ./plugins/prowler` matches every verified real example, so no layout fallback is expected; this is a confirmation, not a gate. It cannot be a `verify:` command (interactive Claude Code action).
- **Live fetch parity:** with a local Chrome present, run the binary (or the skill) against a known-readable page, a JS-rendered/bot-blocked page (browser fallback), and a live Reddit post (`.json` path); confirm readable markdown and correct multi-URL `\n\n---\n\n` joining. These map to the `//go:build integration` tests (run with `-tags integration`) and manual skill use.
- **`go` missing / failed build:** confirm `run.sh` exits non-zero with the stderr message and prints no stdout path when the toolchain is absent or the build fails (the harness documents these as manual checks).
