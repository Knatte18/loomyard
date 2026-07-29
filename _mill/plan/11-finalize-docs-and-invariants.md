# Batch: finalize-docs-and-invariants

```yaml
task: 'codeintel V1 — LSP-backed lookups (Go-only, CLI + EnsureServer)'
batch: finalize-docs-and-invariants
number: 11
cards: 5
verify: go build ./... && go vet ./...
depends-on: [10]
```

## Batch Scope

Lands every remaining Documentation Lifecycle obligation in one place,
last, because none of it can be written truthfully until the whole
feature set — `EnsureServer`, the toolchain manager, `definition`/
`symbol`, batch mode, the exit-code contract — actually exists.
Mirroring this same branch's own `native-clients-migration` plan
precedent (`_mill/plan/09-docs-and-invariants.md` on that branch's
history), package-doc rewrites are deliberately deferred to one final
batch rather than incrementally patched across ten prior batches: a
143-line architectural doc comment like `internal/codeintelengine/doc.go`
edited in small pieces across many separate implementer sessions is far
more likely to drift internally inconsistent than one coherent rewrite
written once the target shape is fully known.

Two doc sites are actively **false** the moment this task's earlier
batches land, not merely incomplete, and both repeat the same claim:
`internal/codeintelengine/doc.go`'s opening describes the package as
having "no in-process go/packages arm" and "No lyx-owned server
install/pin story... lyx does not install, version-pin, or manage
language-server binaries itself" — both false the instant the toolchain
manager (batch 2) and `EnsureServer` (batches 5–7) land. `docs/overview.md`'s
codeintel module-table entry repeats the "references-only" framing.

## Cards

### Card 43: Rewrite `internal/codeintelengine/doc.go`

- **Context:**
  - `internal/codeintelengine/ensureserver.go`
  - `internal/codeintelengine/toolchain.go`
  - `internal/codeintelengine/daemonstate.go`
  - `internal/codeintelengine/definition.go`
  - `internal/codeintelengine/symbol.go`
  - `internal/codeintelengine/refs.go`
  - `internal/codeintelengine/registry.go`
  - `manifest/designs/codeintel-redesign.md`
