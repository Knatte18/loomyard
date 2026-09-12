**If you find issues, REPORT them — do NOT fix them.**

You are an independent code reviewer for **self-report Tier 1: Go-detected structural anomalies**.
You evaluate the complete implementation (every batch) against the approved plan and produce a structured review.

Reviewer model: **sonnethigh**.
Round **1**.

**You MAY use Read, Grep, and Glob to verify claims against source files.**
**CRITICAL: The one exception beyond that is Write -- use it exactly once, to write your full report to the file named in this brief's output-contract footer.**
**CRITICAL: Do NOT use Edit, or run git/bash.**
**CRITICAL: Review-only. Do NOT suggest modifications. Findings only.**
**CRITICAL: Do NOT read `reviews/`. Evaluate fresh each round.**

## Prior non-blocking items

The following items were judged non-blocking in a prior round.
Do NOT escalate any of them to BLOCKING unless NEW information justifies it -- a new diff, a real reproducible failure, or a concrete in-repo convention.
If you escalate, you MUST state the new information explicitly.

Prefer the convention already used by analogous code in the provided source files over a stricter alternative.

(none)

## Constraints
# Constraints

Short, authoritative list of the repo's structural invariants — the FORM code must take.
Guides planning and review. Not a test-coverage index: a new constraint may get its own enforcing test in the same change, but which tests exist today is not tracked here.

## Cwd Resolution Invariant

`internal/lyxcwd` owns cwd resolution alone — never weft, a junction path, or any per-module subdirectory.

- `root` = worktree/repo root; `cwd` = current working directory. Never conflate.
- `Resolve` requires cwd to be a git worktree root and to equal `Join(worktreeRoot, AnchorRel)`.
- Exposes only `RepoName`, `HubPath`, `WorktreeName`, `AnchorRel`, `WorktreePath()`, `AnchorPath()`.
- All cwd/worktree-root queries go through `lyxcwd.Getwd()`/`Resolve()`; raw `os.Getwd`/`git rev-parse --show-toplevel` banned elsewhere.
- A module's own durable subdirectory is its own constant joined onto `AnchorPath()` — never a `lyxcwd` call.
- Imports: stdlib + `internal/gitexec` only.

## Told-Geometry Invariant

An engine is handed the absolute paths it operates on and derives none of its own — no direct import of `internal/lyxcwd`.

- Three tiers: `lyxcwd.Resolve` → `preflight.Check` (fabric wired/synced/clean) → `loomengine.CheckSeed`.
- A producer needs none of the tiers; an orchestrator needs tier 3; a standalone CLI probes tier 1 via `preflight.ResolveMode` only.
- `internal/hubgeom`/`internal/standalonegeom` are the only `Geometry`-struct constructors.
- Bound packages: `internal/tokenvocab`, `pattern`, `buildinfo`, `standalonestate`, `shedengine`, `treadleengine`, `loomshed`, `landingshed`, `mergeresolve`, `shedrecipe`, `shedbuild`, `loomrecipe`, `planparser`, `planglyph`, `configengine`, `shuttleengine`, `reedengine`, `burlerengine`, `websterengine`, `cliwire`.
- A `shuttleengine` runner whose anchor is deliberately outside its worktree root is constructed only through `shuttleengine.NewDetachedRunner`, only from a standalone CLI's own wiring, and `NewRunner`'s containment assertion is never relaxed to accommodate it.

## Cliwire Sole-Wiring Invariant

`internal/cliwire` is the sole owner of standalone/hub CLI wiring resolution for the standalone-capable CLIs.

- A `<module>cli` never re-implements `--target-dir` resolution, the repository-root lift, mode-derived state/plan/stencils resolution, the nested-geometry guard, or the durable-sink redirect;
  it declares its own `cliwire.Module` descriptor and calls in.
- `internal/cliwire` is the only **production** caller of `standalonestate.Derive`, while test files may call it to build fixtures and to assert the real derivation.
- Both halves are enforced by tests in `internal/cliwire`.

## Lyxdirs Single-Declarer Invariant

