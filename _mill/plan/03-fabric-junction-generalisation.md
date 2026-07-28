# Batch: fabric-junction-generalisation

```yaml
task: 'PATTERN wiring: conditional constraint-injection into every agent'
batch: fabric-junction-generalisation
number: 3
cards: 7
verify: go test -tags integration ./internal/fabricengine/... ./internal/initengine/... ./internal/initcli/... ./internal/loomengine/... ./cmd/lyx/...
depends-on: [2]
```

## Batch Scope

This batch makes every `_lyx`-shaped code path in `internal/fabricengine` genuinely per-junction, while `HostJunctions` still returns exactly one entry — so the whole batch is a refactor whose observable behaviour for `_lyx` is unchanged, and every existing test either passes untouched or is updated to a shape that is equivalent for one junction. That ordering is deliberate: it means batch 5's flip to two junctions is a one-line data change against machinery that is already correct, instead of a flip plus five simultaneous generalisations. Five distinct paths are `_lyx`-only today and all five are fixed here: the seeder does not materialise its weft target, the unwire path hardcodes a single junction and reports a bool, `removeHostJunction` tears down only `_lyx`, the junction health check is open-coded in three places, and `loomengine`'s preflight classifies the health check's reason strings by prefix.

The batch also carries two consequences that reach outside `fabricengine`. `UnwireResult`'s shape change cascades through `initengine/undo.go` into `initcli`'s JSON output — a breaking change to `lyx init --undo`, landed atomically in card 8 because a card that changed the struct without its callers would not compile. And the health-check reason strings gain a junction name, which breaks `loomengine/preflight.go`'s prefix matcher — card 12.

Batch-local decision, and the one place two loops in this batch deliberately disagree: `unseedLyxJunction` (card 8) **aborts on the first junction error**, while `removeHostJunction` (card 9) **continues best-effort past one**. They must not be written by analogy to each other. Unwire aborts because a junction inconsistency there is a hard error the operator must see, and `UnwireJunctions` gates the exclude-file update on it. `remove` continues because its call site is `_ = removeHostJunction(l, slug)` — the return value is discarded, exactly like the adjacent `removePortal`/`removeLaunchers` calls — and its whole purpose is to tear down as much as it can.

## Cards

### Card 6: materialise the weft-side target inside `seedLyxJunction`

- **Context:**
  - `internal/initengine/init.go`
  - `internal/fabricengine/checkout.go`
  - `internal/fabricengine/reconcile.go`
  - `internal/fslink/fslink.go`
  - `internal/fabricengine/testmain_test.go`
- **Edits:**
  - `internal/fabricengine/junction.go`
- **Creates:**
  - `internal/fabricengine/junction_pattern_integration_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `seedLyxJunction` (`internal/fabricengine/junction.go`), add `os.MkdirAll(j.Target, 0o755)` as the **first statement of each loop iteration** — before the `os.Lstat(link)` call, not adjacent to either `fslink.CreateDirLink` call — returning a wrapped error if it fails. The placement is the requirement, not an implementation detail: `seedLyxJunction` has two `CreateDirLink` calls (the re-point path and the create path), but the hard error this card exists to remove is the `weft directory does not exist at %s; cannot validate junction target` returned from the link-already-exists branch after `filepath.EvalSymlinks(target)` fails — which is reached **before both** of them. A `MkdirAll` sited next to `CreateDirLink` would therefore still hard-error on exactly the worktree this is about: one whose junction a `checkout` or `reconcile` already left dangling, since `fslink.CreateDirLink` happily creates a link to a nonexistent target (a raw reparse point on Windows, a dangling symlink elsewhere) and only `Init` materialises anything today. Top-of-iteration placement is what gives that worktree a self-repair path. Update `WireJunctions`'s and `seedLyxJunction`'s godoc to state that the seeder now materialises each junction's weft-side target, and that this is why every `WireJunctions` caller — `initengine/init.go`, `fabricengine/checkout.go` and `fabricengine/reconcile.go`, of which only the first materialises anything itself — now leaves a resolvable junction behind. Create `internal/fabricengine/junction_pattern_integration_test.go` with `//go:build integration` as its first non-empty line, asserting: `WireJunctions` on a worktree whose weft-side junction target does not exist creates that directory and leaves a junction that resolves immediately; and a second `WireJunctions` on the same worktree succeeds rather than erroring — the checkout/reconcile path that is broken today. `internal/fabricengine` already has a `TestMain` calling `lyxtest.HermeticGitEnv()` in `testmain_test.go`, so no new hermetic-env wiring is needed.
