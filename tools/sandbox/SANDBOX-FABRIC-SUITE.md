# SANDBOX-FABRIC-SUITE -- lyx fabric black-box agent suite

## What this is

A structured test-loop for exercising `lyx fabric` against **dedicated** GitHub test repos (`Knatte18/lyx-fabric-test` as warp, `Knatte18/lyx-fabric-test-weft` as weft) -- never the shared `lyx-test`/`lyx-test-weft` repos the main and per-module suites use.
This suite proves fabric's stricter `main-weft`-suffixed branch-naming scheme holds up on its own dedicated hub, whose fixtures the shared hub does not exercise.

Like the other suites, the value is a Claude session driving `lyx fabric` by hand in a real hub, treating every break, surprise, or rough edge as a LoomYard finding to record in the report.

## Pre-conditions

Before starting a session:

1. **Deploy a fresh binary.**
   Run `deploy.cmd` so `lyx.exe` on PATH is current source.
   The deployed binary is a snapshot -- re-deploy after any source change you want to test.
2. **Materialize the fabric hub.**
   Run `sandbox/fabric-suite.cmd`.
   Unlike the main and per-module suites (which assume `sandbox/build.cmd` already ran), this launcher clones the dedicated fabric hub itself via `lyx fabric clone` -- idempotently: if `lyx-fabric-test-HUB` already exists it is reused as-is, never reset or re-cloned.
   The very first run on a machine performs the real clone;
   every run after that starts from whatever state the previous session left the hub in.

   **Re-clone adoption.**
   When a clone runs against a weft remote that already carries the suffixed primary branch (a fresh machine, or `clone --reset`), the weft prime adopts `origin/main-weft` as a tracking branch — inheriting the previously synced weft state — rather than forking a new, untracked `main-weft` at `main`'s HEAD.
   After any re-clone, confirm the weft prime's `main-weft` has an upstream (`git -C <weft-prime> branch -vv`) and contains the previously synced `_lyx/` content.
3. **`lyx` on PATH.**
   Confirm `lyx --help` works from any directory.
4. **Weft `_lyx/` must be seeded before `lyx fabric clone`.** `lyx init` is gone;
   `lyx fabric clone` now wires the warp `_lyx` junction to the weft worktree's `_lyx/` directory via fabricengine as part of clone itself.
   On a truly empty weft repo that directory does not exist yet, so clone creates a dangling junction and then fails (`mkdir _lyx: file exists`).
   The dedicated `lyx-fabric-test-weft` repo must therefore have an `_lyx/` directory committed on its primary branch (the operator seeds it once).
   Until then, treat a `clone` failure on this dedicated fabric hub as this known precondition gap, not a fabric defect — fabric's own verbs read their config from that same `_lyx/config/`.

### PowerShell JSON-quoting

See the PowerShell JSON-quoting note in `SANDBOX-CORE-SUITE.md`'s Pre-conditions -- the same caveat applies here whenever a scenario below passes JSON as an argument.

### Operating model

lyx resolves against the current directory's own `_lyx/` and does **not** walk up to a parent.
The hub warp repo is initialized at its root, so the agent runs the entire session from there (cwd is fixed at the root).

## Black-box rule

