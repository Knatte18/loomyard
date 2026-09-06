# Discussion: Reed and Fabric as standalone modules: public API design

```yaml
task: 'Reed and Fabric as standalone modules: public API design'
slug: reed-fabric-standalone-api-design
status: discussing
parent: main
```

## Problem

Two lyx modules keep getting described as "probably extractable": `internal/reedengine` (Reed — the tmux pane/process overlay) and `internal/fabricengine` (Fabric — the warp↔weft git-coordination mechanism).
Neither claim has ever been tested against the actual code.
The question is not "could a Go package be moved into another repo" — anything can be moved — but "what would each module's public contract have to be for that repo to be usable by someone who is not loomyard, and is paying that price worth it".

Why now: the repo has one completed extraction to learn from (`lyx scout` → the standalone `quarry` repo), and the lesson is uncomfortable — quarry shipped months ago and loomyard's `go.mod` still carries no dependency on it;
its adoption is merely a Planned roadmap item.
That is the measured cost of extracting before a consumer exists, and it should be weighed before a second module goes the same way.
A related open question rides along: a strand-to-strand messaging/mailbox concept has been floated as "part of Reed's ecosystem", and where it lives has to be settled before either module's contract can be called final.

The deliverable is a design document, not code.

## Scope

**In:**

- One new design document, `manifest/designs/reed-fabric-standalone-api.md`, covering both modules.
- Per module: the measured contract surface today, the public API contract it would need as its own repo, the consumer-seam design, and an explicit extract-or-not verdict with timing.
- A placement decision for the strand-mailbox concept, with addressing, message model, transport, and sender universe.
- A one-paragraph note on whether the same lens is worth applying to other loomyard modules.
- Recommended follow-up roadmap candidates, named in the doc as recommendations only.

**Out:**

- Any change to `internal/reedengine`, `internal/fabricengine`, or any consumer package.
  No interface is introduced, no import is rewritten, no package is moved.
- Any edit to `manifest/roadmap.md`.
  Whether a recommendation becomes a Planned or Someday item is a separate decision after the doc exists, per the task's own framing.
- Any change to `CONSTRAINTS.md`.
  The doc records that the `gitkit` Leaf Invariant is unaffected (finding below), which is an observation, not a new invariant.
- Building the mailbox/`creel` module, or any part of it.
- A second full investigation of `fslink`/`yamlengine`/`githubclient`.

## Decisions

### deliverable-is-one-combined-doc

- Decision: one document, `manifest/designs/reed-fabric-standalone-api.md`, with a shared front half and a per-module back half — not `reed-standalone-api.md` plus `fabric-standalone-api.md`.
- Rationale: the material that is genuinely shared is more than half the content — the extraction rubric, the quarry precedent, the `logger`→`lyxcwd` transitive-dependency finding (which hits both), and the central finding that a Go module's frozen contract is its exported *type set*, not an interface.
  Two documents would either duplicate all of it or need a third to hold it.
  The filename matches the task slug, which is how the other design docs are named.
- Rejected: two per-module docs (duplication, and the two verdicts are only legible next to each other);
  three docs with a shared preamble (nothing in `manifest/designs/` is structured that way).

### doc-belongs-in-manifest-designs-despite-shipped-modules

- Decision: the doc goes under `manifest/designs/` even though Reed and Fabric are both shipped modules.
- Rationale: `docs/overview.md`'s Documentation Lifecycle scopes `manifest/designs/<module>.md` to *planned, not-yet-built* work, deleted when the module lands.
  This doc describes unbuilt work — a possible future extraction and its prerequisites — not the shipped modules, whose design already lives in their package doc comments (`internal/reedengine/doc.go`, `internal/fabricengine/doc.go`).
  It is therefore a planned-work doc that happens to be named after two existing modules, and it is deleted if and when the extraction it describes either lands or is formally abandoned.
- Rejected: `docs/research/` (that directory holds transient port/fix logs, not design);
  `contracts/specs/` (reserved for cross-module schemas a real consumer honors — there is no consumer).

### the-frozen-contract-is-the-type-set-not-an-interface

- Decision: the doc's central framing is that for a Go module the extraction contract is the set of exported identifiers consumers reference, and that consumer-side interfaces narrow *coupling* and buy testability but do not narrow the contract at all.
- Rationale: `shuttleengine.ReedOps` is held up as the model narrow seam, but every one of its six methods takes or returns a `reedengine` type (`AddSpec`, `Strand`, `Removed`, `StatusResult`).
  A consumer behind that interface still pins four exported types.
  The background framing — that Fabric's blocker is "no interface seam anywhere on the consumer side" — therefore misdiagnoses the problem: adding fifteen interfaces to Fabric's consumers would not shrink its 74-identifier contract by one name.
  What shrinks a contract is deleting or unexporting identifiers, or moving them behind a façade that re-exports a curated subset.
- Rejected: treating interface count as the readiness metric (measures the wrong thing);
  treating raw importer count as the metric (Reed and Fabric have nearly identical transitive-dependent counts, 26 and 27, and are nowhere near equally extractable).

### reed-verdict-defer-and-gate-on-a-second-consumer

- Decision: do not extract Reed now.
  The gate is not code readiness — Reed is already close to ready — it is the existence of a second consumer, or Reed becoming a long-running service rather than a library.
  Recommend the cheap in-repo preparation instead.
