**If you find issues, REPORT them — do NOT fix them.**

You are an independent plan reviewer for **shed: the LLM driver as a generic stepper and mender**.
You evaluate the complete plan (all batches) and produce a structured review.

Reviewer model: **sonnetxhigh**.
Round **2**.

**You MAY use Read, Grep, and Glob to verify claims against source files.**
**CRITICAL: The one exception beyond that is Write -- use it exactly once, to write your full report to the file named in this brief's output-contract footer.**
**CRITICAL: Do NOT use Edit, or run git/bash.**
**CRITICAL: Review-only. Do NOT suggest modifications. Findings only.**
**CRITICAL: Do NOT read `reviews/`. Evaluate fresh each round.**

## Writing style

State the point first — no preamble, and no restating a point before making it.
Cut empty intensifiers ("actually", "really", "simply", "just", "completely"): remove the word, and if the sentence still means the same thing it was padding.
Say each thing once; do not restate it in a summary or a closing recap.
Do not narrate what is already visible in the quoted code or the surrounding context.
Don't pin a perishable specific — a tally, a list of the current callers of a symbol, or a name cited descriptively rather than as a stable identifier: name the source, not the snapshot.
Apply a per-sentence cut test: would the reader act differently if this sentence were missing? If not, cut it.
For any multi-line prose written into a file, use semantic line breaks — one sentence per line, never fixed-column hard-wrap, plain newlines only (never a trailing double-space or backslash).

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
- Bound packages: `internal/tokenvocab`, `pattern`, `buildinfo`, `standalonestate`, `shedengine`, `treadleengine`, `loomshed`, `landingshed`, `mergeresolve`, `shedrecipe`, `shedbuild`, `loomrecipe`, `planparser`, `planglyph`, `configengine`, `shuttleengine`, `reedengine`, `burlerengine`, `websterengine`, `cliwire`, `battenshed`, `battenrecipe`.
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

## Hub Suffix Invariant

`-LYXHUB` is the sole hub container suffix, and no code parses, trims, or recognises the retired `-HUB`.

- The suffix is declared twice by sanction — `internal/lyxcwd` holds the private `hubSuffix` const, used for `RepoName` derivation, and `internal/fabricengine` holds the exported `HubSuffix` const, used by `HubPath`.
  Both move together, and `TestEnforcement_GeometryLiterals`'s `geometryTokenOwners` row is the third site that must move with them.
- Hub discovery is name-independent — the hub is `filepath.Dir(workTreeRoot)` and `looksLikeHub` is structural — so a hub still carrying the retired suffix still resolves;
  only `Location.RepoName`, a display-only value never used to construct a path, degrades.
- A hub carrying the retired suffix is never renamed in place: `PortalLink` and `LauncherDir` materialise links against the hub's absolute path at creation time, and `ServerName` hashes that absolute path into the tmux socket key.
  The operator removes the old container by hand and re-clones;
  `clone --reset` resolves the new suffix only and never reaches it.

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

## Shed Verb-Set Invariant

`internal/shedverbs` owns the generic `run`/`step`/`status`/`pause` verb bodies; no `<module>cli` reimplements one. This first clause is review discipline, not a scan — "reimplements" has no static shape a scan can see.

