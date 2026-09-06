# Discussion: Adopt quarry's glyph alphabet as the plan alphabet

```yaml
task: Adopt quarry's glyph alphabet as the plan alphabet
slug: quarry-glyph-plan-alphabet
status: discussing
parent: main
```

## Problem

A loomyard plan card names what it touches in a flat list that mixes file paths and symbols, distinguished by shape alone.
The symbol half of that alphabet has no unique key: `boardcli.newListCmd` names a package by its *basename*, not its path, so `internal/boardcli` and a hypothetical `tools/boardcli` collide, and nothing mechanically connects the string to a declaration in the repository.
Every downstream consumer inherits that weakness.
`internal/websterengine`'s `deriveEdges` derives the execution DAG by exact string equality over `Targets`/`Uses`, so the graph is only as precise as the spelling.
There is no done-check that a `Create` card created anything, no scope guard beyond an informational changed-files union, and no way to notice that the code moved out from under a plan mid-run.
The strings are also LLM-authored end to end: the `Plan-Write` stencil currently tells the planner "no quarry inventory exists — do the lookups yourself" with `go doc` and `grep`, so a plan's symbol names are the planner's own guesses, checked by nothing.

**Why now.**
quarry has shipped every primitive this needs.
Its public Go facade (`github.com/Knatte18/quarry`, package `quarry`) exposes all six queries — `TOC`, `Resolve`, `Expand`, `Delta`/`DeltaGit`, `Name`, `Glyphs` — and the pure, stdlib-only `glyph` package defines the alphabet itself.
The three primitives GitHub issue #226 listed as "planned" (the `glyphs` flat index, the glyph-maker, and diff-to-symbols) are all merged.
Nothing in the adoption is blocked on quarry any more except one small additive accessor, tracked separately (see the `quarry-unitpath-precondition` decision).

Adopting glyphs closes the ambiguity, makes "does this plan still describe the code?" a mechanical question, and replaces planner guesswork with strings copied verbatim from a quarry answer.

## Scope

**In:**

- `go.mod` dependency on `github.com/Knatte18/quarry`; `lyx` becomes a cgo binary.
- `internal/planparser`: plan format 5 — glyph spelling, `plan:` handles, the `language:` frontmatter key, a rewritten shape classifier, parse-time canonicalization, and a new `RewriteRefs` write path.
- New package `internal/planglyph`: sole owner of every `quarry.Repo` call (`Resolve`, `Name`, `DeltaGit`) and of the resolve-backed validation pass.
- New `lyx quarry` command group: `toc`, `glyphs`, `resolve`, `expand`.
- Placeholder handles: draft-time `plan:` spelling, batched `Name` canonicalization at the validation boundary, `Delta`-driven binding at card completion.
- `Rename` cards on the handle machinery, with the to-side glyph computed mechanically.
- Drift detection at three boundaries, with exact-tier auto-repair and an append-only amendment log.
- Mechanical consumers: done-checks, the glyph scope guard, and the cross-granularity containment check.
- New producer rows plus their parity CLI verbs, per the Gate Self-Check Parity Invariant.
- Docs landing in the same commits: `contracts/specs/loom-plan-spec.md`, `contracts/stencils/loom/loom-template-plan.md`, `manifest/designs/quarry-glyph-plan-alphabet.md`, `docs/overview.md` (new module), `CONSTRAINTS.md` (new invariant), `README.md`/`CLAUDE.md` (cgo prerequisite), `manifest/roadmap.md` (Planned item completed).
- Rewriting the golden fixture `internal/planparser/testdata/goodplan` and the worked example in the spec to glyph spelling.

**Out:**

- **The kick-start pack** — injecting resolved spans into the implementer prompt at dispatch.
  It is the one piece of issue #226 gated on an external measurement (quarry's M4 / `ladder-kickstart`), and everything else is mechanical and independent of that result.
- **LSP-shaped tools in an agent's own hands.**
  Measured flat-to-negative in quarry's ladders; semantics enter the mechanical layer only.
  The planner's toolset stays `toc` + the validator.
- **Non-Go alphabets.**
  Python and C# are specified in quarry's contract but not implemented there.
  This task must not assume Go anywhere the contract says the rule is Go-only, but it implements only what `glyph` implements.
- **Any change inside the quarry repository.**
  The `Glyph.UnitPath()` accessor this task depends on is quarry's own task `glyph-unitpath`, worked in the quarry container.
  Per the worktree-isolation rule this worktree never touches it.
- **Continuous DAG update across waves, and parallel execution.**
  Those belong to the roadmap's Someday `webster: worktree-per-card parallel execution` item, which this task unblocks but does not deliver.
- **Migrating existing plans.**
  `_lyx/plan/` is untracked weft content and nothing matches on `origin/main`, so there is no live plan to migrate.

## Decisions

### dependency-mechanism

- **Decision:** loomyard takes a `go.mod` dependency on `github.com/Knatte18/quarry` and imports the `quarry` facade and the `glyph` package directly, in-process.
- **Rationale:** the facade's own package documentation states this is what it exists for — "what lets Loomyard's own Go code, or any other importer, reach the engine's typed results without a JSON round-trip".
  It also gives version-locking **by construction**: the planner's answers (via `lyx quarry ...`) and the validator's `Resolve` come from the same `go.mod` version of quarry inside the same binary.
- **Rejected:** spawning the upstream `quarry` CLI and parsing its JSON — the installed binary and the library version the validator links can drift apart, so the planner could copy a spelling the validator later rejects, which is exactly the class of failure the copied-verbatim rule exists to make impossible.
  Also rejected: the `quarry-mcp` server, which makes quarry an agent tool rather than mechanical Go.

