# SANDBOX-FABRIC-SUITE -- lym fabric black-box agent suite

## What this is

A structured test loop for exercising `lym fabric`.
It runs against the shared GitHub test repos the main and per-module suites also use:
`Knatte18/lym-test` as warp and `Knatte18/lym-test-weft` as weft.
This suite proves fabric's stricter `main-weft`-suffixed branch-naming scheme holds up under real use.
Its destructive scenarios (re-clone, `--reset`, `prune --apply`, `cleanup --apply`) run in a sandbox Hub built to absorb them.
`sandbox/build.cmd -reset` is the recovery path if a run leaves the Hub in a state another suite would trip on.

The value is a Claude session driving `lym fabric` by hand in a real hub.
The session treats every break, surprise, or rough edge as a LoomYard finding to record in the report.

## Pre-conditions

Before starting a session:

1. **Deploy a fresh binary.**
   Run `deploy-dev.cmd` so the dev `lym.exe` in `.dev-bin` is current source.
   The deployed binary is a snapshot -- re-deploy after any source change you want to test.
2. **Materialize the hub.**
   Run `sandbox/build.cmd`, or `sandbox/build.cmd -reset` to start clean.
   The main and per-module suites share this hub and the same operating model.

   **Re-clone adoption.**
   A weft remote can already carry the suffixed primary branch, as on a fresh machine or with `clone --reset`.
   A clone against it behaves as follows.
   The weft prime adopts `origin/main-weft` as a tracking branch, inheriting the synced weft state.
   It does not fork a new, untracked `main-weft` at `main`'s HEAD.
   After any re-clone, confirm that the weft prime's `main-weft` has an upstream (`git -C <weft-prime> branch -vv`),
   and that it contains the previously synced `_lym/` content.
3. **`lym` on PATH.**
   Confirm `lym --help` works from any directory.

### PowerShell JSON-quoting

See the PowerShell JSON-quoting note in `SANDBOX-CORE-SUITE.md`'s Pre-conditions.
The same caveat applies here whenever a scenario below passes JSON as an argument.

### Operating model

lym resolves against the current directory's own `_lym/` and does **not** walk up to a parent.
The hub warp repo is initialized at its root, so the agent runs the entire session with the root as cwd.

## Black-box rule