- **Commit:** `fabricengine: materialise each junction's weft target in the seeder`

### Card 7: reword the host-pristine refusal to name the real remedy

- **Context:**
  - `internal/fabricengine/doc.go`
- **Edits:**
  - `internal/fabricengine/junction.go`
  - `internal/fabricengine/junction_pattern_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `seedLyxJunction`'s real-directory branch, the error currently reads `host repo already contains a real %s at %s; it predates weft — migrate via the hub-creator`. Keep the refusal — fabric never moves or deletes user content, and this is a deliberate host-pristine guard this task must not erode — but replace the remedy clause, which points at a tool that does not address this case. The reworded message must name what the operator can actually do: move the directory's content into the paired weft worktree's own copy of that directory, or remove the host directory, then re-run `lyx init`, which creates the junction. Keep the junction name (`filepath.Base(link)`) and the offending path in the message, since it must serve `_lyx` and `_pattern` alike. Add a case to `internal/fabricengine/junction_pattern_integration_test.go` asserting that a real, non-link directory at the host junction path is still refused and that the returned error names both the path and the re-run-`lyx init` remedy. This matters specifically for `_pattern`: PATTERN content is described throughout as the host repo's hand-authored invariants, which makes "create `_pattern/` in the repo and start writing" the natural operator mistake, and today that hard-fails `lyx init`, `lyx fabric checkout` and `lyx fabric reconcile` for the worktree with unusable advice.
- **Commit:** `fabricengine: name the real remedy in the host-pristine refusal`

### Card 8: generalise unwire to a per-junction loop and a named-slice result

- **Context:**
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/fabricengine/doc.go`
- **Edits:**
  - `internal/fabricengine/junction.go`
  - `internal/initengine/undo.go`
  - `internal/initengine/undo_test.go`
  - `internal/initcli/initcli.go`
  - `internal/initcli/initcli_test.go`
  - `internal/fabricengine/junction_pattern_integration_test.go`
