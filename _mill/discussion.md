# Discussion: fabric: accumulate the result envelope from mutations, not control flow (slice 14)

```yaml
task: 'fabric: accumulate the result envelope from mutations, not control flow (slice 14)'
slug: fabric-mutation-record-envelope
status: discussing
parent: main
```

## Problem

Fabric's verbs assemble their JSON result envelope from where control flow ended up at the end of the call, not from what the call actually did to the disk.
The envelope can therefore contradict reality, and it has done so twice, in opposite directions.
`lyx fabric pull` with a dirty tracked file in the warp worktree discarded that file through `ResetHard` and returned `{"ok":true}` (crucible round R2, finding `R2-1`, BLOCKING).
`lyx fabric remove ..` resolved `<hub>/_launchers/./..` to `<hub>`, `os.RemoveAll`'d the entire hub, and then returned `{"error":"failed to check worktree status","ok":false}` — claiming nothing happened immediately after the most destructive act the tool can perform (round R5, finding `B0`, BLOCKING).

A single missing check cannot produce both inversions.
What produces both is that `ok:true` is the default for "we reached the end without returning an error", while `ok:false` plus a message is derived from whichever error happened to be in hand at the return — so a late failure overwrites the record of an earlier successful destructive step.
Neither path consults what changed on disk, because nothing tracks it.

**Why now.** This is the only defect class the whole crucible campaign found where the failure *actively misleads the operator* — the other classes produce bad help, this one produces false information in the exact situation where the operator most needs the truth.
It is issue #143 (`bug`), slice 14 of the `fabric-crucible-followups` chain, and it is sequenced last of the three deliberately: slice 12 (`fabric-destructive-chokepoint`, a hard `depends_on`) landed the gate's safety half in each verb's existing error shape, slice 13 landed the live-state harness that drives every verb's result path, and this slice generalises the reporting into one accumulate-as-you-mutate envelope.
Both predecessors are merged;
`internal/fabricengine/fabrictest/doc.go` already names this slice's truthfulness assertions as the work it deliberately deferred.

## Scope

**In:**

- A new shared mutation-record vocabulary in `internal/fabricengine` — a `Mutation` entry type and a `Mutations` accumulator, in a new file `internal/fabricengine/mutation.go`.
- Auto-recording of every destructive primitive inside `internal/fabricengine/destroy.go`'s executors, driven by a recorder threaded into the gate's request types.
- Explicit recording at the success site of every *constructive* mutation in the mutating verbs (worktree created, branch created/pushed/moved, junction wired or repointed, file rewritten).
- Every mutating verb's result type gains the record, and every failure return in those verbs changes from `return ZeroResult{}, err` to `return res, err` with `res` carrying whatever was recorded before the failure.
- The mutating verbs are: `clone`, `add`, `remove`, `checkout`, `prune`, `cleanup`, `unwire`, `reconcile`, `commit`, `push`, `pull`, `sync`.
- `internal/output` gains one additive error-path function that can carry fields alongside `ok:false` and `error` — today `output.Err` can carry nothing but the message.
- `internal/fabriccli` emits `mutations` on both the success and the failure path, plus `partial: true` when an error is returned with a non-empty record, plus a structured `refusal: {check, what, target, reason}` object when the error carries a `*destructiveRefusal`.
- A new **Mutation Record Invariant** in `CONSTRAINTS.md`, machine-guarded by an extension to `cmd/lyx/destructiveguard_test.go`.
- Truthfulness assertions across the existing `internal/fabricengine/fabrictest` matrix: every cell cross-checks the result envelope's record against the manifest diff.
- Conversion of `fabricengine`'s check enum from an unexported `int` to an exported string-backed `Check` as the single declarer, dropping `checkForce` and `String()`, with `internal/fabricengine/fabrictest/refusal.go`'s copy deleted and its consumers (plus `destroy_test.go:596`) repointed.
- Doc updates in the same commit: `internal/fabricengine/doc.go` (or the package docs that describe result shapes), `internal/fabricengine/fabrictest/doc.go`'s deferred-work note, `CONSTRAINTS.md`, and two corrections to `manifest/designs/fabric-crucible-followups.md` — the stale consumer claim at `:419`, and requirement 2 at `:406`, which this discussion supersedes (see the `ok-semantics-and-error-path-fields` Decision).

**Out:**

- **The read-only verbs** `list`, `pairs`, `status`, `diff`. They cannot mutate, so they have no dishonesty to fix and get no record — not even an empty one.
- **Reordering `Remove`'s pre-flight.** `removePortal`/`removeLaunchers` run at `remove.go:61-66`, before the dirty pre-flight at `remove.go:68-76`, so a correctly-refusing dirty `Remove` has already destroyed the portal and launcher paths. This slice makes that *representable and reported*; it does not change the ordering. File a separate issue for the reorder.
- **`fabricengine.Bolt`** (`bolt.go`) and its in-process consumers. `Bolt.Commit` already returns `(sha, committed, err)` — an accumulate-as-you-go shape — and `internal/boardengine` consumes it as Go values, never as JSON.
- **Other modules' envelopes.** The `internal/output` change is purely additive; no other module's output shape changes.
- **`internal/gitexec`'s error shape** (`gitexec-error-shape.md`) — a separate, wider class, explicitly scoped out of this chain.
- **Slice 15** (`corrindex` two-phase race) — sequenced after this one.
- Any change to what the verbs *do*. This slice changes only what they *report*.

## Decisions

### mutation-record-vocabulary

- **Decision:** one shared vocabulary owned by `internal/fabricengine` — a `Mutation` entry type and a `Mutations` accumulator — embedded in every mutating verb's result type under a single JSON key `mutations`. Not per-verb bespoke fields, and not a per-item row.
- **Rationale:** the task states outright that `PruneEntry`'s per-entry honesty should be *generalised rather than reinvented*, and that means one vocabulary, not a fourth one. A single `mutations` key also means one assertion shape for `fabrictest` and one thing for a guard test to pin.
- **Rejected:** per-verb bespoke fields (most precise per verb, but invents a new vocabulary per verb, which is exactly what the task warns against); extending `PruneEntry`'s per-item row to every verb (maximum reuse, but `pull` and `checkout` have no natural "item" to enumerate — their mutations are steps, not rows).

### mutation-entry-shape