**The agent under test works exclusively inside the Hub's warp repo (`lym-test-LYMHUB/lym-test`).
It tests `lym.exe` as a black box -- exactly as a real user with only the binary on PATH.
It must not look for, read, or reason about the lym source tree.
No peeking at `C:\Code\loomyard\` or any other path outside the Hub.**

The agent discovers the command surface with `lym fabric`, `lym fabric <subcommand>`, and `--help` flags alone.
It does not use documentation outside the Hub.

## Fingerprint header

The launcher prepends a "binary under test" fingerprint block to its copy of this file in the fabric Hub's warp repo.
The fingerprint records the binary's absolute path, size, modification time, and short SHA-256 at launch.

The fingerprint also gives the report its provenance.
A separate fetch step, run after this session, stamps it into `meta.fingerprint` of the fetched `sandbox-report.json`.
A maintainer can then reproduce the exact binary that produced each finding.
The agent does not need to transcribe the fingerprint anywhere itself.

## How to run a scenario

For each scenario below:

- Read the **Goal**, which names the task, not the commands.
  Discover the commands via `lym fabric`, `lym fabric <subcommand>`, and `--help` flags, as scenario F0 does.
- **Watch** what lym does.
  Note where it stalls, guesses wrong, or hits an error.
- Record the outcome per the verdict buckets: `OK` (worked) / `WARN` (rough edge) / `FAIL` (broke).

## Verdict key

- `OK`   -- completed without friction
- `WARN` -- completed but with confusion, awkward UX, or a non-fatal error
- `FAIL` -- did not complete;
  lym broke, panicked, or gave wrong output

## Capturing findings

After all scenarios are run, write **all** `WARN`/`FAIL` findings to `./sandbox-report.json` in the warp-repo cwd.
Use this exact schema.
**Always write the file, even when there are zero `WARN`/`FAIL` findings.**
In that case `items` is an empty array.

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
- `ref` is the scenario id, such as `F0` or `F2`.
- `title` is a short one-line summary.
- `body` folds the detail, repro steps, and verdict into one markdown string.

Write only `source` and `items`.
A separate fetch step, run after the session, stamps `meta`, including the binary fingerprint.
Confine all free text to the `title`/`body` string fields so the JSON stays well-formed.

## Scenarios

### F0 -- Discovery (help surface smoke test)

**Goal:** "You have `lym` on PATH and nothing else inside this repo.
Find out what `lym fabric` can do and report its full command tree."

**Watch:** Does `lym fabric` list all of its verbs?
The expected set is the topology verbs `clone`, `add`, `list`, `remove`, `checkout`, `pairs`;
the repair and cleanup verbs `reconcile`, `prune`, `cleanup`, `unwire`;
the sync verbs `status`, `commit`, `push`, `pull`, `sync`, `diff`;
and the merge verbs `merge`, `merge-in`, `merge-stage`.
Does each `--help` explain itself?
Is each description accurate and useful?

**Verdict:** `OK` / `WARN` / `FAIL`

---

### F1 -- Clone geometry

**Covers:** fabric

**Goal:** "Confirm the Hub the launcher just materialized (or reused) looks the way `lym fabric clone` promises."

**Watch:** `_bolt` is a linked worktree of the same weft repo as the weft prime, never a separate clone.
Confirm the same path comes out of both of these:
`git -C <weft-prime> rev-parse --git-common-dir` and `git -C _bolt rev-parse --git-common-dir`.
`_bolt` is checked out on the warp's own unsuffixed default branch, which is **never hardcoded to `main`**.
It is whatever the warp repo's actual default branch is.
`git -C _bolt branch --show-current` should confirm it directly;
this mirrors the assertion shape of `assertBoardIsWeftWorktree` in `internal/fabricengine/clone_adopt_test.go`.
The weft prime's checked-out branch is **`main-weft`**, not `main`.
Fabric's uniform branch-suffix scheme applies from the first pair.
Use `lym fabric pairs` and plain git (`git -C <weft-prime> branch --show-current`) to confirm;
neither should require guessing or `ls`-ing around.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### F2 -- Topology lifecycle

**Goal:** "Add a new warp+weft worktree pair, inspect it, and coordinate-checkout it.
Then reconcile it, prune it, and clean it up."

**Watch:** Run `lym fabric add <slug>` and confirm the new weft branch is named `<warp-branch>-weft`.
The suffix is fixed, not a mirrored name.
For example, slug `foo` with an empty branch prefix yields warp branch `foo` and weft branch `foo-weft`.
Does `lym fabric pairs` report the new pair as in-sync?
Does `lym fabric checkout` switch both sides together and re-point the junction?
Remove the warp side by hand (or via `lym fabric remove`).
Does `lym fabric reconcile` report and repair the drift sanely?
Dry-run `lym fabric prune` and `lym fabric cleanup` first, then run them with `--apply`.
Do they identify the orphaned `<slug>-weft` branch as fabric-managed by its suffix?
Do they handle it per the flag matrix described in `lym fabric cleanup --help`?

**Verdict:** `OK` / `WARN` / `FAIL`

---

### F3 -- Weft content sync

**Goal:** "Make a small, clearly-marked change inside the weft-tracked scope.
Run it through `fabric status`, `commit`, `push`, `pull`, and `sync`."

**Watch:** `fabric status` reports uncommitted changes on both sides as a list of `{path, side}` entries.
`side` is `"warp"` or `"weft"`.
Confirm the test change to the weft-tracked scope shows up as an entry with `side: "weft"`.
The envelope also carries a `merge_in_progress` boolean; confirm it is `false` in this scenario's no-merge state.
Do `commit`/`push` mirror the change to the weft remote?
The commit message is always the fixed string `"weft sync"`.
It is not generated from changed files.
There is no `-m` flag to customize it; confirm via `lym fabric commit --help`.
Every fabric weft commit also carries a trailing `Warp-SHA: <sha>` trailer.
It names the paired warp repo's current HEAD.
Inspect the commit body (`git -C <weft-worktree> log -1`): the trailer must name a real, resolvable warp commit.
`fabric sync` pushes via a detached child process, so `status` immediately after `sync` may lag behind the actual push.
That is a confusing-but-expected rough edge to note as a `WARN`, not to pre-judge here.
Staging is scoped to the directories listed in the fabric config (default `_lym`).
The test change must land inside that scope to be picked up at all.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### F4 -- Reserved-slug guardrail

**Covers:** fabric

**Goal:** "Confirm `lym fabric add` refuses a slug that would collide with the weft-worktree directory namespace.
It must refuse before it can create a booby-trapped pair."

**Watch:** Run `lym fabric add <name>-weft`, a slug ending in the reserved `-weft` suffix, e.g. `add zed-weft`.
It must be **rejected** with an `invalid slug` error and create nothing.
It must NOT create a warp worktree directory `<name>-weft`.
Such a directory is indistinguishable from a weft worktree.
A later `lym fabric prune --apply` would misclassify the warp worktree as an orphaned weft.
It would then delete the warp worktree, destroying any uncommitted work.
Follow the rejected add with `lym fabric list`, `lym fabric pairs`, and a plain `ls` of the hub.
No `<name>-weft` warp worktree should exist.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### F5 -- Junction deactivation (`lym fabric unwire`)

**Covers:** fabric

**Goal:** "Deactivate a wired worktree with `lym fabric unwire --apply`.
Confirm it tears down exactly what it should and nothing more.
Then confirm `lym fabric reconcile` re-wires it afterward."

**Watch:** On a wired worktree (the fabric prime, or a pair added in F2), run bare `lym fabric unwire` first.
Confirm it reports `"applied": false` with the junctions it would remove.
Confirm those junctions are all still wired.
Then run `lym fabric unwire --apply` on the same worktree.
Confirm it removes every fabric junction present on disk (e.g. `_lym`, `.lym`).
`ls`, or `git -C <warp> ls-files --others -i --exclude-standard`, should show the junction entries gone.
Confirm it preserves the weft-side `_lym` content.
`_lym/` under the paired weft worktree must be untouched, not cleared; unwire never touches that content.
Confirm it reverts the junction's own `.git/info/exclude` entry.
There is no committed `.gitignore` block to revert, since junctions are excluded through `.git/info/exclude` alone.
If a SECOND worktree in the same hub is still wired, confirm the exclude entry is KEPT.
That file lives in the repo's shared gitdir.
Removing it would make the other worktree's live junctions show up as untracked dirt.
Check with `git -C <other-warp-worktree> status --porcelain`, which must stay clean.
Run `lym fabric unwire --apply` a second time immediately after.
It must be idempotent and no-op cleanly on an already-unwired worktree, not error.
Finally, confirm the repo-wide records survive unwire:
`.lym-anchor`, `<BoardDir>/_lym/config/fabric.yaml`, and `.lym-warp` must be untouched.
Run `lym fabric reconcile` afterward.
Confirm it re-wires the worktree's junctions from those same records, with no re-clone needed.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### F6 -- Rebased-warp pull recovery

**Covers:** fabric

**Goal:** "Drive `lym fabric pull` against a warp remote that was rebased or force-pushed underneath your clone.
Confirm fabric detects the drift and re-aligns rather than silently fast-forwarding or erroring."
Discover the surface via `lym fabric pull --help`.

**Watch:** `pull` touches **both** warp and weft; confirm both sides move where expected.
A clean local warp (no unpushed commits of its own) should auto-reconcile with no operator intervention:
warp resets to the new remote history, and weft's own correspondence re-anchors to it.
Run the same pull, against the same rewritten remote, with a local warp that carries unpushed commits.
It should abort loudly and make no changes to either repo; confirm neither warp nor weft moved after the abort.
In the auto-reconciled case, inspect the JSON output.
It should report `reconciled: true` and the re-anchor baseline (`anchor_warp_sha`, `anchor_weft_sha`).
It should also report the new anchor commit (`reanchor_weft_sha`).
Before any of that, dirty the warp worktree with an uncommitted edit to a tracked file.
Run `lym fabric pull` against an advanced remote.
It must REFUSE to move warp, and the edit must survive byte-for-byte.
Advancing warp goes through a hard reset, so a pull that proceeded here would silently destroy the edit.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### F7 -- Bound one-argument re-clone

**Covers:** fabric

**Goal:** "The Hub has been cloned once with both URLs, which writes the warp-URL binding onto `weft:main`.
Delete the hub directory outright, then re-clone it supplying only the weft URL.
Confirm the warp side is derived rather than asked for."

**Watch:** Confirm the re-clone succeeds with no warp URL on the command line.
The warp URL must be derived from the binding recorded on the weft side, not prompted for or defaulted.
Confirm the hub comes up at the same path and identically wired.
The `_bolt`-is-a-weft-worktree check and the `main-weft` branch check from F1 must both hold here too.
Confirm the success envelope reports the derived warp URL.
It must also report that the binding was **not** re-written on this re-clone.
The binding already existed from the earlier two-URL clone.
Confirm the binding record itself is present and tracked at the board root;
the record's filename is the one the fabric module documents (`.lym-warp`).
Confirm that exact name rather than guessing at it.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### F8 -- Subpath-anchored hub, end to end

**Covers:** fabric

**Goal:** "Clone a hub anchored at a subdirectory of the warp repo rather than its root.
Confirm the whole verb surface works from the anchored directory and nowhere else."

**Watch:** Clone with `lym fabric clone --subpath <dir> <weft-url> <warp-url>`.
`<dir>` must be a directory that exists in the warp repo.
Confirm `_lym` and `.lym` land as links inside `<warp>/<dir>/`.
Confirm `<warp>/<dir>/` has no `_bolt` entry at all, and the warp repo ROOT has neither `_lym` nor `.lym`.
Use `ls -la` on both.
Confirm every verb runs from `<warp>/<dir>` (`status`, `pairs`, `list`, `reconcile`, `prune`, `cleanup`, `sync`).
Running any of them from the warp repo root, a subdirectory of `<dir>`, or a sibling directory must be REFUSED.
The error must name both cwd and the anchored directory, not be a confusing downstream failure.
Confirm `lym fabric add <slug>` produces a second pair anchored identically (`<hub>/<slug>/<dir>/_lym`).
Confirm `lym fabric commit` after editing `<dir>/_lym/...` commits to the weft at `<dir>/_lym/...`,
and it leaves the warp side untouched (`git -C <warp> status`).
Confirm a bad `--subpath` is refused before anything is created and leaves no hub behind.
The bad values are an absolute path (`--subpath /<dir>`); one escaping the repo (`--subpath ../..`);
one naming a file; and one naming a directory that does not exist.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### F9 -- Reconcile never eats warp content

**Covers:** fabric

**Goal:** "Confirm `lym fabric reconcile`, the repair verb, converges wiring.
It must not delete anything the operator put in the warp repo themselves."

**Watch:** In a wired worktree, commit a real symlink of your own beside the junctions at the anchored directory.
For example: `ln -s <existing-dir> latest && git add latest && git commit`.
Run `lym fabric reconcile`.
The symlink must still be there afterwards, and `git status` must be clean.
Fabric owns only the links it created, those pointing into the paired weft worktree.
A link pointing anywhere else is the operator's, never a "stale junction" to sweep.
Then check the anchor-marker migration guard: rename `<hub>/_bolt/.lym-anchor` to `.fabric-anchor` and run any verb.
Every verb must hard-error naming both marker names and the rename remedy.
It must NOT fall back to treating the repo as root-anchored.
On a subpath hub that would wire a second junction set at the warp repo root.
Rename it back and confirm normal operation resumes.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### F10 -- Destructive-verb guardrails (`remove`, `cleanup`)

**Covers:** fabric

**Goal:** "Confirm the two verbs that delete things refuse the targets that are not theirs to delete.
They must not delete them and report success."

**Watch:** Run `lym fabric remove <prime-slug>`, the hub's own warp worktree directory name.
Every operator can reach it by tab-completion.
It must be **refused**, naming the prime worktree.
`<hub>/<prime>` must still be there afterwards WITH its `.git` directory.
Git declines to remove a main working tree, and fabric must report that rather than deleting the clone.
Then run `lym fabric remove _bolt` and `lym fabric remove <prime>-weft`.
Both must be refused with an `invalid slug` error, and both directories must survive intact.
`_bolt` holds `.lym-anchor`, `.lym-warp` and the repo-wide `fabric.yaml`.
`<prime>-weft` is the entire durable `_lym` store.
`lym fabric add` refuses exactly this name set;
`remove` must refuse the same one.
Then the cleanup half.
With the prime pair on the default branch, move it off with `lym fabric checkout <other-branch>`.
Run `lym fabric cleanup --apply --force`.
Confirm `main-weft` is reported `protected: true` and still exists (`git -C <hub>/<prime>-weft branch`).
The primary weft branch is the durable weft line whatever the prime happens to be checked out on;
promoting it to a deletable orphan destroys any weft commit it alone carries.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### F11 -- Dirty-worktree matrix across the mutating verbs

**Covers:** fabric

**Goal:** "Cross every mutating verb with uncommitted work on disk, and confirm no verb silently discards it."

**Watch:** Make an uncommitted edit to a TRACKED file on one side, then run one mutating verb.
Check the edit is still there byte-for-byte.
Repeat across the verbs `pull`, `sync`, `checkout`, `reconcile`, `remove`;
then across `cleanup`, `unwire --apply`, `prune`, `add`, and `commit`.
Also repeat across the states: dirty warp only, dirty weft only, both dirty, and untracked-only on each side.
A verb that REFUSES is fine.
A verb that proceeds and leaves the work intact is fine.
A verb that proceeds and discards it is a FAIL, whatever it reported.
Expected refusals worth confirming by name:
`pull` refuses a dirty warp (`ErrWarpDirty`, before warp moves at all);
`checkout` refuses a dirty weft;
`add` refuses a dirty source worktree;
`remove` refuses either side without `--force`;
and `prune --apply` reports `protected: true` for a stale pair whose weft worktree is dirty until `--force` is given.
Untracked files are NOT a reason to refuse.
Confirm they survive every verb that proceeds, rather than being swept along with something else.
Do the whole matrix on a `--subpath`-anchored hub: there the anchored pathspecs decide which side an edit lands on.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### F12 -- The hub is not fabric's to sweep (`prune`, `clone --reset`)

**Covers:** fabric

**Goal:** "Park things in the hub that fabric did not create and must never delete.
Then run the two verbs that delete by derived name and confirm both refuse."

**Watch:** First `prune`.
Create `<hub>/notes-weft/` holding a file, as an operator keeping scratch notes beside their worktrees.
Separately, `git init` an unrelated project at `<hub>/proj-weft/` with one committed file and a clean working tree.
Neither is a fabric worktree; both end in the weft suffix, which is all prune's orphan pass enumerates on.
Run `lym fabric prune`, then `lym fabric prune --apply`, then `lym fabric prune --apply --force`.
All three must report each entry `"unowned": true` with `"removed": false`.
Each entry's error must name the weft repo it is not a worktree of.
**Both directories must survive every run byte-for-byte, `.git` included.**
`--force` must NOT get through.
Force answers "discard this uncommitted work", never "this directory is mine".
Also confirm the gate refuses only what is not fabric's.
Make a genuine stale pair: delete a `<slug>/` warp worktree directory by hand, leaving its registration.
Check `prune --apply` still removes its weft side.

Then `clone --reset`.
In an empty directory, create `<name>-LYMHUB/important/data.txt`.
`<name>` is the basename your warp URL derives to.
Run `lym fabric clone --reset <weft-url> <warp-url>` there.
It must be **refused**, naming the path and saying it is not a fabric hub, and `data.txt` must survive.
The hub name is derived rather than typed.
In the one-argument form it comes from the binding recorded on the weft.
The operator never sees the name being deleted.
Then re-run `--reset` against a real hub and confirm the idempotent re-clone still works.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### F13 -- Bootstrap against a genuinely empty weft remote

**Covers:** fabric

**Goal:** "Clone the documented first-ever-setup shape, a brand-new, zero-commit weft remote.
Confirm the hub it produces is usable."

**Watch:** Create a bare weft remote with no commits (`git init --bare`, `HEAD` on `main`).
Create a warp remote with real content.
Then run `lym fabric clone --shortname <shortname> <empty-weft-url> <warp-url>`.
A first-ever bind refuses a clone without `--shortname`.
The clone reports `ok: true`, so the check is what comes after.
Confirm `git -C <hub>/<prime>-weft rev-parse --verify refs/heads/main-weft` RESOLVES.
A branch can be checked out and reported as current while its ref does not exist.
`git checkout -b` on an unborn HEAD writes nothing.
Then run the documented example, `lym fabric add my-task`, and `lym fabric remove my-task`.
Both `lym fabric --help` and `lym fabric add --help` print `add` as their example.
It must work on a freshly bootstrapped hub.
Repeat the whole scenario with `--subpath backend`, since the anchor and the branch are resolved in separate steps.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### F14 -- A slug is a directory name, not a path (`remove`, `add`)

**Covers:** fabric

**Goal:** "Hand every teardown verb a slug that is a relative path element rather than a name.
Confirm nothing outside the named pair is touched."

**Watch:** On a hub with one added pair, run `lym fabric remove` with each of these slugs in turn:
`..`, `.`, `../evil`, `a/b`, and `""`.
Every one must be refused BEFORE any teardown runs.
After all of them the hub must still list exactly what it listed before:
`_bolt _launchers <RepoName>.code-workspace <prime> <prime>-weft <slug> <slug>-weft`.
`_launchers` must still hold `ide-menu.sh` and the pair's directory.
Then confirm an ordinary `lym fabric remove <slug>` still works.
A slug of `.` or `..` would resolve `<hub>/_launchers/<anchor>/<slug>` to `<hub>` itself.
The refusal must therefore come before any removal.
Repeat on a `--subpath backend` hub, where the anchor segment absorbs one `..` and the damage lands one level lower.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### F15 -- Two verbs at once on one hub (`unwire`, `reconcile`, `add`, `remove`)

**Covers:** fabric

**Goal:** "Run fabric verbs simultaneously in different worktrees of one hub.
Confirm the shared state they all write survives the interleaving."

**Watch:** On a hub with six added pairs, append a pattern of your own to `<warp>/.git/info/exclude`.
Use something like `/my-secret-build-dir`.
Note that the file also carries git's default six-line comment block.
That file lives in the repo's COMMON gitdir, so it is ONE file for the whole hub, not one per worktree.
Now launch all six worktrees at once: background five, run one in the foreground, then `wait`.
Alternate `lym fabric unwire --apply` and `lym fabric reconcile` across them.
Repeat for ten rounds, printing the file's line count after each.
The count must never drop, and your own pattern must be present after every round.
`/_lym` and `/.lym` must each appear exactly once.
A lost line destroys the operator's pattern and git's comment block;
a lost fabric exclusion makes every worktree's junctions show as untracked,
so a plain `git add -A` commits symlinks into the warp repo.
Also run the destructive verbs against each other:
`prune --apply` alongside `checkout`, `cleanup --apply` alongside `commit`, and `add` alongside `remove`.
Confirm every verb either succeeds or refuses with git's real message.
Afterwards `lym fabric pairs` must report every surviving pair `in_sync` and `junction_healthy`.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### F16 -- The post-checkout hook actually fires (`clone`, `add`)

**Covers:** fabric

**Goal:** "Confirm the drift-warning hook is installed where git looks for it and runs.
It must neither clobber nor re-enable an operator's own hook."

**Watch:** After `clone`, run `git checkout -b feature` in the warp worktree.
It must print `fabric: warp/weft out of sync`, naming both branches.
`lym fabric checkout feature` must NOT leak that text into its JSON output.
Then three variations, each on its own hub.
(a) Put an EXECUTABLE `post-checkout` of your own in `<warp>/.git/hooks/` and run `lym fabric add <slug>`.
Confirm it was moved to `post-checkout.user`.
Confirm a real `git checkout` runs your hook first and then prints fabric's warning.
(b) Repeat with your hook at mode 0644, a hook you had DISABLED.
The wrapper git ends up with must be executable, and fabric's warning must still fire.
The backup must still be 0644, and your disabled hook must not run.
Git must not print `hint: the hook was ignored because it's not set as executable`.
(c) Set `core.hooksPath` to a directory outside `.git` and run `lym fabric add <slug>`.
Confirm the hook lands in THAT directory, and that `git checkout` onto a diverged branch still warns.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### F17 -- The launcher surface is repairable (`add`, `reconcile`, `remove`)