- **Creates:**
  - `internal/fabricengine/junction_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** This card is one atomic cascade: the struct change and every caller that would otherwise fail to compile land together. (1) In `internal/fabricengine/junction.go`, replace `UnwireResult.JunctionRemoved bool` with `JunctionsRemoved []string` — the `Name` of each junction actually removed, in `HostJunctions` order. A name slice, not a count: the value is CLI-observable and "1 of 2 removed" is useless to an operator who needs to know which. (2) Rewrite `unseedLyxJunction` to iterate `l.HostJunctions(slug)`, applying its existing per-junction validation sequence unchanged (target resolution via `filepath.EvalSymlinks` before the `fslink.IsLink` check, then the resolved-target comparison, then `fslink.Remove`) to each record's own `Link` and `Target` rather than to the hardcoded `HostLyxLink(slug)`/`WeftLyxDirFor(slug)` pair, and return the accumulated names. Keep its abort-on-first-junction-error rule: a junction inconsistency is a hard error, and `UnwireJunctions` must continue to leave the exclude file untouched when it fires. Delete the godoc paragraph that says the function is deliberately scoped to the single `_lyx` junction and asks for it and `UnwireResult` to be revisited together if `HostJunctions` ever grows a second entry — this card is that revisit. (3) Fix the partial-failure contract, which is the part the current code gets wrong: `UnwireJunctions` today returns a **zero** `UnwireResult` when `unseedLyxJunction` errors. With one junction that is merely uninformative; with two it is a lie, because the first may already have been removed before the second failed. The new contract is that a mid-loop failure returns the `JunctionsRemoved` accumulated so far alongside the error, never a zero value — the same rule that already governs the post-junction step, where an exclude-update failure returns the junction outcome. (4) Cascade into `internal/initengine/undo.go`: `UndoResult.LyxJunction string` becomes `JunctionsRemoved []string`, carrying `UnwireResult.JunctionsRemoved` through. (5) Cascade into `internal/initcli/initcli.go`'s `runUndo`, whose emitted map key `lyx_junction` becomes `junctions_removed` carrying the slice. This is a breaking change to `lyx init --undo`'s JSON output, accepted deliberately: the alternative repeats the key per junction and does not scale. Update `runUndo`'s and the `--undo` command's `Long` to state the new key and that it is a breaking output change, per the CLI/Cobra Invariant's help-accuracy obligation. (6) Update `internal/initcli/initcli_test.go`, which pins the emitted key set — change `lyx_junction` to `junctions_removed`, do not delete the assertion — and `internal/initengine/undo_test.go` wherever it reads `LyxJunction`. (7) Add cases to `internal/fabricengine/junction_pattern_integration_test.go`: wire then unwire reports every junction name and removes every exclude line; unwiring an already-unwired worktree is a no-op returning an empty `JunctionsRemoved` and a nil error, not an error. (8) The remaining case — the regression guard for the bug this card fixes, where a later junction fails to unwire after an earlier one succeeded, and the returned `JunctionsRemoved` names the earlier one and is not the zero value — cannot be driven through `l.HostJunctions(slug)` in this batch, since it still returns exactly one entry (the batch's own stated precondition); exercising it needs a synthetic multi-record slice, which the external `fabricengine_test` package cannot construct against an unexported entry point. So `unseedLyxJunction` is split into a thin `(l, slug)` wrapper and an unexported `unseedJunctionRecords(junctions []hubgeometry.HostJunction) ([]string, error)` that owns the actual loop and is what the wrapper calls — this changes no observable behavior, it only factors the loop so it is drivable directly. Creates `internal/fabricengine/junction_test.go` (package `fabricengine`, no build tag: it touches only directories and `fslink`, never git) with a table-driven test against `unseedJunctionRecords` covering exactly this regression guard, alongside the existing single-junction cases the extraction must not change.
- **Commit:** `fabricengine: unwire every junction and report which ones by name`

### Card 9: tear down every junction in `lyx fabric remove`, best-effort

- **Context:**
  - `internal/fabricengine/remove.go`
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/fslink/fslink.go`
- **Edits:**
  - `internal/fabricengine/weftwiring.go`
- **Creates:**
  - `internal/fabricengine/remove_junctions_integration_test.go`
  - `internal/fabricengine/weftwiring_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Generalise `removeHostJunction` in `internal/fabricengine/weftwiring.go` from its single `l.HostLyxLink(slug)` form to a loop over `l.HostJunctions(slug)`, calling `fslink.Remove` on each record's `Link`. The loop is **best-effort and continues past a per-junction error**, accumulating failures and returning a joined error at the end rather than returning on the first — the opposite of `unseedLyxJunction`'s rule from card 8, and deliberately so. Godoc must state that contrast explicitly so a future reader does not "fix" one to match the other: `Remove`'s call site is `_ = removeHostJunction(l, slug)`, discarding the return value exactly as the adjacent `removePortal` and `removeLaunchers` calls in the same teardown do, so aborting on the first junction's failure would silently leave every later junction in place — defeating the goal of this card. The reason this step exists at all is worth preserving in the godoc: step (6) of `Remove`, `fslink.RemoveLinksIn(target)`, scans only immediate children and misses a nested junction when `RelPath != "."`, which is why step (5) removes junctions explicitly. Leaving it `_lyx`-only would reintroduce exactly that documented bug for the second junction. Create `internal/fabricengine/remove_junctions_integration_test.go` with `//go:build integration` first, asserting: after `Remove` on a worktree at a nested `RelPath` with every junction wired, no junction survives — the nested case is the one that matters, since at `RelPath == "."` the root-level safety net masks the bug. The second required case — with one junction in a state that makes its removal fail, the others are still removed — cannot be driven through `l.HostJunctions(slug)` in this batch for the same reason card 8's regression guard could not: it still returns exactly one entry. So, mirroring card 8's `unseedJunctionRecords` split, `removeHostJunction` is split into a thin `(l, slug)` wrapper and an unexported `removeJunctionRecords(junctions []hubgeometry.HostJunction) error` that owns the actual best-effort loop; `internal/fabricengine/weftwiring_test.go` (package `fabricengine`, no build tag — directories and `fslink` only, no git) table-drives `removeJunctionRecords` directly against a synthetic multi-record slice to prove one entry's failure does not stop the others from being removed.
