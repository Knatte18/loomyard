# Reed and Fabric as standalone modules: public API design

## What it is

This document answers, for two lyx modules — `internal/reedengine` (Reed) and `internal/fabricengine` (Fabric) — what each one's public contract would have to be for its repo to be usable by a non-loomyard consumer, and whether paying that price is worth it.
The deliverable is a design document about unbuilt work, not documentation of the shipped modules — the shipped modules' own design lives in their own package doc comments (`internal/reedengine/doc.go`, `internal/fabricengine/doc.go`).

## How the numbers in this document were produced

Every figure in this document was measured in this worktree at commit `ca388c0ce`, using `go list`, `go doc` and `grep`.
Each figure is stated alongside the command that produced it.
One caveat applies to every grep-derived count in this document: a naive grep over a package name also matches doc-comment prose, not just code, and two figures in this task's own first draft were wrong for exactly that reason.
Every grep-derived count in this document therefore means "referenced in code by production packages" — excluding `_test.go` files and doc comments — unless stated otherwise.

## Disposition of this document

This document lands with no owning entry in `manifest/roadmap.md`, **by design**: its content is the evidence needed to decide whether any roadmap entry is warranted at all, so writing the entry first would presuppose the verdict this document exists to reach.
The follow-up decision has two legitimate outcomes: one or more Someday entries are added pointing at this document, or none is.
In the second case, this document is **deleted** rather than left orphaned.
That deletion trigger is named here explicitly, so a later reader of an entry-less designs file under `manifest/designs/` knows the state is intentional and knows what closes it.

This document's lifecycle classification follows the [docs/overview.md](../../docs/overview.md#documentation-lifecycle) Documentation Lifecycle rule: `manifest/designs/<module>.md` is scoped to planned, not-yet-built work, deleted when the module lands or the plan is abandoned.
This document describes a possible future extraction and its prerequisites, not the shipped Reed and Fabric modules themselves — so it satisfies that rule even though both named modules are already shipped.

## The extraction rubric — the frozen contract is the exported type set

For a Go module, the extraction contract is the set of exported identifiers consumers reference in code — not an interface.
A consumer-side interface narrows *coupling* and buys testability, but it does not narrow that contract by one name.

The shipped example that grounds this: `shuttleengine.ReedOps` (`internal/shuttleengine/reed.go`) is held up as the model narrow seam.

```go
type ReedOps interface {
	AddStrand(spec reedengine.AddSpec) (reedengine.Strand, error)
	RemoveStrand(guid string, recursive bool) (reedengine.Removed, error)
	Status() (reedengine.StatusResult, error)
	SendText(guid, text string, submit bool) error
	SendKey(guid, key string) error
	CapturePane(guid string) (string, error)
}
```

Yet every one of its six methods takes or returns a type from the provider package — `reedengine.AddSpec`, `reedengine.Strand`, `reedengine.Removed`, `reedengine.StatusResult` — so a consumer behind this interface still pins four exported types from `reedengine`.

The consequence: the background framing that Fabric's blocker is "no interface seam anywhere on the consumer side" misdiagnoses the problem.
Adding fifteen consumer-side interfaces to Fabric's consumers would not shrink its 74-identifier contract by one name.
What shrinks a contract is deleting or unexporting identifiers, or moving them behind a façade that re-exports a curated subset.

Two metrics are rejected as the readiness measure, and each fails for a distinct reason:

- **Interface count** measures coupling and testability, not contract size — as `ReedOps` demonstrates above, an interface can exist and pin the same four types anyway.
- **Raw importer count** measures the wrong axis of "extractable": Reed and Fabric have nearly identical transitive-dependent counts, 26 and 27 respectively, and are nowhere near equally extractable — Fabric's contract and size dwarf Reed's, as the per-module sections below measure.

## Four corrections to the task's background notes

This investigation found four background claims to be false.
Each is recorded here as a correction, with its evidence, because the doc's value depends on being checkable, and because a naive re-check of any of these four is the obvious way for a later reader to get the same claim wrong again.

1. **The `gitkit`/`gitrepo`/`gitexec` "trio" framing is wrong.**
   `internal/gitkit` is not in Fabric's dependency set at all, transitively or directly — confirmed by `go list -deps ./internal/fabricengine`, which does not list it.
   Its only non-test production importer anywhere in the repo is one file, `internal/hubforge/seed.go`; every other reference to `gitkit` is from a `_test.go` file, which is what makes it test-fixture machinery.
   The `gitkit` Leaf Invariant (`CONSTRAINTS.md`) is therefore untouched by anything in this document.