- **Edits:**
  - `internal/codeintelengine/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Six sections change or are added, absorbing
  `manifest/designs/codeintel-redesign.md`'s durable rationale before
  card 46 deletes that file. **(1) The opening paragraph** currently
  describes the package as finding "every reference to a symbol name or
  an explicit source position" — extend it to name all three verbs
  (references, definitions, workspace-symbol search) and state plainly
  that V1 wires a full daemon lifecycle for Go while the other four
  languages keep the original cold-spawn-per-call design. **(2) A new
  "The `EnsureServer` seam" section**: the `ensureServer`/`ensureNative`/
  `ensureSupervised` split, that `entry.HasNativeDaemon` gates whether a
  language ever calls into this machinery at all (Go only in V1;
  Python/C#/TypeScript/Rust keep the original spawn-per-call path
  completely untouched), and — the plan's own resolved-tension point,
  worth stating in the durable doc since it is genuinely non-obvious from
  the code alone — exactly which connection-teardown rule applies to
  each `connKind` and why (`native`: safe to `close()`/`kill()`, it's
  lyx's own disposable per-call proxy, not the shared daemon; `supervised`:
  never `close()`/`kill()` it, the dial is meant to outlive this call;
  `legacy`: unchanged, closes the real server it directly owns). State
  that `ensureSupervised` is fully built and proven (its own integration
  test against a plain `gopls`) but has no live V1 dispatch path — no
  registry entry requests it yet — so a future `ty`/OmniSharp adapter is
  what will first reach it through `ensureServer`. **(3) A new "Go
  toolchain manager" section**: `$PATH` is never consulted for Go;
  `resolveGoToolchain` installs a pinned `gopls` version
  (`registry.go`'s `builtins()["go"].PinnedVersion`) into
  `os.UserCacheDir()/lyx/tools/go/<version>`, fenced by its own
  install lock distinct from the daemon spawn-race lock; state that this
  cache root is deliberately outside the Hub Geometry Invariant's scope
  (machine-global, not worktree/hub geometry) and why (shared across
  every worktree on the machine, the entire point of pinning-and-caching).
  **(4) A new "Daemon state and concurrency" section**: the `.lyx/codeintel/<lang>/`
  state file + lock pair (`hubgeometry.Layout.CodeintelDaemonStateFile`/
  `CodeintelDaemonLock`), why `.lyx` and never `_lyx` (ephemeral,
  machine-bound runtime state must never be git-committed), the two-part
  staleness check (dead PID via `proc.IsAlive`, or a `ProtocolVersion`
  mismatch — and that this protocol version is lyx's own wire-compat
  marker for the supervised daemon, not `gopls`'s version), the
  deterministic (not randomly chosen) socket path and why that
  simplifies stale-socket cleanup across restarts, and the bounded
  retry-exhaustion contract (`ErrServerSpawnTimeout`) a losing caller
  hits if a live winner never produces a healthy daemon in time. **(5)
  Rewrite the exit-code paragraph** (currently absent — this package's
  doc has never described CLI-level exit codes, since that contract did
  not exist before this task) — actually, note for the writer: exit
  codes are `internal/codeintelcli`'s concern, not this package's: state
  instead, briefly, that `ErrAmbiguousSymbol`/`ErrSymbolNotFound` are
  what let the CLI layer distinguish "ambiguous" from "not found"
  without parsing error strings, and point to `internal/codeintelcli`'s
  own doc (card 44) for the actual contract. **(6) Update "Scope
  boundaries"**: remove the now-false "No lyx-owned server install/pin
  story" bullet entirely (replaced by section 3 above); keep the
  "No call hierarchy, no implementation" and "No in-process go/packages
  arm" bullets, since both remain true in V1; add that `symbol`
  deliberately does not share `resolvePosition`'s ambiguity-collapsing
  behavior, with a one-line pointer to `symbol.go`'s own doc comment for
  the full rationale rather than duplicating `symbol-semantics`'s
  reasoning here.
- **Commit:** `docs(codeintelengine): rewrite package doc for EnsureServer, the toolchain manager, and the daemon lifecycle`

### Card 44: Rewrite `internal/codeintelcli`'s package doc

- **Context:**
  - `internal/codeintelengine/doc.go`
  - `internal/codeintelcli/cli.go`
- **Edits:**
  - `internal/codeintelcli/cli.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** `cli.go`'s package doc comment currently reads
  "currently exposing a single 'refs' verb that looks up every reference
  to a symbol name or an explicit source position" — false since batch 9.
  Rewrite it to name all three verbs (`refs`/`definition`/`symbol`) and
  state the exit-code contract precisely, since this is the one place in
  the codebase that contract's full shape should be documented in prose
  rather than only in code: single-argument calls exit `0` (found), `1`
  (not found or any other engine error), or `2` (ambiguous — the
  response body still carries `"ok":true` with a `"candidates"` field,
  since multiple valid answers is not a process error); a call with 2+
  positional arguments switches to batch mode, returning one JSON entry
  per symbol under a top-level `"results"` array (never the single-symbol
  shape), with a 4th per-entry status, `"error"` (a genuine
  infrastructure failure, distinct from a confirmed-absent `"not_found"`),
  and the process exit code set to the **worst** status present across
  the batch, ranked `found(0) < not_found(1) < ambiguous(2) < error(3)`.
  State explicitly that `symbol` never produces `"ambiguous"`/exit `2`
  in either shape, since returning several workspace-symbol candidates
  is its ordinary successful answer, not an error state needing
  disambiguation.
- **Commit:** `docs(codeintelcli): rewrite package doc for three verbs and the batch-mode exit-code contract`

### Card 45: Update `docs/overview.md`'s codeintel module-table entry

- **Context:**
  - `internal/codeintelengine/doc.go`
  - `internal/codeintelcli/cli.go`
