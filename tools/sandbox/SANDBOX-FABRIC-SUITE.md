# SANDBOX-FABRIC-SUITE -- lyx fabric black-box agent suite

## What this is

A structured test-loop for exercising `lyx fabric` against the same shared GitHub test repos (`Knatte18/lyx-test` as warp, `Knatte18/lyx-test-weft` as weft) the main and per-module suites use.
This suite proves fabric's stricter `main-weft`-suffixed branch-naming scheme holds up under real use -- including this suite's own destructive scenarios (re-clone, `--reset`, `prune --apply`, `cleanup --apply`), which a sandbox Hub exists to absorb; `sandbox/build.cmd -reset` is the recovery path if a run leaves the Hub in a state another suite would trip on.

Like the other suites, the value is a Claude session driving `lyx fabric` by hand in a real hub, treating every break, surprise, or rough edge as a LoomYard finding to record in the report.

## Pre-conditions

Before starting a session:

1. **Deploy a fresh binary.**
   Run `deploy.cmd` so `lyx.exe` on PATH is current source.
   The deployed binary is a snapshot -- re-deploy after any source change you want to test.
2. **Materialize the hub.**
   Run `sandbox/build.cmd` (or `sandbox/build.cmd -reset` to start clean) -- the same operating model as the main and per-module suites, which this suite now shares rather than materializing its own dedicated hub.

   **Re-clone adoption.**
   When a clone runs against a weft remote that already carries the suffixed primary branch (a fresh machine, or `clone --reset`), the weft prime adopts `origin/main-weft` as a tracking branch — inheriting the previously synced weft state — rather than forking a new, untracked `main-weft` at `main`'s HEAD.
   After any re-clone, confirm the weft prime's `main-weft` has an upstream (`git -C <weft-prime> branch -vv`) and contains the previously synced `_lyx/` content.
3. **`lyx` on PATH.**
   Confirm `lyx --help` works from any directory.

### PowerShell JSON-quoting

See the PowerShell JSON-quoting note in `SANDBOX-CORE-SUITE.md`'s Pre-conditions -- the same caveat applies here whenever a scenario below passes JSON as an argument.

### Operating model

lyx resolves against the current directory's own `_lyx/` and does **not** walk up to a parent.
The hub warp repo is initialized at its root, so the agent runs the entire session from there (cwd is fixed at the root).

## Black-box rule

**The agent under test works exclusively inside the Hub's warp repo (`lyx-test-HUB/lyx-test`).
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
- `ref` is the scenario id (`F0`-`F21`).
- `title` is a short one-line summary.
- `body` folds the detail, repro steps, and verdict into one markdown string.

Write only `source` and `items` -- a separate fetch step (run after the session) stamps `meta` (including the binary fingerprint).
Confine all free text to the `title`/`body` string fields so the JSON stays well-formed.

## Scenarios

### F0 -- Discovery (help surface smoke test)

**Goal:** "You have `lyx` on PATH and nothing else inside this repo.
Find out what `lyx fabric` can do and report its full command tree."

**Watch:** Does `lyx fabric` list all 19 verbs (`clone`, `add`, `list`, `remove`, `checkout`, `pairs`, `reconcile`, `prune`, `cleanup`, `status`, `commit`, `push`, `pull`, `sync`, `diff`, `unwire`, `merge`, `merge-in`, `merge-stage`)?
Does each `--help` explain itself?
Is each description accurate and useful?

**Verdict:** `OK` / `WARN` / `FAIL`

---

### F1 -- Clone geometry

**Covers:** fabric

**Goal:** "Confirm the Hub the launcher just materialized (or reused) looks the way `lyx fabric clone` promises."

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
The envelope also carries a `merge_in_progress` boolean; confirm it is `false` in this scenario's no-merge state.
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