**Covers:** fabric

**Goal:** "Break the hub-level `_launchers` artefacts and confirm the repair verb repairs them.
Confirm the teardown verb owns only what it created."

**Watch:** After `lym fabric add <slug>`, confirm `<hub>/_launchers/<slug>/` holds `ide` and `fabric-checkout` scripts.
Delete the launcher directory, then run `lym fabric reconcile`:
it must report `launchers_restored` for that pair, not `already_healthy`, and the directory must be back.
A second `reconcile` must report `already_healthy`.
`pairs` must not report `junction_healthy: true` while the launchers are gone.
Then ownership: put a file of your own inside `<hub>/_launchers/<slug>/` and run `lym fabric remove <slug>`.
Fabric's two scripts must go, your file must survive, and the pair must still be torn down.
Finally confirm the hub's prime worktree is left alone throughout:
it never had a launcher directory, and `reconcile` must not start creating one.
Then the retired-artifact sweep:
plant a link `<hub>/_portals/<anchor>/<slug>` pointing at `<hub>/<slug>/<anchor>/_lym`;
plant a file at `<hub>/_launchers/<anchor>/<prime>.code-workspace`;
plant a foreign file of your own under `<hub>/_portals/<anchor>/`.
Run a verb that regenerates the hub workspace file, such as `lym fabric add`.
Confirm its log warns about the retired artifacts and names `lym fabric reconcile`.
Then run `lym fabric reconcile`:
the planted link and the old workspace file must be gone;
your foreign file and `_portals` must survive, and `hub_details` must name the foreign file.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### F18 -- Merge lifecycle end to end (`merge-in`, `merge-stage`, `merge --continue`, `merge --abort`)