- **Commit:** `fabricengine: remove every host junction during fabric remove`

### Card 10: make the shared junction health check per-junction and name the junction in every reason

- **Context:**
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/fslink/fslink.go`
- **Edits:**
  - `internal/fabricengine/reconcile.go`
  - `internal/fabricengine/status.go`
  - `internal/fabricengine/reconcile_stale_registration_test.go`
  - `internal/fabricengine/junction_repoint_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `checkJunctionHealth(hostLink, weftLyxDir string) (bool, string)` in `internal/fabricengine/reconcile.go` currently takes one pre-resolved pair and returns three hardcoded `_lyx` reason strings. Replace it with a form that loops `l.HostJunctionsHere()` (added in batch 2) and returns first-unhealthy-wins — a junction is unhealthy if its `Link` is missing, is not a link, or resolves somewhere other than its `Target`. Parameterise all three reason strings by junction name: `host _lyx junction missing` becomes `host <name> junction missing`, `host _lyx is not a junction` becomes `host <name> is not a junction`, and `host _lyx junction points elsewhere` becomes `host <name> junction points elsewhere`. All three, not two — with a singular reason string the string is the only thing telling an operator which junction is broken, so any one left unparameterised defeats the premise the next requirement rests on. Rewire both call sites to the new form: `reconcile.go`'s repair gate, which currently builds `hostLink`/`weftLyxDir` from `HostLyxLinkHere()`/`WeftLyxDir()` before calling, and `status.go`'s, which feeds `PairStatus.JunctionHealthy`/`JunctionReason` and folds the verdict into `InSync`. `PairStatus` keeps its shape — `JunctionHealthy bool` and `JunctionReason string` stay singular, so `lyx fabric status`'s output shape does not change and the information an operator needs is in the reason. Also widen `ReconcileActionJunctionRepointed`'s `Detail` string, which names a single `hostLink → weftLyxDir` pair today, so it names whichever junctions were repaired. Update `internal/fabricengine/reconcile_stale_registration_test.go` and `internal/fabricengine/junction_repoint_test.go` wherever they assert a `checkJunctionHealth` signature, an `_lyx`-literal reason, or the old `Detail` shape — update them, never delete them. Note for the implementer: repair itself needs no change, because it already goes through the generic `WireJunctions`; only detection was narrower than repair.
- **Commit:** `fabricengine: check every junction's health and name it in the reason`

### Card 11: make `PairInSync`'s inline junction check per-junction with aligned wording

- **Context:**
  - `internal/fabricengine/reconcile.go`
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/fslink/fslink.go`
- **Edits:**
  - `internal/fabricengine/drift.go`
  - `internal/fabricengine/junction_pattern_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `PairInSync` in `internal/fabricengine/drift.go` does not call `checkJunctionHealth` — it re-implements the same lstat / `IsLink` / `PointsTo` sequence inline for loom's preflight. Generalise that inline sequence to loop `l.HostJunctionsHere()` the same way card 10's helper does, and bring all three of its reason strings onto card 10's parameterised wording: the bare `junction missing` becomes `host <name> junction missing`, `host _lyx is not a junction` becomes `host <name> is not a junction`, and the bare `junction points elsewhere` becomes `host <name> junction points elsewhere`. The two bare strings are worse than merely unparameterised — they name no junction at all, so with a second junction they are ambiguous rather than just imprecise. Aligning them also completes what `drift.go`'s own comment already asks for on the middle string ("Same wording as checkJunctionHealth for this drift shape, so status/reconcile and PairInSync describe it identically"), which today only that one of its three strings honours. Preserve the existing distinction the comment there explains: `fslink.IsLink` reports `(false, nil)` for both a missing entry and a real directory, so the `os.Lstat` check must stay ahead of it and a real directory must never masquerade as merely missing. `PairInSync`'s signature is unchanged and it stays stateless and slug-free — which is exactly why it loops `HostJunctionsHere()` rather than `HostJunctions(slug)`. Add cases to `internal/fabricengine/junction_pattern_integration_test.go` asserting each of the three drift shapes produces the aligned reason wording naming the junction.