`internal/lyxdirs` is the sole declarer of `_lyx` (`LyxDirName`) and `.lyx` (`DotLyxDirName`).

- No other production file names either literal in path-construction context.

## Durable-vs-Ephemeral State Invariant

Every never-tracked file lives under `.lyx`, at the mirrored subpath of the `_lyx` content it relates to. `_lyx` holds tracked content only.

- Siblings under `AnchorPath()` (hub: `BoardDir(hub)`; standalone: `standalonestate.Derive`).
- No engine derives its own `.lyx` path — each module exposes a scratch accessor beside its durable one.
- Structural (`fabricengine.structuralCommittedDirs`/`structuralNeverCommittedDirs`), never from `fabric.yaml`'s `pathspec`.
- Weft content is per-branch and is never a merge participant in either direction.
- `internal/logger`'s durable trace sink arms its cwd-anchored fallback only inside a worktree lyx owns (`_lyx` present at the anchor); a plain checkout gets no sink rather than a `.lyx` lyx does not own.

## Hub Containment Invariant

No hub-level container is ever junctioned into a worktree. `_board`, `_portals`, `_launchers` are reachable from the hub only.

- `_portals`/`_launchers` links point hub-inward only; a per-worktree link to either is banned.

## gitkit Leaf Invariant

`internal/gitkit` imports only stdlib, `lyxcwd`, `weftname`, `configengine`, `lyxdirs`.

- `gitkit.CopyRepo` is callable from `lyxcwd` alone; everyone else takes a hub from `hubforge`.

## hubforge Fabric-Fixture Invariant

Every hub fixture is built by `internal/hubforge` through `fabriccli.CloneAndWire`. No hub is hand-assembled.

- No package in `fabriccli`'s dependency set may import `hubforge`.

## Modelspec Leaf Invariant

`internal/modelspec` imports only stdlib, `configengine`, `gopkg.in/yaml.v3`. Reverse import never allowed.

## Treadle Runner-Seam Invariant

`internal/treadleengine` never imports `burlerengine` or any `internal/*cli` package.

- Import allowlist: stdlib, `lock`, `logger`, `state`, `stencil`, `stencilstore`, `shuttleengine`, `yaml.v3` — not `lyxcwd` directly.

## Shed Producer-Seam Invariant

`internal/shedengine` imports only stdlib, `state`, `lock`.

- `StatusPath`/`LockPath`/`StatusLockPath` are caller-supplied.

## Shed Recipe Registry Invariant

Every value in `internal/shedrecipe`'s registry constructs a `shedengine.ShedProducer`, via one `map[string]Constructor` reached only through `Lookup`/`Names` — no `init()` self-registration, no runtime `Register`.

- No direct import of `lyxcwd`; every path is told.

## Tokenvocab Leaf Invariant

`internal/tokenvocab` imports only stdlib and `internal/stencil`. Reverse import never allowed.

## Buildinfo Leaf Invariant

`internal/buildinfo` imports nothing at all, not even stdlib. Exposes `Channel`/`IsDev()` only.

## Standalonestate Leaf Invariant

`internal/standalonestate` imports only the standard library. Never resolves a working directory; `Derive` creates nothing on disk.

## Pattern Leaf Invariant

`internal/pattern` imports only stdlib, `lyxdirs`, `stencilstore`, `stencil` — never a feature package. Reverse import never allowed.

## Stencil Ownership Invariant

Every producer prompt is read at call time from a told, absolute stencils directory, never embedded bytes.

- `//go:embed` in `contracts/stencils` is seed defaults only.
- `internal/stencilstore` is sole owner of seeding/hashing/reading/validation. A hash-mismatched file is never overwritten.
- Seed/refresh runs once per process pre-run, never lazily inside `Read`.
  A command that reads no stencils may decline the pass entirely by carrying the skip annotation;
  declining is all-or-nothing per command and never defers seeding to a later or lazier point.

## CLI / Cobra Invariant

Every lyx CLI module is a cobra subtree assembled under one root in `cmd/lyx/main.go`.