- `shedverbs` derives no path and imports no resolver — no `lyxcwd`, no `os.Getwd`, no `git rev-parse` — and imports no `<module>cli`, which is what keeps it a leaf and keeps `internal/shedcli`'s own imports acyclic. Enforced by `internal/shedverbs/seam_enforcement_test.go`.
- The `lyx shed` recipe table lives in `internal/shedcli` alone, as one map literal reached through accessors, with every name armed by exactly one arming function and no `init()` self-registration. Enforced by `internal/shedcli/table_test.go`.
- The `step` refusal-kind vocabulary stays closed at its five values. Enforced by `internal/shedverbs/step_test.go`.
- `internal/shedverbs` is not itself a CLI module and is not counted in the CLI/Cobra Invariant's tally at all — it exposes no `Command()`/`RunCLI` seam, only the `Verbs(texts, spec)` constructor the three subtrees build from.
- Neither `shedverbs` nor `shedcli` is added to the Told-Geometry Invariant's bound-packages list: that list binds engines, both sit above that layer. `shedverbs` keeps this invariant's own no-resolver clause verbatim as its no-derived-paths obligation, enforced by `internal/shedverbs/seam_enforcement_test.go`. `internal/shedcli` is carved out of that clause by name, the one site in this pair that resolves: `resolvePersistentPreRun` must read a seed before it knows which recipe to arm, a seed read is a path read, and a path read needs an anchor, so `shedcli` calls `lyxcwd.Resolve` and builds seed paths from the result. Its narrower obligation is that every path it touches comes from an `internal/shedrun` constructor and none is derived locally — `shedcli` still declares no path segment of its own, it just resolves the anchor those `shedrun` constructors need.

## Shed Run-Directory Invariant

`internal/shedrun` is the sole declarer of the `shed` path segment, of the run-id vocabulary including the literal `self`, and the sole parser and writer of `seed.json`.

- No other production file names the `shed` segment or the `seed.json` filename in path-construction context.
- No other package decodes or encodes the `Seed` struct.
- A run-id is validated as a single path segment (`ValidateRunID`) before being joined onto any anchor.

## Driver Choice Single-Site Invariant

A *recorded* seed driver value is read in exactly one place per recipe, that recipe's own bootstrap verb, and the branch on it selects a spawn and nothing else.

- No producer, no generic verb and no engine reads the recorded value, and no code path gates *behaviour* on it.
- One carve-out, and only this shape: a CLI verb may compare a driver value the operator **just typed** against the addressed run's recorded one and refuse on the envelope when they disagree — `internal/battencli`'s `refuseAdoptedSeed`. It selects no spawn and changes no behaviour, and it is the loud command-line failure this invariant's own rationale prefers over silently discarding the typed flag. A refusal decided by the recorded value alone, with nothing typed to compare it against, is not this shape and stays barred.
- This governs the **read, not the vocabulary**: the driver constants are legitimately named at the seeding sites, which validate a flag before a seed exists — those sites validate an argument rather than reading a written seed, so a naive "one consumer of the constants" rule would be false against this very invariant.
- Permitted constant consumers, verified against the merged tree rather than assumed: the shed CLI's `seed` command; batten's own flag validation in `internal/battencli/arm.go`; batten's child-driver param reader in `internal/battencli/wire.go`; and loom's own seed writer in `internal/loomcli/sharedbootstrap.go`. The last two are the ones a shorter list would wrongly omit — neither reads a written seed's `Driver` field (batten's reads a `child_driver` *param*, loom's *writes* the field), so both are constant consumers rather than readers, which is exactly the distinction this invariant turns on. `internal/battencli/arm.go` is on this list twice over: its flag validation is a constant consumer, and its `refuseAdoptedSeed` is the one carve-out above, which does read the field.
- Rationale: the recorded value is a startup choice, and the failure mode a second reader introduces is silent divergence between what a run was seeded as and what it is actually doing — unobservable from either the status file or the envelope. A flag validator, by contrast, fails loudly at the command line, where a mistake is visible immediately.
- Enforcement is a **tripwire, not a completeness proof**, in the same sense the Completion Signal Invariant's own scan is: `internal/loomcli/bootstrap_test.go` asserts that the only production readers of the seed's driver **field** outside the `shedrun` package are `internal/loomcli` and the functions named in its own `driverFieldReadCarveOuts` (keyed by file and enclosing function, so a second reader in a carved-out file still fails), scanning the field selector rather than the driver constants (the constants are legitimately named elsewhere, per the bullet above). Adding a reader fails it and forces a human to confirm, and each carve-out carries its justification beside the entry.

## Tokenvocab Leaf Invariant