### cgo-posture

- **Decision:** accept cgo in `lyx`.
  Every build host needs `CGO_ENABLED=1` and a C toolchain.
- **Rationale:** quarry's engine links tree-sitter's C grammars; `internal/cgoguard` deliberately fails a `CGO_ENABLED=0` build with a readable message rather than a raw linker dump.
  Only the `glyph` package is dependency-free — the design doc's "cgo-free" parenthetical applies to `glyph` alone, not to the facade.
  Accepting cgo is the only option that keeps build-time version-locking with no runtime version check.
- **Practical notes for the plan:** `CGO_ENABLED` already defaults to 1 for a native build when a C compiler is on PATH, so nothing needs setting on this Linux host.
  `go env -w CGO_ENABLED=1` pins it per-user in `go env GOENV`, but it is machine-local, not checked in, and it does not install a compiler — on a Windows box without mingw-w64 it merely swaps `cgoguard`'s readable message for "gcc not found".
  Document the toolchain prerequisite in `README.md` and `CLAUDE.md`, and have `tools/deploy` pass `CGO_ENABLED=1` explicitly on the build it runs.
  No CI is at risk: this repo has no `.github/workflows`.
- **Rejected:** a second, cgo-only binary in loomyard's module keeping `lyx` pure Go — quarantines the toolchain requirement but adds a spawn seam and a two-binary deploy for no correctness gain.

### quarry-version-pin

- **Decision:** cut a real semver tag in quarry and require that tag.
- **Rationale:** quarry is a public repository with no release tags today (only `archive/*` names), so the alternative is a pseudo-version on `main`, which silently turns every quarry commit into a loomyard upgrade decision.
- **Note:** version *numbers* are not the point and nothing here is published — the tag exists to make the plan-format contract reproducible.
  The `go.mod` bump to the quarry version carrying `Glyph.UnitPath()` is part of the disk-check cards' own precondition (see `quarry-unitpath-precondition`).
- **Rejected:** a `replace` directive to the local worktree — unbuildable for anyone else.

### package-ownership

- **Decision:** `internal/planparser` imports the pure `glyph` package only.
  A new package `internal/planglyph` owns every `quarry.Repo` call and the resolve-backed validation pass.
- **Rationale:** keeps `planparser` a tier1-pure leaf under the Test Tier Purity Invariant and preserves the `ValidateFormat`/`Validate` split the Gate Self-Check Parity Invariant depends on.
  `glyph` is stdlib-only with no dependencies, so importing it costs `planparser` nothing.
- **Rejected:** `planparser` importing the facade and resolving inline — one package, but `planparser` stops being a pure leaf and both gate entry points start reading the repository.
  Also rejected: putting the resolve step only in `internal/loomshed`'s producer row, which breaks gate parity because `validate-plan` would no longer do the same work.

### planner-glyph-source

- **Decision:** a new `lyx quarry` command group is the planner's only source of glyphs, and the `Plan-Write` stencil's "No quarry inventory exists — do the lookups yourself" section is replaced by it.
- **Rationale:** the hard rule — the LLM never spells a glyph — is unsatisfiable while the planner has no quarry access at all.
  Routing through `lyx` means the planner's answer and the validator's `Resolve` come from one linked quarry version, and it stays inside the CLI/Cobra Invariant and `lyx`'s help tree.
- **Rejected:** installing the upstream `quarry` binary (version drift, second binary, outside the help tree); leaving the planner on `go doc`/`grep` and canonicalizing its bare names afterwards (rewards guessing, inverting the hard rule).

### format-version

- **Decision:** bump the plan format to `5`.
- **Rationale:** the spelling of every symbol entry changes, `plan:` handles and `language:` are new grammar, and `checkFormatRecognized` accepts exactly one version — an old plan should fail loud rather than half-parse.
  The number itself is not important; being correct is.

### bare-symbol-is-hard-finding

- **Decision:** after this change an entry is a **path or a glyph**.
  A bare package-qualified symbol (`boardcli.newListCmd`) is a hard finding, with its own check ID.
- **Rationale — write it exactly this way, so nobody later softens it into a style preference:** what makes `pkg.Symbol` a hard finding is not that the form is uglier.
  It is the one spelling that **cannot have come verbatim from a quarry answer** — quarry never emits it.
  The rule is the enforcement of copied-verbatim, expressed as a classifier rule.
- **Rejected:** tolerating it with a warning (leaves the guessing path open indefinitely); canonicalizing bare names to glyphs via `Resolve` (makes guessing *rewarded*).

### file-spelling

- **Decision:** both the plain path (`internal/x/focus.go`) and the file self glyph (`internal/x/focus.go#`) are accepted **on the surface**, and the parser canonicalizes to the **glyph** form at ingest.
  The glyph form is canonical because it is the one that arrives verbatim from a quarry answer.
- **Rationale:** the "backup mode" promise — plain paths keep working — is kept without the DAG ever seeing two spellings.
  The conversion is lossless by contract, not by heuristic: `docs/glyph.md` §3 states that for Go, and only for Go, removing the trailing `#` yields the plain repository-relative path in both directions, "this holds only because Go's unit is itself spelled as a repository-relative path".
  quarry's own `resolve` prescribes the same repair — §5: "A bare repository-relative path handed to `resolve` is rejected pre-resolution, with a message naming the fix — append the trailing `#` to make it a self glyph."
  Canonicalizing at ingest adopts quarry's stated fix rather than inventing a convention.