2. **Fabric's cwd coupling is structural, not incidental.**
   True self-resolution of cwd is only two sites (`clone.go:403`, `unwire.go:53`, both calling `lyxcwd.Resolve(cwd)`).
   But six further sites re-derive a `Location` from a stored path via `lyxcwd.ResolveWorktree` (`commit.go:138`, `mergelifecycle.go:199`, `merge.go:121`, `pull.go:456`, `warplayout.go:25`, `worktreelist.go:130`).
   That makes the `lyxcwd` coupling structural typing across the package, not an incidental parameter on a couple of entry points.

3. **The missing-interface framing misdiagnoses Fabric's blocker.**
   See the extraction rubric section above: adding interfaces on the consumer side narrows coupling, never contract size, so "no interface seam anywhere on the consumer side" is not what stands between Fabric and extraction.

4. **`internal/lyxcwd` does not import `internal/fabricengine`.**
   A naive `grep -rl "internal/fabricengine"` lists two files under `internal/lyxcwd` (`lyxcwd.go`, `anchor.go`), but both matches are doc-comment prose, not import statements.
   `go list -deps ./internal/lyxcwd` confirms that package's only internal import is `internal/gitexec`, exactly as the Cwd Resolution Invariant requires.
   This correction is recorded precisely because it is the obvious thing a reader would suspect and the obvious way to get it wrong — the same comment-prose-contamination trap the measurement-method section above already named.

## The quarry precedent — this project's one completed extraction

This project has one completed extraction to learn from: `lyx scout` became the standalone `github.com/Knatte18/quarry` repo, cited here by module path as the reference.

**Layout.**
Quarry's shipped shape is a root façade package (`quarry/`), a separately-importable cgo-free leaf (`glyph/`), an `internal/` tree holding the CLI among other packages (`internal/{cgoguard,cli,engine,gitsrc,mcpserver,repopath}`), and its `cmd/` binaries (`cmd/quarry`, `cmd/quarry-mcp`).

**Façade mechanism.**
The façade re-exports internal types by alias rather than by hand-designed interface, so the extraction never required designing a narrow seam up front — it curated *what* is exported rather than *how*:

```go
type Symbol = engine.Symbol
const KindFunction = engine.KindFunction
```

The façade never had to invent a narrower method set than the engine already had, only decide which of the engine's types the façade's own names re-export.

**The evidence on extraction timing, stated plainly.**
This repo's `go.mod` still carries no requirement on `github.com/Knatte18/quarry`, and adoption is a single unstarted item in `manifest/roadmap.md`'s Planned section ("Adopt quarry's glyph alphabet as the plan alphabet").
Quarry shipped as a standalone repo months before this document was written, and loomyard — the only real candidate consumer — has still not drawn the dependency edge to it.
That is the measured cost of extracting before a consumer exists, and it is the one piece of evidence this document weighs most heavily against extracting Reed or Fabric now.

**Residue cost.**
The port's own record, `docs/research/quarry-holistic-fix-log.md`, documents real cleanup cost after the port: loomyard-internal references surviving in ported comments, a stale checksum file (`go.sum`), and cross-repo review rounds needing a two-repo authorization decision.
None of this is free, even for a clean extraction candidate.

**A caveat on this section's own verifiability.**
The layout and façade observations above were read off a local checkout of the quarry repo, not from anything in this repository.
That local-checkout read is the one block of evidence in this document no other reader can re-verify from this repo alone — it is not backed by a machine-local absolute path, and no claim in this document rests on one; a reader who wants to confirm the layout or façade shape must clone `github.com/Knatte18/quarry` and look themselves.

## Reed — the measured contract today

**Size.**
`internal/reedengine` is 6,615 production lines (17,474 with tests, measured by `wc -l` over non-`_test.go` files).
`internal/reedengine/render` adds 773 production lines (1,893 with tests).

**Direct internal imports.**
`configengine`, `lock`, `logger`, `lyxdirs`, `proc`, `shell`, `state`, `tokenvocab`, `reedengine/render` — from `internal/reedengine/doc.go` and `go list`.