- Rationale: Reed's external contract is 13 package-level identifiers across 9 direct production importers, most of them construction-only, and its public surface is one handle type with 17 methods.
  That is small enough to freeze.
  But loomyard is its only user, three roadmap items still change it (`reed: cross-worktree columns`, `reed: own-window strand anchoring`, `reed: daemon Slack relay`), and the quarry precedent shows what extracting ahead of a consumer buys: a separate release cadence and a dependency edge that, months later, still is not drawn.
  The likelier trigger than "another project wants tmux orchestration" is the daemon story — watchdog plus Slack relay plus mailbox turns Reed into something with a lifecycle of its own, and that is when a repo boundary starts paying.
- Rejected: extract now (no consumer, active churn, quarry's outcome is the counter-evidence);
  never extract (the code genuinely is close, and saying "never" discards a real option).

### reed-preparation-work-is-worth-doing-regardless

- Decision: the doc recommends three in-repo changes as good hygiene independent of any extraction, each a roadmap candidate: (a) decouple `logger`, (b) rename the provider-specific `CleanClaudeEnv`, (c) give `loomcli` a named interface seam so no consumer holds `*reedengine.Engine` concretely except Reed's own CLI.
- Rationale: each has a standalone justification.
  (a) `logger` is the sole reason `lyxcwd` and `gitexec` appear in `go list -deps ./internal/reedengine` at all — Reed's own doc comment already admits this honestly — and Reed is the module the Told-Geometry Invariant is proudest of.
  (b) `reedengine.CleanClaudeEnv` is a Claude-provider-specific name in the public API of a package the Shuttle Provider-Seam Invariant says never references Claude specifics;
  it is the only such name and the fix is a rename plus a parameter.
  (c) `webstercli` already holds Reed as `shuttleengine.ReedOps`, `burlercli` and `shuttlecli` construct and hand off, and only `loomcli` holds the concrete `*Engine` — for six methods Reed could publish as a second named slice.
  Doing these makes a later extraction a mechanical move, which is exactly the "prototype in-repo first" posture quarry's ancestor had.
- Rejected: bundling them into the extraction (they stop being independently justifiable and stall behind a decision with no trigger date);
  doing nothing until the extraction is triggered (leaves three unrelated defects unfixed).

### fabric-verdict-do-not-extract-and-say-why-precisely

- Decision: do not extract Fabric, and state the reason as a size-and-shape mismatch rather than a "not yet".
  What is generic is a roughly 2–3k-loc paired-repo coordination kernel living inside a 14.6k-loc engine;
  getting it out is a rewrite-by-subtraction, not a move.
- Rationale: measured, not assumed.
  Fabric's external contract is 74 identifiers across 15 direct production importers, and the names themselves are hub-layout vocabulary, not git-coordination vocabulary: `BoardDir`, `BoardDirName` (`"_board"`), `HubSuffix` (`"-HUB"`), `HubReservedNames`, `HubScratchDir`, `PortalsDir`, `PortalLink`, `LauncherDir`, `WarpLyxLink`, `StencilsDir`, `StencilBaseByStamp`, `CommitSeededStencils`.
  33 exported functions take `*lyxcwd.Location`, a four-field struct describing lyx's hub layout — so the API is parameterized on lyx's directory model, not on two git URLs.
  `CloneHub(cwd, opts)` genuinely is generic;
  almost nothing else on the surface is.
  On top of that, `fabricengine` directly imports `internal/pattern` and `internal/stencilstore` — LLM prompt-template packages — because `stencilcommit.go`/`stencilhistory.go` put stencil versioning inside the git-coordination engine.
  A standalone "paired git repos" library cannot ship those.
- Rejected: extract the whole engine (ships lyx's hub layout, board, portals, launchers and prompt-stencil versioning as someone else's public API);
  "defer, revisit later" with no verdict (the task asked for a genuine call, and the call is available from the measurements).

### fabric-recommendation-is-an-in-repo-seam-not-a-repo-boundary

- Decision: the doc recommends, as a roadmap candidate with its own justification, splitting Fabric's surface *inside the repo* into a named generic kernel and a named hub-layout half — and explicitly does not tie that recommendation to any extraction.
- Rationale: the split has value on its own terms.
  It makes the Fabric Git Invariant's actual perimeter visible, it isolates the stencil/pattern coupling as a thing to be questioned rather than a thing to be inherited, and it is the only route by which the generic kernel would ever become extractable.
  Framing it as extraction prep would make it hostage to a trigger that will probably never fire.
- Rejected: recommending nothing (leaves a real structural observation unrecorded);
  recommending the split as extraction step 1 (couples a justified refactor to an unjustified goal).

### gitkit-does-not-travel-with-fabric

- Decision: record as a correction that the `gitkit`/`gitrepo`/`gitexec` "trio" framing is wrong, and that the `gitkit` Leaf Invariant is untouched by anything in this doc.
- Rationale: `internal/gitkit` is not in `go list -deps ./internal/fabricengine` at all.
  Its only non-test production importer in the whole repo is `internal/hubforge/seed.go`;
  every other reference is from `_test.go` files.
  gitkit is test-fixture machinery, which is exactly what the hubforge Fabric-Fixture Invariant describes.
  What would actually travel with a Fabric kernel is `gitrepo` (1,614 loc) and `gitexec` (134 loc) — and both have non-Fabric production consumers (`websterengine/gitwrap.go`, `landingshed/publish.go`, and `lyxcwd` itself, which the Cwd Resolution Invariant pins to `gitexec`), so neither could simply move either.
- Rejected: repeating the background's trio claim (it is measurably false, and the doc's credibility rests on its measurements).