- **Why this matters beyond aesthetics:** `deriveEdges` matches refs by exact string equality, so the same file spelled two ways becomes two DAG nodes and the dependency edge between two cards silently disappears.
  One spelling per thing is a correctness requirement.
- **Scope limit:** the file self glyph exists in Go only.
  `docs/glyph.md` §2: Python has no separate file self glyph (the module *is* the file), and C# has none at all.
  Canonicalization is therefore gated on the plan's `language:`.

### package-spelling

- **Decision:** a package is spelled as its unit self glyph (`internal/foo#`) only.
  A bare directory path as a target is a hard finding.
- **Rationale:** the same one-spelling rule as `file-spelling`, pointed the other way.
  A unit glyph is `Resolve`-checkable where a bare directory is only `os.Stat`-checkable.
  No quarry-side extension is needed — the unit self form already exists in the contract (`docs/glyph.md` §3), parses like any other glyph, and in Go the spelling is trivially migratable by the contract's own rule.
- **Rejected:** bare directory paths only (a `Prosa` card over a whole package would get no resolve-backed verdict); forbidding package targets entirely (leaves `Prosa`/`Custom` cards over a package with no natural target).

### directory-classifier-rule

- **Decision:** the classifier splits path-shaped entries syntactically, by extension, and never calls `Resolve` to classify:
  - a path **with** a file extension is a file — legal, canonicalized per `file-spelling`;
  - a path **without** an extension is a hard finding, whose message reads: *spell it as a unit glyph (`internal/foo#`) if it is a package; if it is not code, list the files instead*;
  - a unit glyph that does not resolve (someone wrote `docs#`) is a **resolve** finding raised by `planglyph`, never a classification finding.
- **Rationale:** the boundary case `package-spelling` would otherwise hide is a directory with no Go files — it has no unit and therefore no self glyph, so quarry can never resolve `docs#` and can never have answered it, and a `Prosa`/`Custom` card over a non-code tree could not satisfy a glyph-only rule.
  The split keeps the division of labour consistent with `package-ownership`: the classifier enforces **form** syntactically and stays a pure leaf; `Resolve` enforces **existence** against the repository.

### language-frontmatter

- **Decision:** `00-overview.md`'s scalar-only frontmatter gains `language:`.
  Legal values are the alphabets the `glyph` package implements (today: `go`) plus the literal `none`.
  Anything else is a hard finding — a pure string check, no `Resolve`, tier1-safe.
  Absent defaults to `go`.
- **Rationale:** the frontmatter is already scalar-only, and a loomyard plan is one repository and one language today.
  Validating the value syntactically keeps the check in `planparser`.
- **Migration posture, stated rather than discovered:** because absent defaults to `go`, a legacy plan falls into the strict rules (`bare-symbol-is-hard-finding`, `package-spelling`) unless it opts out with one line of frontmatter.
  That is intended — plans are short-lived and new ones are agent-written.
- **Predeclared escalation path:** when a second language arrives, the extension is a **per-card `language:` override of the plan-level default** — an additive key, backward compatible.
  Until that day, one plan is one language by rule.
  Nobody invents a new format later.

### paths-only-mode

- **Decision:** `language: none` selects paths-only mode.
  No glyph is legal, no `Resolve`/`Name`/`DeltaGit` call runs, canonicalization is a no-op, and every behaviour degrades to today's path-only plan.
- **Rationale:** a repository whose language has no glyph alphabet yet must still be plannable, and that case is expected rather than hypothetical.
  Carrying it on the `language:` key means one place to look and one value to validate.
- **Rejected:** a separate `glyphs: true|false` boolean beside `language:` — two keys that can contradict each other (`language: go` + `glyphs: false`) and a new consistency check to referee them.
  Also rejected: deriving the mode from config or repo detection, which stops the plan being self-describing.

### parse-time-canonicalization

- **Decision:** canonicalization runs once, at parse time, and the `planparser.Plan` model holds **glyphs**.
  The path-shaped checks convert back when they need to test the disk.
- **Rationale:** `deriveEdges` and every other consumer then see exactly one spelling per thing by construction, and it keeps canonicalization where the existing `root:`/`//` normalization already runs.
- **Rejected:** canonicalizing on read in each consumer (the one consumer that forgets reintroduces the split-node bug silently); storing both forms per ref (a two-field ref type rippling through `Targets`/`Uses`/`Pairs` and every consumer).

### glyph-conversion-chokepoint

- **Decision:** loomyard **never performs glyph↔path conversion itself** — not by trimming `#`, and not by reading `Glyph.Unit` and treating it as a disk path, which encodes the same Go-only assumption without the trim.
  Both directions are quarry API:
  - path → glyph is `glyph.Self(lang, path)`, which exists today;
  - glyph → path is a new accessor on the glyph type (`UnitPath() (string, bool)` — per-alphabet, not-ok where the unit is not path-shaped), which does not exist yet.
  All glyph parsing goes through `glyph.Parse` and the glyph type's own `String`, never a loomyard-side grammar or regex.
- **Rationale:** the pure-Go `glyph` import into `planparser` exists exactly for this.
  "One spelling per thing" is then enforced by the contract's own implementation rather than by a copy of it, and the Go-only conversion rule cannot leak into a non-Go plan through a stray `TrimSuffix`.
  `glyph/self.go` already states the same discipline for the forward direction: "This is the one concatenation in the whole system; no consumer performs it."
- **This is a new cross-cutting invariant** and must be recorded in `CONSTRAINTS.md` in the same commit that introduces it.
  Suggested name: **Glyph Conversion Chokepoint Invariant**.