**Transitive internal dependency set.**
`go list -deps ./internal/reedengine` adds `envsource`, `gitexec`, `lyxcwd`, `yamlengine`, `stencil`, `fsx` to the direct set above.
`lyxcwd` and `gitexec` enter *only* through `internal/logger` (`internal/logger/sink.go:21-22`, `:88-93`, where `LogsDir` takes a `*lyxcwd.Location` and `ensureDurableSink` falls back to `lyxcwd.Getwd()`/`Resolve()`).
`reedengine/doc.go` already documents this honestly and is quoted rather than re-derived: reed is told its geometry and derives none of it, so `internal/lyxcwd` is absent from reed's *direct* production imports even though it is present transitively.

**Public surface.**
7 free functions, 20 types, and `*Engine` with **17** exported methods and no value-receiver methods — `Socket`, `SessionName`, `TmuxPath`, `AddStrand`, `UpdateStrand`, `RemoveStrand`, `AttachArgv`, `SendText`, `SendKey`, `CapturePane`, `Up`, `Resume`, `Down`, `Status`, `Watch`, `HeaderText`, `ValidateHeader`.
Producing command: `go doc ./internal/reedengine Engine | grep -c '^func (e \*Engine)'`.

**External contract footprint.**
**13 distinct exported package-level identifiers** are referenced *in code* by production packages outside `reedengine`: `AddSpec`, `ConfigTemplate`, `Engine`, `Geometry`, `LoadConfig`, `LoadState`, `New`, `Removed`, `ServerName`, `SessionName`, `StatusResult`, `Strand`, `StrandStatus`.
The metric's definition is exactly that phrase — "referenced in code by production packages outside the module" — and it is stated here alongside the number because the same shape of count appears throughout this document.
This footprint counts `render`'s own exported names (`Display`, `Strand`, `Box`, `Params`, and the `Anchor` constants) and the 17 methods reached through `*Engine` separately from the 13 above, not folded into it.

A bare `grep -ro 'reedengine\.[A-Za-z0-9_]*'` over non-test files additionally returns three identifiers, none of which is a real external reference: `CleanClaudeEnv` and `AddStrand` appear only as doc-comment prose (`internal/burlerengine/doc.go:205`, `internal/reedcli/add.go:1,4`), and `requireSessionLocked` (`internal/reedcli/attach.go:52`) is not even exported.
Counting these three is how the first draft of the underlying investigation reached 15 external identifiers instead of 13.

**Direct production importers (9) and their consumer shapes.**
`burlercli`, `configreg`, `hubgeom`, `loomcli`, `reedcli`, `shuttlecli`, `shuttleengine`, `standalonegeom`, `webstercli` — transitive dependents: 26 packages.
Per-importer shape:

- `webstercli` already holds Reed behind the transport interface — `shuttleengine.ReedOps` (`internal/webstercli/wiring.go:220,226`).
- `burlercli` and `shuttlecli` construct and hand off: each calls `LoadConfig`+`New` and hands the engine to `shuttleengine.NewRunner`.
- `hubgeom` and `standalonegeom` build the told-geometry value: `Geometry` (plus `ServerName`/`SessionName`).
- `configreg` calls a config-template function only: `ConfigTemplate`.
- `loomcli` is the sole consumer *retaining* a concrete `*reedengine.Engine` as a struct field (`internal/loomcli/cli.go:44`), using `Up`, `Status`, `AddStrand`, `RemoveStrand`, `TmuxPath` and `AttachArgv` (`internal/loomcli/run.go:145-320`, `drive.go:64`).
- `reedcli` is Reed's own CLI and legitimately uses the whole surface.

**Geometry.**
`reedengine.Geometry` (`internal/reedengine/geometry.go`) is eight told string fields with a documented no-validation, no-derivation contract.
Its two constructors — `hubgeom.ReedGeometry(*lyxcwd.Location)` and `standalonegeom.ReedGeometry(target, stateDir, hash8)` — would neither move into a standalone Reed, since both are hub/standalone-layout tellers, not Reed's own concern.

**The one provider-specific name.**
`reedengine.CleanClaudeEnv` (`internal/reedengine/env.go`) is the one provider-specific name in Reed's public API, and it has no caller outside its own package (`internal/reedengine/lifecycle.go:299`).

```text
internal/reedengine/doc.go
internal/reedengine/env.go, lifecycle.go:299
internal/reedengine/geometry.go
internal/loomcli/cli.go:44
```