**Covers:** fabric

**Goal:** "Merge a task pair's branch into the hub's prime pair, resolve the conflict it produces, and conclude it.
Then do the same merge again and abandon it instead."

**Watch:** Create a pair and diverge it from the prime on **both** sides.
The merge must be a real merge, not a fast-forward.
Then run `lym fabric merge-in <slug>` from the prime worktree.
A conflict is a *result*, not a failure to re-run differently:
the envelope is `ok: false`, exit 1, and carries a `conflicts` array of **worktree-relative** paths;
the array is one flat list covering both repos, never two lists and never an absolute path.
Note the exit code:
a conflict result and a hard error both exit 1;
a script's only correct discriminator is the presence of the `conflicts` key.
While the merge is live, confirm the sibling verbs refuse with one fixed message:
`a merge is in progress; run "lym fabric merge --continue" or "lym fabric merge --abort" first`.
Try `commit`, `pull`, `push`, `sync`, `checkout`, and `remove` of the merge's own source pair.
`fabric status` is the read-only way to ask whether the merge is parked;
run it from the **prime worktree**, the pair holding the merge record.
It reports `merge_in_progress: true` for the whole live window,
and it reports `false` again after `merge --continue` or `merge --abort`.
That field does NOT predict every refusal among the sibling verbs:
`remove` of the merge's own source pair refuses on a hub-wide predicate,
and run from *that* pair `merge_in_progress` is correctly `false`.
Resolve every listed path, run `lym fabric merge-stage <those same paths>`, then run `lym fabric merge --continue`.
Confirm both sides carry a merge commit whose subject names a **SHA, never a branch**.
The `merge-stage` step is not optional and is worth checking on its own:
`--continue` gates on the git INDEX, not on file content,
so editing a conflicted file and going straight to `--continue` must still refuse with `unresolved conflicts remain`.
Make at least one of the conflicts land under `_lym/`.
Confirm git REFUSES plain `git add _lym/<file>` from the visible worktree (`pathspec ... is beyond a symbolic link`).
Confirm `merge-stage` accepts the same path:
for a weft-side conflict that verb is the only route, and without it the merge is uncompletable through the CLI.
Also confirm `merge-stage` with a path that is not conflicted fails the WHOLE call;
the genuinely conflicted paths in the same call stay unstaged.
Repeat the whole scenario and take `lym fabric merge --abort` instead:
both sides must return to their exact pre-merge SHAs;
both worktrees must come back clean, and `git status` in each checkout must show no merge in progress.
Also check `committed`, which is true only when a conclude-commit landed:
a merge that fast-forwarded both sides reports `committed: false` while still having advanced the pair.
Then repeat one conflicted round with a filename outside ASCII (e.g. `ä-note.md`) conflicting on each side in turn:
the `conflicts` array must carry the real path, byte for byte;
it must never carry git's C-quoted rendering (`"\303\244..."`, quotes included).
A non-ASCII conflict inside the fabric-managed tree must never abort as *conflicts outside the fabric-managed tree*.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### F19 -- Merge preconditions and hostile local state (`merge`, `merge-in`)

