# Orchestrator review — discussion.md

Reviewed against `main` (unchanged in this worktree — only `_mill/discussion.md`/`status.md` exist so far).

## Citation check

This is piece 4 of the Shed-recipe initiative — the first real consumer of `shedrecipe`/`shedbuild`/`shedcheck`, and the discussion cites deeply into all three plus `loomshed`, `loomcli`, and `landingshed`. Verified every concrete signature, quote, and line citation.

| Claim | Status |
|---|---|
| `internal/loomshed.New()` builds a thirteen-row literal at `loomshed.go:137-151` | Correct (verified in an earlier review this session; row constants confirmed again at `loomshed.go:23-37`) |
| `shed-recipe.md` banner: "an early concept sketch, not a settled design … do not implement piece 4 from this doc as written" | Correct, exact verbatim — line 4 |
| `shedrecipe.websterEntry` calls `requireSeam("Webster", "WebsterRun", env.WebsterRun)` and rejects nil | Correct, exact — `entries_simple.go:157-159` |
| `preflightEntry` already does exactly `preflightshed.NewPreflight(name, env.Cwd)` | Correct, exact — `entries_simple.go:18-26`, matches `requireAbsRoot` call and constructor call verbatim |
| `landingshed.NewPublish` rejects nil `OpenFabric`/`PushBranch` | Correct, exact — `publish.go:60-66` |
| `landingshed/deps.go:73-76` names the gap, says the resolution chain "belongs to the layer that legitimately resolves geometry, and the next roadmap item builds it" | Correct, exact verbatim — `deps.go:73,75` |
| `internal/shedbuild/equivalence_test.go`'s header calls itself a stand-in, says "the conversion item sequenced after this task depends on this fixture staying in sync" | Correct, exact verbatim — file header |
| `internal/shedrecipe/coverage_guard_test.go` lives in `package shedrecipe` and imports `loomshed` | Correct — confirms the cited import-cycle argument (a `shedrecipe`-package test cannot also import `shedbuild`, which itself imports `shedrecipe`) |
| `internal/loomshed/seam_enforcement_test.go`'s allowlist currently includes `shedadapters`, `websterengine`, `landingshed` | Correct, exact — lines 31, 32, 37 |
| `contracts/stencils/stencils.go` is a production Go embed package outside `internal/` | Correct — matches the file's own doc comment describing exactly that role |
| `internal/loomcli/wiring.go:88-110` — the `c.deps = loomshed.Deps{…}` literal | **Off by one.** The literal opens at line 88 (correct) but its closing brace is at line 109, not 110 — a one-line miss, immaterial to the argument. |
| `internal/loomcli/drive.go:45` — `loomshed.New(c.deps)` | Correct, exact |
| `internal/loomcli/run.go:100` — `loomshed.Seed(c.deps.StatusPath, c.deps.StatusLockPath, …)` | Correct, exact |
| `internal/loomcli/drive.go:44` — "also stats `c.deps.StatusPath` before building" | **Off by four.** The `os.Stat(c.deps.StatusPath)` call is at line 40, not 44. Same citation class as prior reviews' near-miss line numbers, slightly larger this time; the surrounding argument (that this second read site also needs repointing) is correct regardless of the exact line. |
| `internal/loomcli/cli.go:41-43` — the `deps` field's type and doc comment | Correct, close enough — the field itself is at line 43 with its doc comment starting two lines above |

Two line-number misses (`wiring.go` off by one, `drive.go` off by four), both non-blocking — the underlying claims (that these call sites exist and must change) are correct in every case, only the exact line pointer drifted. This is a larger discussion than most reviewed in this initiative (thirteen file/line citations into five different packages plus two doc quotes), and the miss rate is still low relative to that surface.

## Design read

**The consumer-package decision (`internal/loomrecipe` above `loomshed`, not inside it) is derived from a real compile-time constraint, not a style preference.** `internal/shedrecipe/registry.go`'s registry already imports `loomshed` for six of the twelve constructors (confirmed: `entries_simple.go` constructs `preflightshed`/`landingshed` producers, and the registry package doc states the four-package span). A `loomshed → shedbuild → shedrecipe → loomshed` cycle is not a judgment call to avoid, it is a Go compiler error — the discussion correctly treats this as settling the question rather than merely favoring it, and the two rejected alternatives (inline in `loomcli`, or hollow out `loomshed`'s six constructors) are both real options it explains away with concrete costs rather than dismissing by assertion.