## Reed — the verdict, its trigger, and the cheap in-repo preparation

**Verdict: do not extract Reed now.**
This is an explicit, unhedged call with a stated trigger — not a "revisit later."

The gate is not code readiness — Reed is already close to ready, per the measured contract above.
The gate is the existence of a second consumer, or Reed becoming a long-running service rather than a library.

**Reasoning.**
Reed's contract is small enough to freeze: 13 external identifiers across 9 direct production importers, most of them construction-only, with one handle type carrying 17 methods.
But loomyard is Reed's only user, and the quarry precedent measures what extracting ahead of a consumer buys — a separate release cadence and a dependency edge that, months later, still is not drawn.

Three further Reed items sit in `manifest/roadmap.md`'s Someday section — `reed: cross-worktree columns`, `reed: own-window strand anchoring`, `reed: daemon Slack relay` — and all three are Someday, committed but unscheduled, **not Planned**.
They are a weak churn argument and must not be read as imminent change.
The verdict stands on the second-consumer gate alone; the Someday items are a secondary note that the surface is not finished, not the load-bearing reason.

The likelier trigger than "another project wants tmux orchestration" is the daemon story: watchdog plus relay plus mailbox together give Reed a lifecycle of its own, and that is when a repo boundary starts paying.

**Two rejected alternatives.**

- **Extract now** — rejected: there is no consumer, and the quarry precedent is the counter-evidence for extracting ahead of one.
- **Never extract** — rejected: the code genuinely is close to ready, and saying "never" discards a real option the second-consumer gate would open.

### Cheap in-repo preparation

Two in-repo changes are recommended as cheap hygiene independent of any extraction, each a roadmap candidate with its own standalone justification.

**(a) Rename the provider-specific environment-cleaning function.**
`reedengine.CleanClaudeEnv`'s name is the only Claude-specific identifier in the public API of a package the Shuttle Provider-Seam Invariant says never references provider specifics.
The depth is fixed here so the follow-up item need not invent it: a provider-neutral name taking the key set as a caller-supplied parameter,

```go
func StripEnvKeys(environ []string, exact []string, prefixes []string) (clean []string, stripped []string)
```

with the `CLAUDECODE`/`CLAUDE_CODE_` literals moving out of the function body into `internal/reedengine/lifecycle.go`'s single call site, which is where the provider knowledge belongs.
Only the identifier's final name is left to the follow-up item; the signature shape and the relocation of the Claude literals are not.

**(b) Give the one retaining consumer a named interface seam.**
`loomcli` is the only consumer that retains a concrete `*reedengine.Engine` as a struct field; give it a named interface seam so that no consumer retains the concrete type except Reed's own CLI.
Stated precisely: two other consumers, `burlercli` and `shuttlecli`, construct the engine and hold it transiently before handing it off, and a seam cannot change that — construction always yields the concrete type.
What (b) removes is the *retained field*, of which `internal/loomcli/cli.go:44` is the only instance.

The logging decoupling is deliberately **not** in this list — see the standalone-layout section below for why.

**Three rejected options for the preparation work.**

- Bundling (a) and (b) into the extraction itself — rejected: each stops being independently justifiable and stalls behind a decision with no trigger date.
- Doing nothing until a trigger fires — rejected: leaves two unrelated defects unfixed for no reason.
- Keeping the logging decoupling in this list — rejected: it is not cheap, unlike (a) and (b); see below for why.

## Reed — the standalone repo shape, its published seams, and the logging price

### Layout

A standalone Reed mirrors quarry's shipped shape directly: a root façade package, a separately-importable dependency-light render leaf, an `internal/` tree including the CLI, and one `cmd/` binary — `reed/` (façade), `render/` (leaf), `internal/` (everything else including the CLI), `cmd/reed`.

The render leaf is the exact analogue of quarry's cgo-free `glyph/` leaf: 773 production lines, zero internal dependencies, and stdlib-only imports.
Producing command: `for f in $(ls internal/reedengine/render/*.go | grep -v _test); do awk '/^import/,/^\)/' $f; done | grep '"'`.
That command's import set is exactly three stdlib packages — `fmt`, `sort`, `strings` (`sort` is used in `policy.go`, so shortening the list to "`fmt` and `strings`" is wrong) — and the leaf is already consumed independently by three packages: `loomcli`, `shuttleengine`, and `shuttlecli`.

