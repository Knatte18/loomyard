# Discussion: fabric: store the warp-URL binding in weft:main; fold bootstrap into clone (slice 10)

```yaml
task: 'fabric: store the warp-URL binding in weft:main; fold bootstrap into clone (slice 10)'
slug: fabric-warp-binding-in-weft
status: discussing
parent: main
```

## Problem

A fabric hub is two git repos: a **warp** (the user's real repo) and a **weft** (lyx's own artefact repo, cloned as a sibling and reached through junctions).
`lyx fabric clone` today requires the operator to name both URLs every single time, in that order — `lyx fabric clone <warp-url> <weft-url>` (`internal/fabriccli/clone.go:32-36`).
Nothing anywhere records which warp a given weft belongs to,
so that pairing lives only in the operator's head and in shell history.

A weft is bound to exactly one warp (many-to-one: one warp can back several wefts), which makes the pairing a durable per-weft fact, not a per-invocation argument.
Slice 5 already established the precedent for storing exactly this kind of repo-wide fact: the lyx-anchor subpath is recorded once on `weft:main` as a plain `.lyx-anchor` marker at the board root, and every later clone adopts it instead of re-asking.
The warp binding is the same shape of fact — **but a distinct one**: the anchor says *where in warp* lyx is rooted, the binding says *which warp repo* this weft pairs with at all.

**Why now:** this is slice 10 of `manifest/designs/fabric-unified-view.md`, the last functional slice of the fabric campaign.
Slice 9 has landed, which was its stated blocker (both slices edit `runCloneWithReset` in the same ~45-line span, so they were deliberately sequenced rather than parallelised).
The design doc is deleted once slice 10 and slice 6's still-open orchestration half are both done, so this is the last structural change to fabric's clone surface before the fabric-v2 hardening pass.

## Scope

**In:**

- New warp-binding record `.lyx-warp` on `weft:main` (a plain single-line file at the board root, beside `.lyx-anchor`), written and read by `internal/fabricengine`, committed through the existing `Bolt` board-commit path.
- `internal/fabricengine`'s `CloneHub` gains an optional warp URL: it resolves the effective warp URL from the record when none is supplied, and enforces the conflict rule when one is.
- A pre-hub **probe** step that reads the record off the weft remote before any hub directory is created.
- **Breaking CLI change:** `lyx fabric clone`'s positional arguments flip to weft-first and the warp URL becomes optional — `lyx fabric clone <weft-url> [<warp-url>]`.
- `CloneHub`'s Go signature changes from four positionals (`CloneHub(cwd, warpURL, weftURL, subpath string)`, `internal/fabricengine/clone.go:85`) to `CloneHub(cwd string, opts CloneOptions) (CloneResult, error)`.
- `--reset`'s hub teardown moves out of `internal/fabriccli/clone.go` and into `CloneHub`, so it works in both argument forms.
- `Topology.Reconcile` backfills the record for hubs that predate it, with the CLI layer committing and pushing the backfill via `Bolt`.
- Every call site, help text, test, sandbox suite doc, and design/constraint doc that spells the clone argument order, updated in the same commit.

**Out:**

- **No new verb.** No `fabric bind`, no `fabric rebind`. Re-pointing a bound weft at a different warp stays a deliberate operation that this task does not provide; it is a hard error here.
- **No change to `.lyx-anchor`** — neither its format, its location, its reader (`internal/lyxcwd`), nor the subpath semantics. The binding does not store the subpath.
- **No change to hub naming.** The hub stays `<cwd>/<warp-name>-HUB`. Two wefts of the same warp collide on that directory name; that is true today, it is not what slice 10 is about, and it is recorded as a known limitation rather than fixed here.
- **No removal of `--reset`.** It has a live consumer (`tools/sandbox/SANDBOX-CORE-SUITE.md:211`) and removing pre-existing CLI surface is a separate decision.
- **Weft remote provisioning at first clone** — still an open question in the design doc, untouched.
- **Slice 6's orchestration half** — untouched.
- `internal/fabricengine`'s `add`/`checkout`/`prune`/`cleanup`/`unwire` verbs gain no binding awareness beyond `unwire` leaving the record alone (see Decisions).

## Decisions

### binding-record-format

- Decision: a plain single-line file named `.lyx-warp`, at the board root (`<BoardDir>/.lyx-warp`, i.e. tracked at the root of `weft:main`, the same directory `.lyx-anchor` lives in). Its entire content is the warp URL plus a trailing newline. It holds the warp URL **only** — not the subpath.
- Rationale: exactly mirrors the shipped `.lyx-anchor` precedent (`internal/lyxcwd/anchor.go:32-41`), which is a plain single-line marker read with `os.ReadFile` + `strings.TrimSpace` and needs no YAML dependency. The subpath already has one authoritative home in `.lyx-anchor`; putting it in a second file creates two records that can disagree, with no rule for which wins.
- Rejected: a `.lyx-warp.yaml` with `warp_url:`/`subpath:` keys — matches the design doc's literal wording ("warp URL + `--subpath`") but duplicates the subpath. Extending `.lyx-anchor` into a keyed two-field file — one file, but it changes a format `internal/lyxcwd` parses and re-couples two deliberately independent facts.
- Note for the plan: `manifest/designs/fabric-unified-view.md:152` says the binding stores "warp URL + `--subpath`". That is superseded by this decision and the doc must be corrected in the same commit.

### binding-ownership

- Decision: a new `internal/fabricengine/warpbinding.go` owns the filename constant and the read/write/normalize helpers. The file is committed onto `weft:main` by the existing `Bolt` board-commit call in `internal/fabriccli/clone.go:60-66` — the same call that already commits `.lyx-anchor` and the repo-wide `fabric.yaml`.
- Rationale: the binding is fabric's own illusion plumbing and has no bearing on cwd resolution, so the Cwd Resolution Invariant puts it in `fabricengine`, not `lyxcwd`. The design doc's "via the `Bolt` handle" is satisfied by the commit path: `Bolt` is a git-verb handle (`Commit`/`Push`/`Sync`), and fabric's established pattern is engine-writes-to-disk / CLI-commits-via-Bolt.
- Rejected: adding `ReadFile`/`WriteFile` methods to `Bolt` — a more literal reading of the design doc, but it widens a deliberately narrow git-verb handle, and the *read* that matters most happens during the probe, when no `BoardDir` exists at all. Putting the reader in `internal/lyxcwd` beside the anchor reader — barred by the Cwd Resolution Invariant.