`internal/tokenvocab` imports only stdlib and `internal/stencil`. Reverse import never allowed.

## Buildinfo Leaf Invariant

`internal/buildinfo` imports nothing at all, not even stdlib. Exposes `Channel`/`IsDev()` only.

## Standalonestate Leaf Invariant

`internal/standalonestate` imports only the standard library. Never resolves a working directory; `Derive` creates nothing on disk.

## Pattern Leaf Invariant

`internal/pattern` imports only stdlib, `lyxdirs`, `stencilstore`, `stencil` — never a feature package. Reverse import never allowed.

## Friction Leaf Invariant

`internal/friction` imports only stdlib, `internal/logger`, `internal/stencil`, and `internal/stencilstore` — never a feature package. Reverse import never allowed.

- `internal/logger` is admitted because the marker-absent helper logs rather than returning a bool for seven callers to duplicate, and `internal/friction` already pulls `logger` transitively through `internal/stencilstore`, so the admission widens nothing in practice.

## Stencil Ownership Invariant

Every producer prompt and every deployed normative spec is read at call time from a told, absolute directory, never embedded bytes.

- `//go:embed` backs two seed-default registries: `contracts/stencils`'s producer prompts, and `contracts/specs`'s deployed normative specs.
- `internal/stencilstore` is sole owner of seeding/hashing/reading/validation for either baseDir. A hash-mismatched file is never overwritten in either, with no force-sync carve-out for specs.
- Seed/refresh runs once per process pre-run for both, never lazily inside `Read`.
  A command that reads no stencils reads no specs either, and may decline the pass entirely by carrying the skip annotation;
  declining is all-or-nothing per command and never defers seeding to a later or lazier point.

## CLI / Cobra Invariant

Every lyx CLI module is a cobra subtree assembled under one root in `cmd/lyx/main.go`.

- Each module exposes `Command() *cobra.Command` and `RunCLI(out io.Writer, args []string) int`; every module but `internal/selfreportcli` also carries `RunCLIIn(cwd, out, args) int`.
- An alias command may delegate into another module's subtree with no seam function of its own.
- Non-empty `Short` on every command.
- Errors are JSON via `internal/output`, one object per line; every `RunE` checks `clihelp.ShouldAbort` first.
- Interactive-handoff exception, narrow and per-command: `reedengine` `attach`/`watchdog`, `lyx loom status --watch`, `lyx loom start`/`lyx start`, `lyx shed status --watch`, `lyx batten status --watch`.
- Package naming: `<module>cli` imports `<module>engine`; engine never imports cli/cobra. Deviations: `stencilcli` → `internal/stencilstore`; `quarrycli` → `internal/planglyph`; `battencli` → `internal/battenshed`, `internal/battenrecipe` (no engine package of its own); `shedcli` → `internal/shedverbs`, `internal/loomcli`, `internal/battencli` (no engine package of its own).

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
- A `*Run` issued by `Start`/`StartGated` has already resolved its provider's startup probe; no caller outside `internal/shuttleengine` probes provider readiness or plays startup-gate keys.

## Shell Mechanics Seam

Pane-shell command strings are built ONLY via `internal/shell`, stdlib-only.
`Quote`/`Invoke`/`ReadFile` are illustrative of the seam's mechanics, not an exhaustive interface listing.

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

## Batten Bookend Invariant

A producer that creates or destroys a task worktree never runs from inside that worktree.

- The batten Shed is driven from the hub's prime worktree; its status file is durable under prime's own anchor and its locks stay ephemeral there, never under the worktree being managed.
- A teardown row sequences session shutdown before worktree removal, in one producer, never two rows.
- Enforcement is review discipline with two partial mechanical proxies, not an enforcing test: the invariant constrains which directory a running process is driven from, which has no static shape an AST scan can see.
  `internal/battenshed`'s seam-enforcement scan bars a direct resolver import so the package cannot resolve its way into the managed worktree, and `internal/battencli`'s path-derivation tests pin the status and lock paths to prime's anchor so a relocation under the managed worktree fails there — neither proves the driver's own working directory, which stays a review obligation.