The façade re-export shape, derived from quarry's alias mechanism:

```go
package reed

import "github.com/Knatte18/reed/internal/engine"

type Engine = engine.Engine
type Geometry = engine.Geometry
type AddSpec = engine.AddSpec
type Strand = engine.Strand

func New(cfg Config) (*Engine, error) {
	return engine.New(cfg)
}
```

### Published seams

A standalone Reed publishes **two** named interface slices with compile-time satisfaction proofs — a transport slice and a session-lifecycle slice — as the documented contract, while consumers stay free to define their own narrower interfaces.

The transport slice is `shuttleengine.ReedOps`, transcribed verbatim from `internal/shuttleengine/reed.go`:

```go
type Transport interface {
	AddStrand(spec reedengine.AddSpec) (reedengine.Strand, error)
	RemoveStrand(guid string, recursive bool) (reedengine.Removed, error)
	Status() (reedengine.StatusResult, error)
	SendText(guid, text string, submit bool) error
	SendKey(guid, key string) error
	CapturePane(guid string) (string, error)
}

var _ Transport = (*Engine)(nil)
```

The session-lifecycle slice is exactly the six methods `loomcli` calls today, derived by `grep -ohE 'c\.reed\.[A-Za-z]+\(' $(ls internal/loomcli/*.go | grep -v _test) | sort -u`:

```go
type Session interface {
	Up() error
	Status() (StatusResult, error)
	AddStrand(spec AddSpec) (Strand, error)
	RemoveStrand(guid string, recursive bool) (Removed, error)
	TmuxPath() (string, error)
	AttachArgv() ([]string, error)
}

var _ Session = (*Engine)(nil)
```

The two slices deliberately overlap on three methods — `AddStrand`, `RemoveStrand`, `Status` — and that overlap is permitted, not a defect.
A reader's first instinct is that two seams over one type should partition its methods; they do not.
Each slice states one consumer's real dependency, and a method two consumers both need legitimately appears in both.

Both slices are declared in the provider package — `reed` in a standalone repo, `reedengine` if the seam is retrofitted in-repo first — not in a consumer, because that placement is what makes the compile-time proof line live next to the type it constrains.

This document does **not** claim a wider lifecycle slice covering the four further session methods (`Down`, `Resume`, `SessionName`, `Socket`): the only consumer calling any of them is Reed's own CLI, which holds the concrete `*Engine` and needs no slice at all.

### The logging decoupling

A standalone Reed takes an optional standard-library structured logger on its config or constructor, defaulting to a discard handler, and drops `internal/logger` entirely.
`internal/logger` is the single edge that pulls three further packages — `lyxcwd`, `gitexec`, `lyxdirs` — into Reed's transitive dependency set.

**This is a standalone-repo design point, not in-repo preparation**, and its in-repo price is written out here rather than worked around.
The Live-Substrate Spawn Observability constraint (`CONSTRAINTS.md`) names `internal/logger` by name and is enforced mechanically, as a file-level import check, by `cmd/lyx/spawnobservability_test.go`: a production file under `internal/` or `cmd/` that contains a real `exec.Command`/`exec.CommandContext` call must either import `internal/logger` or carry a written-reason entry in `spawnObservabilityAllowedSpawners`.
Removing `internal/logger` from `reedengine` in-repo would therefore fail that shipped test unless three allowlist entries were added — one each for `lifecycle.go`, `overlay.go`, `attach.go`, the three files that carry real spawns — and `CONSTRAINTS.md` were amended to document the new exemptions, which this task's scope bars.

The allowlist's own first entry documents the very import cycle at issue, and is worth quoting directly:

> "structurally barred: internal/logger imports internal/lyxcwd, which imports internal/gitexec, so importing logger here would close an import cycle; gitexec.Run already returns a *GitError carrying args, dir, exit code, and stderr, so the diagnostic is not actually lost"

**Three rejected options.**

- A hand-rolled `Logger` interface — rejected: it reinvents `log/slog`'s own `Handler` interface for no benefit.
- Extracting `internal/logger` too — rejected: that is a second extraction with its own trace-id and retention machinery, to avoid one import.
- Listing the decoupling as cheap in-repo hygiene — rejected: it collides with a shipped enforcement test and a `CONSTRAINTS.md` amendment this task's scope bars, so it is not cheap.

