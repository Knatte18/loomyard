**If you find issues, REPORT them — do NOT fix them.**

You are an independent discussion reviewer for **reed: resume/down leak lock directories at the stale pre-rename session-name path**.
Round **3**.
Reviewer model: **opusmedium**.

**You MAY use Read, Grep, and Glob to verify claims against source files.**
**CRITICAL: The one exception beyond that is Write -- use it exactly once, to write your full report to the file named in this brief's output-contract footer.**
**CRITICAL: Do NOT use Edit, or run git/bash.**
**CRITICAL: Review-only. Do NOT suggest modifications. Findings only.**
**CRITICAL: Do NOT read `reviews/`. Evaluate fresh each round.**

---

## Task

Read the discussion at `/home/knatte/Code/loomyard/wts/reed-lock-stale-session-name/_mill/discussion.md`. The discussion file is the authoritative scope. Read files referenced in `## Technical Context` to verify claims.

Constraints:
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
- Bound packages: `internal/tokenvocab`, `pattern`, `buildinfo`, `standalonestate`, `shedengine`, `treadleengine`, `loomshed`, `landingshed`, `mergeresolve`, `shedrecipe`, `shedbuild`, `loomrecipe`, `planparser`, `configengine`, `shuttleengine`, `reedengine`, `burlerengine`, `websterengine`.

## Lyxdirs Single-Declarer Invariant

`internal/lyxdirs` is the sole declarer of `_lyx` (`LyxDirName`) and `.lyx` (`DotLyxDirName`).

- No other production file names either literal in path-construction context.

## Durable-vs-Ephemeral State Invariant

Every never-tracked file lives under `.lyx`, at the mirrored subpath of the `_lyx` content it relates to. `_lyx` holds tracked content only.

- Siblings under `AnchorPath()` (hub: `BoardDir(hub)`; standalone: `standalonestate.Derive`).
- No engine derives its own `.lyx` path — each module exposes a scratch accessor beside its durable one.
- Structural (`fabricengine.structuralCommittedDirs`/`structuralNeverCommittedDirs`), never from `fabric.yaml`'s `pathspec`.
- Weft content is per-branch and is never a merge participant in either direction.

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
- Package naming: `<module>cli` imports `<module>engine`; engine never imports cli/cobra. Deviation: `stencilcli` → `internal/stencilstore`.

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

- Consumers read only from the `planparser.Plan` model. `SetApproved` is the one write path.

## Discussionparser Sole-Parser Invariant

`internal/discussionparser` is the sole reader of `_lyx/discussion/`'s on-disk format. Imports the standard library only.

## Summaryparser Sole-Parser Invariant

In production code, `internal/summaryparser` is the sole declarer of the final-summary artifact's filename and the sole parser of its format. Imports the standard library only.

## Gate Self-Check Parity Invariant

A mechanical gate's `ShedProducer` row and its CLI self-check verb call the same package function for every mode.

- Discussion-Validate ↔ `validate-discussion`: `discussionparser.Validate`. Plan-Validate ↔ `validate-plan`: `planparser.ValidateFormat`. Plan-Revalidate ↔ `validate-plan --require-approved`: `planparser.Validate`.
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

- Degrading: `{shuttleengine, reedengine, websterengine, batcher}`. Strict: `{fabricengine, boardengine, loomengine}`.
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


## Source-grounding rule

Never fabricate file contents or code behaviour you have not actually read.
Do not infer from filenames or positions.

## Criteria (apply briefly to each)

- **Undecided items** — TBDs, unresolved options, multiple alternatives without a choice.
- **Scope** — what's in/out;
  could a plan writer disagree?
- **Constraint coverage** — CONSTRAINTS.md items acknowledged;
  implicit perf/compat constraints stated.
- **Tooling/validator claims** — any testing-plan claim about tooling, validator, or command-prefix requirements (e.g. `PYTHONPATH=`) must be cross-checked against CLAUDE.md and the actual enforcement (e.g. `_plan_validate.py`); a contradiction is `[BLOCKING:consistency]`.
- **Failure modes** — empty states, concurrency, invalid input, partial failures addressed.
- **Testing** — strategy named (unit/integration/e2e);
  absence or non-commital language flagged.