**Goal:** "The Hub has already been cloned once with both URLs, which is what writes the warp-URL binding onto `weft:main`.
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
Confirm `_lyx` and `.lyx` land as links inside `<warp>/<dir>/`, that `<warp>/<dir>/` has no `_board` entry at all, and that the warp repo ROOT has none of `_lyx`/`.lyx` either — `ls -la` both.
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
The symlink must still be there afterwards, and `git status` must be clean: fabric owns only the links it created (those pointing into the paired weft worktree), and a link pointing anywhere else is the operator's, never a "stale junction" to sweep.
Then check the anchor-marker migration guard: rename `<hub>/_board/.lyx-anchor` to `.fabric-anchor` and run any verb.
Every verb must hard-error naming both marker names and the rename remedy — it must NOT fall back to treating the repo as root-anchored, which on a subpath hub would wire a second junction set at the warp repo root.
Rename it back and confirm normal operation resumes.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### F10 -- Destructive-verb guardrails (`remove`, `cleanup`)

**Covers:** fabric

**Goal:** "Confirm the two verbs that delete things refuse the targets that are not theirs to delete, instead of deleting them and reporting success."

**Watch:** Run `lyx fabric remove <prime-slug>` -- the hub's own warp worktree directory name, the one every operator can reach by tab-completion.
It must be **refused**, naming the prime worktree, and `<hub>/<prime>` must still be there afterwards WITH its `.git` directory: git declines to remove a main working tree, and fabric must report that rather than deleting the clone. (Historical: this deleted the entire warp repository, gitdir included, on a clean hub with no `--force`, while the JSON envelope claimed "warp worktree removed".)
Then run `lyx fabric remove _board` and `lyx fabric remove <prime>-weft`.
Both must be refused with an `invalid slug` error and both directories must survive intact -- `_board` holds `.lyx-anchor`, `.lyx-warp` and the repo-wide `fabric.yaml`, and `<prime>-weft` is the entire durable `_lyx` store. `lyx fabric add` already refuses exactly this name set;
`remove` must refuse the same one.
Then the cleanup half: with the prime pair on the default branch, `lyx fabric checkout <other-branch>` to move it off, then run `lyx fabric cleanup --apply --force` and confirm `main-weft` is reported `protected: true` and still exists (`git -C <hub>/<prime>-weft branch`).
The primary weft branch is the durable weft line whatever the prime happens to be checked out on;
promoting it to a deletable orphan destroys any weft commit it alone carries.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### F11 -- Dirty-worktree matrix across the mutating verbs

**Covers:** fabric

**Goal:** "Cross every mutating verb with uncommitted work actually on disk, and confirm no verb silently discards it."

**Watch:** Make an uncommitted edit to a TRACKED file on one side, then run one mutating verb, then check the edit is still there byte-for-byte.
Repeat across the verbs -- `pull`, `sync`, `checkout`, `reconcile`, `remove`, `cleanup`, `unwire`, `prune`, `add`, `commit` -- and across the states: dirty warp only, dirty weft only, both dirty, untracked-only on each side.
A verb that REFUSES is fine. A verb that proceeds and leaves the work intact is fine. A verb that proceeds and discards it is a FAIL, whatever it reported.
Expected refusals worth confirming by name: `pull` refuses a dirty warp (`ErrWarpDirty`, before warp moves at all), `checkout` refuses a dirty weft, `add` refuses a dirty source worktree, `remove` refuses either side without `--force`, and `prune --apply` reports `protected: true` for a stale pair whose weft worktree is dirty until `--force` is given.
Untracked files are deliberately NOT a reason to refuse -- but confirm they genuinely survive every verb that proceeds, rather than being swept along with something else.
Do the whole matrix on a `--subpath`-anchored hub, since that is where the anchored pathspecs decide which side an edit lands on.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### F12 -- The hub is not fabric's to sweep (`prune`, `clone --reset`)

**Covers:** fabric

**Goal:** "Park things in the hub that fabric did not create and must never delete, then run the two verbs that delete by derived name and confirm both refuse."