- "Prime" means the WARP prime: the weft sibling is a repository of its own whose prime is itself, so the name comparison alone admits it, and `battencli`'s pre-run therefore calls `fabricengine.RequireDrivableWorktree` — `RequireWarpWorktree` under the vocabulary-neutral name a non-owner may say at all — ahead of the name check (integration test `TestBattenIntegration_WeftPrimeRefusal`).

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

## Pane Binary Resolution

A strand pane reed creates resolves `lyx` to the binary that spawned it.

- `panebin.go` owns the seam, and `launchStrandLocked` is its only call site.
- Every shell token is emitted through `internal/shell`, per the Shell Mechanics Seam.
- The dialect is `shell.ForGOOS()`, the same selector as the launch command the prelude is joined onto,
  and reed neither derives a dialect of its own nor changes how a pane's shell is started.
- Scope is strand panes only, with Selvage's split and the `new-session` first pane exempt by name,
  and the detached `lyx loom run` and watchdog daemon spawns excluded because both are already
  spawned from the executable path and neither resolves `lyx` from `PATH`.
- An unresolvable executable path degrades to a pane with no prelude plus a named `logger.Warn`,
  never a failed launch.
- Backed by `internal/reedengine/panebin_enforcement_test.go`, which fails if a `split-window`
  pane-creation site appears outside the chokepoint and outside the named allowlist.

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

A mechanical gate's **closure** and its CLI self-check verb call the same package function for every mode.

- Discussion-Write's and Discussion-Burler's gates ↔ `validate-discussion`: `discussionparser.Validate`. Plan-Write's and Plan-Burler's gates ↔ `validate-plan`: `planglyph.ValidateFormat`.
- The verb's `--require-approved` mode, running the full check set, has no recipe counterpart by design: both plan gate sites run strictly before the Plan-Review segment's approve seam writes the approval flag, so demanding it would fail every fix round, and the flag's guarantee rests on that seam failing loudly instead — never on a row re-checking it.
- Adding a mechanical gate means adding its verb and its parity check in the same task.
- Moving a gate from a standalone row into a producer's own closure changed *where* the call sits, never the property this invariant binds, which is why the invariant survives the move rather than retiring with the rows.

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

Every unprefixed path below is relative to `/home/knatte/Code/loomyard/wts/shed-llm-driver`.
- `wiki/` paths are relative to `/home/knatte/Code/loomyard/wiki`

## Files included (N=82)