### no-reverse-dependency-from-lyxcwd-to-fabricengine

- Decision: record explicitly that `internal/lyxcwd` does *not* import `internal/fabricengine`, so there is no import cycle to break.
- Rationale: a naive `grep -rl "internal/fabricengine"` lists `internal/lyxcwd/lyxcwd.go` and `internal/lyxcwd/anchor.go`, but both matches are doc-comment prose.
  `go list` confirms `lyxcwd`'s only internal import is `gitexec`, exactly as the Cwd Resolution Invariant requires.
  The doc states this because it is the obvious thing a reader would suspect and the obvious way to get it wrong.
- Rejected: omitting it (a future reader re-runs the same grep and reaches the wrong conclusion).

### reed-standalone-layout-mirrors-quarry

- Decision: the doc specifies Reed's hypothetical repo layout as a direct mirror of quarry's shipped shape — a root `reed/` façade package, a separately-importable dependency-light `render/` leaf, `internal/` for everything else including the CLI, and `cmd/reed`.
- Rationale: quarry is this project's one completed extraction and its layout is the tested answer.
  Its `quarry/` package is a curated façade that re-exports `internal/engine` types by alias (`type Symbol = engine.Symbol`), which means the extraction never required hand-designing a narrow interface up front — it curated *what* is exported rather than *how*.
  Its `glyph/` package is a cgo-free leaf published separately for consumers who want only the identifier grammar.
  `internal/reedengine/render` is already the exact analogue: 773 production lines, zero internal dependencies, imports only `fmt` and `strings`, and already consumed independently by `loomcli`, `shuttleengine` and `shuttlecli`.
- Rejected: one flat package (loses the render leaf, which is the best-shaped piece);
  separate engine and CLI repos (quarry keeps `internal/cli` in-repo and there is no reason to differ).

### reed-publishes-named-slices-consumers-may-still-narrow

- Decision: a standalone Reed publishes two named interface slices with compile-time satisfaction proofs — a transport slice and a session-lifecycle slice — as the documented contract, while consumers stay free to define their own narrower interfaces.
- Rationale: publishing them is what "a frozen contract to verify against" means for a versioned module, and the proof line (`var _ Transport = (*Engine)(nil)`) is what makes a breaking change fail the build in the provider's own repo rather than in a consumer's.
  The slices are read off real usage, not invented: the transport slice is `shuttleengine.ReedOps` verbatim (`AddStrand`, `RemoveStrand`, `Status`, `SendText`, `SendKey`, `CapturePane`), and the session slice is exactly the six methods `loomcli` calls today — `Up`, `Status`, `AddStrand`, `RemoveStrand`, `TmuxPath`, `AttachArgv`, derived by `grep -ohE 'c\.reed\.[A-Za-z]+\(' $(ls internal/loomcli/*.go | grep -v _test) | sort -u`.
  The two slices deliberately overlap on `AddStrand`, `RemoveStrand` and `Status`;
  the doc must say so explicitly and state that overlap is permitted, because a reader's first instinct is that two seams over one type should partition its methods.
  They do not: each slice is a statement about one consumer's real dependency, and a method two consumers both need appears in both.
  Both slices are declared in the provider package (`reed`, in a standalone repo;
  `reedengine` if the seam is retrofitted in-repo first), not in a consumer — that placement is what makes the compile-time proof line live next to the type it constrains.
  The doc must NOT claim a wider lifecycle slice (`Down`, `Resume`, `SessionName`, `Socket`): `loomcli` calls none of the four, and the only consumer that does is `reedcli`, Reed's own CLI, which legitimately holds the concrete `*Engine` and needs no slice at all.
  Keeping consumer-side definition legal preserves Go's accept-interfaces idiom and is what `shuttleengine` already does.