## Fabric — the measured contract today

**Size.**
`internal/fabricengine` is 14,610 production lines (49,608 with tests, measured by `wc -l` over non-`_test.go` files) — the largest package in the repo.

**Direct internal imports.**
`configengine`, `fslink`, `gitexec`, `gitrepo`, `lock`, `logger`, `lyxcwd`, `lyxdirs`, `pattern`, `proc`, `state`, `stencilstore`, `weftname` — from `internal/fabricengine/doc.go` and `go list`.

**Public surface.**
Roughly 50 free functions, roughly 60 types, 7 constants, 6 sentinel errors, roughly 10 error types, and one handle type, `*Fabric`, with 26 methods.

**External contract footprint.**
**74 distinct exported identifiers** are referenced by production code outside the package, of which `fabriccli` alone accounts for 40.
The 15 direct production importers are `cmd/lyx`, `boardcli`, `boardengine`, `burlercli`, `configreg`, `fabriccli`, `hubforge`, `hubgeom`, `ideengine`, `landingshed`, `loomcli`, `mergeresolve`, `preflight`, `stencilcli`, `webstercli`.

**The core observation: the names are hub-layout vocabulary, not git-coordination vocabulary.**
The measured hub-layout identifiers on the public surface are `BoardDir`, `BoardDirName` (`"_board"`), `BoardWriteLockPath`, `HubSuffix` (`"-HUB"`), `HubPath`, `HubLogsDir`, `HubScratchDir`, `HubReservedNames`, `IsReservedHubName`, `PortalsDir`, `PortalLink`, `LauncherDir`, `WarpLyxLink`, `WarpLyxLinkHere`, `WarpBindingFileName` (`".lyx-warp"`), `StencilsDir`, `StencilBaseByStamp`, `CommitSeededStencils`.

33 exported signatures are parameterized on `*lyxcwd.Location`, a four-field type describing lyx's own directory model — `RepoName`, `HubPath`, `WorktreeName`, `AnchorRel` — so the API is parameterized on lyx's directory model, not on two git URLs.

**What genuinely is generic, per the package's own documentation.**
The hub-clone entry point taking two plain git URLs (`CloneHub`), the handle over two repository values (`Fabric`, holding `warp *gitrepo.Repo` and `weft *gitrepo.Repo`), the `Warp-SHA` commit trailer and its rebuildable correspondence index, the uniform `<branch>` / `<branch>-weft` branch-naming rule, and the two-sided commit/pull/push/merge surface.

**The two prompt-template package imports, named individually, because they land on opposite sides of the split card below draws.**

```text
internal/fabricengine/stencilhistory.go   -- stencilstore.RelPath, stencilstore.BodyHash
internal/fabricengine/pull.go:23          -- import; single code use at pull.go:464
internal/fabricengine/pull.go:423, :441   -- doc-comment prose, not use sites
internal/fabricengine/stencilcommit.go    -- imports neither
```

```text
grep -ln "loomyard/internal/pattern" $(ls internal/fabricengine/*.go | grep -v _test)
grep -ln "loomyard/internal/stencilstore" $(ls internal/fabricengine/*.go | grep -v _test)
```

`internal/stencilstore` enters through **`stencilhistory.go` alone**, on the hub-layout side, which is the expected placement.
`internal/pattern` enters through **`pull.go` alone**, which is a *pair-kernel* file, with exactly one code use, `pattern.PathspecFile`/`PathspecDir` at `pull.go:464`;
the two further occurrences at `pull.go:423` and `:441` are doc-comment prose naming the same identifiers and must not be cited as use sites.
`stencilcommit.go`, the file a reader would most likely assume puts stencil versioning in the engine, imports neither `pattern` nor `stencilstore` — it takes only `gitrepo`, `lock` and `lyxdirs` — so the widely-assumed "`stencilcommit.go`/`stencilhistory.go` put stencil versioning in the engine" framing is half wrong, and this document does not repeat it.

**Candidate travelling companions, and why none simply moves.**
`gitrepo` (1,614 production lines) and `gitexec` (134 production lines) are the two candidates that would have to accompany a pair-kernel extraction, and both have non-Fabric production consumers — `websterengine/gitwrap.go`, `landingshed/publish.go`, `gitrepo/push.go` — and `gitexec` is additionally pinned to `internal/lyxcwd` by the Cwd Resolution Invariant.
Neither could simply move.
The `gitkit`/`gitrepo`/`gitexec` "trio" framing itself is addressed in the corrections section above rather than restated here.