**Watch:** First `prune`. Create `<hub>/notes-weft/` holding a file, as an operator keeping scratch notes beside their worktrees, and separately `git init` a wholly unrelated project at `<hub>/proj-weft/` with one committed file and a clean working tree.
Neither is a fabric worktree; both merely end in the weft suffix, which is all prune's orphan pass enumerates on.
Run `lyx fabric prune`, then `lyx fabric prune --apply`, then `lyx fabric prune --apply --force`.
All three must report each entry `"unowned": true` with `"removed": false` and an error naming the weft repo it is not a worktree of, and **both directories must survive every run byte-for-byte, `.git` included**.
`--force` must NOT get through: force answers "discard this uncommitted work", never "this directory is mine". (Historical: `prune --apply` deleted both, reporting `removed: true`, `ok: true`, exit 0, with no `--force` and no warning -- `git worktree remove` refusing the path was read as licence to `os.RemoveAll` it.)
Also confirm the gate refuses only what is not fabric's: make a genuine stale pair (delete a `<slug>/` warp worktree directory by hand, leaving its registration) and check `prune --apply` still removes its weft side.

Then `clone --reset`. In an empty directory create `<name>-HUB/important/data.txt`, where `<name>` is the basename your warp URL derives to, and run `lyx fabric clone --reset <weft-url> <warp-url>` there.
It must be **refused**, naming the path and saying it is not a fabric hub, and `data.txt` must survive.
The hub name is derived rather than typed -- in the one-argument form it comes from the binding recorded on the weft, so the operator never even sees the name being deleted.
Then re-run `--reset` against a real hub and confirm the idempotent re-clone still works.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### F13 -- Bootstrap against a genuinely empty weft remote

**Covers:** fabric

**Goal:** "Clone the documented first-ever-setup shape -- a brand-new, zero-commit weft remote -- and confirm the hub it produces is actually usable."

**Watch:** Create a bare weft remote with no commits at all (`git init --bare`, `HEAD` on `main`) and a warp remote with real content, then `lyx fabric clone <empty-weft-url> <warp-url>`.
The clone reports `ok: true` -- it always did -- so the check is what comes after.
Confirm `git -C <hub>/<prime>-weft rev-parse --verify refs/heads/main-weft` RESOLVES.
A branch can be checked out and reported as current while its ref does not exist: `git checkout -b` on an unborn HEAD writes nothing.
Then run the documented example, `lyx fabric add my-task`, and `lyx fabric remove my-task`.
(Historical: the weft primary was left on an unborn `main-weft`, so every pair-creating verb died on `fatal: invalid reference: main-weft` -- `add` included, which is the example both `lyx fabric --help` and `lyx fabric add --help` print.)
Repeat the whole scenario with `--subpath backend`, since the anchor and the branch are resolved in separate steps.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### F14 -- A slug is a directory name, not a path (`remove`, `add`)

**Covers:** fabric

**Goal:** "Hand every teardown verb a slug that is a relative path element rather than a name, and confirm nothing outside the named pair is touched."

**Watch:** On a hub with one added pair, run `lyx fabric remove ..`, then `lyx fabric remove .`, then `lyx fabric remove ../evil`, `lyx fabric remove a/b`, and `lyx fabric remove ""`.
Every one must be refused BEFORE any teardown runs, and after all five the hub must still list `_board _launchers _portals <prime> <prime>-weft <slug> <slug>-weft` exactly as before, with `_launchers` still holding `ide-menu.sh` and the pair's directory.
Then confirm an ordinary `lyx fabric remove <slug>` still works.
(Historical: `.` and `..` passed every rule in the shared slug validator -- not empty, no separator, no weft suffix, no reserved hub name -- and `<hub>/_launchers/<anchor>/<slug>` then resolved to `<hub>` itself, whose `os.RemoveAll` deleted the warp clone, the weft clone, `_board`, every pair and all uncommitted work, after which the verb returned `"failed to check worktree status"` -- an error claiming nothing had happened.)
Repeat on a `--subpath backend` hub, where the anchor segment absorbs one `..` and the damage lands one level lower.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### F15 -- Two verbs at once on one hub (`unwire`, `reconcile`, `add`, `remove`)

**Covers:** fabric

**Goal:** "Run fabric verbs simultaneously in different worktrees of one hub and confirm the shared state they all write survives the interleaving."

