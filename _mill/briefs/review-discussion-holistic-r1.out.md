MILL_REVIEW_BEGIN
# Review: lyxtest: build real fabric hubs, invert the lyxtest/fabric dependency

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: Claude (Anthropic) — Opus-class model, exact build unverifiable from inside the session
reviewed_file: _mill/discussion.md
date: 2026-08-12
```

## Findings

### [BLOCKING:consistency] CopyWarpHub disposition contradicts its narrowing
**Section:** "Measured blast radius" table, `fabricengine` row vs "Migrate all 154 above-fabric sites"
**Issue:** The row disposes of `fabricengine`'s in-package uses as "primitives stay on `gitkit`", but `CopyWarpHub` is counted among those primitives (5×, verified in-package: `warplayout_test.go` 1, `hook_test.go` 4) while the Decision and the already-rewritten CONSTRAINTS narrow it to `gitrepo`/`lyxcwd` only — the pinned-caller guard test would fail on exactly those files.
**Fix:** State explicitly that `CopyWarpHub` is a hub-shaped fixture, not a primitive, so all 5 in-package sites move to `package fabricengine_test` + `hubforge`, leaving only `MustRun`/`SeedConfig`/`HermeticGitEnv` as "stays on gitkit".

### [BLOCKING:scope] Documentation sweep enumerates Go files only
**Section:** "Files naming the old packages by path"; Scope/In
**Issue:** Seven markdown files outside the listed scope name `lyxtest`/`fabrictest`: `docs/benchmarks/fixture-copy.md` (13 refs incl. a Reproducing section), `docs/benchmarks/running-tests.md`, `docs/benchmarks/test-suite-timing.md`, `docs/shared-libs/lyxcwd.md`, `manifest/designs/fabric-unified-view.md`, `crucible/review-prompt-template.md` (names the retired "lyxtest Leaf" invariant), and `manifest/roadmap.md` — whose line 22 link `[designs/lyxtest-real-hubs.md](designs/lyxtest-real-hubs.md)` breaks the machine-checked Markdown Link Integrity invariant once the design doc is deleted.
**Fix:** Extend the section to the markdown surface, and resolve the roadmap hedge ("moves only if this is a planned roadmap item") — it is Planned item 1, so the Done move plus link repair is in scope.

### [BLOCKING:decision] `internal/lyxtest/bench_test.go` has no stated disposition
**Section:** "Where the code is today" / Testing
**Issue:** The discussion accounts for `lyxtest_test.go` and `reexecguard_test.go` but never for `bench_test.go`, whose `BenchmarkCopyPaired`/`BenchmarkCopyPairedLocal`/`BenchmarkCopyPairedParallel` call three helpers this task deletes and are the "permanent probes" `docs/benchmarks/fixture-copy.md` documents.
**Fix:** State whether the benchmarks are deleted, retargeted onto `hubforge.NewHub`/the bare-copy step, and what happens to `fixture-copy.md`'s Reproducing section.

### [BLOCKING:design] `Copy*` enumeration is internally inconsistent
**Section:** "`Copy*` call sites, measured 2026-08-12"
**Issue:** The table does not reconcile: `CopyPaired` column entries sum to 56 (stated 57), `CopyWeft` to 50 (stated 51), row totals to 162, column totals to 165, against a stated 163; and the `cmd/lyx` row's 2 sites do not exist — `cmd/lyx` contains only comment mentions (`tierpurity_test.go:50-51`, `boardguard_test.go:50`), which the paragraph above the table says were already excluded. Since 154, the migration batching and the +3.4 s estimate all derive from this count, the counting method is unreliable as stated.
**Fix:** Re-derive the table from one stated, reproducible method (call expressions, not matching lines) and make row/column/grand totals agree before the plan consumes them.

### [NIT:decision] Surviving primitive fixture's new name undecided
**Section:** "Migrate all 154 above-fabric sites"
**Issue:** `CopyWarpHub` is to be "renamed to something that does not advertise itself as a hub", but no name is chosen, while the gitkit guard test and the doc comment both need it.
**Fix:** Pick the name in the discussion.

### [NIT:consistency] `CloneAndWire` described imprecisely
**Section:** "Reusable pieces"
**Issue:** The signature is `CloneAndWire(cwd string, opts fabricengine.CloneOptions) (fabricengine.CloneResult, error)` — first parameter is `cwd`, not `container`, and the result also carries `BoardDir`, `WeftBase`, `WarpURL`, `WarpBindingRecorded` and an embedded `MutationRecord`, not just the three named fields.
**Fix:** Quote the real signature and note that `hubforge` accessors may want `BoardDir`/`WeftBase` too.

## Verdict

REQUEST_CHANGES
Four blockers: contradictory CopyWarpHub disposition, incomplete doc sweep with a link break, undisposed benchmarks, unreliable call-site count.
MILL_REVIEW_END