**The agent under test works exclusively inside the dedicated fabric Hub's warp repo (`lyx-fabric-test-HUB/lyx-fabric-test`).
It tests `lyx.exe` as a black box -- exactly as a real user with only the binary on PATH.
It must not look for, read, or reason about the lyx source tree.
No peeking at `C:\Code\loomyard\` or any other path outside the Hub.**

Discovering the command surface is done via `lyx fabric`, `lyx fabric <subcommand>`, and `lyx fabric <subcommand> --help` alone -- not from documentation outside the Hub.

## Fingerprint header

The launcher prepends a "binary under test" fingerprint block to this file when it copies it into the fabric Hub's warp repo.
The fingerprint records the absolute path, file size, modification time, and a short SHA-256 of the `lyx.exe` binary at launch time.

The same fingerprint identifies the binary for the report's provenance: a separate fetch step (run after this session) stamps it into `meta.fingerprint` of the fetched `sandbox-report.json` so a maintainer can reproduce the exact binary that produced each finding.
The agent does not need to transcribe the fingerprint anywhere itself.

## How to run a scenario

For each scenario below:

- Read the **Goal** -- it names the task, not the commands.
  Discover the commands via `lyx fabric`, `lyx fabric <subcommand>`, and `--help` flags (F0 ethos).
- **Watch** what lyx does.
  Note where it stalls, guesses wrong, or hits an error.
- Record the outcome per the verdict buckets: `OK` (worked) / `WARN` (rough edge) / `FAIL` (broke).

## Verdict key

- `OK`   -- completed without friction
- `WARN` -- completed but with confusion, awkward UX, or a non-fatal error
- `FAIL` -- did not complete;
  lyx broke, panicked, or gave wrong output

## Capturing findings

After all scenarios are run, write **all** `WARN`/`FAIL` findings to `./sandbox-report.json` (in the warp-repo cwd) on this exact schema.
**Always write the file, even when there are zero `WARN`/`FAIL` findings** -- in that case `items` is an empty array.

```json
{
  "source": "sandbox-report",
  "items": [
    {
      "ref": "F2",
      "title": "…",
      "body": "verdict: WARN\n\n…repro…"
    }
  ]
}
```

- `source` is the literal string `"sandbox-report"`.
- `items[]` holds only `WARN`/`FAIL` findings -- do not record `OK` scenarios here.
- `ref` is the scenario id (`F0`-`F5`).
- `title` is a short one-line summary.
- `body` folds the detail, repro steps, and verdict into one markdown string.

Write only `source` and `items` -- a separate fetch step (run after the session) stamps `meta` (including the binary fingerprint).
Confine all free text to the `title`/`body` string fields so the JSON stays well-formed.

## Scenarios

### F0 -- Discovery (help surface smoke test)

**Goal:** "You have `lyx` on PATH and nothing else inside this repo.
Find out what `lyx fabric` can do and report its full command tree."

**Watch:** Does `lyx fabric` list all 16 verbs (`clone`, `add`, `list`, `remove`, `checkout`, `pairs`, `reconcile`, `prune`, `cleanup`, `status`, `commit`, `push`, `pull`, `sync`, `diff`, `unwire`)?
Does each `--help` explain itself?
Is each description accurate and useful?

**Verdict:** `OK` / `WARN` / `FAIL`

---

### F1 -- Clone geometry

**Covers:** fabric

**Goal:** "Confirm the dedicated fabric hub the launcher just materialized (or reused) looks the way `lyx fabric clone` promises."

**Watch:** `_board` is a linked worktree of the same weft repo as the weft prime, never a separate clone -- confirm `git -C <weft-prime> rev-parse --git-common-dir` and `git -C _board rev-parse --git-common-dir` resolve to the same path. `_board` is checked out on the warp's own dynamically-derived unsuffixed default branch (**never hardcoded to `main`** -- whatever the warp repo's actual default branch is), which `git -C _board branch --show-current` should confirm directly;
this mirrors the assertion shape `internal/fabricengine/clone_adopt_test.go`'s `assertBoardIsWeftWorktree` already makes in code.
The weft prime's checked-out branch is **`main-weft`**, not `main` -- fabric's uniform branch-suffix scheme applies from the very first pair, unlike the pre-fabric mirrored (identical) branch-naming convention.
Use `lyx fabric pairs` and plain git (`git -C <weft-prime> branch --show-current`) to confirm;
neither should require guessing or `ls`-ing around.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### F2 -- Topology lifecycle

**Goal:** "Add a new warp+weft worktree pair, inspect it, coordinate-checkout it, reconcile it, then prune and clean it up."

**Watch:** Run `lyx fabric add <slug>` and confirm the new weft branch is named `<warp-branch>-weft` (the fixed suffix, not a mirrored name -- e.g. adding slug `foo` with an empty branch prefix yields warp branch `foo` and weft branch `foo-weft`).
Does `lyx fabric pairs` report the new pair as in-sync?
Does `lyx fabric checkout` switch both sides together and re-point the junction?
After removing the warp side by hand (or via `lyx fabric remove`), does `lyx fabric reconcile` report and repair drift sanely, and do `lyx fabric prune`/`lyx fabric cleanup` (dry-run first, then `--apply`) correctly identify the orphaned `<slug>-weft` branch as fabric-managed (by its suffix) and handle it per the flag matrix described in `lyx fabric cleanup --help`?

**Verdict:** `OK` / `WARN` / `FAIL`

---

### F3 -- Weft content sync

**Goal:** "Make a small, clearly-marked change inside the weft-tracked scope and run it through `fabric status`, `commit`, `push`, `pull`, and `sync`."

**Watch:** `fabric status` now reports the unified both-sides uncommitted-change view: a side-labelled list of `{path, side}` entries (`side` is `"warp"` or `"weft"`), not the old weft-only branch/dirty/ahead/behind map -- confirm the test change to the weft-tracked scope shows up as an entry with `side: "weft"`.
Do `commit`/`push` mirror it to the weft remote?
The commit message is always the fixed string `"weft sync"` -- it is not generated from changed files and there is no `-m` flag to customize it (confirm via `lyx fabric commit --help`).
Every fabric weft commit also carries a trailing `Warp-SHA: <sha>` trailer naming the paired warp repo's current HEAD -- inspect the commit body (e.g. `git -C <weft-worktree> log -1`) and confirm the trailer is present and names a real, resolvable warp commit. `fabric sync` pushes via a detached child process, so `status` immediately after `sync` may lag behind the actual push -- a confusing-but-expected rough edge to note as a `WARN`, not to pre-judge here.
Staging is scoped to the directories listed in the fabric config (default `_lyx`), so the test change should land inside that scope to be picked up at all.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### F4 -- Reserved-slug guardrail

**Covers:** fabric

**Goal:** "Confirm `lyx fabric add` refuses a slug that would collide with the weft-worktree directory namespace, before it can create a booby-trapped pair."

**Watch:** Run `lyx fabric add <name>-weft` (a slug ending in the reserved `-weft` suffix -- e.g. `add zed-weft`).
It must be **rejected** with an `invalid slug` error and create nothing.
It must NOT create a warp worktree directory `<name>-weft`: such a directory is indistinguishable from a weft worktree, so a later `lyx fabric prune --apply` would misclassify the warp worktree as an orphaned weft and delete it (destroying any uncommitted work).
To confirm the guard holds, follow the rejected add with `lyx fabric list`/`lyx fabric pairs` and plain `ls` of the hub -- no `<name>-weft` warp worktree should exist. (Historical: before this guard, `add zed-weft` succeeded and a routine `prune --apply` silently `os.RemoveAll`'d the warp worktree -- a data-loss bug.)

**Verdict:** `OK` / `WARN` / `FAIL`

---

### F5 -- Junction deactivation (`lyx fabric unwire`)

**Covers:** fabric

**Goal:** "Deactivate a wired worktree with `lyx fabric unwire`, confirm it tears down exactly what it should and nothing more, then confirm `lyx fabric reconcile` re-wires it afterward."

**Watch:** Run `lyx fabric unwire` on a wired worktree (the fabric prime,
or a pair added in F2).
Confirm it removes every fabric junction present on disk (e.g. `_lyx`, `.lyx`) — `ls`/`git -C <warp> ls-files --others -i --exclude-standard` or plain directory inspection should show the junction entries gone.
Confirm it preserves the weft-side `_lyx` content, explicitly including `_lyx/PATTERN.md` — `_lyx/` under the paired weft worktree should be untouched, not cleared — since `_lyx` is deliberately never touched by unwire.
Confirm it reverts the junction's own `.git/info/exclude` entry — there is no committed `.gitignore` block to revert, since junctions are excluded through `.git/info/exclude` alone.
If a SECOND worktree in the same hub is still wired, confirm the exclude entry is KEPT (that file lives in the repo's shared gitdir, so removing it would make the other worktree's live junctions show up as untracked dirt) — check with `git -C <other-warp-worktree> status --porcelain`, which must stay clean.
Run `lyx fabric unwire` a second time immediately after: it must be idempotent and no-op cleanly on an already-unwired worktree, not error.
Finally, confirm the repo-wide records survive unwire — `.lyx-anchor`, `<BoardDir>/_lyx/config/fabric.yaml`, and `.lyx-warp` are untouched — by running `lyx fabric reconcile` afterward and confirming it re-wires the worktree's junctions from those same repo-wide records, with no re-clone needed.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### F6 -- Rebased-warp pull recovery

**Covers:** fabric

**Goal:** "Drive `lyx fabric pull` against a warp remote whose history was rebased or force-pushed underneath your local clone, and confirm fabric detects the drift and re-aligns rather than silently fast-forwarding or erroring."
Discover the surface via `lyx fabric pull --help`.

**Watch:** `pull` now touches **both** warp and weft, not weft-only -- confirm both sides move where expected.
A clean local warp (no unpushed commits of its own) should auto-reconcile: warp resets to the new remote history, and weft's own correspondence re-anchors to it, with no operator intervention needed.
A local warp carrying unpushed commits of its own, run against the same rewritten remote, should instead abort loudly and make no changes to either repo -- confirm neither warp nor weft moved after the abort.
In the auto-reconciled case, inspect the JSON output: it should report which `_lyx/PATTERN.md`/`_lyx/pattern/`-touching weft commits need review, since they were written against a warp baseline that no longer exists on the rewritten remote.
Before any of that, dirty the warp worktree with an uncommitted edit to a tracked file and run `lyx fabric pull` against an advanced remote: it must REFUSE to move warp and the edit must survive byte-for-byte -- advancing warp goes through a hard reset, so a pull that proceeded here would silently destroy the edit.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### F7 -- Bound one-argument re-clone

**Covers:** fabric

**Goal:** "The dedicated fabric hub has already been cloned once with both URLs, which is what writes the warp-URL binding onto `weft:main`.
Delete the hub directory outright, then re-clone it supplying only the weft URL, and confirm the warp side is derived rather than asked for."

**Watch:** Confirm the re-clone succeeds with no warp URL on the command line at all -- the warp URL must be derived from the binding recorded on the weft side, not prompted for or defaulted some other way.
Confirm the hub comes up at the same path and identically wired: the same `_board`-is-a-weft-worktree check and the same `main-weft` branch check F1 already makes both hold here too.
Confirm the success envelope reports the derived warp URL, and reports that the binding was **not** re-written on this re-clone -- it already existed from the earlier two-URL clone.
Confirm the binding record itself is present and tracked at the board root;
the record's filename is the one the fabric module documents (`.lyx-warp`), so confirm that exact name rather than guessing at it.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### F8 -- Subpath-anchored hub, end to end

**Covers:** fabric

**Goal:** "Clone a hub anchored at a subdirectory of the warp repo rather than its root, and confirm the whole verb surface works from the anchored directory and nowhere else."

**Watch:** Clone with `lyx fabric clone --subpath <dir> <weft-url> <warp-url>`, where `<dir>` is a directory that really exists in the warp repo.
Confirm `_lyx`, `.lyx` and `_board` land as links inside `<warp>/<dir>/`, and that the warp repo ROOT has none of them — `ls -la` both.
Confirm every verb runs from `<warp>/<dir>` (`status`, `pairs`, `list`, `reconcile`, `prune`, `cleanup`, `sync`), and that running any of them from the warp repo root, from a subdirectory of `<dir>`, or from a sibling directory is REFUSED with an error naming both cwd and the anchored directory — not a confusing downstream failure.
Confirm `lyx fabric add <slug>` produces a second pair anchored identically (`<hub>/<slug>/<dir>/_lyx`), and that `lyx fabric commit` after editing `<dir>/_lyx/...` commits to the weft at `<dir>/_lyx/...` while leaving the warp side untouched (`git -C <warp> status`).
Confirm a bad `--subpath` is refused before anything is created and leaves no hub behind: an absolute path (`--subpath /<dir>`), one escaping the repo (`--subpath ../..`), one naming a file, and one naming a directory that does not exist.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### F9 -- Reconcile never eats warp content

**Covers:** fabric

**Goal:** "Confirm `lyx fabric reconcile`, the repair verb, converges wiring without deleting anything the operator put in the warp repo themselves."

**Watch:** In a wired worktree, commit a real symlink of your own beside the junctions at the anchored directory — e.g. `ln -s <existing-dir> latest && git add latest && git commit`.
Run `lyx fabric reconcile`.
The symlink must still be there afterwards, and `git status` must be clean: fabric owns only the links it created (those pointing into the paired weft worktree or the hub's `_board`), and a link pointing anywhere else is the operator's, never a "stale junction" to sweep.
Then check the anchor-marker migration guard: rename `<hub>/_board/.lyx-anchor` to `.fabric-anchor` and run any verb.
Every verb must hard-error naming both marker names and the rename remedy — it must NOT fall back to treating the repo as root-anchored, which on a subpath hub would wire a second junction set at the warp repo root.
Rename it back and confirm normal operation resumes.

**Verdict:** `OK` / `WARN` / `FAIL`

---

## Session log format

After running all scenarios, record a short session summary:

```
Date: <YYYY-MM-DD>
Binary fingerprint: <copy from the header above>

F0: <OK|WARN|FAIL> -- <one-line note if not OK>
F1: <OK|WARN|FAIL> -- <one-line note if not OK>
F2: <OK|WARN|FAIL> -- <one-line note if not OK>
F3: <OK|WARN|FAIL> -- <one-line note if not OK>
F4: <OK|WARN|FAIL> -- <one-line note if not OK>
F5: <OK|WARN|FAIL> -- <one-line note if not OK>
F6: <OK|WARN|FAIL> -- <one-line note if not OK>
F7: <OK|WARN|FAIL> -- <one-line note if not OK>

sandbox-report.json written: <count of WARN/FAIL items>
```

`./sandbox-report.json` must be written before the session ends, per the Capturing findings section above -- with `items: []` when every scenario was `OK`.

## Notes

- This suite is deliberately scoped to fabric alone and runs against its own dedicated hub -- it does not touch, and is not touched by, `SANDBOX-CORE-SUITE.md`'s warp/weft scenarios against `lyx-test`/`lyx-test-weft`.
- fabric is lyx's sole warp↔weft git-coordination module (see `internal/fabricengine/doc.go`);
  its stricter `main-weft`-suffixed branch-naming scheme is exactly why this suite runs against its own dedicated hub rather than the shared sandbox hub.