- _mill/plan/00-overview.md
- _mill/plan/01-logger-trace-accessors.md
- _mill/plan/02-fabric-mutation-trace.md
- _mill/plan/03-shed-envelope-trace.md
- _mill/plan/04-step-entry-point-integration.md
- _mill/plan/05-ly-drive-recipe-blind.md
- internal/logger/sink.go
- internal/logger/sink_test.go
- internal/logger/logger.go
- internal/fabricengine/mutation.go
- internal/fabricengine/mutation_test.go
- internal/fabricengine/add.go
- internal/fabricengine/weftwiring.go
- internal/fabricengine/checkout.go
- internal/fabricengine/weftgit.go
- internal/fabricengine/coalesce.go
- internal/fabricengine/pushanchored.go
- internal/fabricengine/spawn.go
- internal/fabricengine/pushanchored_integration_test.go
- internal/shedverbs/spec.go
- internal/shedverbs/step.go
- internal/shedverbs/step_test.go
- internal/shedverbs/seam_enforcement_test.go
- internal/loomcli/step_test.go
- internal/shedverbs/status.go
- internal/shedverbs/status_test.go
- internal/loomcli/status_test.go
- internal/shedverbs/doc.go
- CONSTRAINTS.md
- internal/loomcli/arm.go
- internal/loomcli/cli_test.go
- internal/battencli/arm.go
- internal/battencli/step_test.go
- internal/landingshed/publish.go
- internal/landingshed/publish_test.go
- internal/shedcli/parity_test.go
- internal/loomcli/driverprompt.go
- internal/loomcli/driverprompt_test.go
- cmd/lyx/drivercap_test.go
- plugins/ly/skills/ly-drive/SKILL.md
- plugins/ly/skills/INDEX.md
- internal/loomcli/start.go
- internal/loomcli/cli.go
- docs/overview.md
- internal/battenshed/doc.go
- internal/shedadapters/bouncer_seed_test.go
- internal/loomcli/smoke_bootstrapwiring_test.go
- manifest/roadmap.md
- manifest/designs/shed-llm-driver.md
- internal/logger/retention.go
- internal/logger/trace.go
- internal/lyxcwd/lyxcwd.go
- CONSTRAINTS.md
- internal/logger/logger.go
- internal/logger/sink.go
- internal/gitrepo/gitrepo.go
- internal/gitexec/gitexec.go
- internal/fabricengine/destroy.go
- internal/fabricengine/fabric.go
- internal/output/output.go
- internal/shedengine/run.go
- internal/shedverbs/testsupport_test.go
- internal/shedengine/status.go
- internal/loomcli/cli_test.go
- internal/shedverbs/seam_enforcement_test.go
- internal/shedverbs/spec.go
- internal/shedrun/paths.go
- internal/loomcli/cli.go
- internal/loomcli/wiring.go
- internal/loomcli/status_test.go
- internal/battencli/paths.go
- internal/battencli/run_test.go
- internal/lock/lock.go
- internal/loomcli/start.go
- _mill/discussion.md
- internal/shedverbs/step.go
- internal/shedverbs/status.go
- internal/shedrun/seed.go
- internal/fabricengine/mutation.go
- internal/fabriccli/fabric.go
- cmd/lyx/sandbox_coverage_test.go
- internal/loomcli/driverprompt.go

## Plan files to review
- Overview: `_mill/plan/00-overview.md`
- Batches:
- `_mill/plan/01-logger-trace-accessors.md`
- `_mill/plan/02-fabric-mutation-trace.md`
- `_mill/plan/03-shed-envelope-trace.md`
- `_mill/plan/04-step-entry-point-integration.md`
- `_mill/plan/05-ly-drive-recipe-blind.md`

