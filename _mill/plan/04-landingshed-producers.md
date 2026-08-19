# Batch: landingshed producers

```yaml
task: 'landing: Publish + Finalize producers'
batch: 'landingshed producers'
number: 4
cards: 11
verify: go test ./internal/landingshed/... ./internal/configreg/... ./cmd/lyx/...
depends-on: [2, 3]
```

## Batch Scope

`internal/landingshed` — the two `ShedProducer` implementations, their told-value `Deps`, their strict `landing.yaml` config surface, and the stuck-reason machinery that makes two different stuck causes distinguishable despite the producer seam having no reason channel.
It is one batch because `Publish` and `Finalize` share `Deps`, the config, the cancellation helpers, and the stuck-reason writer; splitting them would mean building that shared half twice or handing a half-built package between batches.
The three registration edits (`configreg`, its test's want list, and the strict-loader pinned set) ride along because the config they register is created here and each guard is set-equality — an omission fails the build rather than degrading quietly.

It depends on batch 2 for the owner/repo parser `Publish` calls and on batch 3 for the resolver both producers drive.

The external interfaces batch 5 consumes are `landingshed.Deps`, `NewPublish(Deps)`, and `NewFinalize(Deps)`.

Batch-local decisions beyond `## Shared Decisions`:

- **Nobody fills the two Fabric opener closures in this task, deliberately.** The list constructor has no production caller anywhere in the tree today, there is no loom CLI, and this task adds no command. The closures are declared here, passed through in batch 5, and filled by the next roadmap item. This batch's tests fill them directly.
- **The producers return a bare stuck verdict, always.** A reason is carried by a structured warning log line and a one-line reason file, never by the returned verdict and never by an error — returning an error would flip the run's persisted state from blocked, which a human resumes, to failed, which ends the run.

## Cards

### Card 20: package documentation and told-value Deps

- **Context:**
  - `internal/loomshed/loomshed.go`
  - `internal/loomshed/doc.go`
  - `internal/fabricengine/merge.go`
  - `internal/fabricengine/fabric.go`
  - `internal/mergeresolve/deps.go`
  - `internal/mergeresolve/doc.go`
  - `internal/perchcli/cli.go`
  - `internal/perchcli/wiring.go`
  - `internal/modelspec/registry.go`
- **Edits:** none
- **Creates:**
  - `internal/landingshed/doc.go`
  - `internal/landingshed/deps.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create the package documentation and its told-value surface.

  `internal/landingshed/doc.go` carries the `Package landingshed` godoc: the package owns two general producers that any producer list may name — never special-cased by the engine that drives them — takes told absolute paths, and has no direct production import of `internal/lyxcwd`.
  It describes **one repo** throughout, per the machine-enforced vocabulary walk over every identifier, string literal, and comment in this package.

  `internal/landingshed/deps.go` declares `Deps` with every told value and nothing derived:

  - `WorktreeRoot`, `TaskBranch`, `ParentBranch`, `WebsterDir`, `StencilsDir` — plain told strings;
  - `ScratchDir` — the told absolute scratch directory, which is the only thing this package would otherwise need an anchor path for. There is deliberately **no** anchor-path field: carrying both would be a derived near-duplicate, and deriving the scratch path is doubly forbidden here, since it would name a reserved directory literal this package may not declare and compute geometry this package may not compute;
  - `OriginURL` — the told remote URL string, read by the caller;
  - `PushSkipped bool` — the told skip decision, so the producer can refuse rather than silently produce a pull request for an unpushed branch;
  - `PushBranch func() error` — the injected push closure. The push verb's own name carries a token this package may not write in any identifier, so the layer that names it is the caller and this package only calls the closure;
  - `OpenFabric func() (*fabricengine.Fabric, error)` and `OpenParentFabric func() (*fabricengine.Fabric, error)` — the two lazy opener closures. Laziness is required, not stylistic: the constructor they wrap stat-checks the paired layout, so opening eagerly would fail before the run's own preflight has confirmed anything is wired;
  - `Shuttle mergeresolve.Shuttle` — the session-runner seam, told exactly the way every existing session-driving constructor in this tree takes its own. The resolver's constructor rejects a nil value for it, so without this field neither producer could build a resolver at all, and the conflict session could never spawn;
  - `Registry modelspec.Registry` and `Config Config` — the resolved model-spec registry and the loaded configuration.

  Also declare in this file the narrow one-method resolver seam both producers hold their resolver behind: an unexported interface with the single method `Resolve(ctx context.Context, source string) (mergeresolve.Result, error)`, plus a compile-time assertion that the concrete resolver type satisfies it.
  It is unexported deliberately: production has exactly one way to obtain a resolver — the constructors build it from the told values — and the seam exists so this package's own in-package tests can substitute a fake without a second public construction path anyone outside could reach for by mistake.

  Every field's doc comment states it is told and derived by nobody here.
  Document explicitly, on the two opener fields, that nothing in this task fills them: the resolution chain (list the worktrees, match the entry whose branch equals the parent branch, resolve that path, open it) belongs to the layer that legitimately resolves geometry, and the next roadmap item builds it.
- **Commit:** `feat(landingshed): add package documentation and told-value Deps`

### Card 21: landing.yaml config surface

- **Context:**
  - `internal/loomengine/config.go`
  - `internal/loomengine/configtemplate.go`
  - `internal/loomengine/template.yaml`
  - `internal/configengine/config.go`
  - `internal/modelspec/parse.go`
- **Edits:** none
- **Creates:**
  - `internal/landingshed/config.go`
  - `internal/landingshed/configtemplate.go`
  - `internal/landingshed/template.yaml`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Give this package its own strict config surface, following `loomengine`'s shape exactly.

  `internal/landingshed/template.yaml` carries four keys, each with a trailing comment in the same style as the sibling module template:

  - `require_pr_to_base` — a **list** of base branch names, defaulting to a single entry, `main`. It is a list rather than a bool because tasks branch off other tasks in this workflow, and a bool would force a pull request on every intermediate task-to-task merge;
  - `squash` — defaulting to `true`, because a run produces one commit per card and an ordinary merge floods the parent with implementation noise;
  - `conflict` — the model-spec string for the conflict-resolution session;
  - `conflict_timeout_min` — the conflict session's wall-clock budget in minutes.

  `internal/landingshed/configtemplate.go` embeds it with the `//go:embed template.yaml` directive and exposes `ConfigTemplate() string`, mirroring the sibling module's embed-and-accessor file exactly.

  `internal/landingshed/config.go` declares `Config` with the four matching yaml-tagged fields and `LoadConfig(baseDir, module string) (Config, error)`, which calls `configengine.Load` — the **strict** entry point, where an absent config file is an error.
  Strict is required by the membership rule rather than chosen: neither producer has a standalone entry point, both are reachable only inside a hub, and there an absent config means the hub is broken.
  It rewraps the not-initialized case and validates the `conflict` key's model-spec grammar at load time with `modelspec.Parse`, so a typo fails loudly at load rather than hours into a run when the session first spawns.
- **Commit:** `feat(landingshed): add the strict landing.yaml config surface`

### Card 22: cancellation helpers and the stuck-reason carriers

- **Context:**
  - `internal/loomshed/ctx.go`
  - `internal/shedadapters/ctx.go`
  - `internal/shedadapters/singlellm.go`
  - `internal/shedengine/producer.go`
  - `internal/shedengine/run.go`
  - `internal/landingshed/deps.go`
- **Edits:** none
- **Creates:**
  - `internal/landingshed/ctx.go`
  - `internal/landingshed/stuck.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/landingshed/ctx.go` with this package's own unexported `entryErr` and `cancelErr`, shaped exactly like `internal/loomshed/ctx.go`'s pair and naming this package in their message prefix.

  Create `internal/landingshed/stuck.go` declaring the one helper both producers route every stuck verdict through: it takes the producer name, the reason text, and the told scratch directory, emits a structured warning through the shared logger carrying at minimum a producer field and a reason field alongside the case's own fields, and writes a one-line reason file at `<ScratchDir>/<producer>-stuck.md`, overwritten each attempt.
  It creates the scratch directory with `os.MkdirAll` first, on every write path — a told directory may not exist yet, and making a told path usable is legal where deriving one would not be.
  A write failure is logged rather than swallowed, and never replaces the producer's own verdict: failing to record why something is stuck must not change whether it is stuck.

  Document why both carriers exist rather than one returned string: the producer contract returns only a verdict, an output pointer, and an error, the engine persists a fixed reason string of its own for every stuck verdict regardless of what the producer knows, and the output pointer is never persisted anywhere.
  The log line scrolls away in an unattended run, which is why the file exists; the file is also what the tests assert two stuck causes are distinguishable against, since the engine's own persisted reason is identical in all of them.
- **Commit:** `feat(landingshed): add cancellation helpers and stuck-reason carriers`

### Card 23: Publish

- **Context:**
  - `internal/landingshed/deps.go`
  - `internal/landingshed/config.go`
  - `internal/landingshed/ctx.go`
  - `internal/landingshed/stuck.go`
  - `internal/mergeresolve/deps.go`
  - `internal/mergeresolve/mergeresolve.go`
  - `internal/websterengine/summary.go`
  - `internal/githubclient/parseownerrepo.go`
  - `internal/githubclient/githubclient.go`
  - `internal/selfreportengine/selfreport.go`
  - `internal/shedengine/producer.go`
  - `internal/gitrepo/push.go`
- **Edits:** none
- **Creates:**
  - `internal/landingshed/publish.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/landingshed/publish.go` declaring the `Publish` producer type, `NewPublish(deps Deps) (*Publish, error)`, and its `Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error)` method, with a compile-time assertion that the type satisfies the producer seam.

  `NewPublish` rejects a nil required closure up front with a distinct error naming the field, rather than nil-panicking at call time.

  It builds the resolver **at construction time**, calling `mergeresolve.New` with the told values — the merge surface obtained from the pair-opener closure's own lazily-opened handle, the told session-runner seam, and the told worktree root, scratch directory, stencils directory, model-spec string, registry, and timeout — and stores the result behind the package's own resolver seam.
  Construction time is correct here and is not in tension with the laziness rule the two pair openers carry: that rule exists because the pair constructor stat-checks a layout that may not be wired yet, whereas the resolver's own constructor performs no I/O at all and only rejects nil or empty told values.
  Building it eagerly therefore turns a mis-wired resolver into a construction error, which is exactly where it belongs.
  A construction failure is returned rather than deferred.

  Declare the package-level client seam `var NewGitHubClient = githubclient.New`, exactly as `internal/selfreportengine/selfreport.go` does, so tests can swap it without touching production wiring.
  All authentication goes through that package; this package never builds its own credential path and never invokes the GitHub CLI.

  `Call`'s flow:

  1. `entryErr` first.
  2. If the told parent branch matches no entry in the configured base-branch list → return done immediately: no merge-in, no push, no GitHub call whatsoever.
  3. If the push-skipped flag is set → the stuck path with its own reason, **before** the push closure and before any GitHub call. Relying on the push layer's own skip gating would produce a pull request for an unpushed branch, which is the exact failure the gate exists to prevent. Note that the flag is irrelevant on the no-pull-request branch, because that branch already returned at step 2.
  4. Run the resolver's merge-in against the told parent branch. A stuck result becomes this producer's stuck verdict, carrying the resolver's own reason text.
  5. Call the push closure. This step is mandatory and its position is load-bearing: agents commit per fix and never push, so without it the task branch exists only locally, the create call fails, and the resume query could never match either. A rejected push (the distinguishable rejection sentinel) is its own stuck reason, distinct from a generic push failure; any other push error is stuck too, with the error surfaced. No pull request is attempted in either case.
  6. Resolve owner and repo by calling `githubclient.ParseOwnerRepo` on the told origin URL. An absent, unparseable, or non-GitHub URL is stuck with a distinct reason — never a silent no-pull-request done verdict, which is the one outcome the gate exists to prevent.
  7. Obtain a client through the seam and query for an existing pull request with the task branch as head and the told parent branch as base, under a bounded context timeout so a stalled connection cannot hang an unattended run. Discriminate the three error classes the reference consumer does: an unresolvable token, an API rejection, and everything else.
  8. Branch on what the query found:
     - none → create the pull request with title and body taken verbatim from the parsed summary artifact (`ParseSummary`'s title and body, no model call of any kind), then return **stuck**. Stuck rather than done, because a done verdict would let the run advance to the next row and merge to the parent seconds after opening the pull request, defeating it entirely;
     - open → stuck again, with no second pull request created and no second merge-in. The push at step 5 still ran, so a resumed call refreshes the pull request with any commits added since;
     - closed and merged → done, and the run proceeds;
     - closed and not merged → stuck with a reason distinguishable from the open case. A pull request closed without merging is a human decision to stop, and must never read as proceed.
  9. Every non-success exit consults `cancelErr` first and routes its reason through the stuck helper.

  Document that only the externally visible branch is pushed, without naming either internal side: the pull request is an artifact of the repository the remote service can see, and the other side's remote state belongs to the merge step and the engine's own sync path.
- **Commit:** `feat(landingshed): implement the Publish producer`

### Card 24: Finalize

- **Context:**
  - `internal/landingshed/deps.go`
  - `internal/landingshed/config.go`
  - `internal/landingshed/ctx.go`
  - `internal/landingshed/stuck.go`
  - `internal/mergeresolve/deps.go`
  - `internal/mergeresolve/mergeresolve.go`
  - `internal/fabricengine/merge.go`
  - `internal/fabricengine/mergeerrors.go`
  - `internal/fabricengine/mergeguards.go`
  - `internal/shedengine/producer.go`
- **Edits:** none
- **Creates:**
  - `internal/landingshed/finalize.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/landingshed/finalize.go` declaring the `Finalize` producer type, `NewFinalize(deps Deps) (*Finalize, error)`, and its `Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error)`, with a compile-time assertion that the type satisfies the producer seam.

  `NewFinalize` rejects a nil parent-pair opener closure up front, along with every other nil required closure, each with its own distinct error.
  It builds its resolver at construction time from the told values and stores it behind the package's own resolver seam, exactly as the sibling producer's constructor does and for the same reason — the resolver's constructor performs no I/O, so a mis-wired resolver is a construction error rather than a first-call surprise.
  A construction failure is returned rather than deferred.

  `Call`'s flow:

  1. `entryErr` first.
  2. Run the resolver's merge-in against the told parent branch, from the task worktree. A stuck result becomes this producer's stuck verdict carrying the resolver's reason.
  3. Obtain the parent pair's handle through the injected parent opener closure. An error there means no live pair exists for the parent branch → stuck with a reason naming that branch. This producer never creates a worktree to merge into; materializing a pair is a separate command's job and a human's decision.
  4. Call the parent handle's merge verb with the task branch and the squash flag threaded from configuration.
  5. On the merge-in-required error, re-run the resolver in the task worktree and retry the parent-side merge exactly once; a second failure is stuck. One retry rather than zero because no lock spans the window between this producer's own catch-up merge-in and the later parent-side merge, so a competing task can genuinely land in the parent in between — real drift, not an impossible state.
  6. On a guard error carrying a dirty-worktree reason, surface that reason verbatim and return stuck. Never stash, never reset, never force the merge: someone has uncommitted work in the parent and only they can decide what happens to it.
  7. Any other error from the parent-side merge is stuck with the error surfaced.
  8. Every non-success exit consults `cancelErr` first and routes its reason through the stuck helper.

  Record in this file's own documentation the two durable facts the deleted design document currently carries, since they must survive its removal:

  - what this producer's merge critical section is and that a future regeneration step folds into it rather than becoming a separate row — no hook, no injectable interface, and no scaffolded span with an empty body is built for that today, because an interface with a permanently-nil implementation is exactly the hypothetical-requirement design this codebase avoids;
  - that a merged pull request does not make this producer redundant: the remote service only ever sees one of the two sides, so the other side's branch was never in the pull request at all, and after a remote-side merge the visible side reports already-up-to-date while the other genuinely merges.

  Describe both in single-repository terms, naming neither internal side.
- **Commit:** `feat(landingshed): implement the Finalize producer`

### Card 25: told-geometry seam enforcement

- **Context:**
  - `internal/loomshed/seam_enforcement_test.go`
  - `internal/landingshed/deps.go`
  - `internal/landingshed/publish.go`
  - `internal/landingshed/finalize.go`
  - `internal/landingshed/config.go`
  - `internal/landingshed/stuck.go`
  - `internal/landingshed/ctx.go`
- **Edits:** none
- **Creates:**
  - `internal/landingshed/seam_enforcement_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/landingshed/seam_enforcement_test.go`, modelled directly on `internal/loomshed/seam_enforcement_test.go`: same walk, same imports-only parse, same stdlib rule, same allowlist-membership shape.
  Name the test `TestToldGeometryInvariant_AllowlistOnly` and the map `landingshedAllowedImports`, holding exactly the non-stdlib import paths this package's production files use and nothing more.
  Its comment states, as the model does, why a membership list beats a denylist and that a transitive reach through an allowlisted dependency is explicitly fine.
- **Commit:** `test(landingshed): enforce the Told-Geometry import allowlist`

### Card 26: Publish under test

- **Context:**
  - `internal/landingshed/publish.go`
  - `internal/landingshed/deps.go`
  - `internal/landingshed/stuck.go`
  - `internal/landingshed/config.go`
  - `internal/selfreportengine/selfreport_test.go`
  - `internal/githubclient/githubclient.go`
  - `internal/websterengine/summary.go`
  - `internal/gitrepo/push.go`
- **Edits:** none
- **Creates:**
  - `internal/landingshed/publish_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Write the unit tier for `Publish` against a faked resolver, a faked push closure recording its call order, and a faked GitHub client swapped in through the package-level seam — exactly the way the reference consumer's own test does it, pointing the client at a local test server rather than the real service.
  No test contacts a real service or a real model.
  These tests live in the package itself rather than an external test package, which is what lets them substitute the unexported resolver seam directly; on that path the told session-runner value is never driven, since the resolver's own behaviour is covered by its own tier in batch 3.
  Assert as part of this tier that the constructor rejects a `Deps` whose told session-runner seam is nil, with a distinct error naming that field — a resolver that cannot spawn a session is a construction error, not a first-call surprise.

  Cases:

  - parent branch not in the configured base list → done, with no merge-in and no GitHub call whatsoever;
  - parent branch in the list with no existing pull request → merge-in runs, the pull request is created with title and body byte-identical to the parsed summary, verdict stuck;
  - re-called with an open pull request → stuck, and the create call never happens;
  - re-called with a closed and merged pull request → done;
  - re-called with a closed and unmerged pull request → stuck, with reason-file contents distinguishable from the open case;
  - an unresolvable token → surfaced distinctly from a network failure;
  - origin URL absent, unparseable, or non-GitHub → stuck with a distinct reason, and no pull request attempted;
  - the push runs before the existing-pull-request query and before the create call — assert the recorded ordering, not merely that the push happened;
  - the push fails → stuck, and the create call never happens;
  - the push is rejected by the remote → stuck, with reason-file text distinct from a generic push failure;
  - the push-skipped flag set with a pull request required → stuck before the push closure and before any GitHub call;
  - the push-skipped flag set with no pull request required → plain done, since that branch returns before reaching the push;
  - a missing or malformed summary artifact → fails loudly with no pull request created;
  - the scratch directory absent on disk → the first stuck write creates it rather than failing;
  - every stuck case writes its reason file, and two different stuck causes produce different file contents;
  - cancellation at entry surfaces as a non-nil error, never as a stuck verdict.

  These files stay untagged and must spawn nothing — no git, no subprocess — which the injected closures and the local test server together make possible.
- **Commit:** `test(landingshed): cover Publish's full verdict table`

### Card 27: Finalize under test

- **Context:**
  - `internal/landingshed/finalize.go`
  - `internal/landingshed/deps.go`
  - `internal/landingshed/stuck.go`
  - `internal/landingshed/config.go`
  - `internal/fabricengine/mergeerrors.go`
  - `internal/fabricengine/merge.go`
- **Edits:** none
- **Creates:**
  - `internal/landingshed/finalize_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Write the unit tier for `Finalize` against a faked resolver and a faked parent-pair opener closure returning a scripted merge outcome.
  This tier lives in the package itself too, for the same reason as its sibling: the resolver seam it substitutes is unexported.

  Cases:

  - the happy path → merge-in always runs first, then the parent-side merge, with the squash flag threaded from configuration;
  - the merge-in-required error on the parent-side merge → exactly one resolver re-run and exactly one merge retry, then stuck;
  - the parent opener returning an error → stuck with a reason naming the parent branch, and no worktree created;
  - a guard error carrying the dirty-worktree reason → stuck surfacing that reason, with no stash, no reset, and no retry;
  - an unrecognized merge error → stuck with the error surfaced;
  - each stuck case writes its reason file, and two different causes produce different contents;
  - cancellation at entry surfaces as a non-nil error, never as a stuck verdict.

  A nil parent opener is a construction error rather than a silent no-op — assert the constructor rejects it.
- **Commit:** `test(landingshed): cover Finalize's merge geometry and refusals`

### Card 28: config loading under test

- **Context:**
  - `internal/landingshed/config.go`
  - `internal/landingshed/configtemplate.go`
  - `internal/landingshed/template.yaml`
  - `internal/loomengine/config_test.go`
  - `internal/configengine/config.go`
- **Edits:** none
- **Creates:**
  - `internal/landingshed/config_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Cover the config surface: an absent config errors rather than degrading, which is the whole point of adopting the strict loader; the template's four keys round-trip into the struct with the documented defaults, including the single-entry base-branch list and the squash default; and a malformed `conflict` model-spec is rejected at load time rather than at first use.
  Model the fixture setup on the sibling module's own config test.
- **Commit:** `test(landingshed): cover strict config loading and load-time validation`

### Card 29: register landing in the config module registry

- **Context:**
  - `internal/landingshed/configtemplate.go`
- **Edits:**
  - `internal/configreg/configreg.go`
  - `internal/configreg/configreg_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Register the new module so the config reconcile pass can see its file at all — without this entry the reconcile step never materializes or refreshes `landing.yaml`.
  In `internal/configreg/configreg.go`, add `{Name: "landing", Template: landingshed.ConfigTemplate},` to the slice `Modules` returns, together with the matching import.
  The slice's order is alphabetical and every caller-visible surface renders it that way, so the new entry goes between the `fabric` and `loom` rows — a misordered entry is user-visible.
  Do not set the seed-only flag on it: this module's key set is closed and operator-owned key sets are what that flag is for.
  In `internal/configreg/configreg_test.go`, add `"landing"` to the expected-names list at the same alphabetical position.
- **Commit:** `feat(configreg): register the landing config module`

### Card 30: pin landingshed on the strict side of the Config Strictness Invariant

- **Context:**
  - `internal/landingshed/config.go`
- **Edits:**
  - `cmd/lyx/configstrictness_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add `"internal/landingshed": true,` to `configStrictnessStrictSet` in `cmd/lyx/configstrictness_test.go`.
  The guard is set-equality in both directions, so a package that calls the strict loader without a pinned entry fails the build, and this batch introduces exactly such a call site.
  Leave `configStrictnessDegradingSet` untouched — this module has no standalone entry point, so a config-less invocation is not a supported mode for it.
- **Commit:** `test(cmd/lyx): pin landingshed on the strict config-loading set`

## Batch Tests

`verify:` runs three packages' fast tiers, all untagged.
Every test this batch writes spawns nothing: the resolver, the push, the two pair openers, and the GitHub client are all injected seams, and the client is pointed at a local test server, so the whole batch stays in tier 1 per the Test Tier Purity Invariant.

- `./internal/landingshed/...` runs both producers' verdict tables (cards 26 and 27), the config tier (card 28), and the import allowlist guard (card 25).
- `./internal/configreg/...` runs the registry's own want-list check, which card 29 satisfies on both sides at once.
- `./cmd/lyx/...` runs the two set-equality guards this batch must satisfy: the strict-loader pinned set (card 30) and the GitHub shell-out guard, which must stay green now that a second package talks to the service through the authenticated client.

The real two-worktree behaviour of both producers is covered by the integration tier in batch 5, against real pairs rather than fakes; that tier is deliberately not in this batch's verify scope, since none of its files exist yet.