### pre-hub-probe

- Decision: before any hub directory is created, clone the weft remote into a throwaway probe directory created with `os.MkdirTemp(cwd, ".lyx-clone-probe-")`, using `git clone --depth 1 --filter=tree:0 --no-checkout --single-branch <weft-url> <probe>`, then read the record with `git show HEAD:.lyx-warp`, then `os.RemoveAll` the probe (deferred, so it is removed on every exit path including error).
- Rationale: the hub is named after the warp repo (`DeriveWarpName`, `internal/fabricengine/clone.go:90-96`), so in the one-argument form there is no hub path — and therefore no `BoardDir` and no weft clone — until the warp URL is known. Git has no porcelain that reads a single file from a remote without a local repo directory: `git archive --remote` would, but GitHub does not enable it, and `git ls-remote` sees refs only. A partial clone is the minimal-wire equivalent: `--filter=tree:0` fetches no trees and no blobs, so the transfer is the ref advertisement plus one commit object, and `git show HEAD:.lyx-warp` then lazily fetches just the root tree and that one small blob. Because the probe is discarded, a binding conflict aborts before the hub exists, leaving zero residue and requiring no change to `teardownHub`'s semantics.
- `HEAD` in the probe is the weft remote's default branch, which is `main` — the branch `_board` is a worktree of and the branch `Bolt` commits to. The local `main` → `main-weft` rename (`suffixWeftPrimaryBranch`, `internal/fabricengine/clone.go:262`) happens only in the real weft clone and never touches the remote default.
- **Probe failure taxonomy** — the probe's outcome is three-way, not binary, and each git failure must be classified deliberately. "Record absent" means exactly one of two conditions: (a) the clone succeeded, `git rev-parse --verify HEAD` succeeded, and `.lyx-warp` is simply not present in that commit; or (b) the clone succeeded and HEAD is unborn — the genuinely empty weft remote, which `CloneHub` supports today via `ensureBoardWorktree`'s orphan-create path (`internal/fabricengine/clone.go:161-168`).
  **The absence discriminator must be an explicit presence probe, not a failed read.** A missing path and a broken repo both make `git show HEAD:.lyx-warp` exit nonzero, so the read cannot classify itself. Use `git ls-tree HEAD --name-only -- .lyx-warp`, which exits **0 with empty stdout** when the path is absent and 0 with the path echoed when present; a nonzero `ls-tree` exit is a hard error. `git show HEAD:.lyx-warp` runs only after `ls-tree` reported the path present, and any failure from it at that point is therefore a genuine hard error, not an absence. The old-order guard below needs exactly the same discriminator for its `.lyx-anchor` check and must use `ls-tree` the same way.
  **Everything else is a hard error**, surfaced with git's stderr verbatim behind a `probe weft <url>:` prefix: any nonzero `git clone` (unreachable host, auth failure, no such repository, a typo'd weft URL), a nonzero `ls-tree`, and any `git show` failure once `ls-tree` has confirmed the path exists.
  This costs one extra `git rev-parse --verify HEAD` call and is what keeps a network outage from being reported as `unbound weft` in the one-argument form, or from silently bootstrapping a fresh binding in the two-argument form.
- **Probe directory placement** — the probe lives in `cwd`, not `os.TempDir()`, because `cwd` is provably writable (the hub is about to be created there), it is where every other byte this command produces lands, and it avoids system-temp variance across platforms. A `SIGKILL` mid-probe leaves a recognisably-prefixed directory behind; that residue is inert and blocks nothing, unlike a residual hub, which is why `teardownHub` needs a hard error message and the probe does not. Confirmed during review: `TestEnforcement_GeometryLiterals` matches geometry tokens by exact equality, so the `.lyx-clone-probe-` prefix does not trip it and no rename is needed.
- Known degradation, stated honestly: partial clone requires `uploadpack.allowFilter` on the server. GitHub and GitLab have it. Against a **local bare repo path** — which is what every test in `internal/fabricengine` and `internal/fabriccli` uses — git ignores `--filter` and `--depth` with a warning and performs an ordinary local (hardlinked) clone. Correctness is unaffected; the probe is simply not minimal under test. The implementation must not treat those warnings as failures.
- Rejected: cloning the weft into a temp dir for real and `os.Rename`-ing it into `<Hub>/<name>-weft` once the name is known — avoids the extra fetch, but reorders `CloneHub`'s steps 5 and 6, adds a rename and a new teardown path, and makes the flow harder to follow. Splitting into two code paths (two-arg keeps today's order and conflict-checks after the real weft clone, one-arg probes) — cheapest for bootstrap, but two paths to specify and test for one rule.

### clone-argument-surface

- Decision: `lyx fabric clone [--reset] [--subpath <rel>] [--force-bootstrap] <weft-url> [<warp-url>]`. One or two positionals; three or more, or zero, is a usage error. `--force-bootstrap` exists solely to bypass the old-order guard below.
- Rationale: the design doc pins the weft-first flip as a deliberate breaking change. Weft-first is what makes the warp URL droppable: the required argument comes first, the optional one last.
- The `Use` string, the `Long` body, both examples (`internal/fabriccli/fabric.go:60,68,95-96`), the stale `// clone [--reset] <warp-url> <weft-url> [board-url]` comment at `fabric.go:57`, and the usage string at `internal/fabriccli/clone.go:33` all change together. The `Long` gains a paragraph explaining the two forms and the binding.

### clonehub-signature