Read the overview and every batch listed above. Then read the source files referenced across all batches:
- `internal/logger/sink.go`
- `internal/logger/sink_test.go`
- `internal/logger/logger.go`
- `internal/fabricengine/mutation.go`
- `internal/fabricengine/mutation_test.go`
- `internal/fabricengine/add.go`
- `internal/fabricengine/weftwiring.go`
- `internal/fabricengine/checkout.go`
- `internal/fabricengine/weftgit.go`
- `internal/fabricengine/coalesce.go`
- `internal/fabricengine/pushanchored.go`
- `internal/fabricengine/spawn.go`
- `internal/fabricengine/pushanchored_integration_test.go`
- `internal/shedverbs/spec.go`
- `internal/shedverbs/step.go`
- `internal/shedverbs/step_test.go`
- `internal/shedverbs/seam_enforcement_test.go`
- `internal/loomcli/step_test.go`
- `internal/shedverbs/status.go`
- `internal/shedverbs/status_test.go`
- `internal/loomcli/status_test.go`
- `internal/shedverbs/doc.go`
- `CONSTRAINTS.md`
- `internal/loomcli/arm.go`
- `internal/loomcli/cli_test.go`
- `internal/battencli/arm.go`
- `internal/battencli/step_test.go`
- `internal/landingshed/publish.go`
- `internal/landingshed/publish_test.go`
- `internal/shedcli/parity_test.go`
- `internal/loomcli/driverprompt.go`
- `internal/loomcli/driverprompt_test.go`
- `cmd/lyx/drivercap_test.go`
- `plugins/ly/skills/ly-drive/SKILL.md`
- `plugins/ly/skills/INDEX.md`
- `internal/loomcli/start.go`
- `internal/loomcli/cli.go`
- `docs/overview.md`
- `internal/battenshed/doc.go`
- `internal/shedadapters/bouncer_seed_test.go`
- `internal/loomcli/smoke_bootstrapwiring_test.go`
- `manifest/roadmap.md`
- `manifest/designs/shed-llm-driver.md`
- `internal/logger/retention.go`
- `internal/logger/trace.go`
- `internal/lyxcwd/lyxcwd.go`
- `CONSTRAINTS.md`
- `internal/logger/logger.go`
- `internal/logger/sink.go`
- `internal/gitrepo/gitrepo.go`
- `internal/gitexec/gitexec.go`
- `internal/fabricengine/destroy.go`
- `internal/fabricengine/fabric.go`
- `internal/output/output.go`
- `internal/shedengine/run.go`
- `internal/shedverbs/testsupport_test.go`
- `internal/shedengine/status.go`
- `internal/loomcli/cli_test.go`
- `internal/shedverbs/seam_enforcement_test.go`
- `internal/shedverbs/spec.go`
- `internal/shedrun/paths.go`
- `internal/loomcli/cli.go`
- `internal/loomcli/wiring.go`
- `internal/loomcli/status_test.go`
- `internal/battencli/paths.go`
- `internal/battencli/run_test.go`
- `internal/lock/lock.go`
- `internal/loomcli/start.go`
- `_mill/discussion.md`
- `internal/shedverbs/step.go`
- `internal/shedverbs/status.go`
- `internal/shedrun/seed.go`
- `internal/fabricengine/mutation.go`
- `internal/fabriccli/fabric.go`
- `cmd/lyx/sandbox_coverage_test.go`
- `internal/loomcli/driverprompt.go`

Every path listed above is relative to the root stated in the `## Path roots` block above and must be resolved against it before reading.

## Intentionally deleted (N=2)

- cmd/lyx/drivercap_test.go
- manifest/designs/shed-llm-driver.md

## Source-grounding rule

**Never guess.**
A `## Files included` manifest at the top of the artefact section above lists every file delivered to you in this prompt.
Before emitting `verdict: NEED_CONTEXT`, scan the manifest and confirm the file you claim is missing is genuinely absent from the list.
If a file IS in the manifest but you cannot find its content via the `--- FILE: <path> ---` delimiter, that is a long-context recall failure on your side — re-scan;
do not emit NEED_CONTEXT for files in the manifest.
Only emit `verdict: NEED_CONTEXT` for paths that are NOT in the manifest, and explain under `## Missing context` why each path is needed (one line per path).
The orchestrator will re-fire the review with those files added.
Fabricating file contents — or inferring them from filename / position alone — is a worse failure than halting honestly.

**Mechanism claims must be source-verified.**
A finding that rests on a claim about how the target repo's production code behaves — which branch executes, what a predicate selects, which value survives a mutation — must name the file and the function/method/construct it was verified against, in the finding's own text.
Do not assert a mechanism claim from memory, naming convention, or plausible-sounding inference.
If you cannot verify the claim against source in your context (not bulked into this prompt, and not Read-able in bulk mode), do not assert it: downgrade the finding to a question under `## Missing context`, or drop it — never write an unverified mechanism claim into a BLOCKING or NIT finding as fact.
Tool-use-mode reviewers may Read/Grep the target repo's source directly to verify a mechanism claim even when the relevant file was not bulked into this prompt; bulk-mode reviewers have no such option and must rely on this rule alone.

## Overview verify: scope rule

The overview's module-wide `verify:` field (if set) must stay a cheap compile/vet/smoke command, per `plan-overview.md`'s own documented intent — never a full test-suite run. Do not suggest, as a fix for any finding, converting it into an unscoped full-test command (e.g. `go test ./...`, `dotnet test`, `pytest`) — `_plan_validate`'s `verify-full-suite` check will reject that on the plan's very next validation pass, costing a review round for nothing.