**Watch:** On a hub with six added pairs, append a pattern of your own -- say `/my-secret-build-dir` -- to `<warp>/.git/info/exclude`, and note that the file also carries git's default six-line comment block.
That file lives in the repo's COMMON gitdir, so it is ONE file for the whole hub, not one per worktree.
Now launch all six worktrees at once (background five, run one in the foreground, `wait`), alternating `lyx fabric unwire` and `lyx fabric reconcile`, and repeat for ten rounds, printing the file's line count after each.
The count must never drop and your own pattern must be present after every round, alongside `/_lyx` and `/.lyx` exactly once each.
(Historical: the count collapsed from 10 lines to 3 at round 6 and to 1 at round 9 on two independent hubs -- the operator's pattern and git's comment block destroyed, and fabric's own junction exclusions transiently lost, which makes every worktree's junctions show as untracked and a plain `git add -A` commit symlinks into the warp repo. The identical sequence run sequentially never lost a line.)
Also run the destructive verbs against each other -- `prune --apply` alongside `checkout`, `cleanup --apply` alongside `commit`, `add` alongside `remove` -- and confirm every verb either succeeds or refuses with git's real message, and `lyx fabric pairs` afterwards reports every surviving pair `in_sync` and `junction_healthy`.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### F16 -- The post-checkout hook actually fires (`clone`, `add`)

**Covers:** fabric

**Goal:** "Confirm the drift-warning hook is installed where git looks for it, runs, and neither clobbers nor re-enables an operator's own hook."