- Decision: `CloneHub(cwd string, opts CloneOptions) (CloneResult, error)`, where `CloneOptions` carries `WeftURL`, `WarpURL`, `Subpath`, `Reset`, and `ForceBootstrap`. `CloneResult` gains `WarpURL string` (the effective warp URL actually cloned, whether supplied or derived) and `WarpBindingRecorded bool` (true when this clone wrote the record, false when it adopted an existing one).
- Decision: the CLI's success envelope (`internal/fabriccli/clone.go:88-91`, today `{"hub", "anchor"}`) gains exactly two keys: `"warp"` (string, the effective URL) and `"warp_binding_recorded"` (bool). Named concretely because `internal/fabriccli/cli_test.go` asserts on the envelope's keys.
- Rationale: the alternative is a fifth positional on top of today's four, of which two are optional strings and two are adjacent URLs — exactly the shape that produces silent argument-order bugs, and the argument order is the very thing this task is flipping. Every call site is being touched anyway, so the struct costs nothing extra.
- Rejected: keeping reordered positionals (`CloneHub(cwd, weftURL, warpURL, subpath string, reset bool)`) — smaller diff at the ~12 call sites, but preserves the failure mode.

### old-order-footgun-guard

- The hazard: after the flip, the pre-change invocation `lyx fabric clone <warp-url> <weft-url>` still has **valid arity**. Nothing about it is a usage error. The probe would read the *warp* repo, find no `.lyx-warp`, fire the bootstrap row, clone the user's real repo as the weft, materialize `_board` on it, and then `internal/fabriccli/clone.go:60-66` would commit `.lyx-anchor` + `fabric.yaml` + `.lyx-warp` and **push them to the user's own repo's default branch**. That is the worst outcome this change can produce and it must not be left to muscle memory.
- Decision: **bootstrap is gated on the weft candidate looking like a weft.** During the probe, after the record is found absent, require one of: (a) the weft candidate has an unborn HEAD (empty remote), or (b) its HEAD commit carries `.lyx-anchor` at the root (an existing lyx weft that predates the binding). Anything else is a hard error before the hub is created, naming the likely cause: `refusing to bootstrap <url> as a weft: its history carries neither .lyx-anchor nor an empty tree — check the argument order, clone now takes <weft-url> [<warp-url>]`.
- Decision: the guard has a documented escape, a new `--force-bootstrap` boolean flag, because there is one legitimate false positive — a brand-new weft remote created with an auto-generated `README`, which is neither empty nor `.lyx-anchor`-bearing. The `Long` documents the flag as exactly that and nothing else. Without an escape the guard would block a real first-ever setup with no remedy short of rewriting the remote's history.
- Rationale: the check costs one `git cat-file -e HEAD:.lyx-anchor` against a probe clone that has already resolved HEAD, and it converts a silent, pushed, hard-to-undo corruption of the user's own repo into an error message that names the exact mistake. The alternative — declaring it an accepted hazard documented in `Long` — was rejected: the blast radius is a push to someone's real default branch, and `Long` is not read by muscle memory.
- Note the interaction with the probe taxonomy: condition (a) is the same unborn-HEAD case the taxonomy already classifies as "record absent", so the guard adds a branch, not a new probe call. Condition (b) uses the same `git ls-tree HEAD --name-only -- .lyx-anchor` discriminator the taxonomy mandates, never a failed `git show`.
- Decision: `--force-bootstrap` is **silently ignored** outside the bootstrap row — in the one-argument form, and whenever a record is already present. Rationale: it is a safety-override for one specific gate, and making it a usage error elsewhere would turn a harmless leftover flag in a shell-history re-run into a failure. It is documented in `Long` as applying to bootstrap only.

### conflict-rule

Resolution of the effective warp URL, evaluated after the probe and before the hub is created:

| Record on `weft:main` | `<warp-url>` argument | Outcome |
| --- | --- | --- |
| absent | absent | hard error — unbound weft |
| absent | supplied | bootstrap **if the weft candidate passes the old-order guard above**, then clone with the supplied URL and write the record |
| present | absent | derive: clone with the recorded URL |
| present | supplied, normalizes equal | no-op: proceed, record untouched |
| present | supplied, normalizes different | hard error — never silently re-point |

- Decision: comparison is on **normalized** URLs. Normalization strips one trailing `/`, then one trailing `.git`, and lowercases the scheme and host portion. `https://github.com/u/r`, `https://github.com/u/r.git`, and `https://github.com/u/r/` are the same warp.
- Decision: a transport swap is **not** a match. `git@github.com:u/r.git` against a recorded `https://github.com/u/r` is a differing URL and therefore a hard error.
- Rationale: the trailing-`/`-and-`.git` variants are pure spelling noise that would otherwise produce a baffling error for an operator who typed the same repo. A transport swap is not noise: accepting it silently would leave the record describing something other than what was actually cloned, and the operator can always drop the argument and let the record win.
- Decision: the record is written whenever it is absent and a warp URL is available — including on the *adopt* path, where `.lyx-anchor` already exists (a re-clone of a pre-binding hub) but `.lyx-warp` does not. That is clone-time backfill and it needs no special casing beyond "absent ⇒ write".
- Rejected: byte-exact comparison (simplest and most predictable, but punishes a trailing `.git`); full URL equivalence including transport (hides a real difference).

### unbound-weft-error

- Decision: the message names both the cause and the fix — `weft <url> has no recorded warp binding; supply the warp URL explicitly: lyx fabric clone <weft-url> <warp-url>`.
- Rationale: every hub in existence today is unbound, so this message is the migration instruction the first operator to hit it needs. A terse `unbound weft: warp URL required` makes them go read the help.
- The error surfaces through the ordinary `output.Err` envelope, per the CLI/Cobra Invariant. No hub directory has been created at this point, so there is nothing to tear down.

### reset-folding

- Decision: `--reset` survives unchanged in meaning (remove an existing `<cwd>/<name>-HUB` entirely, then clone fresh), but its implementation moves from `internal/fabriccli/clone.go:38-49` into `CloneHub`, driven by `CloneOptions.Reset`. Removal happens after the effective warp URL and hub name are resolved and immediately before the existing hub-exists check (`internal/fabricengine/clone.go:98-101`). The CLI stops calling `DeriveWarpName` and `fabricengine.RemoveAll` itself.
- Rationale: `--reset` needs the warp URL for one reason only — to compute the hub name — so once the probe has answered that question, the teardown belongs next to the check it short-circuits. It must keep working in the one-argument form, because once a weft is bound the one-argument form *is* the normal invocation and "re-clone this hub from scratch" is exactly the case `--reset` exists for.
- Rejected: requiring an explicit `<warp-url>` alongside `--reset` (makes the bound form second-class); removing `--reset` (live consumer at `tools/sandbox/SANDBOX-CORE-SUITE.md:211`, and it is pre-existing surface outside this slice).