## Criteria (apply to the plan as a whole)

- **Constraint violations** — BLOCKING.
- **Alignment** — plan covers all task requirements.
- **Decision alignment** — every `### Decision:` in `## Shared Decisions` faithfully implemented.
- **Completeness** — every card has `Creates`/`Edits`, `Context`, `Moves`, `Requirements`, `Commit`.
- **Moves well-formed** — each `Moves:` sub-bullet is an `` `old` -> `new` `` pair (backtick-wrapped paths, ASCII ` -> ` arrow);
  bare `none` on the label line is valid;
  any other format is a finding.
- **Rename mechanic present** — any batch whose cards contain a non-empty `Moves:` must include a `## Rename mechanic` section describing the `git mv` + surgical-edit approach;
  absence is a finding.
- **No full-file rewrites of relocated files** — prescribing a write-from-scratch for a file that appears in `Moves:` (rather than `git mv` + surgical edits) is a finding.
- **Sequencing + batch dependencies** — correct order within and across batches;
  `batch-depends` accurate;
  no forward deps.
- **Batch Index DAG integrity** — BLOCKING if the `batches:` block in `00-overview.md` has a cycle, references a batch name not declared, or names a `file:` not present in the plan directory.
- **Edge cases + risks** — failures, empty states, boundaries addressed.
- **Over-engineering** — unneeded abstractions or unrequested features.
- **Codebase consistency** — follows patterns in the source files provided.
- **Test coverage** — error paths + edges.
- **Language pitfalls** — BLOCKING if high-risk (Python: mutable defaults, import side-effects, Windows path sep, CRLF/LF).
- **Integration test reachability** — BLOCKING if integration tests added but `verify:` doesn't run them.
- **Explore targets** — purpose-driven;
  subset of `Context:`.
- **Step granularity + atomicity** — each card small and self-contained.
- **Requirements specificity** — BLOCKING if `Requirements:` uses vague prose ("refactor X", "update to use helper") without naming the specific function, class, or constant being changed.
  Stable identifiers are required.
- **Context field** — non-empty per card;
  Edits: files are implicitly read.
- **Context completeness** — BLOCKING if `Requirements:` mentions a function, class, or constant from a file not listed in `Context:` or `Edits:`.
  The implementer may only read files in `Context:`;
  a missing entry means cold-start exploration.
  This is NOT a finding when the path falls into one of the following exemptions: a path named on a same-line prohibition;
  a path named on a citation, contrast, or escape-marker line, including the markers `signature inlined`, `no file read needed`, and `mentioned, not read`;
  a path inside quoted material — a fenced block or a blockquote line — within `Requirements:`;
  a git-ignored path;
  a path outside the repository, including absolute and home-relative literals;
  a trailing-slash directory reference;
  or a forward reference to a path a later card in the plan declares as its own `Creates:` target.
  Do not raise a finding for any of these — the remedy you would otherwise ask for, adding the path to `Context:`, is either impossible or actively wrong.
- **Global step numbering** — unique, sequential, no gaps across batches.
- **All Files Touched scope** — the overview's `## All Files Touched` section lists the union of `Edits:`/`Creates:`/Move-target paths across all batches;
  `Deletes:` tokens and Move-source paths are excluded by convention.
  A Deletes-only or Move-source-only path missing from that list is correct, not a finding.
- **Platform-behavior-claim verification** — BLOCKING if a plan or discussion claim describes Claude Code's own platform/harness behavior (e.g. agent auto-discovery, plugin manifest semantics) and a manifest or doc file that could confirm or refute the claim is present in your context, bulked or Read-able,
  but the claim was accepted without checking that file.
  Tool-use-mode reviewers may Read `plugin.json`/platform docs directly even when not bulked.

## Output format — STRICT

Wrap your entire output in `MILL_REVIEW_BEGIN` / `MILL_REVIEW_END` markers, each on its own line.
Everything outside these markers is ignored by the backend.
**No preamble inside the markers.**
Per finding: 3–5 lines, short and factual.
The consumer has full context of the plan;
do NOT explain background.
Cite the batch/card, state what's wrong, propose the fix.