## Fabric — the verdict and the in-repo split recommendation

**Verdict: do not extract Fabric.**
This is explicit and unhedged, stated as a **size-and-shape mismatch rather than a "not yet"**.

What is generic is a paired-repo coordination kernel of roughly **6,275 production lines** living inside a 14,610-line engine, and getting it out is a rewrite-by-subtraction, not a move.

**The measured file partition — one defensible assignment, never a canonical answer.**
Pair-kernel side, 6,275 production lines over 28 files, of which the merge surface alone is 2,209 lines:

```text
wc -l internal/fabricengine/{clone,fabric,commit,commitweftpaths,pull,corrindex,trailer,branchname,weftgit,diff,status,ancestors,warpprobe,pushanchored,coalesce,checkout,snapshot,dirtiness,index,revert,merge,mergeerrors,mergeguards,mergelifecycle,mergepaths,mergestage,mergestate,mergestateactive}.go
```

Hub-layout-surface side, 8,321 production lines over 39 files:

```text
wc -l internal/fabricengine/{portals,launchers,launcher_content,boardweft,hubscratch,stencilcommit,stencilhistory,junction,junctionnames,warplayout,warpjunction,weftwiring,anchor,slug,topology,config,template,list,worktreelist,prune,remove,add,cleanup,reconcile,destroy,drift,spawn,origin,ready,hook,gitexclude,classify,refscanner,warpbinding,warpclean,warpforward,unwire,bolt,mutation,doc}.go
```

The two sum to 14,596 against the package's 14,610, the shortfall being a small number of files the partition does not assign.
The kernel side is still not shippable as-is, because those files carry most of the 33 `*lyxcwd.Location`-typed signatures — which is precisely why it is a rewrite-by-subtraction.

**The consequence the discussion requires, because it cuts against the split's own rationale: the pair kernel is not domain-free.**
The `pattern` import identified above exists to populate `PullResult.PatternResidue`, a report naming which post-anchor weft commits touch `_lyx/PATTERN.md`/`_lyx/pattern/` and therefore need review after a warp history rewrite — a loomyard-domain feature living in the most generic-looking file in the package.
`internal/pattern` itself depends on `lyxdirs`, `stencilstore` and `stencil`, so the edge drags the whole prompt-template subtree into the kernel with it.

The honest conclusion: the split isolates the stencil-versioning coupling cleanly on the hub-layout side and does **not** isolate the pattern coupling, which would have to be cut separately by making residue reporting a caller-supplied predicate rather than a package import.
This is a further argument for the no-extract verdict rather than against it.

**Two rejected alternatives.**

- Extract the whole engine — rejected: it ships lyx's hub layout, board, portals, launchers and prompt-stencil versioning as someone else's public API.
- "Defer, revisit later" with no verdict — rejected: the measurements support a genuine call, and a genuine call is what this document exists to make.

### Recommendation: an in-repo split, not tied to any extraction

The recommendation, offered as a roadmap candidate with its own justification, is to split Fabric's surface *inside the repo* into two named halves — a pair kernel and a hub-layout surface.
This is explicitly **not** tied to any extraction.

**Depth, fixed here so the follow-up item need not guess.**
The split is file grouping inside the single package, plus a matching section split in the package's own documentation file (`doc.go`), plus an enforcement test asserting which files may reference which — and deliberately **not** sub-packages.

**Why sub-packages are ruled out by the package's own design, not by taste.**
The `Fabric` handle holds unexported `warp *gitrepo.Repo` and `weft *gitrepo.Repo` fields the package documentation says are reachable only from inside the package, and every hub-layout verb reaches them.
A sub-package boundary would force exporting both and hand every caller exactly the uncoordinated single-sided access the Fabric Git Invariant exists to prevent.

Naming beyond the two half-names is left to the follow-up item.
What this document fixes is the boundary and the mechanism, because those are what determine whether the item is worth picking up.

**Justification, on the split's own terms.**
It makes the Fabric Git Invariant's perimeter visible, and it is the only route by which the pair kernel would ever become extractable.

**Two rejected options.**