**Covers:** fabric

**Goal:** "Confirm a merge refuses every state it cannot safely act on.
Each refusal must leave the hub exactly as it found it."

**Watch:** Each of the following must refuse with **nothing mutated** (`mutations: []`, both HEADs unmoved).
Where the reason set applies, the message is `merge preconditions failed: ...` and names no side and no path:

- A dirty worktree on either side -> `worktree dirty`.
- A detached HEAD on either checkout (`git checkout --detach`) -> `checkout is not on a branch`.
  Without this refusal the merge would land a warp commit reachable from no ref while the weft half landed for good.
- A tag, a raw SHA, `HEAD`, or a branch with no `-weft` counterpart as source -> `source branch is not fabric-managed`.
  A source that exists nowhere gives that plus `source branch not found`.
- Real plain-git merge state you leave behind yourself -> `git merge state exists that fabric did not start`.
  Leave it with `git merge <branch>` conflicted, or `git merge --squash <branch>` conflicted.
  The squash form leaves **no** `MERGE_HEAD`.
  The foreign state must stay untouched for you to finish with plain git.
  `lym fabric commit` over that same foreign state must give the **same** foreign-state message.
  It must not give `a merge is in progress; run "lym fabric merge --continue" or "lym fabric merge --abort" first`.
  Both verbs that message advises would refuse.

Then set hostile git config and confirm fabric is immune to it:
`merge.ff = only` must not break a non-fast-forward merge (fabric pins `--ff`),
and `core.editor` set to something that blocks forever must not hang `merge --continue` (fabric pins `--no-edit`).
A hang here is a blocking defect.
Finally, check the flag pre-flight:
`merge --abort --squash` and `merge --abort -m <msg>` must be rejected as usage errors, not silently ignore the flag.
`merge --continue -m <msg>` must name the conclude-commit.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### F20 -- A merge that changes nothing, and an abort after half the conclude landed (`merge-in`, `merge --abort`, `merge --squash`)

**Covers:** fabric