- **Ambiguity** — requirements needing interpretation ("fast", "handle errors").
- **Feasibility** — technical obstacles not addressed, based on source files read.
- **Decisions** — each `### Decision:` has rationale + rejected alternatives;
  implicit decisions surfaced.

Independently state, in the `reviewer_self_id:` field below, what model/version you believe yourself to be — this is your own best-effort assessment, distinct from the `reviewer_model:` value already dictated to you above.

## Output format — STRICT

Wrap your entire output in `MILL_REVIEW_BEGIN` / `MILL_REVIEW_END` markers, each on its own line.
Everything outside these markers is ignored by the backend.
**No preamble inside the markers.**
No "I reviewed..." sentences.
No narrative intro.

Per finding: 3–5 lines total, short and factual.
The consumer has full context of the discussion;
do NOT explain background.
Cite the section, state what's wrong, propose the fix.

Target length: ~300 tokens for APPROVE (just verdict + brief summary), ~600–900 tokens for REQUEST_CHANGES (one finding block per issue).
If you produce more than ~1200 tokens, you are being verbose — compress.

```
MILL_REVIEW_BEGIN
# Review: reed: resume/down leak lock directories at the stale pre-rename session-name path

```yaml
verdict: APPROVE | REQUEST_CHANGES
reviewer_model: opusmedium
reviewer_self_id: <your own model self-identification, if known>
reviewed_file: <artefact reference>
date: <UTC YYYY-MM-DD>
```

## Findings

### [BLOCKING:design] <short title, <60 chars>
**Section:** <§ or heading> **Issue:** <one sentence — what's missing or ambiguous> **Fix:** <one sentence — what to clarify or add>

### [NIT:scope] <short title>
**Section:** <§> **Issue:** <one sentence> **Fix:** <one sentence>

## Verdict

<APPROVE | REQUEST_CHANGES>
<one sentence — max 20 words>
MILL_REVIEW_END
```

Severity rules:
- `BLOCKING` — must resolve before plan writing can proceed.
- `NIT` — record but do not block.

**Severity vocabulary is closed.** Use ONLY `BLOCKING` or `NIT` as the bracketed label in a finding heading -- never invent another word. If a finding's severity feels ambiguous, default to `BLOCKING`, never `NIT`.

Verdict rules:
- `APPROVE` — zero BLOCKINGs. NITs fine.
- `REQUEST_CHANGES` — one or more BLOCKINGs.

**Class is the second axis, encoded in the same bracket as severity, colon-separated, lowercase: `### [BLOCKING:design] <title>`.**
A finding with no class, or a class outside the four names below, is a reviewer defect.
The four recognised classes, identical in meaning across every review stage:

- `design` — a decision is missing, wrong, or rests on a false premise.
  Example: the discussion never says which of two incompatible caching strategies the plan should use.
- `scope` — the work inventory is incomplete, or the enumeration method is unreliable.
  Example: the discussion names three affected modules but the source tree shows a fourth with the same pattern.
- `decision` — a named artifact with no stated disposition.
  Example: the discussion references a legacy config key it never says whether to keep, migrate, or delete.
- `consistency` — the artefact contradicts itself, carries a superseded statement, or violates an established repo convention.
  Example: the discussion's constraints section says "no new dependencies" while a later section proposes adding one.

**Class governs who decides and when the loop stops, never whether a finding gets fixed.**

Omit the `## Findings` section entirely if there are zero findings. Never invent findings to pad the review.

## Out of scope for this stage

- Call-site enumeration and compile-breakage enumeration belong to the build and to code review, not to discussion review.
- An unreliable enumeration method is ONE `design` finding about the method itself, never N `scope` findings naming individual files.


---

## Output contract

Write your full report to this file: /home/knatte/Code/loomyard/wts/reed-lock-stale-session-name/_mill/briefs/review-discussion-holistic-r3.out.md

Any format the prompt above asks for (including a `MILL_REVIEW_BEGIN` / `MILL_REVIEW_END` wrapped report) is the content of /home/knatte/Code/loomyard/wts/reed-lock-stale-session-name/_mill/briefs/review-discussion-holistic-r3.out.md -- write it there, not into chat.

Your final chat message must be exactly one line and nothing else: `WROTE /home/knatte/Code/loomyard/wts/reed-lock-stale-session-name/_mill/briefs/review-discussion-holistic-r3.out.md`