- **Decision:** each `Mutation` carries exactly three flat fields — `Kind` (a fixed enum string), `Target`, and `Detail` (optional, kind-specific free string). Ordering is carried by array order alone.

  **`Target` convention**, resolving the contradiction between this decision and the harness cross-check:
  - A path **at or below the hub root** is **hub-relative, `filepath.ToSlash`'d** — byte-identical to `CaptureManifest`'s own keying (`fabrictest/manifest.go:133-137` does `filepath.Rel(hubRoot, path)` then `filepath.ToSlash`). This is the case for every mutation `fabrictest` can observe, so the oracle cross-check is a direct string comparison with no normalisation step.
  - A path **outside the hub root** — `clone`'s cwd-side paths, which exist before the hub does — is **absolute, `filepath.ToSlash`'d**. These are never captured by a manifest, so they never enter the cross-check.
  - A **git ref** target is the bare ref name as the executor received it (e.g. `feature-x-weft`), never a path.

  **Who converts, and where the hub root comes from.** The eight gate sites in `destroy.go` receive absolute paths only — `pathRequest` carries `container` (the target's parent), never the hub root — so they cannot produce a hub-relative `Target` themselves.
  The `Mutations` accumulator therefore **carries the hub root as a field**, set once when the verb constructs it at verb entry (where the `*lyxcwd.Location` or hub path is already in hand), and `Mutations.Append` performs the conversion: absolute path in, hub-relative-slashed `Target` out, falling back to absolute-slashed when the path does not descend from the hub root.
  The gate keeps passing the absolute paths it already has and never does path arithmetic, which also keeps hub geometry out of `destroy.go` — consistent with how the rest of the tree keeps geometry ownership at the verb layer rather than in leaf helpers.

  **A `Target` that is the hub root itself.** `CloneHub` mints the hub root via `createExclusiveDir(hubPath)` (`clone.go:225`), whose hub-relative form is `"."`.
  This is recorded honestly as a `dir_created` with `Target: "."` — the hub really was created — but it is **exempt from both cross-check directions**, for two independent reasons: `CaptureManifest` never emits a `"."` key (it returns early at `path == hubRoot`), and `"."` would make every path in the diff a segment-wise descendant, trivially satisfying the omission direction for the entire clone cell and asserting nothing.
  The clone cell asserts hub creation through its own existence assertion instead, which is where that check already lives — there is no "before" manifest for a hub that did not yet exist.

  **Initial `Kind` members**, each mapped to its recording site:

  | `Kind` | Recorded at | Notes |
  | --- | --- | --- |
  | `path_removed` | `removePath` (`destroy.go:610`) | `Detail` is `recursive` (the `RemoveAll` branch, `:624`) or `single` (the `os.Remove` branch, `:630`) |
  | `worktree_removed` | `removeGitWorktree` (`:640`) | |
  | `link_removed` | `removeLink` (`:657`) | also the removal half of a repoint — see below |
  | `branch_deleted` | `deleteBranch` (`:683`) | `Target` is the ref name |
  | `worktree_reset` | `resetHardTo` (`:739`) | `Detail` carries the SHA reset to; the primitive behind defect 1 |
  | `dir_created` | `createExclusiveDir` (`:703`) | constructive minter |
  | `worktree_created` | `createGitWorktree` (`:721`) | constructive minter |
  | `branch_created` | verb success sites | `Target` is the ref name |
  | `branch_pushed` | verb success sites | `Target` is the ref name |
  | `commit_created` | verb success sites | `Detail` carries the SHA |
  | `link_created` | verb success sites (`fslink.CreateDirLink`, `junction.go:169,319`) | also the create half of a repoint |
  | `file_written` | verb success sites (`.git/info/exclude`, config rewrites) | |
  | `push_spawned` | `sync`'s detached-push call site | a push was *launched*, not observed — see the composition section |
  | `worktree_switched` | `Checkout`'s two `git switch` sites (`checkout.go:82-91,131-148`) | `Target` is the switched worktree root, `Detail` the branch switched to |

  Seven are auto-recorded inside `destroy.go` (`path_removed`, `worktree_removed`, `link_removed`, `branch_deleted`, `worktree_reset`, `dir_created`, `worktree_created`);
  the remaining six are hand-recorded at their success sites, since no chokepoint covers them.

  **There is deliberately no `link_repointed` kind.** `repointLink` (`destroy.go:668-678`) executes *by calling* `removeLink`, and the replacement link is created later at the caller's own `fslink.CreateDirLink` (`junction.go:169,319`), outside the gate entirely.
  A repoint is therefore recorded as what physically happens — a `link_removed` from the gate, then a `link_created` from the caller — rather than as one synthetic kind.
  This also resolves a `Detail` that could not be filled: the new target simply does not exist at `repointLink`'s recording site.
  **Rejected:** keeping `link_repointed` and suppressing the inner `removeLink`'s record (one entry per logical act, but it requires passing a nil recorder into `removeLink`, which defeats the compiler-enforced threading the `gate-auto-records` Decision is built on, and still leaves `Detail` unfillable).

  **Rule for adding a member:** a new `Kind` lands in the same commit as its recording site and its guard-test entry, never ahead of either. `mutation.go` is the single declarer of the enum — no other file declares a kind string literal.
- **Rationale:** JSON-stable, trivially assertable, and the enum is precisely what a guard test can pin. It mirrors `PruneEntry`'s existing flat-fields-plus-a-string-reason shape rather than introducing nested payloads into a package whose result types are all currently flat.
- **Rejected:** adding a `Sequence` int (array order already carries ordering; a second encoding of the same fact can only disagree with the first); typed per-kind payloads via a union or `any` field (richest, but yields an unstable JSON shape and forces a type switch at every consumer, for a record whose whole point is being readable at a glance).

### record-survives-the-error-return

- **Decision:** the verb owns a `*Mutations` recorder and threads it into everything it calls;
  every failure return in a mutating verb changes from `return ZeroResult{}, err` to `return res, err` with `res` populated from that recorder.
  Use the named-result plus `defer` idiom so a newly added early return cannot silently drop the record.
- **Rationale:** the recorder is a pointer, so a mutation appended deep inside a helper (`removeLaunchers`, `rollbackAdd`, `repairPairWiring`) is visible at the top-level return without threading a return value back up through every layer. Populating the result on the error path is the literal fix for the defect — today the record is not merely unreported, it is *destroyed at the `return` statement* by the zero-value result.
- **Rejected:** returning the record via a typed error (`*MutatedError{Mutations, Err}`) — the result value stays zero on failure so no existing call site changes meaning, but every consumer must `errors.As` to see what happened, which reproduces the "you have to know to look" failure mode being fixed; caller-supplied recorders (`Remove(l, slug, force, rec *Mutations)`) — the strongest survival guarantee, but changes every verb's signature and every call site in the tree for no gain over verb-owned.

### gate-auto-records

- **Decision:** the destruction gate records every destructive primitive it executes. Constructive mutations outside `destroy.go` are still recorded by hand at each success site.

  **Threading mechanism — an explicit `rec *Mutations` parameter on all eight sites, not a request-type field.**
  Three of the eight take neither request type today: `repointLink(what, container, target string, own pathOwnership)` (`:668`), `createExclusiveDir(path string)` (`:703`) and `createGitWorktree(repoDir string, addArgs []string, target string)` (`:721`).
  Rather than split the mechanism — a struct field for the five request-shaped sites and a parameter for the three others — every one of the eight takes `rec *Mutations` as an explicit leading parameter.
  **Rationale:** a missing struct field is a silent zero value the compiler accepts;
  a missing parameter does not compile. Given that this entire slice exists because a record was silently dropped, the mechanism that makes dropping it a build failure is the right one, and one uniform rule beats two.
  **Recording timing — append only after the primitive observably changed state.**
  The record is a log of what happened, so a call that changed nothing appends nothing. Three concrete cases, all of which would otherwise produce entries the manifest cannot corroborate:
  - `removePath` returns `nil` early when the target is already absent (`destroy.go:615-618`). That is a successful no-op, not a removal — **record nothing**.
  - `removeGitWorktree` (`:640`) and `deleteBranch` (`:683`) return git's own `exitCode` and `stderr` alongside `err`, so a **nonzero exit with a nil `err`** is reachable. Record only when `err == nil` **and** `exitCode == 0`.
  - The two minters record after the create actually succeeds, never before attempting it.

  This rule is what makes the cross-check's commission direction sound: if the record could contain no-ops, "every record entry has a matching diff change" would fail on correct behaviour, and the assertion would have to be weakened to the point of uselessness.
  It is also the rule that keeps auto-recording from committing the *opposite* of the campaign's defect — claiming a destruction that never occurred.

  **Rejected:** adding `rec` to `pathRequest`/`branchRequest` and giving the other three a parameter anyway (less churn at the five request-shaped call sites, but two mechanisms to remember and a silently-omissible field on the majority of them);
  adopting the request types into the three outliers (uniform shape, but `createExclusiveDir` and `createGitWorktree` are constructive minters that have no ownership or dirtiness to declare, so the request fields would be dead weight carried purely for shape).
- **Rationale:** the **Fabric Destruction Chokepoint Invariant** already guarantees, machine-checked by `cmd/lyx/destructiveguard_test.go`, that every destructive primitive in `package fabricengine` routes through `destroy.go`. Recording there makes destructive-mutation coverage *provably total by construction* rather than a per-call-site review obligation. This is the one place in the codebase where that guarantee already exists, and declining to use it would be throwing it away.
- **Rejected:** recording every mutation explicitly at its own success site, destructive included (uniform and obvious to read, but discards the existing total-coverage guarantee and makes a missed destructive site a silent, unguarded gap).

### ok-semantics-and-error-path-fields

- **Decision:** `ok` keeps its current meaning — "no error was returned". The envelope additionally always carries `mutations: [...]`, and carries `partial: true` when an error is returned *and* the record is non-empty. `internal/output` gains an additive error-path function taking fields.
- **Rationale:** requirement 3 of the task is that a verb returning an error with a non-empty mutation record "must say so explicitly" — `partial: true` is that explicit statement, in one unambiguous field. Keeping `ok` unchanged means no existing consumer has to re-learn what it means, and the `internal/output` change being purely additive means no other module is touched. `ok` still isn't a synonym for "nothing happened", but it no longer has to be: `mutations` and `partial` answer that question directly.
- **Fixed key set: `mutations` and `partial` are always present on every mutating verb's envelope**, success and failure alike. `mutations` is always an array, empty rather than `null` when nothing was mutated;
  `partial` is always a bool, `false` rather than absent. A consumer never has to distinguish "absent" from "false", and the key set does not vary by outcome — which is the property that lets a test assert the shape once per verb instead of once per path.
- **The new `internal/output` function:** `ErrFields(w io.Writer, msg string, fields map[string]any) int` — mirroring `Ok`'s existing `fields map[string]any` shape, writing one JSON line and returning exit code 1, with `msg` trimmed exactly as `Err` trims it today. `Err(w, msg)` stays and becomes the zero-field case;
  no existing caller changes.
  **Reserved keys:** `ok` and `error` are injected by the function *after* the caller's fields are laid down, so they always win over a caller-supplied key of the same name — matching `Ok`'s existing in-place `fields["ok"] = true` mutation rather than inventing a second collision policy. Fabric never supplies either key itself;
  the rule exists so the envelope's two invariant fields cannot be shadowed by accident.
- **`partial` derives from exactly one rule:** `error ≠ nil ∧ record non-empty`. Nothing else sets it.
  `PartialCommitError` and `PartialPullError` are **not** a second trigger — they are errors like any other, and when either is returned the record is non-empty anyway (a landed commit is a recorded `commit_created`), so they satisfy the one rule without needing a special case. `PartialPullError.Stage` surfaces as the `Detail` of the relevant mutation rather than as a top-level envelope field, keeping the envelope's key set fixed across verbs.
- **This supersedes requirement 2 of the design doc, and says so.** `manifest/designs/fabric-crucible-followups.md:406` states requirement 2 as "`ok` becomes a statement about that record plus the error, not a synonym for 'no error was returned'". This decision deliberately does the opposite for `ok` itself, and satisfies the *intent* behind requirement 2 — that the envelope as a whole stop lying — through `mutations` and `partial` instead. Redefining `ok` in place would silently change the meaning of a field every existing consumer already reads, which trades one dishonesty for another. That doc line is amended in the same commit as the `:419` consumer-claim correction, so the design doc and the implementation do not disagree.
- **Rejected:** a `status` string (`ok`/`partial`/`refused`/`failed`) with `ok` derived from it (richer and self-describing, but a second source of truth alongside `ok` invites the two to disagree, which is the original defect in a new costume); leaving `internal/output` untouched and having `fabriccli` emit its own envelope (leaves the shared package alone, at the cost of fabric's JSON drifting from every other module's plus a second envelope implementation to keep in sync).

### structured-refusal

- **Decision:** when the returned error carries a `*destructiveRefusal` (found via `errors.As`), the envelope adds a `refusal: {check, what, target, reason}` object alongside the existing flattened `error` string. The `error` string is retained unchanged.
- **Rationale:** `*destructiveRefusal` (`destroy.go:62`) already holds `Check`, `What`, `Target` and `Reason` as separate fields;
  `Error()` flattens all four into one string, and `output.Err` then re-flattens that into the envelope. Un-flattening it is the second half of the honesty story — "three pairs removed, then the ownership check refused on *this* target" is exactly the sentence the task wants representable, and every field needed already exists.
- **Rejected:** flattened-only (smallest surface, but hands the operator a sentence to parse instead of a field to read — precisely the asymmetry `PruneEntry` already fixed at the per-entry level); dropping the flattened `error` when a refusal is present (no duplicated information, but breaks the "every failure carries an `error` string" contract every other module's envelope holds).

### which-verbs

- **Decision:** the twelve mutating verbs only. `list`, `pairs`, `status` and `diff` are untouched and carry no `mutations` key.
- **Rationale:** a record on a verb that cannot mutate is noise, and a read-only verb has no dishonesty to fix.
- **Rejected:** every verb with an always-empty record on the read-only four (perfectly uniform envelope, trivially assertable, but a meaningless key on four verbs forever); destructive verbs only — `remove`, `prune`, `cleanup`, `unwire`, `pull` (smallest change covering both reported defects, but `add`'s rollback path and `checkout`'s both-sides rollback have the identical mutated-then-errored shape and would stay unrepresentable).

### machine-enforcement

- **Decision:** add a **Mutation Record Invariant** to `CONSTRAINTS.md`, machine-guarded by an extension to `cmd/lyx/destructiveguard_test.go`: every destructive executor in `destroy.go` records, and every mutating verb's result type carries the record.
- **Rationale:** the repo's own stated pattern is that each invariant is partly machine-enforced and partly a review obligation. Without a guard, this rule rots the first time a verb is added — and this is precisely the class of rule a new verb silently skips, because nothing fails when it does.
- **Rejected:** a pure review obligation (cheaper, and consistent with the several review-obligation-only invariants already in the file, but the failure mode here is silent); no invariant at all (the types alone don't stop a new failure path returning a zero result).

### remove-ordering-anomaly

- **Decision:** represent, don't reorder. The record honestly reports the portal and launcher deletions alongside the refusal;
  `Remove`'s ordering is left exactly as it is. File a separate issue for the reorder.
- **Rationale:** slice 14 is the truthfulness slice, and this is the canonical "mutated, then refused" case it exists to make representable — it is the best live test case in the tree for the whole slice. Reordering is a behaviour change that belongs next to the gate, not here.
- **Rejected:** represent *and* reorder (fixes the underlying wrong too, but changes `Remove`'s observable behaviour and invalidates the `fabrictest` cell that currently declares those paths as permitted removal roots); reorder only (makes the anomaly disappear rather than representable, destroying the slice's best test case).

### check-enum-single-declarer

- **Decision:** export the check enum from `internal/fabricengine` as the single declarer, and have `fabrictest` consume it and delete its own copy. Not deliberate parallel copies.
- **Rationale:** emitting `refusal.check` as a machine-readable JSON field promotes the enum's rendered values to part of fabric's public contract. Two encodings of a public contract — one production, one test-support — is precisely the setup where the test copy drifts and the harness starts asserting against a vocabulary production no longer emits. The repo already has this exact pattern in the **Lyxdirs Single-Declarer Invariant**, and `fabrictest` already imports `fabricengine` (`fabrictest/hub.go:22`), so consuming the exported enum costs nothing structurally.
- **The exported type is string-backed, with exactly three constants.** `type Check string`, values `containment` / `ownership` / `dirtiness` — the same spellings `fabrictest` uses today, so `RefusedByGate`'s `string(check)+" check failed"` (`fabrictest/refusal.go:56`) keeps working verbatim, and `refusal.check` marshals to JSON with no conversion step. The current `destructiveCheck` is an `int` enum with a `String()` method (`destroy.go:33-56`), which would need a marshal shim for the envelope and would break that `string(check)` conversion.
- **`checkForce` is deleted, not exported.** The round-2 text claimed force was a non-member of both enums;
  that was wrong — `checkForce` **is** a member of `fabricengine`'s `destructiveCheck` (`destroy.go:39`) and `String()` renders it as `"force"` (`:51-52`). Non-membership was only ever true of fabrictest's copy.
  It can be deleted outright rather than carried across: the whole tree references it in exactly two places, its own declaration and that `String()` arm (plus two doc comments), and nothing anywhere constructs a `*destructiveRefusal` with it — because force is consulted only inside `checkPathDirtiness`, where it makes the dirtiness check *pass* rather than fail.
  Moving to a string-backed type removes `String()` entirely, so the arm goes with it and there is no dead member left to explain.
- **The non-constructibility rule survives the deletion**, on the exported type's doc comment: a force member must never be added back, because a refusal can never be attributed to it. `fabrictest/refusal.go:32-37` and `fabrictest/doc.go:41` both currently carry this warning;
  it moves to the exported enum rather than being lost with the copy. `destroy_test.go:596`'s `assertRefusalCheck(t, err, want destructiveCheck)` updates to the exported type.
- **Rejected:** deliberate parallel copies (zero churn, and `fabrictest`'s string-backed matching keeps working untouched — but it leaves two vocabularies for one public contract, with nothing to keep them equal);
  leaving it undecided for mill-plan (which is what round 1 correctly rejected — this decides whether a production type gets exported, which is a design decision, not a planning detail).

### permitted-roots-and-the-oracle

- **Decision:** permitted removal roots suppress **diff noise only**, and never suppress the honesty assertion. The record must still name a permitted-root mutation positively. Concretely, each cell runs `DiffManifest` **twice**: once with the cell's permitted roots, feeding the existing survival assertion (what was allowed to change), and once with a `nil` permitted list, feeding the new honesty assertion (what the record must account for).
- **Rationale:** this is the difference between the slice's headline case asserting something and asserting nothing. The `Remove` anomaly cell declares `_portals/<anchor>/<slug>` and `_launchers/<anchor>/<slug>` as permitted precisely *because* they do get destroyed before the refusal — so if permitted roots also suppressed the honesty check, the one cell that reproduces "mutated, then refused" would assert exactly nothing about the record. Separating the two questions is what makes the cell say: those deletions were allowed to happen **and** the envelope admitted to them.
- **Mechanically free on the diff side:** `DiffManifest(before, after Manifest, permitted []string)` (`fabrictest/manifest.go:289`) already takes the permitted list as a parameter, so calling it a second time with `nil` needs no API change.
- **The record side does need plumbing, and it is not free.** `VerbCase.Run func(tb, h, f) error` (`fabrictest/verbs.go:195-201`) discards the typed result, and the twelve verbs return twelve heterogeneous result types, so there is nothing for a cell to cross-check against today. The seam:
  - Every mutating result type **embeds** the record and exposes a common accessor `Mutated() Mutations`, so a cell can read the record without knowing which of the twelve types it holds.
  - `VerbCase.Run`'s signature becomes `func(tb, h, f) (fabricengine.Mutations, error)`, and each existing cell's `Run` closure returns `res.Mutated()` alongside its error.

  The round-2 claim that the oracle "costs nothing to apply everywhere" was true of the diff side only;
  this is the real cost, and it is a mechanical edit across the existing cells rather than a design problem.
- **Only the manifest-observable Kinds participate in the diff cross-check.** `CaptureManifest` records `.git` itself and each `.git/worktrees/<name>` at existence granularity and excludes everything else beneath `.git`, and states outright that "branch existence is deliberately not carried by the manifest at all;
  it is asserted through git itself in the per-verb effect assertions" (`fabrictest/manifest.go:109-118,184-213`). Splitting the enum accordingly:
  - **Manifest-observable — cross-checked both ways:** `path_removed`, `worktree_removed`, `worktree_created`, `dir_created`, `link_removed`, `link_created`, and `file_written` **when its target is outside `.git/`**.
  - **Git-state — exempt from the *commission* direction, asserted against git itself** via the existing per-verb effect assertions, which is the authoritative oracle rather than an inference from a ref file: `branch_created`, `branch_deleted`, `branch_pushed`, `commit_created`, `worktree_reset`, `worktree_switched`, `push_spawned`, and `file_written` **when its target is under `.git/`** (e.g. `.git/info/exclude`, which the manifest deliberately excludes).
    Note this exemption is one-directional by design: a git-state entry is not *required* to show a diff change, but `worktree_reset` and `worktree_switched` still **provide coverage** for the omission direction, since the working-tree rewrites they cause are real and must be accounted for.

  Without this split the stated reverse direction would fail on every cell that creates a branch or lands a commit — the record would carry entries the manifest can never corroborate, and the assertion would be reporting a lie of commission on entirely correct behaviour.
- **The cross-check, stated in both directions, over the manifest-observable kinds only:** every change in the unfiltered diff must be covered by some record entry (a lie of omission — defect 2's shape), and every manifest-observable record entry must have at least one corresponding unfiltered-diff change (a lie of commission).
- **Matching is segment-wise subtree containment, not path equality.** One `worktree_created` or `path_removed` entry names a single root, while `DiffManifest` emits one `Change` per path beneath it — so a normal `Add` yields dozens of diff entries against one record entry. Under string equality every one of those would read as a lie of omission on correct behaviour. Concretely:
  - *Omission:* every unfiltered-diff change's path must equal, or be a segment-wise descendant of, some record entry's **coverage set** (defined below) — not merely of a `Target`.
- **A worktree-rooted entry covers more than its own subtree.** Matching a diff change against `Target` alone leaves two whole classes of legitimate change uncovered, and both would fire as false lies of omission on correct behaviour:
  - **Git-admin bookkeeping.** The manifest deliberately records `<prime>/.git/worktrees/<slug>` (`manifest.go:184-213`), and the harness declares exactly those paths as permitted roots today via `primeWorktreeAdminPermittedRoot` / `primeWeftAdminPermittedRoot` (`verbs.go:393,403`). The unfiltered honesty pass unmasks them — but `worktree_created`/`worktree_removed` name the *worktree* path, never the prime repo's admin entry, so every `Add` and `Remove` cell would fail.
  - **Git-driven working-tree rewrites.** `Checkout` runs `git switch` in both worktrees (`checkout.go:82-91,131-148`) and `Pull` advances warp by reset — each rewriting tracked files across the worktree. Those emit content/add/remove changes that no `Target` covers. The git-state split exempts entries from *commission* only;
    it never exempted diff changes from *omission*, which was an asymmetry in the round-2 rule rather than a decision.

  **Coverage rule:** an entry whose `Target` is a worktree root — `worktree_created`, `worktree_removed`, `worktree_reset`, `worktree_switched` — covers, for the omission direction, both (i) every path at or beneath that worktree root, and (ii) the corresponding `<prime>/.git/worktrees/<slug>` admin entry, where `<slug>` is derived from the worktree path exactly as the harness's own two helpers already derive it.
  Every other kind covers its `Target` subtree alone.
  This is one rule rather than two exemption lists, and it is honest about *why* the admin entry changed: because a worktree was created or destroyed, which the record does state.
  - *Commission:* every manifest-observable record entry's `Target` must have at least one unfiltered-diff change at or beneath it — **except entries inverted later in the same record** (see below).
- **Commission applies to the record's net effect, not its raw entries.** A mutation performed and then undone *inside one call* nets to zero in the before/after manifest while the record correctly carries both entries. This is not an edge case — it is the rollback behaviour two of the named test cells exist to prove:
  - `Add` mints a worktree (`add.go:111`) and dirs/links, then `rollbackAdd` removes all of them.
  - `Checkout` rolls both sides back via `rollbackSwitch` (`checkout.go:97,110,114`).
  - `reconcile` repointing a **dangling** link removes it and re-creates it at the same target, so `DiffManifest` emits no `Change` at that path at all.

  Under a raw-entry commission rule all three would fail on correct behaviour — the same false-positive class the git-state split was introduced to remove.
  **Rule:** a record entry is exempt from the commission direction when a later entry in the same record inverts it on the same `Target`. The inverse pairs are `dir_created`/`worktree_created` ↔ `path_removed`/`worktree_removed`, and `link_created` ↔ `link_removed`.
  Equivalently: fold the record to its net effect first, then assert commission over what survives the fold.
  The omission direction is unaffected and still runs over **raw** entries — a path the diff shows as changed must be accounted for by *something* in the record, whether or not it was later undone.
  - Segment-wise means the same matching `pathPermitted` already implements for permitted roots — a root of `_portals/x` never matches `_portalsfoo/y` — so the harness has the helper already and the two mechanisms stay consistent.
- **Rejected:** excluding permitted-root changes from the cross-check (simpler, one `DiffManifest` call — and it silently guts the `Remove` anomaly cell, the best live test case the slice has);
  asserting the record only against the filtered diff (same defect, less obviously).

### fabrictest-truthfulness-oracle

- **Decision:** every existing matrix cell additionally asserts that the result envelope's mutation record matches the manifest diff — what the record claims was mutated is exactly what the filesystem shows changed.
- **Rationale:** the harness already captures a before/after manifest (`CaptureManifest` / `DiffManifest`, `fabrictest/manifest.go:121,289`), so the oracle is available for free at every cell. One assertion catches both defect directions: a record claiming nothing after a destruction, and a success reported after a destruction.
- **Rejected:** dedicated cells for only the two reported defects (targeted and cheap, but leaves every other verb's honesty unasserted — which is how both defects survived six hardening rounds); asserting only that the record is non-empty when it should be (much simpler, and much weaker — a record naming the wrong target passes).

## Technical context

**The two layers, and where the record is destroyed today.**

- Engine: every verb returns `(Result, error)` and on *every* failure path returns a zero-valued result — e.g. `internal/fabricengine/remove.go:41` `Remove` has eight or so `return RemoveResult{}, err` sites. The record is not merely unreported; it is destroyed at the `return`.
- CLI: `internal/output/output.go`'s `Err(w io.Writer, msg string) int` marshals exactly `{"ok":false,"error":...}` and accepts no fields. There is currently *no* error-path envelope in the repo that can carry anything. `Ok(w, fields map[string]any)` injects `"ok": true` into the caller's map.

**Engine entry points for the twelve in-scope verbs** (all in `internal/fabricengine`):

| Verb | Entry point |
| --- | --- |
| `clone` | `CloneHub(cwd string, opts CloneOptions) (CloneResult, error)` — `clone.go:124` |
| `add` | `(*Topology).Add(l, slug, opts) (AddResult, error)` — `add.go:37`, with `rollbackAdd` at `add.go:222` |
| `remove` | `(*Topology).Remove(l, slug, force) (RemoveResult, error)` — `remove.go:41` |
| `checkout` | `(*Topology).Checkout(l, branch) (CheckoutResult, error)` — `checkout.go:38`, with `rollbackSwitch` at `checkout.go:189` |
| `prune` | `(*Topology).Prune(l, apply, force) (PruneResult, error)` — `prune.go:77` |
| `cleanup` | `(*Topology).Cleanup(l, apply, force) (CleanupResult, error)` — `cleanup.go:100` |
| `unwire` | `Unwire(cwd string) (UnwireVerbResult, error)` — `unwire.go:56`, over `UnwireJunctions` at `junction.go:368` |
| `reconcile` | `(*Topology).Reconcile(l) (ReconcileResult, error)` — `reconcile.go:150`, with `repairPairWiring` at `reconcile.go:329` |
| `commit` | `(*Fabric).Commit(files, msg, snapshotTags, opts) (CommitResult, error)` — `commit.go:107` |
| `push` | `(*Fabric).PushWeft(opts) error` — `weftgit.go:269`; also `PushWarpAt` (`spawn.go:89`), `CoalescePushBothAt` (`coalesce.go:86`) |
| `pull` | `(*Fabric).Pull(opts) (PullResult, error)` — `pull.go:171` |
| `sync` | composed in `internal/fabriccli/weft_verbs.go` from commit + push |

Note that `push` and `sync` currently return bare `error` with no result type at all (`output.Ok(out, map[string]any{})` at `weft_verbs.go:183,196,265`) — they need a result type introduced, not just extended.

**`push`/`sync` composition, spelled out** (the verb table's `push` row maps to three engine functions, and `sync` has no engine function of its own):

- `PushResult` is introduced once and returned by all three push entry points — `(*Fabric).PushWeft` (`weftgit.go:269`), `PushWarpAt` (`spawn.go:89`), and `CoalescePushBothAt` (`coalesce.go:86`). One type, three producers;
  `CoalescePushBothAt` pushes both sides, so its record simply carries two `branch_pushed` entries.
- **The `push` verb is itself two calls, not one.** `weft_verbs.go:188-192` runs `fab.Commit` and *then* `fab.PushWeft`, both in-process. So `push`'s envelope is already a concatenation — the commit record followed by the push record, in execution order — and needs the same rule this section originally reserved for `sync`. Its `--bypass` branch (`:179-186`) instead calls `CoalescePushBothAt` alone, and carries that one record.
- **`sync` is not "commit + push" and cannot carry a push record at all.** `weft_verbs.go:257-265` runs `fab.Commit` and then `spawnPush`, which delegates to `fabricengine.SpawnDetachedPush` (`fabriccli/spawn.go:13-15`) — a **detached child process**. The push happens in another process, after this one has returned;
  its outcome is unobservable here.
  `sync`'s envelope therefore carries the commit record plus exactly one `push_spawned` entry (`Target`: the weft worktree path;
  `Detail`: `detached`), recording that a push was *launched*.
  It must **never** emit `branch_pushed`, which would assert an outcome this process did not observe — precisely the control-flow-derived lie the slice exists to eliminate. Recording the spawn honestly is the whole point: the operator learns a push is in flight, not that a push succeeded.
- **Composition rule, applying to both verbs:** concatenate the composed calls' records in execution order into one flat `mutations` array;
  `partial` is true when any composed call returned an error and the **combined** record is non-empty. A commit that lands followed by a push that fails is exactly "mutated, then errored", and the combined record is what makes it visible.
- Neither verb needs a `PartialSyncError`/`PartialPushError`;
  the combined record plus `partial` carries the same information without a new error type.

**The gate's destructive executors** (all in `internal/fabricengine/destroy.go`), which are the auto-record sites:

- `removePath(req pathRequest) error` — `:610`, wrapping `RemoveAll` (`:624`) and `os.Remove` (`:630`)
- `removeGitWorktree(req pathRequest, repoDir string)` — `:640`, `git worktree remove`
- `removeLink(req pathRequest) error` — `:657`, `fslink.Remove`
- `repointLink(what, container, target string, own pathOwnership) error` — `:668` (a mutation, not a destruction, but it lives here and changes a link target)
- `deleteBranch(req branchRequest)` — `:683`, `git branch -D`
- `resetHardTo(req pathRequest, repo *gitrepo.Repo, sha string) error` — `:739`, the primitive behind defect 1

The two minters `createExclusiveDir` (`:703`) and `createGitWorktree` (`:721`) are *constructive* and also live in `destroy.go`;
they are natural auto-record sites too and should be treated as such.

**The refusal type:** `destructiveRefusal{Check, What, Target, Reason}` at `destroy.go:62`, with `Error()` at `:70` and `surfaceRefusal` at `:79`.

The check enum exists in two places, in two packages, and the `refusal` object must be serialised from the first, not the second:

- `internal/fabricengine`'s own `destructiveCheck` (`destroy.go:33-40`) is **entirely unexported** — `checkContainment`, `checkOwnership`, `checkDirtiness`, `checkForce` — rendered to prose by `String()` at `destroy.go:42`. This is what a `*destructiveRefusal` actually carries, and therefore what production code in `fabriccli` must serialise the `refusal.check` field from. Grepping `internal/fabricengine` for an exported `CheckContainment` finds nothing.
- `internal/fabricengine/fabrictest`'s exported `Check` (`fabrictest/refusal.go:19-30`) is a string-backed, **`integration`-tagged** fabrictest-owned copy of that unexported enum, built by slice 13 so `RefusedByGate` can match a rendered message without importing the unexported type. It has exactly three live members — `CheckContainment`, `CheckOwnership`, `CheckDirtiness`. Being test-support behind a build tag, production code cannot import it.

`checkForce` is declared and rendered by `String()` but is never constructed into a `*destructiveRefusal` anywhere in the tree — force is consulted only inside `checkPathDirtiness`, where it makes the dirtiness check *pass* rather than fail.
A `CheckForce` constant could therefore never match a real refusal and must not be added to either enum.
`fabrictest/refusal.go` carries this as a standing comment, and `fabrictest/doc.go` documents it again;
the new refusal-serialisation code must not reintroduce it.

Once `refusal.check` is emitted as a machine-readable JSON field, the enum's rendered output becomes part of fabric's public contract and fabrictest's copy becomes a second encoding of it.
This is settled in the `check-enum-single-declarer` Decision above: export the enum from `fabricengine`, have `fabrictest` consume it and delete its copy, and carry the `checkForce` non-membership rule onto the exported enum's own doc comment.

**Prior art to mirror, not duplicate:** `PruneEntry` (`prune.go:38`) carries `WarpWorktree`/`WeftWorktree`/`Reason`/`Removed`/`Protected`/`Unowned`/`Error`;
`CleanupBranchEntry` (`cleanup.go:65`) carries `Branch`/`Deleted`/`Protected`/`Error`.
These per-item rows stay as they are — the new top-level `mutations` record is a different axis (one row per mutation performed, not one row per item enumerated) and coexists with them.

**Existing partial-failure types to align with, not replace:** `PartialCommitError` (`commit.go`, private fields, matched via `errors.As`) and `PartialPullError` (`pull.go`, with a `Stage` naming which warp-side step failed). Both already express "one side landed, the other didn't". They are the closest thing in the tree to the new `partial: true` semantics, and their relationship to it is stated in the `ok-semantics-and-error-path-fields` Decision above: they are ordinary errors, not a second trigger for `partial`, which derives solely from `error ≠ nil ∧ record non-empty`.
They satisfy that rule without a special case, because a landed commit is itself a recorded mutation. `PartialPullError.Stage` surfaces as the `Detail` of the relevant mutation, not as a top-level envelope field.
Neither type is replaced or reshaped by this slice.

**The harness:** `internal/fabricengine/fabrictest` (build tag `integration`).
`CaptureManifest(tb, hubRoot) Manifest` at `manifest.go:121`, `DiffManifest(before, after Manifest, permitted []string) []Change` at `manifest.go:289`. `Change` carries `Path`, `Kind` (`ChangeRemoved`/`ChangeAdded`/…), `Before`, `After`, sorted by `Path` for stable output. Cells are built from `VerbFixture` (`verbs.go:102`) and `VerbCase` (`verbs.go:195`), driven as a cross product in `matrix_test.go`, each cell declaring its permitted removal roots.
The `filepath.ToSlash` hub-relative path convention `DiffManifest` uses is the one the `Mutation.Target` field should match, so the cross-check is a direct comparison rather than a normalisation exercise.

**Consumer enumeration — the design doc's premise is stale and must be corrected.**
`manifest/designs/fabric-crucible-followups.md:419` says `internal/boardengine` "routes through `CommitWeftAt`/`PushWeftAt`" and is the first consumer to check.
Neither function exists any more.
`boardengine` uses `fabricengine.Bolt` (`internal/boardengine/sync.go:38`), an in-process Go API returning `(sha, committed, err)` — no JSON involved, and out of scope per the Decisions above.
Exploration found **no** programmatic parser of fabric's JSON output anywhere in the tree.
The sandbox suites (`tools/sandbox/SANDBOX-FABRIC-SUITE.md`, `SANDBOX-CORE-SUITE.md`) drive `lyx fabric` as prose read by an agent, not as parsed JSON.
The JSON-shape risk this task flags up front is therefore materially lower than assumed — but the design doc line must be corrected in the same commit so the next reader isn't sent to a function that no longer exists.

## Constraints

From `CONSTRAINTS.md`:

- **Fabric Destruction Chokepoint Invariant.** `destroy.go` is the only file in `package fabricengine` permitted to perform a destructive primitive. Banned bypass tokens: `RemoveAll(`, `os.Remove(`, `"worktree", "remove"`, `"branch", "-D"`, `warp.ResetHard(`, `weft.ResetHard(`, `fslink.Remove(`, `createdToken{`. This invariant is what makes gate-auto-recording provably total — and it means the recorder must be threaded *into* `destroy.go`, never worked around by recording at the call sites outside it. Enforced by `cmd/lyx/destructiveguard_test.go`. The gate's four checks always run in the fixed order containment → ownership → dirtiness → force, stopping at the first failure;
  a `*destructiveRefusal` is never discarded on a best-effort path (every such site wraps its executor call in `surfaceRefusal`, or logs via `logger.Warn` where it cannot return an error).
- **CLI / Cobra Invariant.** Errors are JSON via the `internal/output` envelope (`output.Ok`/`output.Err`), one JSON object per line, through the `clihelp.Execute`/root seam — no bare plain-text error paths. The new fields-carrying error function must live in `internal/output` and flow through that same seam. `Short` on every command;
  help accuracy is a review obligation whenever observable behaviour changes, which it does here.
- **Fabric Vocabulary Invariant.** Fabric (capital F) is the wired composite;
  warp and weft name the two sides only where they must be told apart. `host` is retired and machine-banned via the fabric-sense phrase list plus five policed geometry identifiers. `Mutation.Kind` enum values, JSON keys, doc comments and CLI help text are all in scope of this check (production `.go` under `internal/` and `cmd/`, plus `internal/**/*.md`). Enforced by `internal/lyxcwd/enforcement_test.go`'s `TestEnforcement_FabricVocabulary`.
- **Fabric Git Invariant.** Every git operation LYX performs, warp or weft, goes through `internal/fabricengine` in Go, in-process.
- **Test Tier Purity Invariant.** Untagged test files spawn nothing — no `gitexec.RunGit`, `exec.Command`, or `lyxtest.Copy*`. Raw substring match, so even a comment mentioning a banned token trips it. All harness work belongs behind the `integration` tag. Enforced by `cmd/lyx/tierpurity_test.go`.
- **Hermetic Git Test Environment Invariant.** Any git-spawning test package needs a `TestMain` calling `lyxtest.HermeticGitEnv()`. `fabrictest` already has one.
- **Markdown Link Integrity.** Every inline link in `manifest/`/`docs/` `.md` files resolves, file part and `#anchor` alike. The `fabric-crucible-followups.md` correction must keep its links resolving. Enforced by `internal/lyxcwd/docslink_test.go`.
- **Documentation Lifecycle** and `CLAUDE.md`'s task-completion rule: a task introducing cross-cutting infrastructure updates its module docs, `docs/overview.md` if the module table or execution stack changes, and `CONSTRAINTS.md` for the new invariant — all in the same commit. `manifest/roadmap.md` moves only on completing or adding a planned item;
  slice 14 is a planned roadmap item, so it does move here.

Discovered during discussion:

- `internal/output` is shared by every module. The change must be strictly additive — `Ok` and `Err` keep their current signatures and behaviour, and no other module's envelope changes.
- `push` and `sync` have no engine result type at all today. Introducing one for them is part of this slice, not a follow-up.
- The check enum lives unexported in `internal/fabricengine` (`destructiveCheck`) and is mirrored, exported and `integration`-tagged, in `internal/fabricengine/fabrictest` (`Check`). Production serialisation of `refusal.check` must come from the former;
  the latter is test-support behind a build tag and cannot be imported by production code. `checkForce` is a non-member of both and must not be "helpfully" added to any new refusal-serialisation code.

## Testing

**`internal/fabricengine` — unit, untagged (Tier 1).**

- TDD candidate: the `Mutations` accumulator itself (`mutation.go`) — append ordering, JSON marshalling of the three-field entry, empty-record marshalling. Pure data, no spawn, ideal first test.
- TDD candidate: the `*destructiveRefusal` → `refusal` object mapping, including the case where the error wraps a refusal several layers deep (must be found via `errors.As`, not a type assertion) and the case where it carries no refusal at all.
- Existing table tests for each result type extend to assert the record field is present and correctly typed.

**`internal/fabricengine` — the failure paths, tagged `integration` where they spawn git.**

Key scenarios, each of which is a mutated-then-errored path that today returns a zero result:

- `Remove` refusing on a dirty warp worktree *after* `removePortal`/`removeLaunchers` already deleted the portal and launcher paths — the anomaly documented in `fabrictest/doc.go`. The record must name both deletions and `partial` must be true.
  **No `refusal` object is present on this path, and the test must assert its absence rather than its contents:** `remove.go:68-76` returns a bare `fmt.Errorf("worktree has uncommitted changes; use --force")` from its own pre-flight, never a `*destructiveRefusal`, so `errors.As` finds nothing.
  This pre-flight is **not** converted to a gate refusal in this slice — that is a behaviour change belonging next to the gate, exactly as the `remove-ordering-anomaly` Decision holds for the ordering itself. It is the same class of issue and belongs on the same follow-up issue: `Remove` has a verb-level pre-flight that duplicates a gate check without producing a gate refusal.
- `Add` failing partway and running `rollbackAdd` — the record must reflect both the creations and the rollback's own destructions, in order.
- `Checkout` failing partway and running `rollbackSwitch` on both sides.
- `Prune`/`Cleanup` removing some entries and then failing — the "removed three of five pairs, then failed" case the task names as currently unrepresentable.
- `Pull` on a dirty warp worktree — must refuse, must not report the `ResetHard`, and the dirty file must survive (this is defect 1's regression test, and `TestPull_DirtyWarpRefusesBeforeMovingWarp` already exists from slice 12).

**`internal/fabriccli` — envelope shape.**

- Success path: `mutations` present (empty array when nothing was mutated, never `null`), `partial` present and `false`, existing top-level fields (`slug`, `path`, `links_removed`, …) unchanged — backward compatibility of the success envelope is an explicit assertion, not an assumption.
- Failure path with an empty record: `ok:false`, `error` present, `mutations` present and empty, `partial` present and `false`.
- Failure path with a non-empty record: `ok:false`, `error` present, `mutations` populated, `partial: true`.
- Failure path carrying a refusal: the `refusal` object present with all four fields, *and* the flattened `error` string still present.
- Read-only verbs (`list`, `pairs`, `status`, `diff`): assert `mutations` is **absent**, so the scope decision is machine-held rather than a convention.

**`internal/output`.**

- The new function emits `ok:false`, the trimmed message, and the supplied fields;
  `Ok` and `Err` behaviour is unchanged (regression assertion).

**`internal/fabricengine/fabrictest` — the truthfulness oracle, tagged `integration`.**

- Every existing matrix cell gains the cross-check: the mutation record from the verb's result must correspond exactly to the manifest diff between the before and after captures. A path in the diff with no matching record entry is a lie of omission (defect 2's shape);
  a record entry with no matching diff change is a lie of commission.
- The permitted-removal-roots interaction is settled by the `permitted-roots-and-the-oracle` Decision above: each cell calls `DiffManifest` twice — once with its permitted roots for the existing survival assertion, once with `nil` for the honesty assertion — so permitted roots suppress diff noise without ever suppressing the record check. The `Remove` anomaly cell is the case that turns on this, since `_portals/<anchor>/<slug>` and `_launchers/<anchor>/<slug>` are declared permitted there precisely because they *do* get destroyed;
  under this rule that cell asserts both that the deletions were allowed and that the envelope admitted to them.
- The nine-cell sabotage proof in `fabrictest/doc.go`'s own table should gain the truthfulness dimension: with a guarding check neutered, the cell must now fail on *both* the survival assertion and the honesty assertion.

**`cmd/lyx` — the guard.**

- Extend `destructiveguard_test.go`: every executor in `destroy.go` records into the recorder;
  every mutating verb's result type carries the record field. Follow the existing guard's pattern (per-file allowlist entries each carrying a reason) and state its blind spots honestly in the invariant text, as the chokepoint invariant already does for its own raw-substring matching.

## Q&A log

- **Q:** What carries the mutation record — a shared accumulator, per-verb bespoke fields, or a generalised `PruneEntry` row? **A:** Shared `Mutations` accumulator embedded in every mutating verb's result, one `mutations` JSON key.
- **Q:** How does the record survive the error return? **A:** Verb-owned `*Mutations` recorder threaded into helpers, and every failure return changed to `return res, err` with `res` populated;
  named-result plus `defer` so a new early return can't drop it.
- **Q:** Should the destruction gate auto-record? **A:** Yes — the chokepoint invariant makes destructive coverage provably total, so recording inside `destroy.go` gets it for free;
  constructive mutations stay hand-recorded at their success sites.
- **Q:** What does `ok` mean now, and how does the error path carry the record? **A:** `ok` keeps meaning "no error";
  add always-present `mutations` and an explicit `partial: true` when an error returns with a non-empty record;
  extend `internal/output` additively with a fields-carrying error function.
- **Q:** Which verbs get a record? **A:** The twelve mutating verbs only. `list`/`pairs`/`status`/`diff` stay untouched, and a test asserts `mutations` is absent on them.
- **Q:** What does one `Mutation` entry carry? **A:** Three flat fields — `Kind` (fixed enum), `Target` (slashed path or git ref), `Detail` (optional). Ordering by array order alone;
  no `Sequence` field, no per-kind typed payloads.
- **Q:** Is the gate refusal carried structurally or only as the flattened error string? **A:** Structurally — a `refusal: {check, what, target, reason}` object via `errors.As`, *alongside* the retained flattened `error` string.
- **Q:** Machine enforcement, review obligation, or neither? **A:** A new Mutation Record Invariant in `CONSTRAINTS.md`, machine-guarded by an extension to `cmd/lyx/destructiveguard_test.go`.
- **Q:** How far do the `fabrictest` truthfulness cells go? **A:** Every existing matrix cell cross-checks the record against the manifest diff — the harness's before/after capture is the oracle, so it costs nothing to apply everywhere.
- **Q:** The `Remove` pre-flight ordering anomaly — represent, reorder, or both? **A:** Represent only. The record reports the portal/launcher deletions alongside the refusal;
  ordering is left alone and a separate issue is filed for the reorder.
- **Q:** [auto-pick] Does `fabricengine.Bolt` get a record? **A:** No — out of scope. **Why:** `Bolt.Commit` already returns `(sha, committed, err)`, an accumulate-as-you-go shape, and its only consumer (`internal/boardengine`) reads Go values, never JSON.
- **Q:** [auto-pick] Where does the new vocabulary live? **A:** A new file `internal/fabricengine/mutation.go`. **Why:** matches the package's one-concern-per-file layout;
  putting it in `destroy.go` would imply the record is destruction-only, which it isn't.
- **Q:** [auto-pick] Do the constructive minters in `destroy.go` (`createExclusiveDir`, `createGitWorktree`) auto-record too? **A:** Yes. **Why:** they already sit inside the gate file and are the creation counterparts of the executors;
  recording them there is free and keeps creation/destruction symmetric in the record.
- **Q:** [auto-pick] Do `push` and `sync` get result types introduced? **A:** Yes, in this slice. **Why:** they return bare `error` and emit `output.Ok(out, map[string]any{})` today, so there is nowhere to put the record without one — deferring it would leave two of the twelve verbs unfixed.

Review round 1 gaps (auto-picked at the recommended resolution):

- **Q:** [auto-pick] How does the recorder reach `repointLink`, `createExclusiveDir` and `createGitWorktree`, which take neither request type? **A:** An explicit `rec *Mutations` leading parameter on all eight gate sites, rather than a request-type field for five and a parameter for three. **Why:** a missing struct field is a silent zero value the compiler accepts;
  a missing parameter does not compile. This slice exists because a record was silently dropped, so the mechanism that turns dropping it into a build failure is the right one — and one uniform rule beats two.
- **Q:** [auto-pick] What are the initial `Kind` members? **A:** Thirteen, tabulated in the `mutation-entry-shape` Decision — eight auto-recorded inside `destroy.go` (one per executor and minter) plus five hand-recorded constructive kinds (`branch_created`, `branch_pushed`, `commit_created`, `link_created`, `file_written`). **Why:** the guard test can only pin an enumerated set, and `Kind` was described as "precisely what a guard test can pin" while naming no member. Adding a member requires its recording site and guard entry in the same commit;
  `mutation.go` is the single declarer.
- **Q:** [auto-pick] What covers the git-admin bookkeeping paths and the file churn from a branch switch, in the omission direction? **A:** One coverage rule: an entry whose `Target` is a worktree root (`worktree_created`/`_removed`/`_reset`/`_switched`) covers both its own subtree and the corresponding `<prime>/.git/worktrees/<slug>` admin entry. A new `worktree_switched` kind is added for `Checkout`'s two `git switch` sites. **Why:** round 2 gave *commission* an exemption mechanism and left *omission* with none — an asymmetry, not a decision. Without this, every `Add`/`Remove`/`Checkout` cell reports a false lie of omission on correct behaviour.
- **Q:** [auto-pick] How does a harness cell actually read the record, given `VerbCase.Run` returns only `error`? **A:** Every mutating result type embeds the record and exposes `Mutated() Mutations`;
  `VerbCase.Run` becomes `func(tb, h, f) (fabricengine.Mutations, error)`. **Why:** the twelve verbs return twelve heterogeneous types, so a cell needs one accessor to read them uniformly. The round-2 "costs nothing" claim was true of the diff side only.
- **Q:** [auto-pick] Are `mutations`/`partial` always present, or only when meaningful? **A:** Always — `mutations` an array (empty, never `null`), `partial` a bool (`false`, never absent), on success and failure alike. **Why:** a fixed key set means a consumer never distinguishes absent from false, and a test asserts shape once per verb rather than once per path.
- **Q:** [auto-pick] What is the new `internal/output` function called? **A:** `ErrFields(w io.Writer, msg string, fields map[string]any) int`, mirroring `Ok`'s shape; `Err` stays as the zero-field case. `ok` and `error` are injected after the caller's fields and always win, matching `Ok`'s existing in-place mutation rather than inventing a second collision policy.
- **Q:** [auto-pick] How does the commission check survive a mutation that is rolled back inside the same call? **A:** Commission applies to the record's **net effect** — an entry is exempt when a later entry in the same record inverts it on the same `Target`. Omission still runs over raw entries. **Why:** `Add`'s mint-then-`rollbackAdd`, `Checkout`'s `rollbackSwitch`, and `reconcile`'s dangling-link repoint all net to zero in the manifest while the record correctly carries both halves;
  a raw-entry rule would fail all three on correct behaviour.
- **Q:** [auto-pick] Who converts an absolute path to the hub-relative `Target`, given the gate has no hub root? **A:** The `Mutations` accumulator carries the hub root as a field, set at verb entry, and `Mutations.Append` converts. The gate keeps passing absolute paths and does no path arithmetic. **Why:** `pathRequest` carries only `container`, never the hub root, so the gate cannot convert;
  one conversion site also keeps hub geometry out of `destroy.go`.
- **Q:** [auto-pick] What about a `Target` that is the hub root itself (`CloneHub`'s `createExclusiveDir(hubPath)`)? **A:** Recorded honestly as `dir_created` with `Target: "."`, but exempt from both cross-check directions;
  the clone cell asserts hub creation through its own existence assertion. **Why:** `CaptureManifest` never emits a `"."` key, and `"."` would make every diff path a descendant — trivially satisfying omission for the whole cell while asserting nothing.
- **Q:** [auto-pick] Does the exported check enum keep `checkForce`, and what backing does it have? **A:** String-backed `type Check string` with exactly three constants;
  `checkForce` is deleted rather than exported. **Why:** the round-2 text was wrong that force was a non-member of both enums — it *is* a member of `fabricengine`'s, rendered `"force"` by `String()`. Nothing constructs it, it appears in only two code locations, and a string backing removes `String()` altogether while keeping `RefusedByGate`'s `string(check)` and the JSON marshal free. The never-add-it-back rule moves to the exported type's doc comment.
- **Q:** [auto-pick] Which Kinds can the manifest oracle actually observe? **A:** Only the filesystem-visible ones. Git-state kinds (`branch_*`, `commit_created`, `worktree_reset`, `push_spawned`, and `file_written` under `.git/`) are exempt from the diff cross-check and asserted against git itself via the existing per-verb effect assertions. **Why:** `CaptureManifest` excludes everything under `.git` bar `.git` itself and `.git/worktrees/<name>`, and says outright that branch existence is deliberately not carried;
  cross-checking those kinds against the diff would report a lie of commission on correct behaviour.
- **Q:** [auto-pick] How do record entries match diff changes, given one entry can name a whole subtree? **A:** Segment-wise subtree containment in both directions, never path equality — reusing the same segment matching `pathPermitted` already implements. **Why:** one `worktree_created` names a root while the diff emits one change per path beneath it, so equality matching would read a normal `Add` as dozens of lies of omission.
- **Q:** [auto-pick] How is a link repoint recorded, given `repointLink` executes by calling `removeLink`? **A:** Drop `link_repointed` entirely;
  a repoint records `link_removed` (auto, at the gate) then `link_created` (hand, at the caller's `fslink.CreateDirLink`). **Why:** that is what physically happens, it avoids double-recording one act, and it fixes a `Detail` that was unfillable — the new target does not exist at `repointLink`'s recording site.
- **Q:** [auto-pick] When does the gate append — unconditionally, or only on observed effect? **A:** Only after the primitive observably changed state: nothing for `removePath`'s already-absent early return, and nothing for `removeGitWorktree`/`deleteBranch` when git exits nonzero with a nil error. **Why:** a record containing no-ops would fail the commission direction of the cross-check on correct behaviour, and would commit the campaign defect's mirror image — claiming a destruction that never happened.
- **Q:** [auto-pick] What can `sync` record, given its push is a detached child? **A:** The commit record plus one `push_spawned` entry, never `branch_pushed`. `push` (a different verb) is in-process and concatenates its commit and push records. **Why:** `sync` calls `SpawnDetachedPush`, so the push outcome is unobservable in this process — asserting `branch_pushed` there would be exactly the control-flow-derived lie this slice exists to kill.
- **Q:** [auto-pick] Do permitted removal roots suppress the truthfulness cross-check as well as the diff noise? **A:** No — permitted roots suppress diff noise only;
  the record must still name a permitted-root mutation positively. Each cell calls `DiffManifest` twice, once with its permitted roots (survival) and once with `nil` (honesty). **Why:** the `Remove` anomaly cell declares the portal and launcher paths permitted precisely because they get destroyed before the refusal, so suppressing the honesty check there would leave the slice's headline case asserting nothing at all.