**Goal:** "Drive the two merge shapes where fabric's own bookkeeping and git's can come apart:
a merge whose result changes nothing, and an abort issued after one side's conclude-commit has landed."

**Watch:** First the empty-result merge.
On the warp side, branch `dup` off the current branch and change a file;
then go back to the current branch and make the **same** change again, independently.
`dup` is not an ancestor of HEAD, yet merging it produces no difference.
Do the same on the weft side for `dup-weft`.
Run `lym fabric merge-in dup`.
It must land a real conclude-commit and report `"committed": true` -- **not** `"already_up_to_date": true`.
Then check both checkouts with plain `git status`: neither may say "you are still merging".
The blocking failure mode is a merge that fabric reports as a clean no-op while a live `MERGE_HEAD` remains.
The record and git then disagree,
and every later merge verb (including `merge --abort`) refuses the pair as carrying state fabric did not start.
Confirm recovery is unnecessary by running `lym fabric merge --abort`:
it must say *no merge in progress*, never *git merge state exists that fabric did not start*.

Then the squash companion: repeat the same fixture and run `lym fabric merge dup --squash`.
Squash has nothing to commit here, so `"already_up_to_date": true` with `"committed": false` **is** the right answer,
and no `MERGE_HEAD` may appear.

Finally the half-concluded abort.
Build a normal divergent merge on both sides.
Install a `pre-commit` hook that does `exit 1` in the weft checkout (`.git/hooks/pre-commit`).
Run `lym fabric merge-in <branch>`.
The warp conclude lands and the weft conclude fails.
The output is *merge conclude did not finish; run "lym fabric merge --continue" again*.
It carries a `merge_committed` mutation for the warp side.
Now run `lym fabric merge --abort`.
It must **refuse**, naming `merge conclude already landed`.
The warp conclude-commit must still be there (`git -C <warp> log -1`).
An abort that reports `"ok": true` here destroys committed work.
It silently resets the warp past its landed conclude-commit.
In the conflict flow that commit carries your own hand-written resolutions.
Remove the hook and confirm `lym fabric merge --continue` finishes the job, skipping the side that already landed.

Last, the invisible landed conclude:
the crash shape where a side's conclude-commit landed but the record never learned its SHA.
Build a conflicted `merge-in` and resolve the conflict.
Run `git add` on the resolved files, then commit yourself with plain `git commit --no-edit` in that checkout.
The on-disk state is then identical to a kill between fabric's conclude-commit and its record re-save:
`*_committed` is still empty, HEAD is on the merge commit, and there is no `MERGE_HEAD`.
`lym fabric merge --abort` must refuse (`merge conclude already landed`).
`lym fabric merge --continue` must **succeed by adoption**:
`"committed": true` and a `merge_committed` mutation carrying the hand-landed SHA;
HEAD unmoved (no second commit), record gone, sibling verbs unblocked.
A `--continue` that loops forever on the *merge conclude did not finish* refusal is the failure mode:
the pair is then permanently wedged, since no fabric verb can clear the record and plain git cannot reach it.

Now the adversarial twin of that shape, which looks identical to fabric from the outside and must NOT be adopted.
Build the conflicted `merge-in` again, then discard it with plain `git merge --abort` in the warp checkout.
Make one ordinary commit of your own there: any unrelated file will do.
HEAD has moved off the recorded pre-merge SHA with no `MERGE_HEAD`, exactly as a landed conclude leaves it,
but nothing was merged.
`lym fabric merge --continue` must **refuse**:
*merge conclude did not finish; run "lym fabric merge --continue" again*, with the record still on disk.
`"ok": true` / `"committed": true` naming your unrelated commit is the blocking failure mode:
the record is deleted, the source is still un-merged, and there is nothing left to inspect.
`merge --abort` refusing too (`merge conclude already landed`) is correct here, not a second bug:
the pair is stuck, and plain git is the documented way out.

Finally the squash version of the same question, which has no evidence either way and must therefore also refuse.
Install the failing `pre-commit` hook in the **warp** checkout and run `lym fabric merge <branch> --squash`.
Remove the hook, then land the squash conclude yourself with plain `git commit -m ...`.
`lym fabric merge --continue` must refuse with *merge conclude did not finish*;
it must leave the record on disk and leave your commit untouched.
A squash conclude is a one-parent commit indistinguishable from any other, so adopting it would be a guess.

Last, the octopus: a commit that satisfies every *other* adoption clause and still must be refused.
Build the merge so the warp side merges CLEANLY and the weft side conflicts:
the warp side is a real non-fast-forward merge of a branch that touches different files.
`merge-in` then returns conflicts, and the record survives with the warp side staged and unconcluded.
Then, in the warp checkout, run `git merge --abort`.
Branch a decoy off the repo's ROOT commit (`git rev-list --max-parents=0 HEAD`) with one unrelated file on it.
Merge BOTH at once: `git merge <the record's warp_source> <decoy>`.
Read the record at `<weft checkout>/.git/fabric-merge.json` for that source SHA.
Rooting the decoy outside the merge's own history stops git dropping the pre-merge tip as a redundant parent.
Confirm with `git rev-list --parents -n 1 HEAD` that you got THREE parents:
the first is the record's `warp_start` and the second its `warp_source`.
`lym fabric merge --continue` must refuse with:
*merge conclude did not finish; run "lym fabric merge --continue" again*.
It must leave the record on disk.
Reporting `"committed": true` here is the blocking failure mode:
fabric would be claiming a commit it can never build.
It starts every non-squash merge with a single `git merge --ff --no-commit <sha>`,
so its own conclude has exactly two parents.
The branch would silently carry decoy content that no side of the merge brought in and no `merge_staged` entry names.
The record would be deleted, with nothing left to inspect.

Last of all, the same questions asked WITHOUT committing, which is the half the adoption evidence never sees.
Everything above reaches the adoption arm because the operator committed;
leaving the hand-made merge uncommitted routes into the conclude's plain `git commit` instead.
Build the warp-merges-cleanly / weft-conflicts fixture again.
Resolve and `lym fabric merge-stage` the weft conflict, so `--continue` reaches the warp side.
Then in the warp checkout run `git merge --abort` and, in place of committing anything, do each of these in turn:

- `git merge --no-commit --no-ff <an unrelated branch>`: a different merge left live.
- `git merge --no-commit <the record's warp_source> <a decoy rooted at the repo's root commit>`:
  an uncommitted octopus.
  Confirm with `git rev-parse --verify --quiet MERGE_HEAD` that it prints exactly the record's `warp_source`:
  that truncated answer is what a first-head-only check would accept,
  and `cat "$(git rev-parse --git-path MERGE_HEAD)"` must show two SHAs.
- Nothing merged at all: `git add` an unrelated new file, leaving no `MERGE_HEAD` but a commit that would succeed.

In each case `lym fabric merge --continue` must refuse:
the message is *merge preconditions failed: checkout no longer carries the recorded merge*.
It must leave the record on disk, land no commit, and leave the warp HEAD where the operator left it.
`"ok": true` / `"committed": true` here is the blocking failure mode:
fabric commits and claims a merge it never started;
it records correspondence for a pair whose warp side does not carry the source at all;
it deletes the record.
This is the same silent false success as the adoption cases, reached by not committing first.
Then confirm the refusal did not wedge the pair:
`lym fabric merge --abort` must still succeed and restore both sides to their recorded pre-merge SHAs.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### F21 -- A target diverged from an upstream it has not fetched yet (`merge`)

**Covers:** fabric

**Goal:** "Confirm the not-synced guard refuses a diverged target.
This holds even when the divergence has not been fetched at the moment the guard runs."

**Watch:** This is the ordinary shape, not a contrived one:
a teammate pushes while you are working, and you have not fetched since.
Create a target pair and a source pair.
Push a commit to the target branch's own remote **from a separate clone**.
Only then make one local commit in the target worktree.
Never run `git fetch` in it.
At this point the target is diverged.
Its own `origin/<branch>` still names a commit that is an ancestor of its HEAD,
so anything reading remote-tracking refs sees "merely ahead".
Confirm that first, so you know the scenario is the unfetched shape:
`git -C <target> rev-parse origin/<branch>` must differ from the bare remote's own tip,
and `git -C <target> merge-base --is-ancestor origin/<branch> HEAD` must succeed.

Now run `lym fabric merge <source-slug>` from the target worktree.
It must **refuse** with `merge preconditions failed: branch not synced to upstream`.
It must also report `mutations: []` and leave both HEADs unmoved.
Run the same check with the divergence on the **weft** side instead of the warp side:
both sides carry their own upstream, and each is decided separately.

Then confirm the guard closes a window rather than blocking the verb:
`git fetch` and reconcile the divergence, and the same `merge` must succeed.
Finally check the opposite state is NOT refused.
A target merely **behind** its upstream must be fast-forwarded by the pre-merge sync step, then merged.
The mutation record shows `repo_advanced`; the sync step exists for this case.

**Verdict:** `OK` / `WARN` / `FAIL`

---

### F22 -- Batten status and refusal surface (`batten status`, `batten run`, `batten step`, `shed seed`)

**Covers:** batten

**Goal:** "Exercise everything `lym batten`'s run/status/step surface can prove without an LLM:
the status verb round-tripping a hand-written fixture;
the run and step verbs' shared refusals on a `done` status and a held run lock;
the prime-only refusal;
`lym shed seed` pre-seeding a batten run ahead of batten's own auto-seed;
and the create row halting blocked against a dirty prime."

**Fixture note:** This scenario never drives a fresh pair past the create row:
a freshly created pair with no task status file spawns a real driver into the LLM rows;
that happens the first time either `batten run` or `batten step` reaches past create with the run lock free,
and neither verb exposes a way to interpose a fixture mid-run.
`step`'s refusals below are exercised the same way `run`'s are:
against a fixture that refuses before the engine is called, never against a completed step.

**Watch:** Hand-write a slug's `_lym/shed/<slug>/status.json` as a fixture in the shape of `shedengine.Status`:
`current_producer`, `state`, `error`, `activity`, and `history`.
Confirm `lym batten status <slug>` returns every one of those fields unchanged in its JSON envelope.
The envelope also carries `history_length` and `history_truncated: false`.
A fixture of a few entries stays under the cap the envelope applies to a long `Run-Shed` watch,
so nothing is dropped.
Then hand-write a `.lym/shed/<slug>/Worktree-Create-stuck.md` beside a fixture whose `state` is `blocked`,
and `current_producer` is `Worktree-Create`.
Confirm the envelope carries that file's text as `stuck_reason`:
Worktree-Create, Seed-Child and Worktree-Teardown carry no `on_stuck`, so their own reason is the persisted `error`;
the key is kept for the Run-Shed block through the budget arm, whose `error` stays the fixed budget literal.
Set the fixture's `state` to `done`.
Confirm `lym batten run <slug>` and `lym batten step <slug>` both refuse.
The refusal names the per-slug directory to delete to run it again.
Hold `.lym/shed/<slug>/run.lock` yourself (any advisory-lock-compatible hold).
Confirm `lym batten run <slug>` and `lym batten step <slug>` both refuse.
The refusal names that lock path.
Run `lym batten run <slug>`, `lym batten step <slug>`, and `lym batten status <slug>` from a task worktree.
Each must refuse, naming both worktree names and telling the operator to re-run it from the hub's prime.
Run the same verbs from the weft prime (`<hub>/<warp>-weft`) and from `_bolt`.
Each must refuse, naming the records sibling or the `_bolt` checkout.
The weft prime is a repository of its own whose prime is itself,
so a name comparison alone would let batten's bookend rows drive fabric against the weft repository.
With a slug's durable `_lym/shed/<slug>/status.json` in place, delete its ephemeral `.lym/shed/<slug>/` directory.
A fresh clone on another machine has that state.
Confirm `lym batten status <slug>` and `lym batten pause <slug>` answer from the durable file.
Neither may fail to open a lock.
Follow the `done`-slug refusal's remedy verbatim.
Confirm it names the durable `_lym/shed/<slug>/` run directory AND the torn-down pair's leftover branches, local and remote.
Removing the run directory alone lands the re-run on the create row's leftover-branch refusal;
removing all of them lets the slug run again.
Upsert a Board task with `"type":"batten"`.
Confirm `lym batten step <slug>` halts `blocked` at `Seed-Child` with a `stuck_reason` naming the Board type;
no seed may be written in the task worktree and nothing committed on its branch.
With a slug's task worktree already created, delete prime's own `_lym/shed/<slug>/status.json`.
That is the state a process killed between the create finishing and its transition being persisted leaves behind.
Confirm `lym batten step <slug>` reports `Worktree-Create` done and advances to `Seed-Child`.
It must not halt `blocked` on fabric's pre-existing-branch refusal, and the task worktree stays untouched.
Before touching a fresh slug, run `lym shed seed <a-fresh-slug> --recipe batten` by hand.
Confirm it writes `_lym/shed/<a-fresh-slug>/seed.json` naming `recipe: batten`.
Then dirty the prime worktree with an uncommitted edit to a tracked file and run `lym batten run <a-fresh-slug>`:
the create row must halt `blocked` rather than `stuck`;
the reason must surface fabric's own dirty-worktree refusal text;
no task worktree may be created;
and batten's own auto-seed step must leave the hand-written seed untouched rather than treat it as a disagreeing seed.
Let a fresh slug's `Worktree-Create` complete.
Hand-write a disagreeing seed at the task worktree's own `_lym/shed/<slug>/seed.json` and commit it there.
For example, use a different `driver` than prime's own `child_driver` param would produce.
Run `lym batten step <slug>`.
`Seed-Child` must halt `blocked` with a `stuck_reason` naming the disagreement,
not fail hard with no `stuck_reason` at all.