- Recommending nothing — rejected: it leaves a real structural observation unrecorded.
- Recommending the split as extraction step 1 — rejected: it couples a justified refactor to an unjustified goal.

## creel — where strand messaging lives

This section decides where the strand-messaging concept lives, answering placement, addressing, message model and sender universe — the four questions the task named — plus transport, and stating what it deliberately leaves unspecified.

**Placement: a sibling module named `creel`, not part of Reed and not dropped.**
Reed's own package documentation states its contract as the "dumb carrier" for its caller's strand data: it stores every field a caller writes into a strand and reads none of them semantically, and there is deliberately no domain `type` field on a strand.
A mailbox must read addresses semantically, so putting it inside Reed contradicts the one invariant Reed states about itself.

The structural argument is the same conclusion from a different angle: Reed's geometry type carries a worktree root, a repository name and a hub path, but no slug, no branch and no role, so the addressing a mailbox needs lives in webster's and loom's state, not Reed's.

This is not "drop it": `manifest/roadmap.md`'s Someday section already carries `reed: daemon Slack relay`, which is bidirectional messaging by another name and would be `creel`'s first non-agent sender.

**Addressing: `<worktree-slug>/<role>`, optionally with a round, resolved to a live strand identifier only at delivery time.
An identifier is never an address.**
The measured lifetime justifies this: the engine mints a fresh 128-bit random identifier on every strand add, and webster re-mints on every batch respawn — removing the prior strand by its persisted identifier and adding a fresh one.
An identifier therefore names an *incarnation*, and an address must outlive one.

The durable semantic identity already exists on both sides — in webster's per-role batch state and in the shuttle run record — and resolution at delivery time uses those records plus a liveness query.
An address that resolves to nothing leaves the message queued, which is the point of a mailbox rather than a pipe.

Two rejected addressing schemes:

- Identifier addressing — rejected: it dies at every respawn.
- tmux pane id — rejected: it is server-global and restarts at `%0` on every server rebirth.

**Message model: a durable FIFO inbox per address;
delivery appends a message and sends one keystroke notification;
the recipient reads on its own turn boundary.
No interrupt tier.**
Interruption already exists as a separate, synchronous, explicitly-authorized operation sitting on the engine's key-send method.
Folding it into the mailbox would make every queued message a potential context-destroying interrupt the recipient cannot distinguish from an ordinary one, while the sender universe is deliberately open — so the authority to interrupt would be granted to anyone who can write a file.
Keeping the two separate means the module needs no authorization model beyond filesystem permissions.

Two rejected models:

- A two-tier model with an interrupt severity — rejected: it grants an open sender set the power to destroy an agent's context.
- Polling with no notification — rejected: a recipient blocked on a tool call never checks.

**Transport and sender universe: a directory of JSON files under the hub's scratch area, plus a CLI send verb for humans and non-Go processes, plus a Go API for in-process senders.
No MCP server, no JSON-RPC, no socket.**
This follows the task's own principle that a wire-format protocol earns its cost only across a real OS-process boundary with no shared substrate;
here the filesystem is that shared substrate and is already this project's idiom for exactly this kind of state.
It delivers the open sender universe for free: any process that can write a file and knows an address is a sender, with no client library, no schema negotiation and no daemon required.

Two rejected transports:

- A protocol server (MCP or JSON-RPC) — rejected: it pays a protocol cost for a boundary that does not exist.
- In-memory queues — rejected: they lose everything on restart, defeating a mailbox whose recipients respawn.

**Illustrative sketch — no shipped source, so this listing is not a measured or verifiable contract:**

```go
type Message struct {
	To        string    // address: <worktree-slug>/<role>[/<round>]
	From      string    // sender identity, opaque to creel
	Body      string
	SentAt    time.Time
	MessageID string // opaque, sender- or creel-assigned
}
```

**Naming rationale.**
A creel is the rack holding one bobbin per feed position, which is the same object as a rack of per-address inboxes.
The name is verified unused in this repo, as are seven other loom-vocabulary candidates the discussion lists: `heddle`, `temple`, `pirn`, `bobbin`, `selvedge`, `sley`, `beam`.
This is a name proposed by a design document, not a rename instruction for any existing identifier.

**What this section deliberately stops short of.**
A full module design, which would need the durable address registry designed against webster's and loom's state — a task of its own.
This card does not create a module directory, a config-registry entry or a CLI verb.
