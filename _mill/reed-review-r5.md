# reed — independent review, round 5 (`opus-high-r5`)

Scoped round: **state-loss / state-corruption recovery only**, per `_mill/reed-review-prompt.md`'s "Scope".
Clean-room: every finding below was formed without reading any `_mill/reed-review-*` file or `_mill/reed-shuttle-HANDOFF.md`.
Model/effort: Opus / High.

## Substrate

- Host Linux, `tmux 3.6` at `/usr/bin/tmux`, resolved via `PATH`.
- Dev binary deployed with `./deploy-dev` (`.dev-bin/lyx`), redeployed after every source change.
- Fixtures built by hand under the session scratchpad: `<scratch>/r5/<name>-HUB/<worktree>` — a plain `git init` worktree
  one level under a `-HUB` parent, which is all `lyxcwd.Resolve` needs (the anchor marker is absent so `AnchorRel`
  falls back to `"."`, and reed's config degrades to its embedded template per the Config Strictness Invariant).
- Two worktrees under one hub (`kappahub-HUB/{svc-alpha,svc-beta}`) so the **shared per-hub socket** is exercised,
  which turns out to matter a great deal (R5-F4).
- Teardown evidence uses `ps -eo comm | grep -cx 'tmux: server'` and `tmux -L <socket> ls`, never `pgrep -x tmux`.

## What was tested

### Baseline

```
$ ps -eo comm | grep -cx 'tmux: server'   -> 0      (clean start)
$ go build ./...                           -> OK
$ go vet ./internal/reedengine/... ./internal/reedcli/... ./internal/hubgeom/...   -> clean
$ ./deploy-dev                             -> Deployed lyx @ a3d2dec7
$ tmux -V                                  -> tmux 3.6
```

In `kappahub-HUB/svc-alpha`:

```
$ lyx reed up
{"ok":true,"session":"svc-alpha","socket":"lyx-kappahub-HUB-64c9b3a1","strands":0}
panes: %1 top=0 h=25 (header)   %0 top=26 h=24
$ lyx reed add --cmd 'sleep 100000' --name alpha1   -> pane %0 (adopted)
$ lyx reed add --cmd 'sleep 100001' --name alpha2   -> pane %2 (split)
$ lyx reed status -> both live:true          (truthful here)
```

**Pre-existing red gate found while establishing the baseline** (before any file was touched — see R5-F7):

```
$ go test -count=1 ./internal/reedengine/...
--- FAIL: TestWithOpLock_RefusesAnUnusableAnchorPathBeforeCreatingState (0.00s)
    server_test.go:300: withOpLock created the lock file ".lyx/reed.lock" despite refusing the told geometry
```

`internal/reedengine/.lyx/reed.lock` existed in the package source directory, mtime `10:38`, i.e. before this
round began. Moving it aside made the suite green (`ok internal/reedengine 0.159s`) and re-running the suite did
**not** regenerate it — so it is stale litter from a run against a binary predating `validateToldAnchorPath`,
and the guard is green from clean. The guard is nevertheless permanently disarmed by exactly the litter it exists
to prevent; see R5-F7.

### Scenario 1 — `reed.json` truncated / corrupted

Truncated `reed.json` to its first 120 bytes (a valid prefix, invalid JSON) with the session up and two strands
genuinely running:

```
$ lyx reed status   -> {"error":"load state: unmarshal state: unexpected end of JSON input","ok":false}  exit 1
$ lyx reed add ...  -> same error, exit 1
$ lyx reed up       -> same error, exit 1
$ lyx reed resume   -> same error, exit 1
$ lyx reed attach   -> same error, exit 1   (Status() pre-flight)
tmux: session still alive, %0 %1 %2 all dead=0
$ lyx reed down     -> {"ok":true,"session":"svc-alpha"}   (the only verb that works — and it destroys everything)
```

Silent variant:

```
$ printf 'null' > .lyx/reed.json
$ lyx reed status -> {"ok":true,...,"strands":[]}          <-- SILENT total loss of the strand table
$ printf ''     > .lyx/reed.json
$ lyx reed status -> {"error":"load state: unmarshal state: unexpected end of JSON input","ok":false}
```

### Scenario 2 — stale `reed.json` while panes have moved on

**2a — server rebirth resets pane ids, stale file restored afterwards** (backup tool / `git stash` / hand-copy):

```
gen 1: up + add alpha1  -> reed.json { alpha1 -> %0 (running "sleep 111111"), header %1 }
       cp .lyx/reed.json gen1.json
       down ; up        -> new server, ids restart: %0 = bare bash (initial pane), %1 = header
       cp gen1.json .lyx/reed.json
$ lyx reed status -> {"strands":[{"name":"alpha1","paneId":"%0","live":true}]}   <-- FALSE
$ lyx reed resume -> {"ok":true,"resumed":0}                                     <-- refuses to rebuild
$ pgrep -af 'sleep 111111' -> NONE
```

**2b — cross-worktree pane targeting on the shared per-hub socket** (the prompt's "hand-copying a `.lyx`
directory between worktrees"):

```
svc-beta: up + add beta1 -> panes %2 (sleep 222222), %3 (header); reed.json { beta1 -> %2 }
cp svc-beta/.lyx/reed.json svc-alpha/.lyx/reed.json
$ (in svc-alpha) lyx reed remove e5673efd...  -> {"ok":true,"removed":[{"name":"beta1"}]}
tmux: svc-beta now has ONLY %3 — its %2 is gone
$ pgrep -x sleep -a | grep 222222 -> KILLED
$ (in svc-beta) lyx reed status -> beta1 live:false      (beta learns only that its strand died)
```

**2c — a strand's `PaneID` equal to `HeaderPaneID`** (an ordinary collision once ids restart at `%0` each server
generation). Session had `%1` (header, 1 row) and `%0` (a live pane):

```
reed.json: strand "collide" -> paneId %1, headerPaneId %1
$ lyx reed up -> {"ok":true,"session":"svc-alpha","strands":1}
tmux BEFORE: %1 h=1, %0 h=48
tmux AFTER : %1 h=50            <-- %0 DESTROYED
$ lyx reed status -> collide live:true on %1  (%1 is the header running "lyx reed header --blocking")
```

**2d — two strands sharing one `PaneID`**:

```
panes %5 (header), %4 (sleep 555551), %6 (sleep 555552); both strands rewritten to paneId %4
$ lyx reed up -> {"ok":true,"strands":2}
tmux AFTER: %5, %4 only        <-- %6 DESTROYED, "sleep 555552" gone
$ lyx reed status -> d1 live:true on %4 AND d2 live:true on %4    <-- d2 is FALSE
```

### Scenario 3 — `.lyx` deleted (lock file included) while an op is in flight

Measured with an external `flock` holder standing in for an in-flight op, control vs. race:

```
CONTROL (hold .lyx/reed.lock 12s, do NOT delete .lyx):
  lyx reed status waited 11027 ms          (correctly blocked)
RACE (hold .lyx/reed.lock 12s, then rm -rf .lyx):
  lyx reed status waited   107 ms          (MUTUAL EXCLUSION VOIDED)
```

Reasoned, not driven: the same property holds for `reed.json.lock` (`internal/state`), since both use
`gofrs/flock` on a path, and a flock follows the *inode*, not the name.

### Scenario 4 — worktree directory renamed

```
omegahub-HUB/svc-orig: up + add orig1 (sleep 444444) -> panes %0 (strand), %1 (header)
mv svc-orig svc-moved      (session svc-orig still running)
$ (in svc-moved) lyx reed status -> {"error":"no reed session (1 strands persisted); run \"lyx reed resume\"..."}
$ (in svc-moved) lyx reed resume -> {"ok":true,"resumed":1,"session":"svc-moved"}
$ tmux ls -> svc-moved  AND  svc-orig            <-- both alive
$ pgrep -x sleep | grep -c 444444 -> 2           <-- the strand now runs TWICE
reed.json re-stamped: "session":"svc-moved", paneId %2, headerPaneId %3
$ (in svc-moved) lyx reed down -> ok
$ tmux ls -> svc-orig                            <-- orphan survives
$ pgrep 444444 -> still running                  <-- unreapable by any reed verb
```

**R3-F2 re-verified as claimed:** targeting is derived fresh from the current worktree path on every invocation —
after the rename the engine addressed `svc-moved`, and `reed.json`'s `socket`/`session` were re-stamped. R3-F2 was
a diagnostic-only staleness, **not** a targeting bug. Confirmed, not merely cited.

### R4-F4 / R4-F5 re-verified under scenarios 2 and 3

- R4-F4 (header split retry behind an even-vertical re-tile) held throughout: every `up` in scenarios 2c/2d
  succeeded against a 1-row top band rather than wedging on `no space for new pane`. It generalizes.
- R4-F5 (adoption narrowed to the sole-alive-non-header pane) held: no adoption of a busy pane was observed in
  any scenario. **However**, R4-F5's *symptom* — a strand reported `live:true` against a pane running something
  else — is fully reachable again by a different route (2a/2c/2d), because R4-F5 fixed the adoption path only,
  not the binding-trust path. That is R5-F2/R5-F3 below.

### Teardown

```
$ lyx reed down (svc-alpha, svc-beta) ; tmux -L lyx-omegahub-HUB-2d5911c7 kill-server
$ ps -eo comm | grep -cx 'tmux: server' -> 0
$ tmux -L lyx-kappahub-HUB-64c9b3a1 ls  -> no server running
$ tmux -L lyx-omegahub-HUB-2d5911c7 ls  -> no server running
$ every "sleep 1111../2222../4444../55555." strand process -> gone
```

---

## Findings

Severity legend per the prompt: BLOCKING / MEDIUM / LOW / NIT. All are fixed in Job 2 regardless of severity.

### R5-F1 — MEDIUM — CONFIRMED — a corrupt `reed.json` wedges every verb but `down`, with an unactionable message; a `null` document loses the table silently

`internal/reedengine/state.go:53` (`LoadState`), `internal/reedengine/spawn.go:185` (`loadOrInitStateLocked`),
`internal/state/state.go:92`.

**Failure scenario.** A crash, `kill -9`, full disk, power loss, or an external tool leaves `.lyx/reed.json`
partially written or invalid. `LoadState` propagates `json.Unmarshal`'s bare error, `loadOrInitStateLocked` wraps
it as `load state: …`, and **every** verb that loads state — `up`, `resume`, `status`, `add`, `remove`, `attach` —
refuses with:

```
load state: unmarshal state: unexpected end of JSON input
```

which names neither the file, nor the worktree, nor any remedy. The tmux session and every strand process stay
alive and healthy, so the operator has running work they can no longer see, resume, or even `attach` to. The one
verb that still functions is `down`, and it is discoverable nowhere and destroys the session and every strand
process it was supposed to rescue.

**Second, worse shape.** A `reed.json` whose entire content is the JSON document `null` decodes to a valid zero
`ReedState` (`json.Unmarshal` accepts it, `found` is `true`), so `status` answers `ok:true` with `strands: []`.
The persisted table is discarded **silently** — the false-healthy shape this round is told to weight highest.

**Suggested fix.** Make the read path self-describing and reject `null`:
name the absolute `reed.json` path and both remedies (`lyx reed down`, or delete the file to keep the session and
lose only strand tracking) in the error, and treat a `null` document as corrupt rather than as an empty state.

### R5-F2 — MEDIUM — CONFIRMED — pane bindings from a previous session generation are trusted, producing a false `live:true` and a `resume` that refuses to rebuild

`internal/reedengine/spawn.go:185` (`loadOrInitStateLocked`), `internal/reedengine/lifecycle.go:637,686`
(the `if booted` clears), `internal/reedengine/reconcile.go:99` (`planReconcile`'s "present ⇒ binding stays").

**Failure scenario.** tmux pane ids are server-global and restart at `%0` on every server rebirth. reed's only
defence is `clearAllPaneBindings` under `if booted` — which fires **only when this very invocation spawned the
server**. Any route by which a `reed.json` reaches disk *after* a boot it does not belong to keeps its bindings:
a restored backup, a resurrected untracked copy, an operator hand-copy, or simply a state file older than the
session now running. Those bindings then name panes that exist but belong to something else.

Driven live (scenario 2a): `status` reported `alpha1 live:true` on `%0` while `%0` was the new session's bare
initial shell and `sleep 111111` was not running anywhere; `resume` — the verb whose entire job is recovery —
answered `resumed:0` because `planResumeLaunches` saw the binding as live. Nothing anywhere reports a problem.

`reconcileLocked` cannot help: it distinguishes "recorded pane is absent" from "recorded pane is present", and
treats "present but a different, unrelated process" as identical to "present and ours". The prompt asks whether
it *should* distinguish them — it should, and it cannot today, because nothing on disk ties a `PaneID` to the
session incarnation it was minted in.

**Suggested fix.** Persist a *pane generation* alongside the bindings — the identity of the tmux session
incarnation they were bound against (`#{session_name}` / `#{session_id}` / server `#{pid}` / `#{session_created}`,
all available from one `display-message` round trip) — and clear every `PaneID` plus `HeaderPaneID` when the live
generation differs from the recorded one. That generalizes `booted` from "I spawned it this invocation" to
"the session I recorded against is not the session that is live now", which is the property actually needed.
Probe failure must leave state untouched (fail closed against spurious clearing).

### R5-F3 — BLOCKING — CONFIRMED — internally inconsistent bindings make `up` destroy unrelated live panes and their processes while reporting `ok:true`

`internal/reedengine/apply.go:68` (`planLayout`), `internal/reedengine/render/rules.go:64` +
`render/layout.go:57` (`bandHeader` splices the header cell independently of the stack body),
`internal/reedengine/reconcile.go:90` (the untracked-pane reap).

**Failure scenario A — strand `PaneID` == `HeaderPaneID`.** `planLayout` keeps `headerPaneID` because the id is
present, and `toRenderStrands`/`partitionByAnchor` also place the strand because its `PaneID` is present. `Rules`
then emits the header cell via `bandHeader` **and** the same pane number again inside the stack body. tmux accepts
a layout string carrying a duplicate pane number with exit 0 and assigns cells positionally — and answers the
resulting cell/pane mismatch by destroying the panes it has no cell for.

Driven live: a session holding `%1` (header, 1 row) and `%0` (a live pane) was reduced to `%1` alone by a single
`lyx reed up`, which reported `{"ok":true,"strands":1}`. `status` then reported the strand `live:true` on `%1` —
the header pane running `lyx reed header --blocking`. That is R4-F5's exact symptom (a nonexistent process
reported live) reached by a different route, **plus** collateral destruction of an unrelated live pane.

**Failure scenario B — two strands sharing one `PaneID`.** The duplicate makes the second strand's real pane
untracked, so `planReconcile`'s deterministic untracked reap kills it. Driven live: `%6` and its
`sleep 555552` were destroyed by a `lyx reed up` reporting `{"ok":true,"strands":2}`, after which `status`
reported *both* strands `live:true` on `%4`.

Neither shape is reachable in the normal single-instance flow (`planPaneTarget` excludes the header,
`validateSplitCreatedNewPane` guarantees a fresh id) — but the merge bar for this round is explicit that
deliberately induced state corruption **is** the gate, and both shapes are one stale/edited/partially-restored
`reed.json` away. Rated BLOCKING because it silently destroys running work in panes reed does not own and then
reports the session healthy.

**Suggested fix.** Two layers. (1) Sanitize the loaded table before any op reads it: a strand `PaneID` equal to
`HeaderPaneID`, or already claimed by an earlier strand, is not a binding this strand owns — clear it (first
writer wins, deterministic) and log a `Warn`. (2) Make the layout path structurally incapable of emitting a pane
number twice, so a future inconsistency cannot reach `select-layout` even if layer 1 is bypassed.

### R5-F4 — MEDIUM — CONFIRMED — a recorded `PaneID` is spent as a tmux target with no check that it belongs to this worktree's session, so corrupt state destroys a *sibling worktree's* live strand

`internal/reedengine/strand.go:462` (`RemoveStrand`'s `kill-pane` loop),
`internal/reedengine/io.go:26` (`resolveLivePaneID`, feeding `SendText`/`SendKey`/`CapturePane`).

**Failure scenario.** The tmux socket is **per hub**, shared by every worktree in it, and tmux pane ids are
**server-global**. `RemoveStrand` takes its pane ids straight from the persisted records — before any reconcile —
and issues `kill-pane -t <id>` unconditionally. `resolveLivePaneID` likewise returns a recorded id with no
membership check, and its three callers deliberately skip reconcile ("pure transport").

Driven live (scenario 2b): with `svc-beta`'s `reed.json` copied into `svc-alpha`, a `lyx reed remove` run in
`svc-alpha` killed `svc-beta`'s pane `%2` and its `sleep 222222` process, and reported
`{"ok":true,"removed":[{"name":"beta1"}]}`. `svc-beta`'s own `status` afterwards showed `beta1 live:false` with no
indication of why. One worktree destroyed another worktree's running agent, with a success envelope on both ends.

This does not require the hand-copy: any stale `reed.json` whose recorded id is currently allocated to a sibling
worktree's session on the shared server reaches the same place, and with ids restarting at `%0` per server
generation that collision is ordinary rather than exotic.

The layout path is **not** exposed — `planLayout` filters through `liveIDSet(live)`, which is session-scoped, and
`planReconcile`'s kills come from the same session-scoped `live`. The exposure is exactly the two sites named.

**Suggested fix.** Never spend a persisted `PaneID` as a tmux target without confirming it is present in *this*
session's `list-panes`. `RemoveStrand` already performs that `listPanes` call for its reap snapshot, so the filter
is free there; the transport ops need one round trip, which is proportionate to the alternative (typing one
agent's input into another agent's pane, or killing it).

### R5-F5 — MEDIUM — CONFIRMED — after a worktree rename, `resume` silently double-launches every strand and orphans the old session beyond any reed verb's reach

`internal/reedengine/lifecycle.go:664` (`Resume`), `internal/reedengine/spawn.go:185`.

**Failure scenario.** `SessionName` derives from the worktree basename, so renaming the directory changes the
session reed targets while `.lyx/reed.json` travels with the directory. The old session keeps running. `resume`
in the renamed worktree boots a *new* session, sees `booted`, clears the bindings, and relaunches every strand.

Driven live (scenario 4): after `mv svc-orig svc-moved`, `lyx reed resume` reported `{"ok":true,"resumed":1}` and
`sleep 444444` was running **twice** — once in the orphaned `svc-orig` session, once in `svc-moved`. `lyx reed down`
in the renamed worktree tore down `svc-moved` only; `svc-orig` and its process survived, addressable by no reed
verb ever again, because no worktree directory of that name exists to derive it from.

For an LLM strand this means a rename silently doubles a running agent and leaves the orphan burning tokens
invisibly. Nothing in the output hints at either.

**Suggested fix.** The pane generation R5-F2 introduces already records the session name the bindings were minted
under. When it differs from the told session name, probe whether the recorded session is still alive on this
socket **and is the same incarnation** (matching `session_id` + server `pid`, so a legitimately new worktree that
merely reuses the old name is not mistaken for the orphan). If it is, refuse the op and name both the orphan and
the exact command that clears it, rather than silently creating a second copy of the operator's work.
Refusing, not auto-killing: the orphan's panes may hold live agent work, and destroying it unasked is not reed's
call to make.

### R5-F6 — LOW — CONFIRMED — deleting `.lyx` while an op holds `reed.lock` voids mutual exclusion silently

`internal/reedengine/lock.go:78` (`withOpLock`), `internal/lock/lock.go:21`.

**Failure scenario.** `gofrs/flock` locks the *inode*. `rm -rf .lyx` (or `git clean -xdf`, which the
Durable-vs-Ephemeral State Invariant makes a sanctioned operator action) unlinks `reed.lock` while op A still
holds it. Op B's `os.MkdirAll` recreates `.lyx`, `flock`'s `O_CREATE` mints a **new** inode, and B's lock is
granted immediately. Two reed ops then mutate the same tmux session and the same `reed.json` concurrently, with
last-writer-wins on the file.

Measured live: control 11 027 ms (correctly blocked), race 107 ms (granted immediately). The same reasoning applies
to `reed.json.lock` — stated as reasoned rather than driven, since it is the identical mechanism one layer down.

Rated LOW rather than higher because it needs three coincidences (an in-flight op, a concurrent deletion, and a
second op inside that window) and the damage is a lost strand record or a duplicated pane, recoverable by
`down` + `resume` — unlike R5-F1..F5, which are single-command and deterministic.

**Suggested fix.** `withOpLock` cannot *prevent* the second op (the unlinked inode is unreachable by name), so it
should stop pretending it did: capture the lock file's identity right after acquiring, re-check it after the
operation body, and report a named error when the file it locked is no longer the file at that path. That turns
an invisible loss of exclusion into a loud, diagnosable one. The honest limit — detection, not prevention — must
be stated in the code, not implied away.

### R5-F7 — LOW — CONFIRMED — R4-F3's own regression guard is permanently disarmed by the litter it exists to prevent

`internal/reedengine/server_test.go:284`
(`TestWithOpLock_RefusesAnUnusableAnchorPathBeforeCreatingState`).

**Provenance, stated plainly:** surfaced while establishing this round's baseline hermetic gate — i.e. during
Job 1, before any file was touched.

**Failure scenario.** The test sets `AnchorPath = ""`, so `stateDir()` resolves to a bare `.lyx` **relative to the
test process's working directory**, which is the package source directory. It then asserts `.lyx/reed.lock` does
not exist there. A single run of any binary predating `validateToldAnchorPath` — precisely the bug R4-F3 fixed —
creates that file permanently, and the guard is red from then on in that checkout, with a message
(`withOpLock created the lock file …`) that reads as a live production regression rather than as stale litter.

This is exactly the state on this branch right now: `internal/reedengine/.lyx/reed.lock` existed with an mtime
predating this round, `go test ./internal/reedengine/...` was RED, moving the file aside made it green, and
re-running the suite did not recreate it. So the guard passes from clean and the production behaviour is intact —
but the branch as handed to me did not pass its own gate, and any future recurrence is unrecoverable without
someone diagnosing stale scratch by hand.

**Suggested fix.** Make the assertion independent of what earlier runs left behind: clear the cwd-relative `.lyx`
before asserting and again on cleanup, with a comment naming why the subject path is cwd-relative at all. The
guard then measures "this call created the file", which is what it means to measure.

### R5-F8 — NIT — CONFIRMED — `requireSessionLocked` swallows a state-read failure and reports "0 strands persisted"

`internal/reedengine/lifecycle.go:1050`.

**Failure scenario.** `if st, err := LoadState(e.stateDir()); err == nil && st != nil` discards the error. With a
session that is down *and* a corrupt `reed.json`, the operator is told
`no reed session; run "lyx reed up"` — a message asserting there is nothing persisted, when in fact reed could not
read what is persisted. Running the suggested `up` then fails with R5-F1's opaque unmarshal error, so the two
messages together actively mislead.

**Suggested fix.** Distinguish "read and empty" from "could not read": keep the friendly no-session text, but say
the persisted state is unreadable (and name the file) instead of claiming a strand count of zero.

---

## Observations that are deliberately NOT findings

Recorded so they are not mistaken for oversights, and because each has its fix outside this round's subject.

- **`fsx.AtomicWriteBytes` does not `fsync`** the temp file or the parent directory before/after `os.Rename`
  (`internal/fsx/fsx.go:53`). On a power loss the rename can be durable while the data is not, which is one real
  route into R5-F1's corrupt file. The fix belongs in `internal/fsx`, which `state`, `websterengine`, `shedengine`
  and others share — a repo-wide durability decision, not reed's to make unilaterally in a scoped round.
  R5-F1 hardens the *read* side, which is what reed owns.
- **A `kill -9` between `os.CreateTemp` and `os.Rename` leaves a `.tmp-*` file in `.lyx` forever**; nothing ever
  prunes them (`reed down` removes only `reed.json`). Litter, not corruption — and the same shared-`fsx` argument
  applies to any cleanup policy.
- **Windows behaviour was not driven** — this host is Linux, as the prompt anticipates. The flock inode semantics
  behind R5-F6 differ on Windows (`LockFileEx` on an open handle), so R5-F6's *repro* is POSIX-shaped even though
  the detection fix is portable. Named, not driven.
- **Genuine process-level concurrency for R5-F6** was stood in for by an external `flock` holder rather than two
  real `lyx` processes racing. The control/race timing pair pins the property being claimed; a second `lyx`
  process would add wall-clock cost without changing what is demonstrated.

## Counts

| Severity | Count | Findings |
| --- | --- | --- |
| BLOCKING | 1 | R5-F3 |
| MEDIUM | 4 | R5-F1, R5-F2, R5-F4, R5-F5 |
| LOW | 2 | R5-F6, R5-F7 |
| NIT | 1 | R5-F8 |
| **Total** | **8** | |

All eight are CONFIRMED (reproduced live or, for R5-F6's `reed.json.lock` half, reasoned from an identical
mechanism whose sibling was reproduced). None are PLAUSIBLE-only.

## Convergence assessment (review-side)

This round's mandate was to decide whether the campaign converges. It does not: the four scenarios turned up a
coherent, previously unexplored defect class — **reed trusts a persisted `PaneID` absolutely**, with no record of
which session incarnation minted it, no check that it belongs to this worktree's session, and no check that the
table is internally consistent. Rounds 1–4 hardened the *construction* of substrate; this is the first round to
probe what happens when the *record* of that substrate goes wrong, and every one of the four scenarios produced a
real defect, three of them false-healthy and two of them silently destructive of a sibling's running work.