Build a slug whose task worktree is present in its durable status but missing on disk:
create the pair, then remove only the worktree, e.g. `lym fabric remove <slug>` leaving the warp branch behind.
Run `lym batten run <slug>` from prime.
The refusal must name no `lym fabric` command that would restore the worktree:
`lym fabric checkout` switches the CALLER's own worktree onto the named branch rather than recreating anything;
every batten verb runs from prime, so the caller is prime.
The text must say the operator has to resolve it by hand instead of naming that command.
Follow that refusal's abandon path verbatim: delete the named run directory and the pair's local and remote branches.
Confirm `lym batten step <slug>` then reaches a fresh `Worktree-Create`,
not a re-refusal at the row the run had reached.
Re-run a slug whose pair was torn down cleanly:
delete only its durable `_lym/shed/<slug>/` run directory on prime's weft and commit.
Confirm `Worktree-Create` halts `blocked` with a `stuck_reason`.
The `stuck_reason` must name the leftover branch and its deletion locally and on the remote.
It must also name `lym fabric cleanup --apply --remote` for the orphaned weft branch.
It must not suggest `lym fabric checkout`;
from prime that verb would switch prime's own pair onto the task's branches.
Then delete only the local branch and step again.
The remote copy must reject the push, and the row must halt `blocked` again rather than half-creating the pair.
Step a slug to `Worktree-Teardown` (hand-write the task worktree's own status as `done` and commit it there).
Then run `lym fabric remove <slug>` before stepping teardown.
That is the state a process killed right after teardown's own removal leaves.
Confirm the next `lym batten step <slug>` reports `done` rather than halting on the missing pair.
Step another slug to `Worktree-Teardown` the same way.
Then remove only its warp worktree (`git -C <prime> worktree remove <slug>`), leaving `<slug>-weft` on disk.
That is the state a process killed between fabric's warp removal and its weft teardown leaves.
Confirm `lym batten step <slug>` halts `blocked`, never `done`.
The `stuck_reason` must name the leftover `<slug>-weft` path and `lym fabric prune --apply`.
Run that prune from prime.
Confirm the next step reports `done` with `<slug>-weft` gone locally and on the remote.
Step another slug to `Worktree-Teardown` and point the weft prime's `origin` at a missing path.
Confirm `lym batten step <slug>` removes the pair but halts `blocked`, naming the other-side branch's remote copy.
Restore `origin` and confirm the next step reports `done` with that branch gone from the remote.
From prime, create only a fresh slug's task worktree with `git worktree add -b <branch_prefix><slug> <hub>/<slug>`.
That is the state a create killed right after its first step leaves.
Confirm `lym batten step <slug>` halts `blocked` at `Worktree-Create`.
It must name `lym fabric remove --force <slug>` and `git branch -D <branch_prefix><slug>`.
Run both verbatim from prime and confirm the next step creates the pair.
Step a fresh slug to `Run-Shed` and run that one step with `LYM_REED_TMUX` pointed at a nonexistent path,
so the child's bootstrap fails after seeding its status.
Confirm a named spawn error.
Confirm the next plain `lym batten step <slug>` spawns the bootstrap again, and a `lym loom run` driver appears.
It must not report "still running" over a child with no driver.
Run `lym shed seed <run-id> --recipe tapestry` from `_bolt` and from the weft prime.
Each must refuse and leave no `_lym/shed/` behind in that checkout.
Run `lym shed seed <run-id> --recipe batten` from a task worktree.
It must refuse with batten's own prime-only wording and leave no `_lym/shed/<run-id>/` in that pair,
while the same command with `--recipe tapestry` there succeeds.

Say in the report that **no automated test drives a real child bootstrap**:
the `integration`-tagged end-to-end test walks every row against a real hub,
but it replaces `Env.InnerRun.Spawn` with a no-op and `Env.InnerRun.ReadStatus` with an in-memory stub,
so the tapestry run itself is the part it does not drive.
Treat that as a stated gap, not as coverage this scenario is narrower than.
`integration`-tagged tests do drive the unstubbed seams on either side of the child's own bootstrap:
`Run-Shed`'s first status read over the child's real paths;
the seed `Seed-Child` writes for that bootstrap to accept.
The real-provider drive between them stays a manual exercise by design.

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
F22: <OK|WARN|FAIL> -- <one-line note if not OK>

sandbox-report.json written: <count of WARN/FAIL items>
```

`./sandbox-report.json` must be written before the session ends, per the Capturing findings section above.
Its `items` is `[]` when every scenario was `OK`.

## Notes

- This suite is scoped to fabric alone, but runs against the same shared Hub (`lym-test`/`lym-test-weft`) as `SANDBOX-CORE-SUITE.md` and every per-module suite -- it is not isolated from them. Several scenarios here are destructive by design (F7's delete-and-re-clone, `--reset`, `prune --apply`, `cleanup --apply`); if a run leaves the Hub in a state another suite would trip on, `sandbox/build.cmd -reset` is the recovery path, not a fabric defect.
- fabric is lym's sole warp↔weft git-coordination module (see `internal/fabricengine/doc.go`);
  its stricter `main-weft`-suffixed branch-naming scheme is exactly what this suite exists to exercise, on the same Hub every other suite already proves the ordinary CLI surface against.
