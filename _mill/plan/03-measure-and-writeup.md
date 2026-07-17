# Batch: measure-and-writeup

```yaml
task: 'Spike: structured Go reference/call-graph lookup (go/packages / gopls)'
batch: measure-and-writeup
number: 3
cards: 2
verify: go build ./...
depends-on: [1, 2]
```

## Batch Scope

The empirical heart of the spike, in two committing cards: Card 6 wires the throwaway
`.lsp.json` CC-native baseline (committing `.lsp.json`); Card 7 **runs every arm across the
benchmark symbols, hand-verifies precision against ground truth, and writes the primary
deliverable `docs/research/codeintel-spike.md`** (committing the doc). Card 7 is a single card
because the measurement produces only gitignored `.scratch/codeintel/` output (Shared Decision
`measurement-artifacts-to-scratch`) with no separately-committable artifact — the doc is the
only tracked product, so run + cross-check + write are one commit rather than three
empty-commit cards (the r1 review's blocking finding). This batch adds no new harness code — it
*uses* the batches 1–2 harness. The findings-doc content (verdict rubric, benchmark symbol
set, transitive method) is drawn from `_mill/discussion.md`, read as Context.

## Cards

### Card 6: CC-native LSP baseline (`.lsp.json`) + recipe/outcome

- **Context:**
  - `_mill/discussion.md`
- **Edits:** none
- **Creates:**
  - `.lsp.json`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create the repo-root `.lsp.json` exactly per `_mill/discussion.md` →
  Technical context → CC-native-LSP wiring: `{"go": {"command": "gopls", "args": [],
  "extensionToLanguage": {".go": "go"}, "transport": "stdio", "initializationOptions": {},
  "settings": {}, "maxRestarts": 3}}`. Then attempt to characterize Claude Code's native LSP
  tool (`ENABLE_LSP_TOOL=1`): enabling it requires an interactive CC session with that env set
  and `gopls` installed, which the mill-go implementer session likely **cannot** toggle. Per
  `_mill/discussion.md` → `cc-native-lsp-mismatch` (Accepted-outcome note), a **docs-only
  characterization is an accepted, non-blocking outcome**: if the tool cannot be driven here,
  record that fact plus the intended recipe (`ENABLE_LSP_TOOL=1` + this `.lsp.json`, then an
  LLM-issued reference query) to `.scratch/codeintel/cc-native.md` for card 7 to fold into the
  doc. Do **not** block or mark the task stuck on the CC-native arm. The `.lsp.json` is
  throwaway and is deleted in batch 4.
- **Commit:** `chore(codeintel-poc): add throwaway .lsp.json + characterize CC-native LSP`

### Card 7: Run all arms, cross-check precision, and write the findings doc

- **Context:**
  - `_mill/discussion.md`
  - `docs/research/session-fork-spike.md`
  - `tools/codeintel-poc/main.go`
  - `tools/codeintel-poc/gopackages.go`
  - `tools/codeintel-poc/callers.go`
  - `tools/codeintel-poc/gopls.go`
  - `tools/codeintel-poc/callgraph.go`
  - `internal/state/state.go`
  - `internal/shuttleengine/engine.go`
  - `internal/shuttleengine/claudeengine`
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/output`
- **Edits:**
  - `tools/codeintel-poc/gopackages.go` — `resolveSymbol`'s `<Type>.<Method>` branch
    always wraps the named type in `types.NewPointer` before calling
    `types.LookupFieldOrMethod`, which is correct for a concrete type's method set but
    returns `nil` for an **interface** type (`LookupFieldOrMethod` on a
    pointer-to-interface never resolves — the interface's method set lives on the
    named type itself, not a synthesized pointer to it). This silently blocks
    resolving the batch's own primary interface-satisfaction benchmark symbol
    (`shuttleengine.Engine.Prepare`) with `resolve symbol ...: method "Prepare" not
    found on type "Engine"`, discovered while running Card 7 step (b). Fix:
    branch on whether the named type's underlying type is `*types.Interface` and, if
    so, look up on the named type directly instead of a pointer to it.
  - `tools/codeintel-poc/callgraph.go` — `transitiveCallers`'s generic-origin seed
    condition `fn.Origin() == origin` is unconditionally true for **any two
    non-generic functions**, not just for `target` itself: `(*ssa.Function).Origin()`
    returns `nil` for a function that is not a generic instantiation (per its own
    doc comment), so for a non-generic `target`, `origin := target.Origin()` is
    `nil`, and every other non-generic `fn` in the graph also has `fn.Origin() ==
    nil`, making the loop seed the walk from **every non-generic node in the whole
    program**, not just target's. Confirmed empirically while running Card 7 step
    (b): `-symbol=...claudeengine.Claude.Prepare` and
    `-symbol=...hubgeometry.Layout.WeftWorktree` (two unrelated targets) produced
    byte-identical 15,872-entry transitive-caller sets under `-algo=cha`. Fix: match
    a graph node's function against target with `fn == target ||
    (fn != nil && fn.Origin() == target)` — the first arm covers the ordinary
    non-generic case exactly, the second still covers every generic instantiation
    of a generic target (resolveSymbol always resolves to the generic origin
    declaration, never a specific instantiation, so `target` is always the origin
    to compare instantiations' `Origin()` against).
- **Creates:**
  - `docs/research/codeintel-spike.md`
- **Deletes:** none
- **Moves:** none
- **Requirements:** This is one card because the measurement exists only to produce the doc,
  and all intermediate output is gitignored `.scratch/` (no separately-committable artifact) —
  the single committed product is the findings doc. Perform, in order:
  **(a) Choose symbols.** Finalize the benchmark set from `_mill/discussion.md` →
  `benchmark-symbols` (interface satisfaction: `shuttleengine.Engine`; generics:
  `state.WriteJSON`/`state.ReadJSON`; a high-fan-in `hubgeometry` exported function chosen by
  actual call count; a method with many call sites; a reflection-adjacent/negative case around
  `internal/output` or yaml/json struct tags); record the chosen symbols and why to
  `.scratch/codeintel/symbols.md`.
  **(b) Run every arm.** Best-effort `go install golang.org/x/tools/gopls@latest` (record the
  resolved version; if blocked, mark the gopls arms docs-characterized per Shared Decision
  `network-prerequisites`). For each symbol run the applicable harness modes (`refs`,
  `callers`, `gopls-refs`, `gopls-cli-refs`, `callgraph -algo=cha|rta|vta`), capturing to
  `.scratch/codeintel/` (gitignored): the **warm-up-once-per-run tax** and **per-query
  steady-state** timings separately per mechanism, plus the full reference / caller /
  transitive-caller position sets. Record machine + Go toolchain version.
  **(c) Cross-check precision.** For each symbol establish **direct-reference ground truth** by
  hand (grep/read actual call sites) and diff against the harness `refs`/`callers` sets,
  counting **false negatives** (disqualifying per the rubric) and **false positives**
  (bounded/explainable tolerated). For **transitive**, apply `_mill/discussion.md` → Testing →
  CHA/RTA/VTA method exactly: treat **VTA as the reference/gold** set, quantify how much CHA
  and RTA over-approximate relative to VTA, **and** hand-enumerate the *complete* transitive
  caller set for one deliberately small, shallow symbol (≤2–3 hops) as an absolute-truth
  anchor confirming VTA misses none of it.
  **(d) Write `docs/research/codeintel-spike.md`** following the
  `docs/research/session-fork-spike.md` shape (verdict up front, then method, data, lessons),
  containing: (1) the **adopt/defer/drop verdict** applying the `recommendation-rubric` from
  `_mill/discussion.md`, with a **separate callgraph sub-verdict**; (2) an inline **cost
  table** (warm-up + steady-state per mechanism: `go/packages` in-process, `gopls` held-open,
  `gopls` cold CLI, CC-native characterization); (3) an inline **precision table** (per symbol,
  false-neg/false-pos vs ground truth); (4) the **CHA/RTA/VTA** divergence table + the
  small-symbol soundness anchor + **the exact callgraph roots used**; (5) the **run-scoped
  warm-host model** framing and the CC-native **architectural mismatch** note; (6) if and only
  if the verdict is adopt, a **runnable how-to recipe** verified by having actually run it in
  step (b). State machine/toolchain; numbers are order-of-magnitude. Fold in the CC-native
  characterization from `.scratch/codeintel/cc-native.md` (card 6). Do **not** touch
  `docs/modules/`, `docs/overview.md`, or `docs/roadmap.md` (Documentation Lifecycle — a spike
  is not a milestone).
- **Commit:** `docs(research): codeintel-spike findings + recommendation`

## Batch Tests

`verify: go build ./tools/codeintel-poc/` (Go native runner, no `PYTHONPATH=` prefix) — the
harness is still present in this batch and must keep compiling; cards 6–7 do not change harness
source, so the build is a cheap regression guard. The batch's real output (cost + precision
numbers, the findings doc) is inherently empirical and is validated by the hand-verified
ground-truth cross-check in card 7 step (c), not by an automated test — this is a measurement
spike, so there is no runnable assertion surface for the numbers themselves.