**Watch:** After `clone`, `git checkout -b feature` in the warp worktree must print `fabric: warp/weft out of sync` naming both branches, and `lyx fabric checkout feature` must NOT leak that text into its JSON output.
Then three variations, each on its own hub.
(a) Put an EXECUTABLE `post-checkout` of your own in `<warp>/.git/hooks/`, run `lyx fabric add <slug>`, and confirm it was moved to `post-checkout.user`, that a real `git checkout` runs your hook first and then prints fabric's warning.
(b) Repeat with your hook at mode 0644 -- a hook you had DISABLED. The wrapper git ends up with must be executable (fabric's warning still fires), the backup must still be 0644, and your disabled hook must not run. (Historical: `os.WriteFile` applies its perm argument only when it CREATES the file, so the wrapper inherited 0644 and git printed `hint: the hook was ignored because it's not set as executable` -- silently retiring both hooks.)
(c) Set `core.hooksPath` to a directory outside `.git`, run `lyx fabric add <slug>`, and confirm the hook lands in THAT directory and that `git checkout` onto a diverged branch still warns. (Historical: fabric composed `<git-common-dir>/hooks`, which ignores `core.hooksPath`, wrote into a directory git no longer consults, and reported success.)

**Verdict:** `OK` / `WARN` / `FAIL`

---

### F17 -- The portal and launcher surface is repairable (`add`, `reconcile`, `remove`)

**Covers:** fabric

**Goal:** "Break the hub-level `_portals`/`_launchers` artefacts and confirm the repair verb repairs them and the teardown verb owns only what it created."

**Watch:** After `lyx fabric add <slug>`, confirm `<hub>/_portals/<slug>` is a link resolving through to `<hub>/<slug>-weft/_lyx` and `<hub>/_launchers/<slug>/` holds `ide` and `fabric-checkout` scripts.
Delete both, then run `lyx fabric reconcile`: it must report `portal_restored` for that pair -- not `already_healthy` -- and both artefacts must be back. A second `reconcile` must report `already_healthy`. (Historical: reconcile reported `already_healthy` and `pairs` reported `junction_healthy: true` for a pair whose portal and launchers were gone, and restored neither, leaving remove-and-re-add as the only recovery.)
Then ownership: put a file of your own inside `<hub>/_launchers/<slug>/` and run `lyx fabric remove <slug>`. Fabric's two scripts must go, your file must survive, and the pair must still be torn down. Do the same with `<hub>/_portals/<slug>` replaced by a real directory holding a file -- it too must survive.
Finally confirm the hub's prime worktree is left alone throughout: it never had a portal or a launcher directory, and `reconcile` must not start creating them.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### F18 -- Merge lifecycle end to end (`merge-in`, `merge-stage`, `merge --continue`, `merge --abort`)

**Covers:** fabric

**Goal:** "Merge a task pair's branch into the hub's prime pair, resolve the conflict it produces, conclude it -- then do the same merge again and abandon it instead."

**Watch:** Create a pair, diverge it from the prime on **both** sides so the merge is a real merge and not a fast-forward, then run `lyx fabric merge-in <slug>` from the prime worktree.
A conflict is a *result*, not a failure to re-run differently: the envelope is `ok: false`, exit 1, and carries a `conflicts` array of **worktree-relative** paths -- one flat list covering both repos, never two lists and never an absolute path.
Note the exit code: a conflict result and a hard error both exit 1, so the only correct discriminator for a script is the presence of the `conflicts` key.
While the merge is live, confirm the sibling verbs refuse with the single fixed message `a merge is in progress; run "lyx fabric merge --continue" or "lyx fabric merge --abort" first` -- try `commit`, `pull`, `push`, `sync`, `checkout`, and `remove` of the merge's own source pair.
`fabric status`, run from the **prime worktree** (the pair holding the merge record), is the read-only way to ask whether the merge is parked: it reports `merge_in_progress: true` for the whole live window and `false` again after both `merge --continue` and `merge --abort`.
That field does NOT predict every refusal in the list two lines above: `remove` of the merge's own source pair refuses on a hub-wide predicate, and run from *that* pair `merge_in_progress` is correctly `false`.
Resolve every listed path, then `lyx fabric merge-stage <those same paths>`, then `lyx fabric merge --continue`, and confirm both sides carry a merge commit whose subject names a **SHA, never a branch**.
The `merge-stage` step is not optional and is worth checking on its own: `--continue` gates on the git INDEX, not on file content, so editing a conflicted file and going straight to `--continue` must still refuse with `unresolved conflicts remain`.
Make at least one of the conflicts land under `_lyx/`, and confirm plain `git add _lyx/<file>` from the visible worktree is REFUSED by git (`pathspec ... is beyond a symbolic link`) while `merge-stage` accepts the same path -- for a weft-side conflict that verb is the only route, and without it the merge is uncompletable through the CLI. (Historical: `merge-stage` had no CLI surface at all, so this help text told the operator to run a step that could not be run.)
Also confirm `merge-stage` with a path that is not conflicted fails the WHOLE call, leaving the genuinely conflicted paths in the same call still unstaged.
Repeat the whole scenario and take `lyx fabric merge --abort` instead: both sides must return to their exact pre-merge SHAs, both worktrees must come back clean, and `git status` in each checkout must show no merge in progress.
Also check `committed`: it is true only when a conclude-commit actually landed, so a merge that fast-forwarded both sides reports `committed: false` while still having advanced the pair.
Then repeat one conflicted round with a filename outside ASCII (e.g. `ä-note.md`) conflicting on each side in turn: the `conflicts` array must carry the real path, byte for byte -- never git's C-quoted rendering (`"\303\244..."`, quotes included) -- and a non-ASCII conflict inside the fabric-managed tree must never abort as *conflicts outside the fabric-managed tree*. (Historical: `ConflictedFiles` read `--name-only` without `-z`, so `core.quotepath` quoting made exactly that happen.)

**Verdict:** `OK` / `WARN` / `FAIL`

---

### F19 -- Merge preconditions and hostile local state (`merge`, `merge-in`)

**Covers:** fabric

**Goal:** "Confirm a merge refuses every state it cannot safely act on, and that each refusal leaves the hub exactly as it found it."

**Watch:** Each of the following must refuse with **nothing mutated** (`mutations: []`, both HEADs unmoved) and, where the reason set applies, a `merge preconditions failed: ...` message that names no side and no path:

- A dirty worktree on either side -> `worktree dirty`.
- A detached HEAD on either checkout (`git checkout --detach`) -> `checkout is not on a branch`. This one matters: without it the merge would land a warp commit reachable from no ref while the weft half landed for good.
- A source that is a tag, a raw SHA, `HEAD`, or a branch with no `-weft` counterpart -> `source branch is not fabric-managed`; a source that exists nowhere -> that plus `source branch not found`.
- Real plain-git merge state you leave behind yourself (`git merge <branch>` conflicted, or `git merge --squash <branch>` conflicted -- the latter leaves **no** `MERGE_HEAD`) -> `git merge state exists that fabric did not start`, with the foreign state left untouched for you to finish with plain git. `lyx fabric commit` over that same foreign state must give the **same** foreign-state message -- not `a merge is in progress; run "lyx fabric merge --continue" or "lyx fabric merge --abort" first`, whose advice both those verbs would refuse.

Then set hostile git config and confirm fabric is immune to it: `merge.ff = only` must not break a non-fast-forward merge (fabric pins `--ff`), and `core.editor` set to something that blocks forever must not hang `merge --continue` (fabric pins `--no-edit`). A hang here is a blocking defect.
Finally, check the flag pre-flight: `merge --abort --squash` and `merge --abort -m <msg>` are both rejected as usage errors rather than silently ignoring the flag, while `merge --continue -m <msg>` really does name the conclude-commit.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### F20 -- A merge that changes nothing, and an abort after half the conclude landed (`merge-in`, `merge --abort`, `merge --squash`)

**Covers:** fabric

**Goal:** "Drive the two merge shapes where fabric's own bookkeeping and git's can come apart: a merge whose result changes nothing, and an abort issued after one side's conclude-commit has already landed."

**Watch:** First the empty-result merge. On the warp side, branch `dup` off the current branch and change a file;
then go back to the current branch and make the **same** change again, independently, so `dup` is not an ancestor of HEAD but merging it produces no difference.
Do the same on the weft side for `dup-weft`.
Run `lyx fabric merge-in dup`.
It must land a real conclude-commit and report `"committed": true` -- **not** `"already_up_to_date": true`.
Then check both checkouts with plain `git status`: neither may say "you are still merging".
A merge fabric reported as a clean no-op while leaving a live `MERGE_HEAD` behind is the failure mode here, and it is a blocking one -- the record and git disagree, and every later merge verb (including `merge --abort`) refuses the pair as carrying state fabric did not start.
Confirm recovery is unnecessary by running `lyx fabric merge --abort`: it must say *no merge in progress*, never *git merge state exists that fabric did not start*.

Then the squash companion: repeat the same fixture and run `lyx fabric merge dup --squash`.
Squash genuinely has nothing to commit here, so `"already_up_to_date": true` with `"committed": false` **is** the right answer, and no `MERGE_HEAD` may appear.

Finally the half-concluded abort. Build a normal divergent merge on both sides, install a `pre-commit` hook in the weft checkout that just does `exit 1` (`.git/hooks/pre-commit`), and run `lyx fabric merge-in <branch>`.
The warp conclude lands, the weft conclude fails, and you get *merge conclude did not finish; run "lyx fabric merge --continue" again* with a `merge_committed` mutation for the warp side.
Now run `lyx fabric merge --abort`.
It must **refuse**, naming `merge conclude already landed`, and the warp conclude-commit must still be there (`git -C <warp> log -1`).
An abort that reports `"ok": true` here and silently resets the warp past its landed conclude-commit is destroying committed work -- in the conflict flow that commit carries your own hand-written resolutions.
Remove the hook and confirm `lyx fabric merge --continue` finishes the job, skipping the side that already landed.

Last, the invisible landed conclude -- the crash shape where a side's conclude-commit landed but the record never learned its SHA.
Build a conflicted `merge-in`, resolve the conflict, `git add` it, then commit it yourself with plain `git commit --no-edit` in that checkout -- on-disk state now identical to a kill between fabric's conclude-commit and its record re-save (`*_committed` still empty, HEAD on the merge commit, no `MERGE_HEAD`).
`lyx fabric merge --abort` must refuse (`merge conclude already landed`), and `lyx fabric merge --continue` must **succeed by adoption**: `"committed": true`, a `merge_committed` mutation carrying the hand-landed SHA, HEAD unmoved (no second commit), record gone, sibling verbs unblocked.
A `--continue` that loops forever on *merge conclude did not finish; run "lyx fabric merge --continue" again* here is the failure mode: the pair is then permanently wedged, since no fabric verb can clear the record and plain git cannot reach it.

Now the adversarial twin of that same shape, which looks identical to fabric from the outside and must NOT be adopted.
Build the conflicted `merge-in` again, then instead of resolving it, discard it with plain `git merge --abort` in the warp checkout and make one ordinary commit of your own there — anything, an unrelated file.
HEAD has now moved off the recorded pre-merge SHA with no `MERGE_HEAD`, exactly as a landed conclude leaves it, but nothing was merged.
`lyx fabric merge --continue` must **refuse**: *merge conclude did not finish; run "lyx fabric merge --continue" again*, with the record still on disk.
`"ok": true` / `"committed": true` naming your unrelated commit is the failure mode, and it is a blocking one — the record is deleted, the source is still un-merged, and there is nothing left to inspect.
`merge --abort` refusing too (`merge conclude already landed`) is correct here, not a second bug: the pair is honestly stuck, and plain git is the documented way out.

Finally the squash version of the same question, which has no evidence either way and must therefore also refuse.
Install the failing `pre-commit` hook in the **warp** checkout, run `lyx fabric merge <branch> --squash`, remove the hook, then land the squash conclude yourself with plain `git commit -m ...`.
`lyx fabric merge --continue` must refuse with *merge conclude did not finish*, leave the record on disk, and leave your commit untouched — a squash conclude is a one-parent commit indistinguishable from any other, so adopting it would be a guess.

Last, the octopus — a commit that satisfies every *other* adoption clause and still must be refused.
Build the merge so the warp side merges CLEANLY (a real non-fast-forward merge of a branch that touches different files) while the weft side conflicts, so `merge-in` returns conflicts and the record survives with the warp side staged and unconcluded.
Then, in the warp checkout, `git merge --abort`, branch a decoy off the repo's ROOT commit (`git rev-list --max-parents=0 HEAD`) with one unrelated file on it, and merge BOTH at once: `git merge <the record's warp_source> <decoy>`.
Read the record at `<weft checkout>/.git/fabric-merge.json` for that source SHA;
rooting the decoy outside the merge's own history is what stops git dropping the pre-merge tip as a redundant parent, so confirm with `git rev-list --parents -n 1 HEAD` that you really got THREE parents, the first being the record's `warp_start` and the second its `warp_source`.
`lyx fabric merge --continue` must refuse with *merge conclude did not finish; run "lyx fabric merge --continue" again* and leave the record on disk.
Reporting `"committed": true` here is the failure mode and it is a blocking one: fabric would be claiming a commit it can never build — it starts every non-squash merge with a single `git merge --ff --no-commit <sha>`, so its own conclude has exactly two parents — and the branch would silently carry the decoy's content, brought in by no side of the merge and named by no `merge_staged` entry, with the record deleted and nothing left to inspect.

Last of all, the same three questions asked WITHOUT committing, which is the half the adoption evidence never sees.
Everything above reaches the adoption arm because the operator committed;
leaving the hand-made merge uncommitted routes into the conclude's plain `git commit` instead, and that arm demanded no evidence at all until this round.
Build the warp-merges-cleanly / weft-conflicts fixture again, resolve and `lyx fabric merge-stage` the weft conflict so `--continue` really reaches the warp side, then in the warp checkout run `git merge --abort` and, in place of committing anything, do each of these in turn:

- `git merge --no-commit --no-ff <an unrelated branch>` — a different merge left live.
- `git merge --no-commit <the record's warp_source> <a decoy rooted at the repo's root commit>` — an uncommitted octopus. Confirm with `git rev-parse --verify --quiet MERGE_HEAD` that it prints exactly the record's `warp_source`: that truncated answer is what a first-head-only check would accept, and `cat "$(git rev-parse --git-path MERGE_HEAD)"` must show two SHAs.
- nothing merged at all, just `git add` an unrelated new file — no `MERGE_HEAD`, but a commit that would succeed.

In all three, `lyx fabric merge --continue` must refuse with *merge preconditions failed: checkout no longer carries the recorded merge*, leave the record on disk, land no commit, and leave the warp HEAD exactly where the operator left it.
`"ok": true` / `"committed": true` here is the failure mode and it is a blocking one: fabric commits and claims a merge it never started, records correspondence for a pair whose warp side does not carry the source at all, and deletes the record — the same silent false success as the adoption cases, reached by not committing first.
Then confirm the refusal did not wedge the pair: `lyx fabric merge --abort` must still succeed and restore both sides to their recorded pre-merge SHAs.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### F21 -- A target diverged from an upstream it has not fetched yet (`merge`)

**Covers:** fabric

**Goal:** "Confirm the not-synced guard refuses a genuinely diverged target even when the divergence has not been fetched at the moment the guard runs."

**Watch:** This is the ordinary shape, not a contrived one: a teammate pushes while you are working, and you have not fetched since.
Create a target pair and a source pair, then push a commit to the target branch's own remote **from a separate clone**, and only then make one local commit in the target worktree -- never running `git fetch` in it.
At this point the target is genuinely diverged, but its own `origin/<branch>` still names a commit that is an ancestor of its HEAD, so anything reading remote-tracking refs sees "merely ahead".
Confirm that first, so you know the scenario is really the unfetched shape: `git -C <target> rev-parse origin/<branch>` must differ from the bare remote's own tip, and `git -C <target> merge-base --is-ancestor origin/<branch> HEAD` must succeed.

Now run `lyx fabric merge <source-slug>` from the target worktree.
It must **refuse** with `merge preconditions failed: branch not synced to upstream`, `mutations: []`, and both HEADs unmoved.
(Historical: it returned `ok: true` with `committed: true` and landed the merge, because the guard read `@{u}` before the call's own fetch and the sync step then discarded the divergence it had just made visible. `git rev-list --left-right --count 'HEAD...@{u}'` afterwards reported a real divergence in both directions.)
Run the same check with the divergence on the **weft** side instead of the warp side -- both sides carry their own upstream, and each is decided separately.

Then confirm the guard closes a window rather than blocking the verb: `git fetch` and reconcile the divergence, and the same `merge` must now succeed.
Finally check the opposite state is NOT refused -- a target merely **behind** its upstream must be fast-forwarded by the pre-merge sync step (`repo_advanced` in the mutation record) and then merged, never refused, since that is the whole reason the sync step exists.

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
F8: <OK|WARN|FAIL> -- <one-line note if not OK>
F9: <OK|WARN|FAIL> -- <one-line note if not OK>
F10: <OK|WARN|FAIL> -- <one-line note if not OK>
F11: <OK|WARN|FAIL> -- <one-line note if not OK>
F12: <OK|WARN|FAIL> -- <one-line note if not OK>
F13: <OK|WARN|FAIL> -- <one-line note if not OK>
F14: <OK|WARN|FAIL> -- <one-line note if not OK>
F15: <OK|WARN|FAIL> -- <one-line note if not OK>
F16: <OK|WARN|FAIL> -- <one-line note if not OK>
F17: <OK|WARN|FAIL> -- <one-line note if not OK>
F18: <OK|WARN|FAIL> -- <one-line note if not OK>
F19: <OK|WARN|FAIL> -- <one-line note if not OK>
F20: <OK|WARN|FAIL> -- <one-line note if not OK>
F21: <OK|WARN|FAIL> -- <one-line note if not OK>

sandbox-report.json written: <count of WARN/FAIL items>
```

`./sandbox-report.json` must be written before the session ends, per the Capturing findings section above -- with `items: []` when every scenario was `OK`.

## Notes

- This suite is deliberately scoped to fabric alone, but runs against the same shared Hub (`lyx-test`/`lyx-test-weft`) as `SANDBOX-CORE-SUITE.md` and every per-module suite -- it is not isolated from them. Several scenarios here are destructive by design (F7's delete-and-re-clone, `--reset`, `prune --apply`, `cleanup --apply`); if a run leaves the Hub in a state another suite would trip on, `sandbox/build.cmd -reset` is the recovery path, not a fabric defect.
- fabric is lyx's sole warp↔weft git-coordination module (see `internal/fabricengine/doc.go`);
  its stricter `main-weft`-suffixed branch-naming scheme is exactly what this suite exists to exercise, on the same Hub every other suite already proves the ordinary CLI surface against.
