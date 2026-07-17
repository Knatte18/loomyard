# Batch: measure-and-writeup

```yaml
task: 'Spike: structured Go reference/call-graph lookup (go/packages / gopls)'
batch: measure-and-writeup
number: 3
cards: 4
verify: go build ./tools/codeintel-poc/
depends-on: [1, 2]
```

## Batch Scope

The empirical heart of the spike: wire the CC-native LSP baseline, **run every arm across the
benchmark symbols to gather real cost + precision numbers**, hand-verify precision against
ground truth, and write the primary deliverable `docs/research/codeintel-spike.md`. Raw output
goes to `.scratch/codeintel/` (gitignored, Shared Decision `measurement-artifacts-to-scratch`);
only distilled tables + the verdict land in the committed doc. This batch produces no new
harness code — it *uses* the batches 1–2 harness — except the throwaway `.lsp.json` for the
CC-native arm. The findings doc content (verdict rubric, benchmark symbol set, transitive
method) is drawn from `_mill/discussion.md`, which each measurement card reads as Context.

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
  LLM-issued reference query) to `.scratch/codeintel/cc-native.md` for card 9 to fold into the
  doc. Do **not** block or mark the task stuck on the CC-native arm. The `.lsp.json` is
  throwaway and is deleted in batch 4.
- **Commit:** `chore(codeintel-poc): add throwaway .lsp.json + characterize CC-native LSP`

### Card 7: Run all arms across the benchmark symbols; capture cost + result sets

- **Context:**
  - `_mill/discussion.md`
  - `tools/codeintel-poc/main.go`
  - `tools/codeintel-poc/gopackages.go`
  - `tools/codeintel-poc/callers.go`
  - `tools/codeintel-poc/gopls.go`
  - `tools/codeintel-poc/callgraph.go`
  - `internal/state/state.go`
  - `internal/shuttleengine/engine.go`
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/output`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Finalize the benchmark symbol set from `_mill/discussion.md` →
  `benchmark-symbols` (interface satisfaction: `shuttleengine.Engine`; generics:
  `state.WriteJSON`/`state.ReadJSON`; a high-fan-in `hubgeometry` exported function chosen by
  actual call count; a method with many call sites; a reflection-adjacent/negative case around
  `internal/output` or yaml/json struct tags) — record the exact chosen symbols and *why* to
  `.scratch/codeintel/symbols.md`. Best-effort `go install golang.org/x/tools/gopls@latest`
  and record the resolved `gopls version`; if the install is blocked, note it and mark the
  gopls arms docs-characterized (Shared Decision `network-prerequisites`). Then run, for each
  symbol, the applicable harness modes (`refs`, `callers`, `gopls-refs`, `gopls-cli-refs`,
  `callgraph -algo=cha|rta|vta`), capturing to `.scratch/codeintel/` (gitignored): the
  **warm-up-once-per-run tax** and **per-query steady-state** timings separately per mechanism,
  and the full reference/caller/transitive-caller position sets. Record the machine + Go
  toolchain version alongside (numbers are noisy — order-of-magnitude, matching
  `docs/benchmarks/` house style). No committed files — all output is scratch; this card's
  product is the raw data cards 8–9 consume.
- **Commit:** `chore(codeintel-poc): capture cost + result sets across benchmark symbols`

### Card 8: Precision cross-check vs ground truth (direct + transitive)

- **Context:**
  - `_mill/discussion.md`
  - `internal/state/state.go`
  - `internal/shuttleengine/engine.go`
  - `internal/shuttleengine/claudeengine`
  - `internal/hubgeometry/hubgeometry.go`
  - `internal/output`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** For each benchmark symbol, establish **direct-reference ground truth** by
  hand — `grep`/read the actual call sites — and diff it against the harness's `refs`/`callers`
  sets from card 7, counting **false negatives** (real caller missed — disqualifying per the
  rubric) and **false positives** (bounded/explainable tolerated). Record per-symbol counts to
  `.scratch/codeintel/precision.md`. For **transitive** precision, apply
  `_mill/discussion.md` → Testing → CHA/RTA/VTA method exactly: treat **VTA as the
  reference/gold** set and quantify how much **CHA and RTA over-approximate relative to VTA**
  (set-difference sizes from card 7's dumps); **additionally** pick one deliberately small,
  shallow symbol (few callers, ≤2–3 hops), hand-enumerate its *complete* transitive caller set
  as an **absolute-truth anchor**, and confirm VTA misses none of it. Record the divergence
  table + the anchor result to `.scratch/codeintel/precision.md`. No committed files.
- **Commit:** `chore(codeintel-poc): precision cross-check vs hand-verified ground truth`

### Card 9: Write the findings-and-how-to doc

- **Context:**
  - `_mill/discussion.md`
  - `docs/research/session-fork-spike.md`
- **Edits:** none
- **Creates:**
  - `docs/research/codeintel-spike.md`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Write `docs/research/codeintel-spike.md` following the
  `docs/research/session-fork-spike.md` shape (verdict up front, then method, data, lessons).
  It must contain, from cards 6–8's `.scratch/codeintel/` data: (1) the **adopt/defer/drop
  verdict** applying the `recommendation-rubric` from `_mill/discussion.md` (two-dimensional
  cost + precision; no-false-negatives-on-ordinary-code required for adopt), with a **separate
  callgraph sub-verdict**; (2) an inline **cost table** — warm-up-once-per-run tax + per-query
  steady-state, per mechanism (`go/packages` in-process, `gopls` held-open, `gopls` cold CLI,
  CC-native characterization); (3) an inline **precision table** — per symbol, false-neg /
  false-pos vs ground truth; (4) the **CHA/RTA/VTA** divergence table + the small-symbol
  soundness anchor + **the exact callgraph roots used**; (5) the **run-scoped warm-host model**
  framing and the CC-native **architectural mismatch** note from the discussion; (6) if (and
  only if) the verdict is adopt, a **runnable how-to recipe** (exact imports / call sequence
  for the in-process arm, or the exact gopls LSP request) — verified by having actually run it
  in card 7, not written from memory. State the machine/toolchain and that numbers are
  order-of-magnitude. Do **not** touch `docs/modules/`, `docs/overview.md`, or
  `docs/roadmap.md` (Documentation Lifecycle — this is a spike, not a milestone).
- **Commit:** `docs(research): codeintel-spike findings + recommendation`

## Batch Tests

`verify: go build ./tools/codeintel-poc/` (Go native runner, no `PYTHONPATH=` prefix) — the
harness is still present in this batch and must keep compiling; the measurement/doc cards do
not change harness source, so the build is a cheap regression guard. The batch's real output
(cost + precision numbers, the findings doc) is inherently empirical and is validated by the
hand-verified ground-truth cross-check in card 8, not by an automated test — this is a
measurement spike, so there is no runnable assertion surface for the numbers themselves.