Target length: ~300 tokens for APPROVE, ~600–1200 tokens for REQUEST_CHANGES across multiple batches.
If you produce more than ~1500 tokens, compress.

```
MILL_REVIEW_BEGIN
# Review: shed: the LLM driver as a generic stepper and mender — holistic

```yaml
verdict: APPROVE | REQUEST_CHANGES | NEED_CONTEXT
reviewer_model: sonnetxhigh
reviewed_file: plan/
date: <UTC YYYY-MM-DD>
```

## Findings

### [BLOCKING:design] <short title, <60 chars>
**Location:** <batch / card number> **Issue:** <one sentence> **Fix:** <one sentence>

### [NIT:consistency] <short title>
**Location:** <batch / card> **Issue:** <one sentence> **Fix:** <one sentence>

## Missing context
(include ONLY when verdict is NEED_CONTEXT — omit the section otherwise)

- `path/to/file.py` — <one-line reason the reviewer needs this file>

## Verdict

<APPROVE | REQUEST_CHANGES | NEED_CONTEXT>
<one sentence — max 20 words>
MILL_REVIEW_END
```

Severity:
- `BLOCKING` — must fix before the plan is approved.
- `NIT` — record but do not block.

Verdict:
- `APPROVE` — zero BLOCKINGs.
- `REQUEST_CHANGES` — one or more BLOCKINGs.
- `NEED_CONTEXT` — missing source files; orchestrator will re-fire.

**Severity vocabulary is closed.** Use ONLY `BLOCKING` or `NIT` as the bracketed label in a finding heading -- never invent another word (e.g. `MAJOR`, `MINOR`, `CRITICAL`, `MEDIUM`, `HIGH`). If a finding's severity feels ambiguous, default to `BLOCKING`, never `NIT` -- an over-cautious BLOCKING can be pushed back on by the orchestrator; a mislabeled NIT (or an unrecognized label) can silently skip review entirely.

**Class is the second axis, encoded in the same bracket as severity, colon-separated, lowercase: `### [BLOCKING:design] <title>`.**
A finding with no class, or a class outside the four names below, is a reviewer defect.
The four recognised classes, identical in meaning across every review stage:

- `design` — a decision is missing, wrong, or rests on a false premise.
  Example: a card's `Requirements:` never states which of two conflicting approaches from the discussion to implement.
- `scope` — the work inventory is incomplete, or the enumeration method is unreliable.
  Example: a batch's `Context:` list omits a file the card's own `Requirements:` names.
- `decision` — a named artifact with no stated disposition.
  Example: a Shared Decision references a config key the plan never says whether the card should add, migrate, or leave alone.
- `consistency` — the artefact contradicts itself, carries a superseded statement, or violates an established repo convention.
  Example: two cards in different batches prescribe different commit messages for the same file.

**Class governs who decides and when the loop stops, never whether a finding gets fixed.**

Omit `## Findings` if zero findings. Never invent findings to pad.

## Out of scope for this stage

- Per-line code correctness belongs to code review, not to plan review.
- A plan reviewer judges whether the plan's method for enumerating work is reliable, not whether it re-enumerates the work itself.


---

## Output contract

Write your full report to this file: /home/knatte/Code/loomyard/wts/shed-llm-driver/_mill/briefs/review-plan-holistic-r2.out.md

Any format the prompt above asks for (including a `MILL_REVIEW_BEGIN` / `MILL_REVIEW_END` wrapped report) is the content of /home/knatte/Code/loomyard/wts/shed-llm-driver/_mill/briefs/review-plan-holistic-r2.out.md -- write it there, not into chat.

Your final chat message must be exactly one line and nothing else: `WROTE /home/knatte/Code/loomyard/wts/shed-llm-driver/_mill/briefs/review-plan-holistic-r2.out.md`