### reconcile-backfill

- Decision: `Topology.Reconcile` writes `.lyx-warp` into the board worktree when the record is **absent** and the warp side has an `origin` remote, taking the URL from `git remote get-url origin`. `internal/fabriccli`'s reconcile handler drives the `Bolt` commit + push from the result, exactly as the clone handler already does.
- Rationale: nothing has ever written this record, so every hub that exists today — the loomyard hub, the sandbox `lyx-test-HUB`, every operator clone — is unbound. Without backfill the one-argument form fails for all of them and the only migration is a manual two-argument re-clone per hub. Reconcile already runs against a wired worktree that has the warp `origin` sitting right there, so this is a read plus a one-line write. `git remote get-url` is read-only and therefore exempt from the Fabric Git Invariant's mutating-warp-git rule, and it is inside `fabricengine` regardless.
- Decision: the binding check runs **exactly once, after the pair loop**, and is reported at the repo-wide level — never per-pair. `Topology.Reconcile` (`internal/fabricengine/reconcile.go:95-176`) is a per-warp-worktree loop over `List(l.WorktreePath())` and `ReconcileResult` holds nothing today but `Pairs`. The binding is a once-per-hub fact written to `BoardDir`, so running it inside the loop would repeat it N times and leave "which pair owns a repo-wide fact" unanswerable. The warp `origin` URL is read from `l.WorktreePath()` — the worktree reconcile was invoked from — not from a loop entry.
- Decision: `ReconcileResult` gains exactly two repo-wide fields beside `Pairs`: `WarpBinding` (an enum-typed string) and `WarpBindingDetail` (free text — the two URLs on divergence, the error on failure, empty otherwise).
- Decision: **`runReconcile` builds the envelope by hand and must be edited to carry them.** It emits `map[string]any{"pairs": r.Pairs}` today (`internal/fabriccli/fabric.go:458-460`) and never serializes `ReconcileResult` itself, so struct tags on the new fields would have no effect on output. The handler adds `"warp_binding"` unconditionally (it always holds one of the five values) and `"warp_binding_detail"` only when non-empty. `internal/fabriccli/cli_test.go`'s reconcile assertions must be updated for the always-present key.
- Decision: **the engine and the CLI own different halves of `WarpBinding`, and `record_failed` is CLI-only.** `Topology.Reconcile` returns only what it can know from disk: `present` (a record already there and matching), `diverged`, `skipped` (no warp `origin`), `deferred` (dirty board), or `recorded` (it wrote the file into the board worktree). It never returns `record_failed`, because the commit and push that can fail happen CLI-side after `Reconcile` has returned. The handler commits **only** on `recorded`, and pushes on both `recorded` and `present` (see the unpushed-retry decision below); on success it leaves the value alone, and on failure it overwrites `WarpBinding` to `record_failed` and replaces `WarpBindingDetail` with the error. `diverged`, `skipped`, and `deferred` pass through untouched with no git call at all.
- Decision: when the record is **present but differs** from the warp `origin`, reconcile does **not** overwrite it and does **not** hard-error: outcome `diverged`, both URLs in the detail. Rationale: never silently re-point (the same rule clone follows), but reconcile is the repair verb, and hard-erroring would block junction repair on an unrelated URL mismatch.
- Decision: the divergence comparison uses **the same `normalizeWarpURL`** the clone conflict rule uses, so a trailing `/` or `.git` never produces a divergence line. A transport-only difference (recorded `https://…` against an ssh `origin`) *is* still reported, consistent with clone treating a transport swap as differing, but the detail wording says so explicitly — it is advisory information that the record does not describe the transport in use, not a fault. Rationale: two different notions of "same URL" in one module is exactly the inconsistency that produces a divergence line on every reconcile forever with no way to tell whether it matters.
- Decision: **the handler pushes on `present` as well as on `recorded`; it commits only on `recorded`.** Without this, a push that fails after a successful local commit is never retried: the next `Reconcile` sees the record on disk, returns `present`, and a commit-only-on-`recorded` handler skips it forever — exactly the "record local-only indefinitely" state this section rejects as an alternative. Pushing on `present` costs nothing when there is nothing to push: `Bolt.Push` → `pushWeftAt` → `gitrepo.PushCoalesced` (`internal/gitrepo/push.go:111-126`) checks `HasUnpushed` — a purely local `git rev-list --count @{u}..HEAD` — and returns nil without contacting the remote when HEAD is in sync. A push failure on the `present` path yields `record_failed` with a detail saying a previously-committed record could not be pushed. Caveat to carry into the plan: `HasUnpushed` treats *no configured upstream* as unpushed, so a board worktree with no upstream would attempt a network push on every reconcile; the board branch tracks `origin/<default>` from the clone, so this is not the normal case, but the test fixture must configure an upstream or expect the attempt.
- Decision: **the engine refuses to write the record when the board worktree is dirty**, returning outcome `deferred` with a detail naming the condition. `Bolt.Commit` is stage-all — `commitWeftAt` → `gitrepo.StageAllAndCommit` (`internal/fabricengine/bolt.go:23`, `internal/fabricengine/weftgit.go:314-318`) — which is safe at clone time because the board was created seconds earlier, but at reconcile time the board is long-lived and may carry unrelated uncommitted content that a backfill commit would sweep up and push. The check is a read-only `git status --porcelain` at `BoardDir` before writing, so no half-written state is ever left behind. Rationale for putting it in the engine rather than the handler: the engine is what writes the file, and refusing to write is strictly better than writing and then discovering the commit cannot be made safely. Rejected: widening `Bolt` with a scoped-pathspec commit — `Bolt` is deliberately a narrow stage-all handle for the board, and a one-off backfill is not the reason to change that contract.
- Decision: the `WarpBinding` value set is therefore `recorded` / `present` / `diverged` / `skipped` / `deferred` / `record_failed`. `skipped` means no warp `origin`; `deferred` means a dirty board.
- Decision: a failed backfill **commit or push is non-fatal**: outcome `record_failed`, the error in the detail, reconcile still succeeds with an unchanged exit code. This mirrors the precedent already in the same function — `wireBoardLink`'s failure is appended as a Detail note and explicitly may never downgrade a reconcile verdict (`internal/fabricengine/reconcile.go:156-167`). `runReconcile` (`internal/fabriccli/fabric.go:436-461`) is entirely local today, and making the first post-upgrade reconcile of every hub fail on a network problem would break offline reconcile for a fact the next reconcile can persist just as well.
- Decision: when the warp side has no `origin` remote (a `lyxtest` synthetic hub, a locally-initialised warp), backfill is silently skipped: outcome `skipped`, empty detail. Rationale: an absent remote is a legitimate state, not an error condition for reconcile.
- Rejected: reconcile committing via `Bolt` itself (fewer moving parts, but makes the engine silently push to the weft remote); write-but-never-commit (leaves an uncommitted file loose in `_board` indefinitely); commit-locally-never-push (offline-safe, but leaves the record local-only indefinitely, so another machine's one-argument clone still fails); a fatal push failure (the operator learns immediately, at the cost of every offline reconcile on an unbound hub).

### unwire-and-other-verbs

- Decision: `unwire` leaves `.lyx-warp` on `weft:main` untouched, exactly as it already leaves `.lyx-anchor` and the repo-wide `fabric.yaml`. No other fabric verb reads or writes the record.
- Rationale: those three are the repo-wide records that let a later `lyx fabric reconcile` re-wire a hub with no re-clone; the binding is a fourth member of that set. Post-slice-9 `unwire` reverses wiring only and never deletes weft-side content, so this requires no code change — only that the plan does not add one.

## Technical context

**The two packages.**
`internal/fabricengine` is the git/geometry engine; `internal/fabriccli` is the cobra layer.
`fabricengine` must never import `internal/configsync` (there is a documented `fabricengine → configsync → configreg → fabricengine` cycle), which is why `CloneHub` returns a `CloneResult` describing geometry and the CLI drives config materialization, the weft:main commit, and junction wiring from those fields — see the `CloneResult` doc comment at `internal/fabricengine/clone.go:36-49`.
Keep that split: the probe, the binding read/write, and the effective-URL resolution are all engine-side; the `Bolt` commit stays CLI-side.

**`CloneHub`'s current step order** (`internal/fabricengine/clone.go:85-244`), which the probe slots in front of:

1. Derive the warp repo name from the warp URL (`DeriveWarpName`).
2. Compute `hubPath = <cwd>/<name>-HUB`.
3. Error if the hub already exists (this is where `--reset`'s teardown lands).
4. `MkdirAll` the hub, then `<hub>/.lyx`.
5. Clone warp to `<Hub>/<name>`; install the post-checkout hook (non-fatal).
6. Clone weft to `<Hub>/<name>-weft`.
   6b. `suffixWeftPrimaryBranch` renames the freshly-cloned branch to its `-weft` pairing and returns the pre-rename warp branch name.
7. `ensureBoardWorktree` materializes `<Hub>/_board` as a second weft worktree on that branch.
8. Resolve the lyx-anchor adopt-or-create, guard against a stale `.fabric-anchor`, write `.lyx-anchor` to disk.
9. `lyxcwd.Resolve(primeCwd)`, `wireBoardLink`, build and return `CloneResult`.

Everything from step 1 onward is unchanged in shape; the probe becomes a step 0 that yields the effective warp URL feeding step 1, and step 8 gains a sibling write of `.lyx-warp` on the absent-record path.
Any failure from step 5 onward still routes through `teardownHub`.

**The `.lyx-anchor` precedent to copy.**
`internal/lyxcwd/anchor.go:32-41` declares `AnchorFileName = ".lyx-anchor"` with a comment explaining that it is a structural geometry artifact, never a config/env override.
`readRecordedAnchor` (`anchor.go:100-111`) is the read shape: `os.ReadFile`, `strings.TrimSpace`, empty-after-trim treated as absent.
`internal/fabricengine/clone.go:189-217` is the write shape.
Note that `clone.go:29-34` keeps a `staleFabricAnchorName = ".fabric-anchor"` constant purely to hard-error on a pre-rename marker; the new binding has no such legacy and needs no equivalent.

**Call sites to update for the signature and argument flip:**

- `internal/fabriccli/clone.go` — `runCloneWithReset`: usage string (line 33), argument parsing (lines 32-36), the `--reset` block (lines 38-49) which is deleted, and the `CloneHub` call (line 51).
- `internal/fabriccli/fabric.go` — line 57 (stale comment), 60 (`Use`), 68 (`<warp-name>` explanation), 95-96 (both examples); add the two-form explanation to `Long`.
- `internal/fabricengine/clone.go` — `CloneHub` itself, plus its doc comment's phase list.
- `internal/fabricengine/doc.go:358` — the package doc's clone-does-everything paragraph.
- `tools/sandbox/main.go` — **two** call sites, both warp-first, both flipping: `cloneRun` at line 48 (`exec.Command(lyxPath, "fabric", "clone", warpURL, weftURL)`, the shared `lyx-test-HUB` launcher) and `fabricCloneRun` at line 67 (the dedicated fabric hub). Enumeration rule for the plan: **grep every string-literal `"fabric", "clone"` occurrence** — these are `exec.Command` argument lists, so an argument-order flip is completely invisible to the compiler and the inventory is the only safety net. Missing `cloneRun` would point the core sandbox launcher at `Knatte18/lyx-test` as a weft, which is exactly the old-order footgun above, fired automatically.
- Test files calling `CloneHub` — 13 invocations, re-derived by grep and verified in review round 2: `internal/fabricengine/clone_adopt_test.go` (11), `internal/fabricengine/clone_test.go:123` (1, unqualified in-package call), `internal/fabricengine/boardjunction_integration_test.go:38` (1). `internal/configcli/configcli_integration_test.go` and `internal/fabricengine/add_rollback_adopt_test.go` mention `CloneHub` in comments only and call nothing — do not touch them.
- `internal/fabriccli/cli_test.go` — `TestRunCLI_CloneRequiresExactlyTwoArgs` (lines 404-440) needs rewriting for the new arity (one or two args valid, zero and three invalid), plus the two end-to-end clone tests at lines 501 and 584.

**Docs that must change in the same commit** (per CLAUDE.md's task-completion rule and the Documentation Lifecycle):

- `manifest/designs/fabric-unified-view.md` — slice 10 section (lines 149-169): mark shipped, correct `.fabric-anchor` → `.lyx-anchor`, record the warp-URL-only divergence from "warp URL + `--subpath`", record the hub-name collision as a known limitation. **The two example lines at 158-159 already show the correct weft-first order and stay as they are** — it is line 163's prose claim ("this file's own examples above, which still show today's order and must be corrected in the same commit as the flip") that is inverted and must be deleted. Also lines 146-147, inside the **slice 9** section: "Sequenced before slice 10 (not parallel) … slice 10 is still pending and still collides on `runCloneWithReset`" is falsified by this task and must be updated in the same commit. The doc's header says it is deleted once slice 10 *and* slice 6's open half are both done — slice 6's half is still open, so the file survives; fold the durable behaviour into `internal/fabricengine/doc.go`'s package comment.
- `CONSTRAINTS.md:161` — the Fabric Vocabulary Invariant's example `lyx fabric clone <warp-url> <weft-url>` flips to `lyx fabric clone <weft-url> [<warp-url>]`.
- `CONSTRAINTS.md:31` — drive-by: the Cwd Resolution Invariant still says `.fabric-anchor`, stale since the rename. Fix it while in the file.
- `docs/overview.md` — lines 150 and 254 mention clone; check whether either spells the argument order and update if so.
- `docs/sandbox-hub.md:63` — spells the full command warp-first; flip it.
- `tools/sandbox/SANDBOX-CORE-SUITE.md:205,211` — the `--subpath` scenario spells the order; flip it.
- `tools/sandbox/SANDBOX-FABRIC-SUITE.md` — flip any spelled order and add the new one-argument scenario (see Testing).
- `manifest/roadmap.md:21` — slice 10 moves to completed. (This is a planned-item completion, which is exactly what the roadmap moves for.)

**Gotcha — `tools/sandbox/SANDBOX-CORE-SUITE.md:211` is already stale** for an unrelated reason: it claims `unwire` "clears the weft-side `_lyx` content", which slice 9 changed to always-preserve.
That is not this task's bug. Do not fix it as part of this change; it belongs to whoever audits the sandbox docs.

**Vocabulary.** `.lyx-warp` and the identifiers around it name warp explicitly. That is fine: `internal/fabricengine` and `internal/fabriccli` are both in the Fabric Vocabulary Invariant's owner set. The record must never be read from `internal/lyxcwd`, which is not.

## Constraints

From `CONSTRAINTS.md`:

- **Cwd Resolution Invariant** — the binding is not cwd resolution and must not enter `internal/lyxcwd`. `internal/lyxcwd`'s import cap (stdlib + `internal/gitexec`) and its ban on exposing weft/junction/per-module paths both stand. `root` means the git worktree root and `cwd` means the current working directory; never name one for the other.
- **Fabric Git Invariant** — every mutating git operation goes through `internal/fabricengine`. The probe clone is a `fabricengine`-internal `gitexec.RunGit` call, which is where it belongs. `git remote get-url origin` and `git show` are read-only and exempt, but they live in `fabricengine` anyway.
- **Fabric Vocabulary Invariant** — machine-checked by `internal/lyxcwd/enforcement_test.go`'s `TestEnforcement_FabricVocabulary` over production `.go` under `internal/` and `cmd/`, plus `internal/**/*.md`. `fabricengine`/`fabriccli` are owners for the warp/weft vocabulary; the `host` ban applies to them too. `tools/` and `sandbox/` are outside the walk — vocabulary there is a review obligation.
- **CLI / Cobra Invariant** — `Short` stays non-empty; the `Long` gains concrete examples of both forms; all errors go through the `internal/output` envelope, no bare plain-text paths. Help accuracy is a review obligation, and this change alters observable behaviour, so every affected `Short`/`Long` must be re-read. `cmd/lyx/helptree_test.go` and `drift_test.go` will exercise the new `Use` string.
- **Test Tier Purity Invariant** — anything that spawns git (the probe, every clone test) must live in a file whose first non-empty line is a `//go:build` constraint mentioning `integration`. Untagged files may not name `gitexec.RunGit` or `exec.Command` even in a comment or string literal.
- **Hermetic Git Test Environment Invariant** — any git-spawning test package needs a `TestMain` calling `lyxtest.HermeticGitEnv()`. `internal/fabricengine` and `internal/fabriccli` already have one (`testmain_test.go`), and no new package gains a git-spawning test, so this needs no new work — only that any new test file lands in one of those two packages.
- **Never Force-Add Invariant** — no `git add -f` anywhere near the new record.
- **Lyxdirs Single-Declarer Invariant** — do not write the `_lyx`/`.lyx` literals in path construction. The probe directory name `.lyx-clone-probe-*` is a `MkdirTemp` prefix, not a `_lyx`/`.lyx` path token, and `TestEnforcement_GeometryLiterals` matches geometry tokens by exact equality, so the prefix is safe as written — confirmed in review round 1, no rename needed.
- **Sandbox Suite Coverage** — `fabric` is already a covered module; the new scenario adds depth, not a new coverage row.
- **Documentation Lifecycle** — see the docs list under Technical context.

From `CLAUDE.md`:

- Docs land in the same commit as the code.
- Markdown uses semantic line breaks, never fixed-column hard-wrap.
- This is a task worktree, so never push to `main` from here.

## Testing

TDD candidates — the genuinely unit-testable, git-free cores, which should be written test-first:

1. **`normalizeWarpURL`** — table test over: bare URL unchanged; one trailing `/` stripped; one trailing `.git` stripped; both together; scheme and host lowercased while the path keeps its case; scp-form left as-is (and therefore not equal to the https spelling); empty string.
   **A local filesystem path must be covered too** — every integration fixture supplies one (`filepath.ToSlash(warpSrc)`, e.g. `internal/fabricengine/clone_test.go:123`), so `/home/u/fixtures/bare` and a Windows `C:/Code/repo` are the common case, not an exotic one. The case-lowering step applies only to a recognised `<scheme>://<host>` prefix; a path with no such prefix is left byte-identical, drive letter included. The trailing-`/` and trailing-`.git` strips still apply to it, since a bare fixture path can carry either.
2. **The effective-URL resolver** — a pure function over `(recorded string, found bool, supplied string)` returning `(effective string, writeRecord bool, err error)`. Table test covering all five rows of the conflict-rule table, plus the normalization-equal row and the transport-swap row. This is where the whole conflict rule lives, so it should be provable without git.

Integration tests (`//go:build integration`, against local bare fixture repos, in `internal/fabricengine`):

- Bootstrap: two-argument clone against a weft with no record writes `.lyx-warp` at the board root with the supplied URL, and the file is committed on `weft:main`.
- Derive: a one-argument clone against a weft whose `main` carries a record clones the recorded warp and produces the same `CloneResult` geometry as the two-argument form.
- Match: two-argument clone against a matching record is a no-op — the record's bytes are unchanged and the clone succeeds.
- Normalized match: the supplied URL differs only by a trailing `.git` and is still treated as matching.
- Conflict: two-argument clone against a differing record hard-errors, the error names both URLs, **and no hub directory is left behind** (this is the property the probe buys — assert on the filesystem, not just the error).
- Unbound: one-argument clone against a weft with no record errors with the message naming the two-argument form, and creates no hub.
- Probe taxonomy — empty weft remote: a one-argument clone against an empty weft remote (unborn HEAD) reports unbound, not a git error; a two-argument clone against the same remote bootstraps successfully and writes the record, preserving today's orphan-create path.
- Old-order guard — the regression test that matters most: a two-argument clone with the **pre-change** order (a real source repo as the first positional) hard-errors, creates no hub, and — asserted explicitly — leaves the repo passed as the weft candidate with no new commit and no new ref. This is the test that proves the footgun is closed.
- Old-order guard — `.lyx-anchor`-bearing weft: a weft whose HEAD carries `.lyx-anchor` but no `.lyx-warp` (a pre-binding hub) passes the guard and bootstraps.
- Old-order guard — `--force-bootstrap` lets a weft candidate with an ordinary non-empty history through, and is the only way to do so.
- Probe taxonomy — unreachable weft: a clone against a nonexistent weft URL hard-errors with git's stderr behind the `probe weft <url>:` prefix, in **both** argument forms, and never reports "unbound" and never bootstraps.
- Clone-time backfill: a weft carrying `.lyx-anchor` but no `.lyx-warp` (a pre-binding hub) plus an explicit warp URL writes the record.
- `--reset` in both argument forms tears down an existing hub and re-clones.
- Reconcile backfill: an unbound wired hub gains the record after `lyx fabric reconcile`, committed and pushed, with `WarpBinding == "recorded"` reported once at the repo-wide level and nothing on any pair result.
- Reconcile backfill runs once on a multi-worktree hub: with several warp worktrees present, the record is written once and `WarpBinding` appears exactly once.
- Reconcile normalization: a record differing from the warp `origin` only by a trailing `.git` reports `present`, not `diverged`.
- Reconcile divergence: a genuinely differing record is left untouched, reports `diverged` with both URLs in the detail, and reconcile still succeeds with an unchanged exit code.
- Reconcile backfill failure: an unreachable weft remote yields `record_failed` with the error in the detail, and reconcile still succeeds — the assertion is on the exit code, mirroring the `wireBoardLink` precedent.
- Reconcile with no warp `origin` reports `skipped` and succeeds.
- Reconcile with a dirty board worktree reports `deferred`, writes nothing, and succeeds — assert the board's unrelated uncommitted file is neither committed nor pushed.
- Reconcile unpushed-retry: after a backfill whose push failed, a second reconcile against a now-reachable remote pushes the already-committed record and reports `present`, with no second commit created.
- Probe absence discriminator: a weft whose HEAD exists but lacks `.lyx-warp` is classified absent (not a git error), and a weft whose HEAD object is unreadable is a hard error — the two must be distinguishable, which is the whole point of the `ls-tree` probe.
- Unwire leaves `.lyx-warp` in place.

CLI-level (`internal/fabriccli/cli_test.go`):

- Arity: zero args and three args are usage errors; one and two args are accepted. This replaces `TestRunCLI_CloneRequiresExactlyTwoArgs` (lines 404-440), whose name and table both encode the old rule.
- The two existing end-to-end clone tests (lines 501, 584) flip to weft-first and keep asserting the same wiring outcomes.
- The unbound-weft error text is asserted through the `output.Err` envelope.

Whole-suite obligations:

- `cmd/lyx/helptree_test.go`, `drift_test.go`, `registration_test.go`, `longlist_test.go` must pass with the new `Use`/`Long`.
- `internal/lyxcwd/enforcement_test.go`'s `TestEnforcement_FabricVocabulary` and `TestEnforcement_GeometryLiterals` must pass with the new identifiers and the new `.md` prose.
- `cmd/lyx/tierpurity_test.go` must pass — no git spawning from an untagged file.
- `cmd/lyx/sandbox_coverage_test.go` must still pass after the suite-doc edits.

Sandbox (black-box, `tools/sandbox/SANDBOX-FABRIC-SUITE.md`):

- A new scenario covering the one-argument bound clone: after the dedicated fabric hub has been cloned once with both URLs (which writes the binding), delete the hub and re-clone with `lyx fabric clone <weft-url>` alone, confirming the warp is derived and the hub comes up identically wired. Tag it `**Covers:** fabric`.

## Q&A log

- **Q:** How does clone learn the warp URL before the hub directory exists — probe clone, clone-then-rename, or two separate code paths? **A:** Probe. The operator asked whether exactly the one small file could be fetched from the weft remote rather than a whole repo; a `--depth 1 --filter=tree:0 --no-checkout` partial clone plus `git show HEAD:.lyx-warp` is precisely that, and git offers nothing that avoids a local scratch directory entirely (`git archive --remote` is not enabled on GitHub).
- **Q:** Does the probe stay minimal everywhere? **A:** No, and this was stated rather than glossed: against local bare-repo fixtures — every test in the suite — git ignores `--filter`/`--depth` and does an ordinary hardlinked clone. Correct, just not minimal. The implementation must not treat the resulting warnings as failures.
- **Q:** Does the record hold the subpath as the design doc says? **A:** No — warp URL only. `.lyx-anchor` stays the single source of truth for the subpath, and the design doc's "warp URL + `--subpath`" is corrected in this commit.
- **Q:** Is "matching URL" byte-exact? **A:** No: normalize away a trailing `/` and a trailing `.git` and lowercase scheme+host. But a transport swap (scp-form vs https for the same repo) counts as differing and hard-errors, because accepting it would leave the record describing something other than what was cloned.
- **Q:** Why must `--reset` work when no warp URL is given, and what does it even do? **A:** It removes `<cwd>/<name>-HUB` and clones fresh — the blow-it-away-and-redo path, rarely needed. It needs the warp URL solely to compute the hub name, which the probe now answers first. It must keep working one-argument because once a weft is bound, one-argument is the normal invocation. The operator noted it is nearly the same as deleting by hand; it is kept because `tools/sandbox/SANDBOX-CORE-SUITE.md:211` uses it and removing pre-existing CLI surface is a separate decision.
- **Q:** Why is a reconcile backfill needed at all? **A:** Because nothing has ever written this record, so every hub in existence — including the one this task is being written in — is unbound. Without backfill the one-argument form fails everywhere and the only migration is a manual two-argument re-clone per hub.
- **Q:** What happens when a record is present but disagrees with the warp `origin` at reconcile time? **A:** Reported, never overwritten, never fatal. Same never-silently-re-point rule as clone, but reconcile is the repair verb and must not be blocked by an unrelated URL mismatch. (Decided during write-up, not asked — flagged here as such.)
- **Q:** `CloneHub`'s signature — options struct or reordered positionals? **A:** Delegated to the assistant; options struct chosen, because five positionals with two adjacent optional URLs is the exact shape that produces argument-order bugs, and the argument order is what this task is flipping.
- **Q:** Should the hub-naming collision between two wefts of the same warp be fixed here? **A:** No. Hub naming stays warp-derived; the collision is pre-existing and gets recorded as a known limitation in the design doc.
- **Q:** _(review round 1 gap)_ Which probe git failures mean "no record" and which are hard errors? **A:** Explicit two-condition taxonomy — absent means a missing path in a valid commit, or an unborn HEAD (empty weft remote, which the orphan-create path supports today). Every other git failure is a hard error with stderr verbatim. Costs one extra `rev-parse`, and is what stops a network outage from being reported as `unbound weft`.
- **Q:** _(review round 1 gap)_ Where does the reconcile binding check live, given `Reconcile` is a per-worktree loop with no repo-wide phase? **A:** Once, after the pair loop, reported on two new repo-wide `ReconcileResult` fields; nothing per-pair. The `origin` URL comes from `l.WorktreePath()`, not a loop entry.
- **Q:** _(review round 1 gap)_ Is a failed backfill commit/push fatal to reconcile? **A:** Non-fatal — outcome `record_failed`, error in the detail, exit code unchanged. Mirrors `wireBoardLink`'s existing rule in the same function that a convenience repair may never downgrade a reconcile verdict, and keeps offline reconcile working.
- **Q:** _(review round 2 gap)_ How do the new reconcile fields reach the CLI output, given `runReconcile` hand-builds `{"pairs": …}` and never serializes `ReconcileResult`? **A:** The handler is edited to add `"warp_binding"` unconditionally and `"warp_binding_detail"` when non-empty; struct tags alone would do nothing.
- **Q:** _(review round 2 gap)_ Who sets `record_failed`, given the engine cannot know a push failed? **A:** The CLI. The engine returns only `present`/`diverged`/`skipped`/`recorded` from what it did on disk; the handler attempts the `Bolt` commit + push only on `recorded` and overwrites to `record_failed` with the error on failure.
- **Q:** _(review round 3 gap)_ What stops the pre-change `clone <warp-url> <weft-url>` — which still has valid arity after the flip — from cloning the user's real repo as a weft and pushing lyx artefacts to its default branch? **A:** A pre-bootstrap guard: bootstrap requires the weft candidate to have an unborn HEAD or carry `.lyx-anchor`, otherwise it hard-errors naming the argument-order change. A new `--force-bootstrap` flag is the documented escape for the one legitimate false positive, a fresh weft remote with an auto-generated README. Rejected: declaring it an accepted hazard documented in `Long` — the blast radius is a push to someone's real default branch.
- **Q:** _(review round 4 gap)_ How is "record absent" told apart from "git failed", when both make `git show` exit nonzero? **A:** With `git ls-tree HEAD --name-only -- .lyx-warp` as an explicit presence probe (exit 0 + empty stdout means absent); `git show` runs only after presence is confirmed, so any failure from it is genuinely an error. The old-order guard's `.lyx-anchor` check uses the same discriminator.
- **Q:** _(review round 4 gap)_ What retries a backfill that committed locally but failed to push? **A:** The handler pushes on `present` as well as `recorded`. `PushCoalesced` short-circuits locally via `HasUnpushed` when there is nothing to send, so this is free in the common case and self-healing in the failure case.
- **Q:** _(review round 4 gap)_ `Bolt.Commit` is stage-all — what stops a reconcile backfill from sweeping up unrelated uncommitted board content and pushing it? **A:** The engine checks `git status --porcelain` at `BoardDir` before writing and returns `deferred` on a dirty board, so nothing is written and no commit is attempted. Rejected: widening `Bolt` with a scoped commit, which would change a deliberately narrow contract for a one-off.
- **Q:** Does the sandbox suite exercise the new one-argument form? **A:** Yes — the sandbox call site flips to weft-first two-argument, and a new `SANDBOX-FABRIC-SUITE.md` scenario covers the one-argument bound clone.