- **Edits:**
  - `docs/overview.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Two edits, both in the "Modules" section (`##
  Modules`). **(1)** The codeintel bullet's closing status clause reads
  "✅ Implemented (v1 scope: references-only, no call hierarchy, no
  in-process Go arm)" — replace with a clause naming what V1 actually
  ships: three verbs (`refs`/`definition`/`symbol`, each with single- and
  batch-argument modes), an `EnsureServer` daemon lifecycle wired for Go
  (`native`, `gopls -remote=auto`) with the `supervised` strategy built
  and proven standalone for a future non-Go language, and a Go toolchain
  manager that pins and installs `gopls` independent of `$PATH` — while
  still noting what remains true and unchanged: no call hierarchy, no
  `implementation` method, no in-process `go/packages` arm. Also update
  the bullet's verb-list clause (currently `` `lyx codeintel refs
  <symbol|file:line:col>` ``) to `` `lyx codeintel
  refs|definition|symbol <symbol|file:line:col>` ``. **(2)** The "Other
  docs" section's `internal/codeintelengine` package-documentation
  pointer (currently "multi-language reference lookup over LSP (`lyx
  codeintel refs`)") gets the same verb-list correction, to
  "`lyx codeintel refs|definition|symbol`".
- **Commit:** `docs(overview): correct codeintel's module-table entry for V1's full verb set and EnsureServer`

### Card 46: Flip `manifest/roadmap.md` Planned→Done, delete the design doc

- **Context:**
  - `internal/codeintelengine/doc.go`
  - `docs/overview.md`
- **Edits:**
  - `manifest/roadmap.md`
- **Creates:** none
- **Deletes:**
  - `manifest/designs/codeintel-redesign.md`
- **Moves:** none
- **Requirements:** Move the codeintel bullet from `## Planned` to `##
  Done`. Unlike the `native-clients-migration` plan's precedent (which
  had to **rewrite** its Planned entry into Done because the shipped
  scope was narrower than what was originally planned), this Planned
  bullet's own title is "V1 Go-only, built for multi-language" — i.e.
  the Planned item's stated scope *is* this task's V1, and nothing of
  that stated scope remains unshipped once this task's batches land (the
  `ty`/OmniSharp adapters were always framed, both in the Planned bullet
  and the design doc, as future work beyond this item's own definition —
  per `_mill/discussion.md`'s Technical context section, which already
  worked out this exact precedent-vs-this-task distinction). The Done
  entry can therefore summarize what shipped in the Planned bullet's own
  words, trimmed of the now-resolved `[designs/codeintel-redesign.md]`
  link (the target is deleted by this same card) and of forward-looking
  language ("will be" → "is") — **and corrected on one naming point the
  Planned bullet gets wrong**: it describes the CLI surface as
  `` `lyx codeintel references|definition|symbol` ``, but the shipped
  verb (unchanged by this task, per `cli-verb-naming`) is `refs`, not
  `references` — card 45 already makes this same correction in
  `docs/overview.md`; apply it here too rather than carrying the stale
  name into the permanent Done record. Delete
  `manifest/designs/codeintel-redesign.md` in the same commit — its
  durable rationale is already folded into
  `internal/codeintelengine/doc.go` (card 43) and
  `internal/codeintelcli/cli.go`'s package doc (card 44), per the
  Documentation Lifecycle.
- **Commit:** `docs(roadmap): move codeintel V1 to Done, delete the design doc`

### Card 47: Whole-repo verification

- **Context:**
  - `CONSTRAINTS.md`
  - `cmd/lyx/tierpurity_test.go`
  - `cmd/lyx/hermeticenv_test.go`
  - `cmd/lyx/sandbox_coverage_test.go`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Verification-only card, no diff. Run, in order:
  `go build ./...`; `go vet ./...`; the untagged Tier-1 suite (`go test
  -count=1 ./...`); then the integration-tagged Tier-2 suite on a
  machine with `gopls` installed (`go test -tags integration -race
  -count=1 ./internal/codeintelengine/... ./internal/proc/...`) — `-race`
  matters here specifically because batch 6's spawn-race lock and batch
  4's concurrent state-file access are exactly the kind of
  shared-mutable-coordination code a single-threaded run cannot catch
  bugs in, mirroring the `native-clients-migration` plan's identical
  reasoning for its own shared-state batches. Explicitly re-run the
  guard tests this task's batches touched or newly rely on:
  `internal/codeintelengine/leaf_enforcement_test.go` (now allowlisting
  `internal/lock` and `internal/proc`), `cmd/lyx/tierpurity_test.go`,
  `cmd/lyx/hermeticenv_test.go`, `cmd/lyx/sandbox_coverage_test.go`
  (the pre-existing `codeintel` exclusion entry, now backed by a matching
  `CONSTRAINTS.md` line since batch 2), `cmd/lyx/helptree_test.go`,
  `cmd/lyx/registration_test.go`, `cmd/lyx/longlist_test.go`,
  `cmd/lyx/drift_test.go`. Report the actual result — a suite that was
  not run (in particular the integration tier, which needs a real
  network-installed `gopls`) is not a suite that passed, and this card's
  whole purpose is closing that gap before the task is considered done.
- **Commit:** none

## Batch Tests

`verify:` runs `go build ./... && go vet ./...` — deliberately the
cheapest possible mechanical gate, since this batch's real verification
is card 47's manual, explicit whole-repo run (including the
integration tier no automated per-batch `verify:` in this plan ever
exercises). Consider setting `pipeline.done_gate` in `mill-config.yaml`
to `go build ./... && go vet ./... && go test -count=1 ./...` for this
repo generally: every batch in this plan scopes its own `verify:` to the
one or two packages it touches, so nothing catches a regression in an
unrelated package until this batch's card 47 runs at the very end.
</content>