- **Rejected outright, not deferred:** a loomyard-side Go-only helper implementing the trim until the quarry accessor lands.
  That is precisely the grammar copy this decision forbids, and "later" is how a copy becomes permanent.

### quarry-unitpath-precondition

- **Decision:** the disk-shaped checks (`path-missing`, `card-path-malformed`, and anything else that maps a glyph back to a file on disk) are their own late cards, and their precondition is the merged quarry `Glyph.UnitPath()` accessor plus the `go.mod` bump to the quarry version carrying it.
  Everything else proceeds today.
- **Rationale:** canonicalization via `Self`, `Resolve`-backed validation, handles, binding, drift, and the containment check are all unblocked now.
  The blocked edge stays a named, isolated few cards rather than a gate on the whole task.
- **Status:** the quarry side is already in motion — quarry task `glyph-unitpath`, mill-quick-sized, spawned 2026-09-06 — and is expected merged before planning ends.
- **Verified absent:** `Glyph` today exposes `Lang`, `Unit`, `Owner`, `Name`, `Params` and the methods `IsSelf`/`String`.
  Nothing maps a unit back to a disk path.

### create-declaration-grammar

- **Decision:** a `**Create:**` group's sub-bullet grammar extends to `` `plan:<draft-handle>` -> `<declaration head>` ``.
- **Rationale:** `quarry.Name` takes `{Unit, Decl}`.
  The unit is already inside the handle (left of `#`); the declaration head is not anywhere in the format today.
  This bullet supplies both halves from one place, reuses the arrow grammar `Rename` already parses, and keeps the declaration adjacent to the handle it declares — satisfying the issue's "the expected declaration must ride on the card" literally.
- **Rejected:** a separate `**Declares:**` field (two lists that must stay in sync, plus a new check family to enforce it); the declaration head in `**Intent:**` prose (not machine-readable, so `Name` can never be called and the whole canonicalization stage collapses).

### handle-spelling-and-canonicalization

- **Decision:** handles are spelled `plan:<expected-glyph>`, canonicalized in two stages.
  **Drafting:** the planner writes the declaration on the `Create` card and references it elsewhere with a self-chosen `plan:` spelling.
  **Validation:** the pipeline (mechanical code in `planglyph`, never the LLM) batch-calls `quarry.Name` on every `Create` declaration in **one** call, alongside the batched `Resolve` for existing glyphs, at the same boundary and never inside the planning loop, then rewrites every draft handle to the canonical `plan:<expected-glyph>` across the whole plan.
- **Rationale:** a handle has no reality to point at — it only has to be internally consistent, which is fully mechanically checkable — so letting the LLM invent the *draft* spelling is safe in a way that letting it invent a real glyph is not.
  The `plan:` prefix carries the entire distinction: `:` is outside the glyph alphabet so `resolve` rejects it unaided, it cannot be a plausible path, and it survives shell, YAML and Markdown.
  quarry never sees a handle.
- **Plan-time checks against the expected name, treated as provisional:** collisions between `Create` handles, already-exists, and well-formedness via `Name` (per-entry status with a `target` echo, so one bad declaration fails its own entry and not the batch).
  A dangling handle — no matching `Create`/`Rename`-to — is rejected; an unreferenced `Create` is flagged.

### rename-pair-grammar

- **Decision:** on a `Rename` group, `old` must resolve `found`, and `new` is a `plan:` handle whose content the pipeline **computes and overwrites**: resolve `old`, take its declaration verbatim, swap the identifier, call `Name`.
  The planner's draft spelling on the to-side is never trusted.
  A `Rename` card therefore needs no declaration head of its own, unlike `Create`.
- **Rationale:** this is the zero-LLM-spelling rule applied to the exact side issue #226 singles out, and it mirrors quarry's own exact-tier philosophy in diff-to-symbols: assert only what is mechanically derived.
- **File-rename pairs stay** as plain path pairs in the same group.
  Files are the backup-mode domain, a file rename has no declaration head to `Name`, and a separate card type would be format for its own sake.

### rewrite-write-path

- **Decision:** one new `planparser` primitive — `RewriteRefs(planDir, map[string]string)` — is the single write path all three rewrite occasions call: handle canonicalization, handle binding at card completion, and drift auto-repair.
- **Rationale:** issue #226 already says binding and rename propagation are one mechanism on two occasions, and canonicalization is the same old→new substitution.
  One write path keeps the Planparser Sole-Parser Invariant honest alongside `SetApproved`.
- **Rejected:** three purpose-built writers (triples the surface that can rewrite `_lyx/plan/` bytes); a full re-render from the parsed model — `planparser` is deliberately *lenient* at the card level, preserving malformed bullets so the validator can enumerate every defect, so a round-trip would silently normalize away exactly the defects the validator exists to report.

### resolve-status-policy