**The recipe-location decision (embedded under `contracts/`, no seeding, no operator edit path) is the discussion's most consequential judgment call, and it is argued from the *nature of the artifact* rather than from precedent alone.** The core distinction — a producer graph is a structural definition of what loom *is*, versus a stencil being a prompt a human legitimately tunes — is a real, checkable difference in blast radius: a bad stencil edit produces a worse review, a bad recipe edit produces a silently mis-routed run or a validation failure with no operator warning path. Rejecting the Stencil Ownership Invariant's machinery here isn't just "different enough" hand-waving — it correctly notes that invariant exists specifically to let a human retune a *prompt*, a job this artifact doesn't have. The accepted consequence — `shedbuild.Load` gets no production caller — is stated plainly rather than glossed over, with an explicit instruction not to manufacture a call just to justify the function's existence; that's a level of honesty about a decision's own loose end that's easy to skip.

**The `landing-parity` decision is the discussion's best piece of restraint.** It would have been easy to notice, while converting, that `Publish`/`Finalize` construction already fails in production for want of `Env.Landing`/`Deps.Landing`, and quietly "fix" it as a drive-by improvement. Instead the discussion names the existing failure explicitly, cites its own prior documentation of the gap (`landingshed/deps.go:73-76`, verified verbatim above), and states outright that fixing it here would "dominate the task and obscure whether the conversion itself is correct" — then puts a concrete implementer obligation in place (every test that builds the real list must supply `Landing` test doubles) so the parity claim is actually exercised, not just asserted.

**The `env-webster-run` decision catches a real, silent regression a naive port would introduce**, and traces it to its exact source: `wiring.go` today leaves `WebsterRun` nil relying on `shedadapters.NewWebsterProducer`'s own nil-defaulting, but `websterEntry` (verified above) calls `requireSeam` and errors on nil — so the same "leave it nil" pattern that works today would break the build. Correctly rejects "relax the entry" as scope creep into a package this task only consumes.

**The test-ownership reasoning for moving `coverage_guard_test.go` is grounded in an import-cycle fact I independently confirmed**, not just architectural taste: the file lives in `package shedrecipe` (confirmed) and would need to import `shedbuild` to test the recipe-built list — but `shedbuild` already imports `shedrecipe`, so that import would not compile in place. The discussion also flags its own real weakening honestly: the moved guard must now tolerate three registered-but-unused engines (`SingleLLM`, `Bouncer`, `BurlerRound`) rather than treating any registry surplus as an error, and says this explicitly rather than letting a reader discover the guard got laxer by diffing the test.

**Scope discipline holds against a specific, concrete temptation this task is unusually well-positioned to fall into.** Because this is the loader/registry/checker's first real caller, any genuine defect found in one of those three packages would be trivially easy to "just fix inline" — the discussion forecloses this explicitly ("This task is those packages' first consumer, not their reviser ... a genuine defect found in one of them is a finding to report, not a licence to widen scope"), which matters more here than in a typical scope-discipline statement because this task is structurally the first place such a defect would actually surface.

One thing worth flagging for the plan stage, not a defect: the discussion notes the recipe's content is "already written" — `internal/shedbuild/testdata/loom-recipe.yaml` — and instructs copying it over with only the header comment changed. That file is a test fixture proven equivalent to `loomshed.New`'s current output by `equivalence_test.go`, so this isn't a shortcut around verification (the loomrecipe-side shape-assertion test re-proves the same claim against the production copy) — but it's worth the plan stage double-checking that fixture is still byte-for-byte in sync with `loomshed.go`'s live literal at implementation time, since some time has passed since the fixture and the literal were last compared.

## Verdict

Sound. Nothing here should block moving to Plan. Two citation line-numbers to fix if convenient — `internal/loomcli/wiring.go`'s literal closes at line 109, not 110, and `drive.go`'s `os.Stat(c.deps.StatusPath)` call is at line 40, not 44 — neither affects any decision's soundness, both are pointer-only misses on claims that are otherwise correct.