- **Commit:** `fabricengine: generalise PairInSync's junction check and align its wording`

### Card 12: keep loom's preflight classifying junction faults after the reword

- **Context:**
  - `internal/fabricengine/drift.go`
- **Edits:**
  - `internal/loomengine/preflight.go`
  - `internal/loomengine/preflight_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `internal/loomengine/preflight.go` classifies `PairInSync`'s reason string by prefix: `strings.HasPrefix(reason, "host on ")` maps to `CheckWeftSync`, and `strings.HasPrefix(reason, "junction")` maps to `CheckJunction` **and** sets `check3BlocksSeed = true`, with everything else falling to a `default` of `CheckWeftSync`. Cards 10 and 11 reword drift's junction reasons to start `host <name> junction …`, which matches neither case — so without this card a genuinely broken junction would be reported as a weft-sync failure and would stop blocking the seed check, and `internal/loomengine/preflight_integration_test.go`'s pinned `CheckJunction` classification would fail. Change the junction case to `strings.Contains(reason, "junction")`, evaluated **after** the `"host on "` case. Ordering keeps the two disjoint: the branch-mismatch reason is `host on <a>, weft on <b> (want <c>)` and contains no `"junction"`, but relying on the ordering rather than on that fact is the safer arrangement. A `Contains` match survives junction-name interpolation, which a prefix match cannot. Document the resulting contract in a comment at the classification site: `PairInSync`'s junction reasons are a consumed string format, and any future reword must keep the substring `junction` in them. Extend `internal/loomengine/preflight_integration_test.go` so all three drift shapes — missing, not-a-link, and points-elsewhere — are asserted to classify as `CheckJunction` and to set `check3BlocksSeed`, rather than only the shape it covers today. *(A typed reason from `PairInSync` would be cleaner long-term and was considered; it was rejected as disproportionate because it expands this task into `loomengine`'s API.)*
- **Commit:** `loomengine: match junction faults by substring after the reason reword`

## Batch Tests

`verify: go test -tags integration ./internal/fabricengine/... ./internal/initengine/... ./internal/initcli/... ./internal/loomengine/... ./cmd/lyx/...` covers exactly the four packages this batch edits plus the repo-wide guards. The `-tags integration` flag is required rather than optional here: the two new test files (`internal/fabricengine/junction_pattern_integration_test.go`, `internal/fabricengine/remove_junctions_integration_test.go`) and the existing `internal/loomengine/preflight_integration_test.go` are all integration-tagged, so a plain `go test` would compile none of them and this batch's central assertions would silently not run. Go's build tags are additive, so `-tags integration` runs the untagged tier in these packages as well. `./cmd/lyx/...` is in scope because both new test files contain `gitexec.RunGit`/`exec.Command`-class tokens and `lyxtest` fixture calls: `cmd/lyx/tierpurity_test.go` fails on a raw-substring match if either file's `//go:build integration` line is missing or misplaced, and `cmd/lyx/hermeticenv_test.go` scans every test file regardless of build tag. Neither new file introduces a new git-spawning *package* — both live in `internal/fabricengine`, which already has a `TestMain` calling `lyxtest.HermeticGitEnv()` — so the Hermetic Git Test Environment Invariant needs no new wiring. Existing tests updated rather than deleted in this batch: `internal/initcli/initcli_test.go`, `internal/initengine/undo_test.go`, `internal/fabricengine/reconcile_stale_registration_test.go`, `internal/fabricengine/junction_repoint_test.go`, `internal/loomengine/preflight_integration_test.go`.