- **Decision:** per resolve status —
  - `found` passes;
  - `multipart` **passes** (a legitimate single symbol the language lets be declared in several places: Go `init`, a C# `partial` type or method; every part is returned);
  - `ambiguous` is a hard finding listing the candidates;
  - `not_found` is a hard finding whose message branches on the unit status — `unit: found` means a misspelled member, `unit: not_found` means a misspelled unit.
- **Rationale:** mirrors quarry's own contract wording in `docs/glyph.md` §5.
  Rejecting `multipart` would reject `internal/logger#init` and every C# partial type as defects, which the contract says they are not.
  Downgrading `ambiguous` to a warning would let a plan proceed against a target nobody has picked — the ambiguity GitHub issue #225 existed to kill.

### create-target-verdict

- **Decision:** per-group inversion.
  A `Create` group's targets must resolve `not_found`; **both** `unit: found` and `unit: not_found` pass.
  A `found` or `multipart` verdict is the `create-already-exists` finding.
- **Rationale:** demanding `unit: found` is not merely stricter, it is incoherent in Go — a package does not exist apart from its files, so the "separate unit-creating card" it would require has nothing to target, its own unit being equally absent, and the regress never grounds.
  Creating a package *is* creating its first symbol.
- **Rider:** `unit: not_found` is ambiguous between a deliberate new package and a typo in the unit path, so a misspelled unit would otherwise sail through as `not_found` and the plan would silently create a package nobody intended.
  Keep it a pass, but surface it: every `Create` group whose unit is `not_found` produces an **informational** finding — "this card introduces a new unit `<unit>`" — so each new package name appears explicitly in the validation report.
- **Note:** this also discharges the spec's own card-types table obligation for `Create` ("none — check nothing equivalent exists first"), which had no mechanical implementation before.

### cross-granularity-containment-check

- **Decision:** a new containment check in **two tiers**, split along the `package-ownership` seam, each reporting its own finding ID:
  - a **syntactic** tier in `planparser` — a member glyph's unit prefix against another card's unit or file self glyph, pure string work, tier1;
  - a **resolve-backed** tier in `planglyph` — a member glyph mapped to its owning file, taken from the `Resolve` answer which already carries the file per symbol, matched against file self glyphs on other cards.
- **Rationale:** this is the hole that "the symbol-granular DAG is nearly free" would otherwise hide.
  String equality correctly sees that two cards touching different members do not serialize.
  But a card targeting `internal/reedengine/render#Focus.Reset` and a card targeting the file `internal/reedengine/render/focus.go#` overlap **physically** — the same file is edited — with no string equality between them: no edge, blind parallel dispatch, merge conflict.
  The same applies to member versus package self glyph.
  This is small, but it is genuinely new logic and must not disappear into "free".
- **Rejected:** resolve-backed only (a collision obvious from the strings alone goes unreported by the pure gate); syntactic only (the member→file half is exactly what string prefixes cannot see, since a package's symbols are spread across its files — it misses the motivating example).

### dag-needs-no-scheduler-change

- **Decision:** the "execution DAG at symbol granularity" deliverable needs no change to `websterengine.SequenceBatches`/`deriveEdges`.
- **Rationale:** `deriveEdges` already matches `Targets`/`Uses` by exact string equality and explicitly does not consult ref classification — its own comment: "Ref classification (symbol-shaped vs. path-shaped) is not consulted: both kinds participate identically."
  Once refs are glyphs, both of issue #226's claimed gains fall out of the alphabet: same file / different symbols no longer serializes, and different files / same symbol no longer runs in parallel blindly.
  Handles participate identically — a card `Uses`-ing `plan:X` cannot precede the card whose `Create` target is `plan:X`, by the same string equality, until binding rewrites both sides together.
- **What remains there is the containment check above, not a scheduler rewrite.**

### drift-boundaries

- **Decision:** three boundaries re-resolve, each as a producer row **and** its parity CLI verb:
  - `begin-batch` — the dispatch boundary re-resolves; never cached;
  - `record-batch` — done-checks, `Delta`-driven handle binding, and the scope guard;
  - `Plan-Revalidate` — one batched `Resolve` over the remaining plan after each card merge.
- **Rationale:** the Gate Self-Check Parity Invariant requires the verb-plus-row pairing for any new mechanical gate, and adding a gate means adding its verb and its parity check in the same task.
  Covering only `Plan-Revalidate` would catch drift a merge late and still allow a stale pack to dispatch.

### delta-call-site

- **Decision:** the `Delta` call runs at `record-batch`, as `DeltaGit(BatchState.StartSHA, report.HeadSHA, ".")`.
- **Rationale:** both SHAs already exist and are already cross-checked against the worktree's real HEAD there (`recordbatch.go` rejects a report whose `head_sha` disagrees with `actualHead`), it is the one place a card's completion is observed, and it serves all three consumers at once — handle binding, done-checks, and the scope guard.
  Under the identity batcher this is exactly one card's diff, with no new state to introduce.
- **The `"."` whole-repo range is right, not wasteful:** the scope guard needs the whole-repo delta anyway, and narrowing it would blind the one consumer that exists to catch out-of-scope changes.
- **Rejected:** a separate post-merge boundary distinct from `record-batch` (a second place that can disagree about a card's range); assembling before/after bytes ourselves for `Delta(entries)` (loomyard's own byte-gathering goes through `gitexec`, equally tier1-barred, so it buys nothing and duplicates `DeltaGit`).

### drift-signal-and-repair

- **Decision:** the drift signal is deterministic — diff-deleted symbols intersected with the remaining plan's references — with two gates before any rewrite:
  1. a rename that matches a `Rename` card is that card's expected outcome (binding, not drift);
  2. a renamed symbol that nothing in the remaining plan references is logged only, never rewritten.
  Past the gates, the rename tiers split the response: an **exact-tier** detection auto-repairs (plan-wide rewrite old→new via `RewriteRefs`, revalidate with one batched `Resolve`, log an amendment), and an **evidence-tier** candidate goes to review as a visible rename-versus-genuine-delete decision, never a silent guess.
- **Rationale:** placeholder binding and rename propagation are the same operation on two occasions.
  Auto-repairing only the tier quarry itself asserts (AST-exact, body token streams identical modulo the renamed identifier, no threshold) keeps loomyard from deciding what quarry deliberately returns as undecided.

### amendment-log

- **Decision:** an append-only `_lyx/plan/amendments.md`, one entry per repair, carrying timestamp, card, old glyph → new glyph, tier, and triggering SHA.
- **Rationale:** the plan directory is the thing that changed, so the log lives beside it, and `planparser` stays the sole writer of `_lyx/plan/` bytes because it owns the file.
- **Consequence, accepted explicitly:** `index-file-mismatch` must learn that `amendments.md` is a **known non-card file** — an allowlist entry, not a heuristic.
- **Rejected:** the existing batch digest / status trail (amendment history scatters across per-batch state instead of sitting with the plan it amended); git commit messages only — terminal flaw: a squash-merge erases the history, so the record exists only where nothing can read it back.

### blocking-policy

- **Decision:** split by determinism.
  **Done-check failures block** — a `Create` that does not resolve, a `Delete` that still resolves, a card-count mismatch on binding.
  **Evidence-tier drift candidates and the glyph scope guard stay informational**, surfaced in the report for review.
- **Rationale:** this mirrors quarry's own two-tier philosophy exactly — block on what is mechanically asserted, stay informational on what quarry itself refuses to decide.
  A `Create` that did not resolve is not a judgment call.
  Gating on evidence-tier candidates would make loomyard decide what quarry deliberately returns as undecided.
  It also preserves the existing, deliberate posture that plan-predicted file impact is frequently incomplete, so deviation alone never fails a fork.
- **Note for the plan:** the `Delete` gate would ideally also want quarry's parked `assert-no-callers`; it is not available and is not in this task's scope.

### cli-verb-surface

- **Decision:** four verbs under `lyx quarry`, JSON out — `toc <path>`, `glyphs <dir>`, `resolve <glyph>...`, `expand <glyph>`.
  `delta` and `name` are deliberately absent.
- **Rationale:** `glyphs` and `resolve` are what the planner needs for copied-verbatim spelling; `toc` and `expand` are what the stencil's replaced "do the lookups yourself" section becomes.
  `delta` and `name` are pipeline-internal and never agent-facing — `name` in an agent's hands is a glyph-spelling machine, the one thing the hard rule exists to prevent — which keeps the planner's toolset at the issue's stated "`toc` + the validator".
- **Rider, applying to all four verbs:** they delegate to the facade queries with their frozen presets and **never re-shape the answer**.
  The stencil's "copy the line verbatim" instruction only works if the answer is quarry's answer.
  `quarry.GlyphsOptions()` is the frozen preset for `glyphs`; use it rather than assembling `TOCOptions` locally.

## Technical context

**The current format and its parser.**
`internal/planparser` is the sole reader and writer of `_lyx/plan/` (Planparser Sole-Parser Invariant).
Its files: `parse.go` (600 lines, lenient at card level), `validate.go` (688 lines, seventeen checks), `classify.go` (the shape classifier), `normalize.go` (`root:`/`//` path resolution at parse time), `plan.go` (the `Plan`/`Card`/`TargetGroup`/`MovePair` model), `approve.go` (`SetApproved`, the one write path), `sections.go`, `doc.go`.
The pinned contract is `contracts/specs/loom-plan-spec.md`; the LLM-facing subset is `contracts/stencils/loom/loom-template-plan.md`.
The golden fixture is `internal/planparser/testdata/goodplan`.

**The classifier is the first thing that breaks.**
`classify.go`'s `classifyRef` applies three rules in order: a `/` anywhere means path; otherwise an all-lowercase-alphanumeric final dot-segment means path; otherwise symbol.
Every Go glyph contains `/` (`internal/shedrecipe#Lookup`), so rule 1 currently sends every glyph to `refKindPath`.
A `#`-keyed glyph rule must precede the `/` rule, and the trailing-`#` file-self form is what separates a file glyph from a plain path.

**The two validator entry points.**
`ValidateFormat` runs sixteen checks; `Validate` adds `plan-unapproved`.
The Gate Self-Check Parity Invariant binds them: `Plan-Validate` ↔ `validate-plan` calls `planparser.ValidateFormat`, and `Plan-Revalidate` ↔ `validate-plan --require-approved` calls `planparser.Validate`.
Any new mechanical gate needs its verb and its parity check in the same task.

**The DAG.**
`internal/websterengine/sequence.go`'s `SequenceBatches` derives edges from `Targets`/`Uses` matching, condenses strongly-connected components, and returns a deterministic topological order plus the cycles it condensed.
`refsIntersect` compares by exact string equality after `strings.TrimSpace`.
Sequencing is unconditional and every batch-computation site must sequence — `Run` plus the four `internal/webstercli` bracket verbs.
`internal/batcher` owns grouping (Batcher Registry+Config Invariant); `websterengine` owns only sequencing.

**Batch state and SHAs.**
`beginbatch.go` records `BatchState.StartSHA` (repo HEAD captured before the call returns) and renders the fork prompt.
`recordbatch.go` verifies `report.HeadSHA` against the worktree's actual HEAD and writes `CardSHAs` — one element under the identity batcher.
The fork-return contract is `status: OK|FAILED`, a `head_sha`, and an informational `deviations` list; a fork returns FAILED only on a non-zero build/unit gate or a non-zero per-card `verify:`.

**quarry's surface, as verified in `/home/knatte/Code/quarry/wts/quarry`.**
Module `github.com/Knatte18/quarry`, Go 1.26, public on GitHub, **no release tags today**.
Package `quarry` (the facade): `Open(root)`, `(*Repo).TOC`, `.Glyphs`, `.Resolve([]string) ([]ResolveResult, error)`, `.Expand`, `.Delta([]DeltaEntry)`, `.DeltaGit(from, to, target)`, package-level `Name([]Declaration) []NameResult`, `GlyphsOptions()`, plus JSON/text renderers.
`Declaration` is `{Unit, Decl}`; `NameResult` is `{Unit, Target, ID, Kind, Error, Reason}` — positional, always the same length as the input, `ID`/`Kind` on success only, `Error`/`Reason` on failure only.
Package `glyph` (pure, stdlib-only): `Parse(lang, s)`, `Self(lang, path)`, `Glyph{Lang, Unit, Owner, Name, Params}`, `(Glyph).IsSelf()`, `(Glyph).String()`, `Language`/`Go`, `Reason`/`Reasons`/`ParseError`.
The engine requires `CGO_ENABLED=1`; `internal/cgoguard` enforces it with a readable compile error.
`internal/gitsrc` uses `exec.Command("git", ...)`, which is what makes `DeltaGit` a process-spawning call.

**quarry's contract documents worth reading during planning:** `docs/glyph.md` (the alphabet; §1 the form, §2 the unit per language and the trailing-`#` table, §3 the member per language and the Go-only strip rule, §5 resolution and the four statuses) and `quarry/doc.go` (the facade's own posture on delegation and renderers).

**No live plan migration.**
`_lyx/plan/` is untracked weft content; nothing matches on `origin/main`.
The rewrite surface is the golden fixture, the spec's worked example, and the stencil.

**Docs that must land in the same commits as the code that changes them**, per this repo's Task-completion rule: `contracts/specs/loom-plan-spec.md` (format 5, the new checks, the worked example), `contracts/stencils/loom/loom-template-plan.md` (glyph spelling, the hard rule, the `lyx quarry` verbs replacing the `go doc`/`grep` section), `manifest/designs/quarry-glyph-plan-alphabet.md`, `docs/overview.md` (the new `internal/planglyph` module in the module table), `CONSTRAINTS.md` (the Glyph Conversion Chokepoint Invariant), `README.md` and `CLAUDE.md` (the cgo/C-toolchain prerequisite), and `manifest/roadmap.md` (the Planned item completes).

## Constraints

From `CONSTRAINTS.md`, the ones this task is bound by:

- **Planparser Sole-Parser Invariant** — `internal/planparser` stays the sole parser and writer of `_lyx/plan/`.
  Consumers read only from the `planparser.Plan` model.
  `SetApproved` is joined by exactly one new write path, `RewriteRefs`; no third.
- **Test Tier Purity Invariant** — untagged test files perform no `gitexec.Run`/`RunGit`, `exec.Command`/`CommandContext`, `gitkit.Copy*`, or `hubforge.NewHub`.
  `planparser` stays a tier1-pure leaf; `quarry.DeltaGit` spawns `git`, so every test that reaches it is `integration`- or `smoke`-tagged.
- **Gate Self-Check Parity Invariant** — a mechanical gate's `ShedProducer` row and its CLI self-check verb call the same package function for every mode, and adding a gate means adding its verb and its parity check in the same task.
- **CLI / Cobra Invariant** — the new `lyx quarry` group goes through the module `Command()`/`RunCLI` seam, carries `Short` on every command, and updates the help-tree tests.
- **Told-Geometry Invariant** — an engine is handed the absolute paths it operates on and derives none of its own; no direct import of `internal/lyxcwd`.
  `internal/planglyph` is bound by this and must be added to the invariant's bound-packages list if it takes a geometry.
- **Batcher Registry+Config Invariant** — webster's execution unit is the batchifier-derived batch; batching selection stays owned by `internal/batcher`.
- **Config Strictness Invariant**, **Hermetic Git Test Environment Invariant**, **Documentation Lifecycle** — unchanged, but binding on anything this task adds.

New, introduced by this task and to be recorded in `CONSTRAINTS.md` in the same commit:

- **Glyph Conversion Chokepoint Invariant** — loomyard performs no glyph↔path conversion of its own.
  `glyph.Self` is the only path→glyph call; `Glyph.UnitPath()` is the only glyph→path call; `glyph.Parse` and `Glyph.String` are the only glyph grammar.
  No `TrimSuffix("#")`, no reading `Glyph.Unit` as a disk path, no local regex.

## Testing

**`internal/planparser` — tier1, untagged, no spawns.**
This is the TDD-heaviest surface and the natural place to lead with tests.

- The rewritten classifier: glyph versus path versus handle versus bare `pkg.Symbol`, member glyph, unit self glyph, file self glyph, the extension-versus-no-extension directory rule, and the documented `shedrecipe.lookup`-style edge that used to misclassify.
- Parse-time canonicalization: a plain path and its file self glyph must land on the identical model string; the same for a directory-shaped entry rejected as a finding; canonicalization is a no-op under `language: none`.
- `language:` frontmatter: `go` accepted, `none` accepted, absent defaults to `go`, an unknown value is a hard finding.
- The `Create` sub-bullet arrow grammar and the `Rename` pair grammar, including malformed bullets landing in `RenameRaw` rather than failing the parse.
- Handle well-formedness, dangling handles, unreferenced `Create` handles, and `Create`-handle collisions.
- The syntactic tier of the containment check.
- `RewriteRefs`: substitution across every card and every field, idempotence, and a no-op map leaving bytes untouched.
- Every new check ID's presence and its exact `Check:` string, plus the updated golden fixture round-trip.

**`internal/planglyph` — tier1 where it can be, tagged where it cannot.**

- `Resolve`-verdict policy (`found`, `multipart`, `ambiguous`, `not_found` × unit status) is table-driven against a small fixture repository; the `Create`-target inversion and the new-unit informational finding are the key cases.
- Batched `Name` canonicalization: one call for all `Create` declarations, positional result matching, one bad declaration failing its own entry and not the batch.
- The resolve-backed tier of the containment check.
- Anything reaching `DeltaGit` is `integration`-tagged and runs under the hermetic git test environment.

**Drift, binding and repair — integration-tagged.**

- Exact-tier rename auto-repairs: plan rewritten, revalidated, amendment appended.
- Evidence-tier candidate is surfaced and does **not** rewrite.
- Gate 1: a rename matching a `Rename` card binds and is not reported as drift.
- Gate 2: a renamed symbol nothing references is logged only.
- Handle binding at `record-batch`: exact-tier count match binds; a miss degrades to candidates; a count mismatch means the card is not done.

**Done-checks and blocking policy.**

- A `Create` card whose target still does not resolve blocks; a `Delete` card whose target still resolves blocks; a `Delete` whose target was in fact renamed is caught by the detector rather than passing.
- Scope-guard and evidence-tier findings appear in the report and do **not** block.

**Gate parity and CLI.**

- The existing parity test extends to every new producer row / verb pair.
- Help-tree tests cover the four `lyx quarry` verbs; a golden test pins that each verb's output is the facade's answer unmodified.

**Cross-cutting.**

- A constraint test for the Glyph Conversion Chokepoint Invariant: no production file outside the sanctioned call sites contains a `#`-trimming conversion or reads `Glyph.Unit` as a path.

## Q&A log

- **Q:** Scope of this task? **A:** Full issue #226 adoption minus the kick-start pack, which alone is gated on quarry's M4 measurement.
- **Q:** How does loomyard reach quarry? **A:** `go.mod` dependency on the facade — version-locking by construction: planner answers and validator `Resolve` come from the same quarry version in the same binary, so the planner can never copy a spelling the validator later rejects.
- **Q:** Is the facade cgo-free? **A:** No — only the `glyph` package is.
  The facade links tree-sitter through cgo; `lyx` becomes a cgo binary and every build host needs a C toolchain.
  Accepted deliberately.
- **Q:** Can `CGO_ENABLED=1` be a default rather than set every time? **A:** It already is for native builds with a compiler on PATH; `go env -w CGO_ENABLED=1` pins it per-user, but it is machine-local and does not install a compiler.
- **Q:** How to pin the quarry version? **A:** Cut a real semver tag.
  Version numbers are unimportant before publication; reproducibility is the point.
- **Q:** Do glyphs cover packages and files too? **A:** Yes — unit self form `internal/foo#` and, in Go only, file self form `internal/x/focus.go#`.
  Python has no separate file self glyph; C# has none at all.
- **Q:** So a file has two legal spellings? **A:** Yes, and that is a correctness problem, not a style one — `deriveEdges` matches by exact string equality, so two spellings become two DAG nodes and the edge between the cards silently disappears.
  Resolved by accepting both on the surface and canonicalizing to the glyph form at ingest.
- **Q:** Is the path↔glyph conversion safe? **A:** For Go it is lossless *by contract* (`docs/glyph.md` §3), not by heuristic, and quarry's own `resolve` prescribes the same repair.
  It does not hold for Python or C#.
- **Q:** May loomyard do the conversion itself? **A:** No.
  Not by trimming `#`, and not by reading `Glyph.Unit` as a path — the same Go-only assumption without the trim.
  Both directions are quarry API, which makes `Glyph.UnitPath()` a hard precondition on the disk-shaped checks.
- **Q:** What about a language with no glyph alphabet? **A:** Expected, not hypothetical — `language: none` selects paths-only mode and every glyph behaviour becomes inert.
- **Q:** Why is a bare `pkg.Symbol` a hard finding? **A:** Because it is the one spelling that cannot have come verbatim from a quarry answer.
  The rule is copied-verbatim enforcement expressed as a classifier rule, not a style preference — written this way so nobody later softens it.
- **Q:** Isn't the symbol-granular DAG free? **A:** Nearly, but not entirely.
  Cross-granularity is the hole: a member glyph and a file self glyph in the same package overlap physically with no string equality between them.
  That is a real new check in two tiers, and it must not disappear into "free".
- **Q:** Must a `Create` card's unit already exist? **A:** No, and demanding it is incoherent in Go — a package does not exist apart from its files, so the "unit-creating card" it would require has nothing to target and the regress never grounds.
  A new unit passes, with an informational finding naming it so a typo cannot silently create an unintended package.
- **Q:** Where is the amendment history kept? **A:** `_lyx/plan/amendments.md`, append-only, with `index-file-mismatch` gaining an allowlist entry.
  Commit messages are terminal-flawed: a squash-merge erases them.
- **Q:** What blocks and what is informational? **A:** Done-checks block; evidence-tier drift and the scope guard stay informational.
  Gating on evidence-tier would make loomyard decide what quarry deliberately returns as undecided.
- **Q:** Which quarry verbs does the planner get? **A:** `toc`, `glyphs`, `resolve`, `expand`.
  Not `delta`, not `name` — `name` in an agent's hands is a glyph-spelling machine, which is the one thing the hard rule exists to prevent.
- **Q:** How is the blocked quarry accessor handled? **A:** The disk-shaped checks become their own late cards with the merged accessor and the `go.mod` bump as their precondition; quarry task `glyph-unitpath` is already in motion.
  A temporary loomyard-side helper is banned outright, not deferred.