- Rejected: provider-side interfaces as the only permitted form (fights the language's idiom for no gain);
  consumer-side only (a versioned module with no published interface has nothing to prove compatibility against).

### reed-logger-decoupling-is-an-injected-slog-logger

- Decision: a standalone Reed takes an optional `*slog.Logger` on `Config` or `New`, defaulting to a discard handler, and drops `internal/logger` entirely.
- Rationale: `internal/logger` is the single edge that pulls `lyxcwd`, `gitexec` and `lyxdirs` into Reed's transitive dependency set — `logger/sink.go` exposes `LogsDir(*lyxcwd.Location)` and falls back to `lyxcwd.Getwd()`+`Resolve()`.
  `log/slog` is stdlib, so the swap adds no dependency, and loomyard keeps its own logging by passing a handler that writes where `internal/logger` writes today.
  The Live-Substrate Spawn Observability invariant is satisfied by the injected logger exactly as it is by the package-level one — the invariant requires that spawns are logged, not which package logs them.
- Rejected: a hand-rolled `Logger` interface (reinvents `slog.Handler`);
  extracting `internal/logger` too (a second extraction with its own trace-id and retention machinery, to avoid one import).

### mailbox-lives-in-a-sibling-module-named-creel

- Decision: the strand-messaging concept is a sibling module, `internal/creelengine` (module name `creel`), not part of Reed and not dropped.
- Rationale: Reed's package doc states its own contract as the "dumb carrier" for its caller's strand data — it stores every field a caller writes and reads none of them semantically, and there is deliberately no domain `type` field on a strand.
  A mailbox must read addresses semantically, so putting it inside Reed contradicts the one invariant Reed states about itself.
  Reed also structurally lacks the vocabulary: `reedengine.Geometry` carries `WorktreeRoot`, `RepoName` and `HubPath` but no slug, no branch, no role — the addressing a mailbox needs lives in webster's and loom's state, not Reed's.
  And the concept is not "drop it": the roadmap already carries `reed: daemon Slack relay`, which is bidirectional messaging by another name and would be creel's first non-agent sender.
- Rejected: inside Reed (breaks its stated contract, and Reed has none of the addressing vocabulary);
  drop it (a sibling on the roadmap already needs the same machinery).
- Naming: a creel is the rack that holds one bobbin per feed position, feeding the loom — a rack of per-address inboxes is the same object.
  Verified unused anywhere in the repo, as are `heddle`, `temple`, `pirn`, `bobbin`, `selvedge`, `sley` and `beam`.
  This is a name proposed by a design doc, not a rename instruction for any existing identifier.

### creel-addressing-is-slug-plus-role-never-a-guid

- Decision: a creel address is `<worktree-slug>/<role>` (optionally `/<round>`), resolved to a live strand GUID only at delivery time.
  A GUID is never an address.
- Rationale: `reedengine.newGUID` mints a fresh 128-bit random identifier on every `AddStrand`, and webster re-mints on every batch respawn — `beginbatch.go` and `recoverbatch.go` both call `removeStrandIfLive(deps.Reed, prior.StrandGUID)` and then add a new strand.
  A GUID therefore names an *incarnation*, and an address must outlive one.
  The durable semantic identity already exists on both sides: webster persists `BatchState.StrandGUID` keyed by role and batch number, and `shuttleengine.RunState` (`run.json`) records `StrandGUID` alongside `RunID` and `SessionID`.
  Resolution at delivery time uses those records plus `Engine.Status()` for liveness;
  an address that resolves to nothing leaves the message queued, which is the entire point of a mailbox rather than a pipe.
- Rejected: GUID addressing (dies at every respawn — the wrinkle the task brief already named);
  tmux pane id (server-global, restarts at `%0` on every server rebirth, which is why `ReedState.PaneGeneration` exists at all).

### creel-is-notify-only-with-no-interrupt-tier

- Decision: a durable FIFO inbox per address;
  delivery appends a message and sends one keystroke notification;
  the recipient reads on its own turn boundary.
  No interrupt tier.
- Rationale: interruption already exists as a separate, synchronous, explicitly-authorized operation — `shuttleengine.Runner.Interrupt(guid)`, sitting on `reedengine.Engine.SendKey`.
  Folding it into the mailbox would make every queued message a potential context-destroying interrupt that the recipient cannot distinguish from an ordinary one, and the sender universe is deliberately open, so the authority to interrupt would be granted to anyone who can write a file.
  Keeping the two separate means creel needs no authorization model beyond filesystem permissions.
- Rejected: a two-tier model with an interrupt severity (grants an open sender set the power to destroy an agent's context);
  polling with no notification (a recipient blocked on a tool call never checks).

### creel-transport-is-on-disk-plus-a-cli-verb-no-wire-protocol

- Decision: the wire is a directory of JSON files under the hub's scratch area, plus a `lyx creel send` CLI verb for humans and non-Go processes, plus a Go API for in-process senders.
  No MCP server, no JSON-RPC, no socket.
- Rationale: this follows the task's own stated principle — a wire-format protocol earns its cost only across a real OS-process boundary with no shared substrate — and here there *is* a shared substrate: the filesystem, which every sender can already reach and which is this project's existing idiom for exactly this (`internal/state`, `internal/lock`, `run.json`, reed's own `reed.json`).
  It also delivers the open sender universe for free: any process that can write a file and knows an address is a sender, with no client library, no schema negotiation and no daemon to be running.
  Reed's `Engine.SendText`/`SendKey` are the notification half and need no new plumbing.
- Rejected: an MCP or JSON-RPC server (pays a protocol cost for a boundary that does not exist, and makes every sender a client);
  in-memory queues (lose everything on restart, which defeats a mailbox whose recipients respawn).

### creel-is-sketched-not-specified

- Decision: the doc gives creel a section — placement, addressing, message model, transport, sender universe, and its relationship to the existing Slack-relay roadmap item — and explicitly stops short of a full module design.
- Rationale: the task asked for a placement decision with reasoning, not a third module design.
  A full spec would need the durable address registry designed against webster's and loom's state, which is a task of its own.
- Rejected: a complete design (out of scope, and would dwarf the two module sections);
  a bare one-liner (the task named addressing, message model and sender universe as things to decide).

### other-modules-are-a-note-not-a-follow-up-task

- Decision: one short section noting that `internal/fslink`, `internal/yamlengine` and `internal/githubclient` are clean leaves but poor extraction candidates, and that `internal/reedengine/render` is the best-shaped candidate in the repo — with no follow-up task proposed.
- Rationale: the economics do not work at that size.
  `fslink` is 410 production lines with zero internal dependencies and five consumers;
  `yamlengine` is smaller with four;
  `githubclient` depends on `proc` and wraps the `gh` binary, and exists to satisfy the GitHub Auth Invariant rather than to be reusable.
  A separate repo, module path, release cadence and CI for 400 lines is never worth it absent an external consumer, and the same second-consumer gate that defers Reed defers all three a fortiori.
  `render` is worth naming because it is already stdlib-only and pure, and it is the piece that would ship first if Reed ever ships.
- Rejected: a follow-up survey task (the survey's answer is already "no", and the doc can say so in a paragraph);
  omitting the section (the task explicitly asked for the flag).

### doc-carries-real-go-listings

- Decision: the doc includes real Go code blocks — the two proposed Reed interface slices, the proposed façade re-export shape, and the creel message struct — rather than describing them in prose.
- Rationale: this is an API-design document;
  its whole content is signatures.
  Prose descriptions of a method set are strictly less checkable than the method set.
  Every listed signature is transcribed from the shipped code or derived from it, never invented, so a reader can verify each against the source.
- Rejected: prose only (unverifiable, and would make the doc longer, not shorter).

## Technical context

Everything below was measured in this worktree at `ca388c0ce`, with `go list`, `go doc` and `grep`.
The numbers should be re-stated in the doc as measured facts with their method named, because several of them correct the task brief's background notes.

### Reed — measured

- Size: `internal/reedengine` is 6,615 production lines (17,474 with tests);
  `internal/reedengine/render` adds 773 production lines (1,893 with tests).
- Direct internal imports: `configengine`, `lock`, `logger`, `lyxdirs`, `proc`, `shell`, `state`, `tokenvocab`, `reedengine/render`.
- Transitive internal dependency set (`go list -deps`): the above plus `envsource`, `gitexec`, `lyxcwd`, `yamlengine`, `stencil`, `fsx`.
  `lyxcwd` and `gitexec` enter *only* through `internal/logger` (`internal/logger/sink.go:21-22`, `:88-93`).
  `reedengine/doc.go` already documents this honestly and should be quoted rather than re-derived.
- The brief's claim that Reed imports `tokenvocab` is correct (`geometry.go`, `header.go`, `headertemplate.go`), and `tokenvocab` pulls in `internal/stencil`.
- Public surface: 7 free functions, 20 types, and `*Engine` with **17** exported methods — `Socket`, `SessionName`, `TmuxPath`, `AddStrand`, `UpdateStrand`, `RemoveStrand`, `AttachArgv`, `SendText`, `SendKey`, `CapturePane`, `Up`, `Resume`, `Down`, `Status`, `Watch`, `HeaderText`, `ValidateHeader`, and no value-receiver methods.
  Producing command: `go doc ./internal/reedengine Engine | grep -c '^func (e \*Engine)'`.
- External contract footprint: **13 distinct exported package-level identifiers** referenced *in code* by production packages outside `reedengine` — `AddSpec`, `ConfigTemplate`, `Engine`, `Geometry`, `LoadConfig`, `LoadState`, `New`, `Removed`, `ServerName`, `SessionName`, `StatusResult`, `Strand`, `StrandStatus`.
  Plus `render`'s `Display`, `Strand`, `Box`, `Params` and the `Anchor` constants, and the 17 methods reached through `Engine`, counted separately above.
- The metric is deliberately "referenced in code by production packages", and the doc must state that definition alongside the number.
  A bare `grep -ro 'reedengine\.[A-Za-z0-9_]*'` over non-test files also returns `CleanClaudeEnv`, `AddStrand` and `requireSessionLocked`, all three of which are **doc-comment prose only** — `internal/burlerengine/doc.go:205`, `internal/reedcli/add.go:1,4`, `internal/reedcli/attach.go:52` — and `requireSessionLocked` is not even exported.
  None is a real external reference, and counting them is how the first draft of this discussion reached 15 instead of 13.
  The same trap produced the false `lyxcwd`→`fabricengine` edge recorded below;
  the doc should name comment-prose contamination once, as a shared caveat on its grep-derived numbers.
- `CleanClaudeEnv` is therefore exported but referenced only inside `reedengine` (`lifecycle.go:299`).
  That does not weaken the rename recommendation — it is still a provider-specific name on a public API — it only means the rename breaks no external caller, which strengthens it.
- Direct production importers (9): `burlercli`, `configreg`, `hubgeom`, `loomcli`, `reedcli`, `shuttlecli`, `shuttleengine`, `standalonegeom`, `webstercli`.
  Transitive dependents: 26 packages.
- Consumer shapes, per grep of each importer: `webstercli` already holds Reed as `shuttleengine.ReedOps` (`internal/webstercli/wiring.go:220,226`);
  `burlercli` and `shuttlecli` only call `LoadConfig`+`New` and hand the engine to `shuttleengine.NewRunner`;
  `hubgeom` and `standalonegeom` build `Geometry` (plus `ServerName`/`SessionName`);
  `configreg` calls `ConfigTemplate` only;
  `loomcli` is the sole consumer holding a concrete `*reedengine.Engine` (`internal/loomcli/cli.go:44`), using `Up`, `Status`, `AddStrand`, `RemoveStrand`, `TmuxPath` and `AttachArgv` (`internal/loomcli/run.go:145-320`, `drive.go:64`);
  `reedcli` is Reed's own CLI and legitimately uses the whole surface.
- `reedengine.Geometry` (`internal/reedengine/geometry.go`) is eight told string fields with a documented no-validation, no-derivation contract.
  The two constructors are `hubgeom.ReedGeometry(*lyxcwd.Location)` and `standalonegeom.ReedGeometry(target, stateDir, hash8)`;
  neither would move into a standalone Reed.
- `reedengine.CleanClaudeEnv` (`internal/reedengine/env.go`) is the one provider-specific name in Reed's public API;
  its only production caller is `internal/reedengine/lifecycle.go:299`.
  The Shuttle Provider-Seam Invariant says `reedengine` never references Claude specifics.
- `newGUID` (`internal/reedengine/name.go`) is `crypto/rand` 128-bit hex, minted per `AddStrand`.
  Respawn re-mints: `internal/websterengine/beginbatch.go:249-250` and `recoverbatch.go:143` remove the prior strand by its persisted GUID and add a fresh one.

### Fabric — measured

- Size: `internal/fabricengine` is 14,610 production lines (49,608 with tests) — the largest package in the repo.
- Direct internal imports: `configengine`, `fslink`, `gitexec`, `gitrepo`, `lock`, `logger`, `lyxcwd`, `lyxdirs`, `pattern`, `proc`, `state`, `stencilstore`, `weftname`.
- `internal/gitkit` is **not** a dependency, transitively or directly.
  Its only non-test production importer anywhere is `internal/hubforge/seed.go`.
- Candidate travelling companions and their sizes: `gitrepo` 1,614 production lines, `gitexec` 134, `fslink` 410, `weftname` 35.
  `gitrepo` and `gitexec` both have non-Fabric production consumers (`websterengine/gitwrap.go`, `landingshed/publish.go`, `gitrepo/push.go`, and `lyxcwd` for `gitexec`).
- Public surface: ~50 free functions, ~60 types, 7 constants, 6 sentinel errors and ~10 error types, plus `*Fabric` with 26 methods.
- External contract footprint: **74 distinct exported identifiers** referenced by production code outside the package.
  `fabriccli` alone accounts for 40 of them.
- Direct production importers (15): `cmd/lyx`, `boardcli`, `boardengine`, `burlercli`, `configreg`, `fabriccli`, `hubforge`, `hubgeom`, `ideengine`, `landingshed`, `loomcli`, `mergeresolve`, `preflight`, `stencilcli`, `webstercli`.
  Transitive dependents: 27 packages.
- Hub-layout vocabulary on the public surface: `BoardDir`, `BoardDirName` (`"_board"`), `BoardWriteLockPath`, `HubSuffix` (`"-HUB"`), `HubPath`, `HubLogsDir`, `HubScratchDir`, `HubReservedNames`, `IsReservedHubName`, `PortalsDir`, `PortalLink`, `LauncherDir`, `WarpLyxLink`, `WarpLyxLinkHere`, `WarpBindingFileName` (`".lyx-warp"`), `StencilsDir`, `StencilBaseByStamp`, `CommitSeededStencils`.
- `*lyxcwd.Location` appears in 33 exported signatures.
  `lyxcwd.Location` itself is four fields — `RepoName`, `HubPath`, `WorktreeName`, `AnchorRel` — i.e. a description of lyx's hub layout.
- True self-resolution of cwd is only two sites (`clone.go:403`, `unwire.go:53`, both `lyxcwd.Resolve(cwd)`), matching the brief.
  But six further sites re-derive a `Location` from a stored path via `lyxcwd.ResolveWorktree` (`commit.go:138`, `mergelifecycle.go:199`, `merge.go:121`, `pull.go:456`, `warplayout.go:25`, `worktreelist.go:130`), so the `lyxcwd` coupling is structural typing, not an incidental parameter.
- `internal/lyxcwd` does **not** import `internal/fabricengine`.
  Naive grep says otherwise;
  the matches in `lyxcwd/lyxcwd.go` and `lyxcwd/anchor.go` are doc-comment prose.
- The genuinely generic core, for the doc's kernel sketch: `CloneHub` (two plain git URLs), the `Fabric` handle over two `gitrepo.Repo` values, the `Warp-SHA` trailer and rebuildable correspondence index, the uniform `<branch>` ↔ `<branch>-weft` naming rule, and the two-sided commit/pull/push/merge surface — all described in `internal/fabricengine/doc.go`.

### The quarry precedent — measured

- The standalone repo is at `/home/knatte/Code/quarry/wts/quarry`, module `github.com/Knatte18/quarry`.
- Layout: `quarry/` (public façade), `glyph/` (separately importable, cgo-free leaf), `internal/{cgoguard,cli,engine,gitsrc,mcpserver,repopath}`, `cmd/quarry`, `cmd/quarry-mcp`.
- The façade re-exports internal types by alias (`type Symbol = engine.Symbol`, `const KindFunction = engine.KindFunction`) and keeps rendering separate from the model (`RenderResolveJSON`/`RenderResolveText`).
  A `Repo` handle is obtained via `quarry.Open(root)`.
- loomyard's `go.mod` contains no `quarry` requirement.
  Adoption is the single item in `manifest/roadmap.md`'s Planned section (`Adopt quarry's glyph alphabet as the plan alphabet`).
  This is the doc's central piece of evidence on extraction timing and must be stated plainly, not softened.
- The port's own record is `docs/research/quarry-holistic-fix-log.md`, which documents the residue-sweeping cost: loomyard-internal references in ported comments, a stale `go.sum`, and cross-repo review rounds needing a two-repo authorization decision.

### Where things live

- `manifest/designs/` — target directory;
  read `loom.md`, `hardener.md` and `semantic-index.md` for the prevailing heading style (`## What it is`, `## <mechanism>`, `## Open questions`, `## Related`).
- `docs/overview.md#documentation-lifecycle` — the lifecycle rule the doc's placement must satisfy.
- `manifest/roadmap.md` — not edited by this task;
  its Maintenance section explains the numbering if a later task adds an item.

## Constraints

From `CONSTRAINTS.md`, the ones this task touches:

- **Markdown Link Integrity** — every inline markdown link in a `.md` file under `manifest/` or `docs/` must resolve, file part and `#anchor` alike.
  Enforced by `TestEnforcement_MarkdownLinks` in `internal/lyxcwd/docslink_test.go`.
  Every link the new doc adds must resolve;
  no allowlist entry may be added.
- **Documentation Lifecycle** — governs the doc's placement;
  see the `doc-belongs-in-manifest-designs-despite-shipped-modules` decision.
- **Fabric Vocabulary Invariant** — *warp*/*weft* name the two sides, *Fabric* names the wired composite, "repo" alone never substitutes for warp, and `host` is retired.
  The doc is not in the owner set, so it must use *Fabric* for the composite and only say *warp*/*weft* where the two must be told apart.
- **gitkit Leaf Invariant** — the doc records that nothing here changes it, per the `gitkit-does-not-travel-with-fabric` decision.
- **Cwd Resolution Invariant**, **Told-Geometry Invariant**, **Shuttle Provider-Seam Invariant**, **Live-Substrate Spawn Observability** — cited by the doc as facts about the current design;
  none is modified.

From `CLAUDE.md`:

- **Task completion — docs land in the same commit** — this task's product *is* a doc.
  `manifest/roadmap.md` deliberately does not move: no planned item is completed or added here.
- **Markdown: semantic line breaks** — one sentence per line, plus breaks at internal independent-clause boundaries;
  no fixed-column hard-wrap, no trailing double-spaces, no backslash line breaks.
  Table cells and blockquotes stay on one line.
  See the `mill:markdown` skill.

Discovered during exploration:

- The doc must not introduce a new module directory, config-registry entry or CLI verb for `creel`.
  It names the concept;
  `internal/configreg`'s `Modules()` list and `cmd/lyx/main.go`'s command tree stay untouched.
- Every number the doc states must be reproducible by a named command (`go list -deps`, `go doc -short`, `wc -l` over non-`_test.go` files).
  Several published background claims were wrong;
  the doc's value depends on being checkable.

## Testing

There is no Go code in this task, so there is no unit test to write and no TDD candidate.
Verification is entirely mechanical and consists of three checks:

- **Link integrity.**
  `go test ./internal/lyxcwd/ -run TestEnforcement_MarkdownLinks` must pass with the new doc in place, and no entry may be added to `docsLinkAllowlist`.
  Every *repo-relative* cross-reference the doc makes — to `docs/overview.md#documentation-lifecycle`, to sibling docs under `manifest/designs/` — must resolve as written, and anchors must match a real heading under GitHub's slug rule.
  **External links are not covered by this test.**
  `internal/lyxcwd/docslink_test.go:363` skips every `http://`, `https://` and `mailto:` target, and a dedicated subtest asserts that skip.
  So any GitHub-issue or external URL the doc carries — the quarry repo, any issue reference — is a manual-review item, verified by reading, never by `go test`.
- **Repo-wide build and test are unaffected.**
  `go build ./...` and `go test ./...` must be unchanged from `main`, since no `.go` file is touched.
  This is the check that catches an accidental code edit.
- **Markdown convention.**
  The doc must satisfy the semantic-line-break rule.
  `tools/mdreflow` exists for this and its behaviour is specified by `tools/mdreflow/reflow_test.go`;
  reviewing the doc against that tool's rules is the check, not a new test.

Scenarios a reviewer of the finished doc should confirm are covered, in addition to the above:

- Both modules receive an explicit, unhedged extract-or-not verdict with a stated trigger condition, not a "revisit later".
- Every quantitative claim names the command that produced it.
- The four corrections to the task's background notes are stated as corrections, with evidence: gitkit is not a Fabric dependency;
  Fabric's `lyxcwd` coupling is structural rather than incidental;
  the missing-interface framing misdiagnoses Fabric's blocker;
  `lyxcwd` does not import `fabricengine`.
- The creel section answers all four questions the task named — placement, addressing, message model, sender universe — and says what it deliberately leaves unspecified.
- The doc reads as a design document about unbuilt work, not as documentation of the shipped modules, so its lifecycle classification is unambiguous.

## Q&A log

- **Q:** One combined design doc, or one per module? **A:** [auto-pick] One combined doc at `manifest/designs/reed-fabric-standalone-api.md`. **Why:** the shared rubric, the quarry precedent, the `logger`→`lyxcwd` finding and the contract-is-the-type-set framing apply to both and exceed half the content;
  two docs would duplicate them.
- **Q:** Does a doc about two shipped modules belong under `manifest/designs/`, given the Documentation Lifecycle scopes that directory to unbuilt work? **A:** [auto-pick] Yes. **Why:** the doc describes an unbuilt possible extraction and its prerequisites, not the shipped modules — whose design already lives in their package doc comments.
- **Q:** What is the extract-or-not verdict for Reed? **A:** [auto-pick] Do not extract now;
  gate on a second consumer or on Reed becoming a service, and do the cheap in-repo preparation meanwhile. **Why:** the contract is small enough to freeze (15 identifiers, 9 importers), but there is no second consumer, three roadmap items still change it, and quarry shipped standalone months ago with loomyard's `go.mod` still carrying no dependency on it.
- **Q:** What is the extract-or-not verdict for Fabric? **A:** [auto-pick] Do not extract, on a size-and-shape mismatch rather than a "not yet". **Why:** 74 external identifiers across 15 importers, 33 signatures parameterized on `*lyxcwd.Location`, hub/board/portal/launcher/stencil vocabulary throughout the public names, and direct imports of `pattern` and `stencilstore`;
  the generic part is a ~2–3k-loc kernel inside a 14.6k-loc engine, so extracting it is a rewrite-by-subtraction with no consumer to pay for it.
- **Q:** Where does the strand mailbox/messaging concept live? **A:** [auto-pick] A sibling module, not inside Reed. **Why:** Reed's own package doc defines it as the dumb carrier that reads no strand field semantically, a mailbox must read addresses semantically, and Reed's `Geometry` carries no slug, branch or role to address by.
- **Q:** What layout would a standalone Reed repo take? **A:** [auto-pick] Mirror quarry — root `reed/` façade, separately-importable `render/` leaf, `internal/` including the CLI, `cmd/reed`. **Why:** quarry is the project's one completed extraction, its façade re-exports internal types by alias so no narrow interface had to be invented up front, and `reedengine/render` is already the exact analogue of quarry's `glyph` (773 lines, zero internal deps, `fmt` and `strings` only).
- **Q:** Should the seam be published by Reed or defined by consumers? **A:** [auto-pick] Reed publishes named slices with compile-time proofs;
  consumers may still define their own. **Why:** a versioned module needs something to prove compatibility against, and both slices are read off real usage — the transport slice is `shuttleengine.ReedOps` verbatim, the session slice is the six methods `loomcli` actually calls.
  The two overlap on three methods, which is correct: a slice describes one consumer's dependency, not a partition of the type.
- **Q:** How does a standalone Reed shed `internal/logger`? **A:** [auto-pick] An injected `*slog.Logger` defaulting to a discard handler. **Why:** `logger` is the sole edge pulling `lyxcwd` and `gitexec` into Reed's transitive set;
  `log/slog` is stdlib, so the swap costs no dependency and loomyard keeps its own sink by passing a handler.
- **Q:** What does the doc recommend for Fabric, given the no-extract verdict? **A:** [auto-pick] An in-repo split into a named generic kernel and a named hub-layout half, justified on its own terms and explicitly not framed as extraction prep. **Why:** it makes the Fabric Git Invariant's perimeter visible and isolates the stencil/pattern coupling as a question rather than an inheritance;
  tying it to extraction would make a justified refactor hostage to a trigger that may never fire.
- **Q:** What are creel's addressing, message model, transport and sender universe? **A:** [auto-pick] Address `<worktree-slug>/<role>` resolved to a GUID at delivery time;
  durable FIFO inbox per address;
  notify-only via one `SendKey`, no interrupt tier;
  on-disk JSON directory plus a `lyx creel send` verb plus a Go API, open to any process that can write a file. **Why:** GUIDs are per-incarnation and webster re-mints on every respawn;
  interruption already exists separately as `Runner.Interrupt` and folding it in would hand an open sender set the power to destroy an agent's context;
  and a wire protocol earns nothing when senders already share a filesystem.
- **Q:** What should the mailbox module be called? **A:** [auto-pick] `creel`. **Why:** the rack holding one bobbin per feed position matches a rack of per-address inboxes, and it is unused in the repo — as are `heddle`, `temple`, `pirn`, `bobbin`, `selvedge`, `sley` and `beam`.
- **Q:** Does this task edit `manifest/roadmap.md`? **A:** [auto-pick] No — the doc names recommendations;
  promoting any of them is a separate decision afterwards. **Why:** the task brief says so explicitly, and `CLAUDE.md` moves the roadmap only on completing or adding a planned item.
- **Q:** Do `fslink`/`yamlengine`/`githubclient` warrant a follow-up task? **A:** [auto-pick] No — one paragraph in the doc. **Why:** 410, fewer, and `proc`-dependent production lines respectively;
  a separate repo, module path, release cadence and CI never pays for that size without an external consumer, and the same gate that defers Reed defers all three more strongly.
- **Q:** Should the doc contain real Go listings or prose? **A:** [auto-pick] Real listings, every signature transcribed from or derived from shipped code. **Why:** an API-design doc's content is signatures, and prose descriptions of a method set are not checkable.