- Each module exposes `Command() *cobra.Command` and `RunCLI(out io.Writer, args []string) int`; eleven of twelve also carry `RunCLIIn(cwd, out, args) int`.
- An alias command may delegate into another module's subtree with no seam function of its own.
- Non-empty `Short` on every command.
- Errors are JSON via `internal/output`, one object per line; every `RunE` checks `clihelp.ShouldAbort` first.
- Interactive-handoff exception, narrow and per-command: `reedengine` `attach`/`header --blocking`, `lyx loom status --watch`, `lyx loom run`/`lyx run`.
- Package naming: `<module>cli` imports `<module>engine`; engine never imports cli/cobra. Deviations: `stencilcli` → `internal/stencilstore`; `quarrycli` → `internal/planglyph`.

## Completion Signal Invariant

Every code path in `internal/shuttleengine` that finalizes a NEGATIVE answer to "did this run finish" consults `allOutputFilesExist` over the run's `OutputFiles` first.

- The negative answers are `OutcomeDied`, `OutcomeTimeout`, a mechanism-failure `error`, and `verdictRespawnEligible`.
  The check is reached directly or through the three helpers that own it: `classifyDeadlineExpiry` and `finishedDespiteMechanismFailure` (`wait.go`), `soleFinishedCandidate` (`attach.go`).
- A clock expiring, `reed` losing a strand, `reed.Status` erroring, the events file staying unreadable, `reed.json` being absent or undecodable — each answers "has something gone wrong", never "did this run finish", and the agent's output files are its return value.
- Stated at length in `wait.go`'s own file doc comment, under the same heading;
  six instances were fixed across crucible rounds `opus5-high-r4` (both deadlines), `fable5-high-r5` (both retry-exhausted caps), `fable5-xhigh-r6` (`Attach`'s `dispositionCandidate`) and `opus5-high-r7` (`Attach`'s three reed-state gates).
- Enforcement is a **tripwire, not a completeness proof** (`internal/shuttleengine/completionsignal_enforcement_test.go`): two AST scans pin the audited negative-verdict return sites and the audited `allOutputFilesExist` call sites in `wait.go`/`attach.go`, so adding an exit or deleting a guard fails loudly and forces a human to confirm. Deliberately NOT a `shape.go`-style ledger — that mechanism needs a closed value enum, and these outcomes span three types in two files.

## Shuttle Provider-Seam Invariant

Provider specifics live ONLY under `internal/shuttleengine/claudeengine`.

- `shuttleengine`/`reedengine` never reference Claude specifics; `shuttleengine` never imports `claudeengine`.

## Shell Mechanics Seam

Pane-shell command strings are built ONLY via `internal/shell` (`Quote`/`Invoke`/`ReadFile`, stdlib-only).

## Fabric Vocabulary Invariant

**Fabric** names the wired composite. **warp**/**weft** name the two sides, used only where they must be told apart. "repo" alone never substitutes for warp. **`host` is retired** in the fabric sense, everywhere.

- Owner set (bare weft/warp carve-out): `fabricengine`, `fabriccli`, `weftname`, `gitkit`, `hubforge`, `boardengine`, `configsync`.

## Fabric Git Invariant (warp + weft)

Every git op LYX's own code performs, on either weft or warp, goes through `internal/fabricengine` in Go, in-process — never raw git, never an LLM agent. Binds LYX's own code only.

- Weft-internal git and warp↔weft topology both go through `fabricengine` only; read-only verbs (SHA, `status --porcelain`) exempt.
- The weft commit is Go calling the engine at a round/phase boundary loom controls, never an agent. Agents write into `_lyx` via the junction; Go reads and commits. An agent commits its own code to warp only, never weft.
- **Board carve-out:** `boardengine`'s writes to `weft:main` may fire from any worktree/session, always through `Bolt`.
- Every weft-commit caller passes a positive-only file list via `fabricengine.ScopedPathspec`.
- `structuralNeverCommittedDirs` paths route to a third bucket in `classifyPaths`; `Commit` hard-errors on a non-empty third bucket.
- Junction exclusion is `.git/info/exclude` on both sides, mutated only via `fabricengine.mutateGitExclude`, never a tracked `.gitignore`.
- `Unwire` removes warp junctions/exclude entries only — weft-side `_lyx`/`.lyx` content always preserved.

## Fabric Destruction Chokepoint Invariant

`internal/fabricengine/destroy.go` is the only file in `package fabricengine` permitted a destructive primitive.

- Every executor checks, in order, stopping at first failure: containment, ownership, dirtiness, force.
- `--force` answers dirtiness only, never containment or ownership.
- A gate refusal is never silently discarded.
- The `rec *Mutations` recorder is threaded into `destroy.go` only.

## Fabric Write-Side Containment Invariant

A `fabricengine` write to a hub-level structural container (`_launchers/…`, `_portals/…`) routes through an `os.Root` rooted at the hub — never a raw `os.MkdirAll`/`os.WriteFile`/`fslink`.

## Mutation Record Invariant

Every mutating fabric verb accumulates a `*Mutations` record; every mutating result type exposes it under a fixed envelope key set.

- An executor appends its primitive only after it observably changed state.
- Every mutating result type embeds `MutationRecord`; a read-only one must not.
- Envelope: `mutations` always an array, `partial` always a bool. Pre-flight failures emit a bare `output.Err` with neither key.

## Markdown Link Integrity

Every inline markdown link in a `.md` file under `manifest/` or `docs/` resolves — file part and `#anchor`.

- `manifest/`/`docs/` name scan sources only, not valid targets.
- Allowlist keyed by `(file, target)`, each entry naming its owning task.

## Review Round Invariant

One review+fix round: review written to disk before any target file is touched; every finding fixed, all severities; no self-grading; commit-per-fix on warp source, never push.

## Live-Substrate Spawn Observability

Every code path reachable from a `lyx` command that starts a real OS process logs its spawn via `internal/logger`, and logs its teardown wherever it waits for one — `Info` for a lifecycle spawn, `Debug` for a spawn inside a polling probe.

- A detached spawn (`Start` with no `Wait`) logs the spawn alone; there is no teardown to observe.
- A site structurally barred from importing `internal/logger` is exempt, and carries a written reason wherever the exemption is recorded.
- Test-fixture machinery, standalone harnesses, and dev tooling under `tools/` are outside this rule, not exemptions to it.
- Never re-exec `os.Executable()` under `go test`.
- A retry loop around a real spawn caps attempt COUNT, not only elapsed time.

## Sandbox Suite Coverage

Every registered lyx module is exercised by the sandbox suite or explicitly excluded with a reason.

## Test Tier Purity Invariant

Untagged test files perform no expensive spawns; Tier 1 stays offline and fast.

- No `gitexec.Run`/`RunGit`, `exec.Command`/`CommandContext`, `gitkit.Copy*`, `hubforge.NewHub` outside `integration`/`smoke`-tagged files.
- `time.Sleep(...)` ≥ 1s in an untagged file is flagged unless allowlisted.

## Hermetic Git Test Environment Invariant

Every test package whose tests spawn git runs under the hermetic git test environment.

- `TestMain` calls `gitkit.HermeticGitEnv()` before `m.Run()`, or is allowlisted (`internal/proc`).

## Dev/Prod Binary Separation

Sandbox tooling resolves the dev binary via `resolveLyx` (`.dev-bin` first, then PATH) — never a bare-PATH `lyx` lookup.

## Planparser Sole-Parser Invariant

`internal/planparser` is the SOLE parser and writer of the on-disk plan format (`_lyx/plan/`).

- Consumers read only from the `planparser.Plan` model. `SetApproved` (approval), `RewriteRefs` (ref substitution across the plan), and `AppendAmendment` (the append-only amendment log) are the three write paths — and no others.

## Ref-Shape Registry Invariant

`internal/planparser` is the sole declarer of ref-shape vocabulary — classification (`classifyRef`/`refKind`) and the `plan:` handle grammar (`HandlePrefix` and the exported handle helpers).

- Every ref-shape decision in `internal/planparser` and `internal/planglyph` routes through the kind-policy ledger in `internal/planparser/shape.go` or the exported handle vocabulary.
- Named enforcement: the `refKind` enum↔`allRefKinds` sync meta-test, the `refGate` constants↔`ledger` keys sync meta-test, the ledger completeness meta-test, and the AST-based boundary-enforcement scans with per-scan package-qualified exempt sets (`{internal/planparser/classify.go, internal/planparser/shape.go}` for the `refKind` scan, plus `internal/planparser/handle.go` for the `plan:`-op scan; no planglyph file is exempt).
- The two sync meta-tests are separate obligations and both parse a const block from the AST, because Go cannot reflect over constants: ledger completeness ranges `ledger`'s own keys and therefore cannot see a gate missing from it entirely.

## Glyph Conversion Chokepoint Invariant

loomyard performs no glyph↔path conversion of its own.

- `glyph.Self` is the only path→glyph call. `Glyph.UnitPath` is the only glyph→path call. `glyph.Parse` plus `Glyph.String` are the only glyph grammar.
- Forbidden: a `#`-trimming suffix operation over a glyph-typed value, reading `Glyph.Unit` as a disk path, and a local regex over a glyph string.

## Quarry CGO Requirement Invariant

`lyx` is a cgo binary: `github.com/Knatte18/quarry`'s engine links tree-sitter's C grammars, and its own `internal/cgoguard` deliberately fails the build outright under `CGO_ENABLED=0` — a hard requirement this module cannot relax from this side, since quarry lives outside this worktree.

- Every build of this module needs `CGO_ENABLED=1` and a C compiler on `PATH` (gcc/clang on POSIX, mingw-w64 on Windows). `CGO_ENABLED` already defaults to `1` for a native build when a compiler is on `PATH`.
- `tools/deploy/main.go`'s build command sets `CGO_ENABLED=1` explicitly, so a deploy from a cgo-disabled environment fails at the compiler rather than shipping a broken binary.
- `cmd/lyx/crosscompile_test.go`'s `TestCrossCompileLinux` builds for `GOOS=linux`/`GOARCH=amd64` under `CGO_ENABLED=1`, never `=0` — this module is no longer a static, cgo-free cross-compile target. On a host that is not natively linux/amd64, the gate needs a genuine linux/amd64 C cross-toolchain (signalled by `CC` being set) and skips rather than fails when one is not configured.

## Discussionparser Sole-Parser Invariant

`internal/discussionparser` is the sole reader of `_lyx/discussion/`'s on-disk format. Imports the standard library only.

## Summaryparser Sole-Parser Invariant

In production code, `internal/summaryparser` is the sole declarer of the final-summary artifact's filename and the sole parser of its format. Imports the standard library only.

## Gate Self-Check Parity Invariant

A mechanical gate's `ShedProducer` row and its CLI self-check verb call the same package function for every mode.

- Discussion-Validate ↔ `validate-discussion`: `discussionparser.Validate`. Plan-Validate ↔ `validate-plan`: `planglyph.ValidateFormat`. Plan-Revalidate ↔ `validate-plan --require-approved`: `planglyph.Validate`.
- Adding a mechanical gate means adding its verb and its parity check in the same task.

## Recipe-Format Sole-Parser Invariant

`internal/shedbuild` is the SOLE parser of the recipe file format; declares no on-disk location for recipe files.

## Batcher Registry+Config Invariant

webster's execution unit is the batchifier-derived batch, not the raw plan card.

- Batching is selected by `internal/batcher`'s registry plus `batcher.yaml`'s `active:` key — owned by `batcher`, not webster.

## Producer Pointer-Rule Invariant

An instruction file never duplicates or paraphrases another producer's format-contract content — only points at it.

## Config Strictness Invariant

`internal/configengine` offers `Load` (strict) and `LoadOrTemplate` (degrades to embedded template) — a caller adopts exactly one.

- Degrading: `{shuttleengine, reedengine, websterengine, batcher}`. Strict: `{fabricengine, boardengine, loomengine, landingshed}`.
- A template list is a default, not a minimum length.

## GitHub Auth Invariant

All GitHub authentication goes through `internal/githubclient`; no other production package shells out to `gh`.

## gitrepo Client Boundary Invariant

`internal/gitrepo` splits local-vs-remote by client: go-git owns local reads; `gitexec` owns anything remote-authenticating or working-tree-mutating.

## gitexec Checked-Call Invariant

`gitexec.Run`/`runChecked` is the default entry point; the raw forms (`gitexec.RunGit`/`r.run`) survive only at pinned, `//gitexec:raw`-marked call sites.

## Never Force-Add Invariant

Fabric/gitrepo never runs `git add -f`. Transients stay out of the index via each repo's own `.git/info/exclude`.

## Documentation Lifecycle

Which docs are kept vs deleted: see [docs/overview.md#documentation-lifecycle](docs/overview.md#documentation-lifecycle).


## Path roots

Every unprefixed path below is relative to `/home/knatte/Code/loomyard/wts/self-report-tier1`.
- `wiki/` paths are relative to `/home/knatte/Code/loomyard/wiki`

## Files included (N=52)

- _mill/plan/00-overview.md
- _mill/plan/01-leaf-surfaces.md
- _mill/plan/02-anomaly-detector.md
- _mill/plan/03-drive-wiring.md
- internal/selfreportengine/selfreport.go
- internal/selfreportengine/selfreport_test.go
- internal/selfreportcli/cli.go
- internal/shedadapters/ledgeraccess.go
- internal/shedadapters/ledgeraccess_test.go
- internal/loomengine/config.go
- internal/loomengine/loomstatus_test.go
- cmd/lyx/notransients_test.go
- cmd/lyx/constructoranchoring_test.go
- internal/loomengine/template.yaml
- internal/loomengine/config_test.go
- internal/loomengine/anomaly.go
- internal/loomengine/anomaly_test.go
- internal/loomengine/anomalybody.go
- internal/loomengine/anomalybody_test.go
- internal/loomcli/selfreport.go
- internal/loomcli/selfreport_test.go
- internal/loomcli/selfreport_github_test.go
- internal/loomcli/drive.go
- internal/loomcli/smoke_test.go
- internal/loomcli/wiring_test.go
- manifest/designs/self-report-tier1.md
- manifest/roadmap.md
- docs/overview.md
- internal/selfreportcli/cli_test.go
- internal/shedadapters/bouncerfiles.go
- internal/shedadapters/round.go
- internal/shedadapters/burler.go
- internal/lyxdirs/dirs.go
- internal/configengine/config.go
- internal/loomengine/configtemplate.go
- internal/yamlengine/reconcile.go
- internal/loomengine/coherence.go
- internal/loomengine/status.go
- internal/shedengine/status.go
- internal/shedengine/run.go
- internal/shedengine/producer.go
- internal/shedengine/errors.go
- internal/state/state.go
- internal/lock/lock.go
- internal/logger/logger.go
- internal/loomcli/wiring_commitstatus_test.go
- internal/loomengine/seed.go
- internal/loomcli/cli.go
- internal/loomcli/run.go
- internal/loomrecipe/loomrecipe.go
- manifest/designs/loom.md
- manifest/designs/quarry-glyph-plan-alphabet.md

## Plan + source files to review
- Overview: `_mill/plan/00-overview.md`
- Batch file(s):
  - `_mill/plan/01-leaf-surfaces.md`
  - `_mill/plan/02-anomaly-detector.md`
  - `_mill/plan/03-drive-wiring.md`

Read the overview and every batch file above. Then read every source file listed below for full context (includes cross-batch ancestor creates already on disk):
- `internal/selfreportengine/selfreport.go`
- `internal/selfreportengine/selfreport_test.go`
- `internal/selfreportcli/cli.go`
- `internal/shedadapters/ledgeraccess.go`
- `internal/shedadapters/ledgeraccess_test.go`
- `internal/loomengine/config.go`
- `internal/loomengine/loomstatus_test.go`
- `cmd/lyx/notransients_test.go`
- `cmd/lyx/constructoranchoring_test.go`
- `internal/loomengine/template.yaml`
- `internal/loomengine/config_test.go`
- `internal/loomengine/anomaly.go`
- `internal/loomengine/anomaly_test.go`
- `internal/loomengine/anomalybody.go`
- `internal/loomengine/anomalybody_test.go`
- `internal/loomcli/selfreport.go`
- `internal/loomcli/selfreport_test.go`
- `internal/loomcli/selfreport_github_test.go`
- `internal/loomcli/drive.go`
- `internal/loomcli/smoke_test.go`
- `internal/loomcli/wiring_test.go`
- `manifest/designs/self-report-tier1.md`
- `manifest/roadmap.md`
- `docs/overview.md`
- `internal/selfreportcli/cli_test.go`
- `internal/shedadapters/bouncerfiles.go`
- `internal/shedadapters/round.go`
- `internal/shedadapters/burler.go`
- `internal/lyxdirs/dirs.go`
- `internal/configengine/config.go`
- `internal/loomengine/configtemplate.go`
- `internal/yamlengine/reconcile.go`
- `internal/loomengine/coherence.go`
- `internal/loomengine/status.go`
- `internal/shedengine/status.go`
- `internal/shedengine/run.go`
- `internal/shedengine/producer.go`
- `internal/shedengine/errors.go`
- `internal/state/state.go`
- `internal/lock/lock.go`
- `internal/logger/logger.go`
- `internal/loomcli/wiring_commitstatus_test.go`
- `internal/loomengine/seed.go`
- `internal/loomcli/cli.go`
- `internal/loomcli/run.go`
- `internal/loomrecipe/loomrecipe.go`
- `manifest/designs/loom.md`
- `manifest/designs/quarry-glyph-plan-alphabet.md`

Every path listed above is relative to the root stated in the `## Path roots` block above and must be resolved against it before reading.

## Source-grounding rule

**Never guess.**
A `## Files included` manifest at the top of the artefact section above lists every file delivered to you in this prompt.
Before emitting `verdict: NEED_CONTEXT`, scan the manifest and confirm the file you claim is missing is genuinely absent from the list.
If a file IS in the manifest but you cannot find its content via the `--- FILE: <path> ---` delimiter, that is a long-context recall failure on your side — re-scan;
do not emit NEED_CONTEXT for files in the manifest.
Only emit `verdict: NEED_CONTEXT` for paths that are NOT in the manifest, and explain under `## Missing context` why each path is needed (one line per path).
The orchestrator will re-fire the review with those files added.
Fabricating file contents — or inferring them from filename / position alone — is a worse failure than halting honestly.

## Criteria (apply to the implementation as a whole)

- **End-to-end plan alignment** — every batch's cards are realised;
  every file listed across all batches' `Context:`/`Edits:`/`Creates:` is present in the source files provided.
- **Shared-decisions alignment** — the `## Shared Decisions` subsections are applied consistently across all batches;
  deviation is BLOCKING.
- **Out-of-plan files** — BLOCKING if any source file is present that is not accounted for in any batch's reference lists.
  If the implementer added it, the batch file must have been updated first;
  a review with surprise files means that discipline was skipped somewhere.
- **Cross-batch contracts** — interfaces produced by one batch and consumed by another are compatible.
  Dependency order implied by `depends-on:` is reflected in the code (consumers don't assume behaviour the producer doesn't guarantee).
- **Integration correctness** — the pieces work together, not just per-batch.
  Call sites match signatures;
  shared state is consistently managed;
  error surfaces compose.
- **Global utility duplication** — BLOCKING if two batches independently reimplement the same helper.
  Consolidate into a shared module.
- **Test coverage across the whole surface** — happy paths + errors for every batch's entry point.
  Integration tests reach across batch boundaries where appropriate.
- **Constraint violations** — BLOCKING.
- **Codebase consistency** — naming, error handling, imports, and style match the conventions visible in the source files provided.
- **Language pitfalls** — BLOCKING if high-risk (Python: mutable defaults, import side-effects, Windows path sep, CRLF/LF).

## Output format — STRICT

Wrap your entire output in `MILL_REVIEW_BEGIN` / `MILL_REVIEW_END` markers, each on its own line.
Everything outside these markers is ignored by the backend.
**No preamble inside the markers.**
Per finding: 3–5 lines, short and factual.
Cite file and line, state the issue, propose the fix.

Target length: ~400 tokens for APPROVE, ~800–1500 tokens for REQUEST_CHANGES across multiple batches.
If you produce more than ~1800 tokens, compress.

~~~markdown
MILL_REVIEW_BEGIN
# Review: self-report Tier 1: Go-detected structural anomalies — holistic

```yaml
verdict: APPROVE | REQUEST_CHANGES | NEED_CONTEXT
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: <UTC YYYY-MM-DD>
```

## Findings

### [BLOCKING:design] <short title, <60 chars>
**Location:** `path/to/file.py:42` (or `:42-58`)
**Issue:** <one sentence>
**Fix:** <one sentence>

### [NIT:consistency] <short title>
**Location:** `path/to/file.py:N`
**Issue:** <one sentence>
**Fix:** <one sentence>

## Missing context
(include ONLY when verdict is NEED_CONTEXT — omit the section otherwise)

- `path/to/file.py` — <one-line reason the reviewer needs this file>

## Verdict

<APPROVE | REQUEST_CHANGES | NEED_CONTEXT>
<one sentence — max 20 words>
MILL_REVIEW_END
~~~

Severity / verdict rules match review-code-batch.md.

**Severity vocabulary is closed.**
Use ONLY `BLOCKING` or `NIT` as the bracketed label in a finding heading -- never invent another word (e.g. `MAJOR`, `MINOR`, `CRITICAL`, `MEDIUM`, `HIGH`).
If a finding's severity feels ambiguous, default to `BLOCKING`, never `NIT` -- an over-cautious BLOCKING can be pushed back on by the orchestrator;
a mislabeled NIT (or an unrecognized label) can silently skip review entirely.

**Class is the second axis, encoded in the same bracket as severity, colon-separated, lowercase: `### [BLOCKING:design] <title>`.**
A finding with no class, or a class outside the four names below, is a reviewer defect.
The four recognised classes, identical in meaning across every review stage:

- `design` — a decision is missing, wrong, or rests on a false premise.
  Example: the implementation fixes the symptom at one call site but never resolves which layer owns the validation.
- `scope` — the work inventory is incomplete, or the enumeration method is unreliable.
  Example: a card's `Edits:` file was converted but a sibling file with the identical helper was left unconverted.
- `decision` — a named artifact with no stated disposition.
  Example: a config key the plan introduced is added but never wired into the loader that reads it.
- `consistency` — the artefact contradicts itself, carries a superseded statement, or violates an established repo convention.
  Example: two batches' implementations of the same interface handle the error case differently.

**Class governs who decides and when the loop stops, never whether a finding gets fixed.**

Omit `## Findings` if zero findings.
Never invent findings to pad.

## Out of scope for this stage

- Re-litigating a decision already recorded in `discussion.md` is out of scope unless new evidence contradicts it.


---

## Output contract

Write your full report to this file: /home/knatte/Code/loomyard/wts/self-report-tier1/_mill/briefs/review-code-holistic-r1.out.md

Any format the prompt above asks for (including a `MILL_REVIEW_BEGIN` / `MILL_REVIEW_END` wrapped report) is the content of /home/knatte/Code/loomyard/wts/self-report-tier1/_mill/briefs/review-code-holistic-r1.out.md -- write it there, not into chat.

Your final chat message must be exactly one line and nothing else: `WROTE /home/knatte/Code/loomyard/wts/self-report-tier1/_mill/briefs/review-code-holistic-r1.out.md`
