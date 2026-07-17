# Discussion: Spike: structured Go reference/call-graph lookup (go/packages / gopls)

```yaml
task: 'Spike: structured Go reference/call-graph lookup (go/packages / gopls)'
slug: codeintel-spike
status: discussing
parent: main
```

## Problem

An LLM implementer's most expensive per-turn work is usually not re-deriving general
codebase orientation — it is **finding every call site / reference affected by the one
specific edit a card makes**, and confirming every follow-on fix lands. Today that
ripple/impact search is done via LLM-driven Grep/Read: slow, imprecise (textual, not
semantic), and token-hungry. Go has a direct structural analog to what Roslyn gives
C#/.NET — `go/packages` + `go/types` (whole-module, type-checked semantic analysis),
which `gopls` (the official language server) is built on. It can answer "find all
references", "call hierarchy" (incoming/outgoing), and "implementations" precisely and
deterministically, with `golang.org/x/tools/go/callgraph` (CHA/RTA/VTA) for transitive
impact.

**Why now:** the idea surfaced while discussing webster's parallel-execution design
(`docs/modules/websterv2_extension.md`, currently an uncommitted draft in the
`loomyard`/main worktree). Its value is **not** contingent on webster or loom landing —
it would cut tool-call/token burden for `builder`'s implementers running in production
today, and for `burler`'s reviewers. This task is a **standalone feasibility spike**:
measure cost and precision on *this actual repo*, then recommend adopt-now / defer / drop.
It is measurement and a prototype, **not a production build** — no integration into
`builder`/`webster`/`burler`, and no dependency on those landing first.

## Scope

**In:**

- A **throwaway harness** under `tools/codeintel-poc/` (run manually via `go run`, not a
  registered `lyx` subcommand and not a `go test`) that, given a symbol on this repo,
  returns a structured list of references / callers.
- Measure, on this repo, **three mechanisms**:
  1. `go/packages` + `go/types` **in-process** (primary candidate).
  2. `gopls` as a **held-open LSP subprocess** over stdio (comparison).
  3. Claude Code's **native LSP tool** (`ENABLE_LSP_TOOL=1` + `.lsp.json`) as a baseline.
- Measure **cost** as two distinct numbers under a **run-scoped warm-host model** (see
  Decisions → warm-host-model): the **one-time warm-up tax per run** (load + type-check
  the whole module) and the **per-query steady-state** cost after warm-up.
- Measure **precision** on a fixed set of real loomyard symbols chosen to stress static
  analysis (interface satisfaction, generics, reflection-adjacent code) — see
  Decisions → benchmark-symbols.
- **Transitive impact:** a `callgraph` **CHA vs RTA vs VTA** precision/cost comparison on
  real symbols, graded separately from direct references.
- A **findings-and-how-to doc** at `docs/research/codeintel-spike.md` carrying the verdict,
  the cost/precision numbers inline, the CHA/RTA/VTA sub-verdict, and — if the verdict is
  adopt — a concrete "here is the mechanism and how a future Go verb calls it" recipe
  (including the exact imports / gopls invocation that worked).

**Out:**

- No production Go verb; no `<module>cli`/`<module>engine` split; no registration in
  `newRoot()`; no integration into `builder`/`webster`/`burler`. Those are a **follow-up
  adoption task** if the spike recommends adopting.
- No dependency on `webster` or `loom` landing first.
- No full IDE-feature parity with `gopls` — only the references / call-hierarchy /
  implementations slice, plus the callgraph comparison, matters here.
- The harness is **deleted before merge** (like the `session-fork-spike`'s `tools/fork-poc/`);
  nothing ships to `main` except `docs/research/codeintel-spike.md`. The `golang.org/x/tools`
  dependency the harness pulls in (both **`go.mod` and `go.sum`**) and any throwaway
  **`.lsp.json`** are **reverted before merge** — `x/tools` is not vetted or pinned for
  production by this task. The plan MUST carry this as an **explicit final step** (see
  Testing → final revert-and-verify step), not an assumption: revert harness + `go.mod` +
  `go.sum` + `.lsp.json`, then confirm the branch's merge diff against `main` is
  **doc-only** (`docs/research/codeintel-spike.md` and nothing else).

## Decisions

### deliverable-shape

- Decision: Deliver a **throwaway harness** in `tools/codeintel-poc/` (committed
  incrementally on the task branch, **removed before merge**, referenceable via the
  archive tag) plus the **primary deliverable**: a findings-and-how-to doc at
  `docs/research/codeintel-spike.md` with the cost/precision numbers inline.
- Rationale: Mirrors the established `session-fork-spike` precedent exactly
  (`docs/research/session-fork-spike.md` + throwaway `tools/fork-poc/`, deleted before
  merge). CONSTRAINTS.md's CLI/Cobra Invariant explicitly blesses "a throwaway
  proof-of-concept meant to be deleted once it proves its point" (the `muxpoc`
  precedent). The user stated the **doc is the more important artifact** — the prototype
  is disposable instrumentation, the doc is what survives and is consumed by the
  follow-up adoption task.
- Rejected: (a) a real `codeintelcli`+`codeintelengine` module now — contradicts
  "no full build / no integration" and is premature before precision is proven; (b) a
  `muxpoc`-style throwaway *registered* subcommand — needless Cobra/helptree/registration
  ceremony for a thing that never ships; (c) keeping the harness permanently in `tools/` —
  contradicts the throwaway precedent and would drag the unvetted `x/tools` dep into `main`.

### mechanisms-measured

- Decision: Measure all **three** mechanisms, with `go/packages`+`go/types` **in-process
  as the primary**, `gopls`-held-open-subprocess as the comparison, and Claude Code's
  native LSP tool as a baseline.
- Rationale: In-process `go/packages` is the best architectural fit for loomyard — it is a
  native computation the long-lived orchestrator process invokes and folds into a digest
  (the file-contract model), and it requires **no separate process at all** (see
  warm-host-model). `gopls`-subprocess is the natural comparison (external binary + LSP
  protocol overhead vs. reimplementing a small slice of gopls's logic). The CC-native LSP
  tool is the cheapest thing to try and driving it once *is* the "how you use LSP" recipe.
- Rejected: measuring only the brief's original two (drops the CC-native baseline, which
  the `zircote/go-lsp` finding surfaced); CC-native-first-and-maybe-stop (the
  architectural mismatch below means it likely cannot be the answer, so it is a baseline,
  not the lead candidate).

### cc-native-lsp-mismatch

- Decision: Include Claude Code's native LSP tool as a **measured baseline** — actually
  wire up `ENABLE_LSP_TOOL=1` + a `.lsp.json` pointing at `gopls` and drive **one real
  reference query, timeboxed**; if it misbehaves within the timebox, fall back to
  characterizing it from docs. Document the recipe regardless.
  - **Accepted-outcome note:** enabling `ENABLE_LSP_TOOL=1` requires an interactive Claude
    Code session with that env toggled and `gopls` installed, which may not be togglable in
    the spike author's harness at all. A **docs-only characterization is an accepted,
    non-blocking outcome** if the tool cannot be enabled here *at all* — not only if it
    "misbehaves" once enabled. The spike does not stall or block on making the native tool
    runnable; the two Go mechanisms are the load-bearing measurements.
- Rationale: The repo the user flagged, `github.com/zircote/go-lsp`, is **not a Go library**
  — it is a thin Claude Code *plugin* (only `.go` file is a test sample): a `.lsp.json`
  launching `gopls` over stdio, a setup command installing gopls + lint/security tools, and
  an **empty** `hooks.json` (despite advertising "14 hooks"). Its real signal is that
  Claude Code has a **native LSP tool** (`ENABLE_LSP_TOOL`). Driving it once produces the
  usage recipe the user asked to document.
- Rejected: docs-only characterization (no working recipe — the user explicitly wants the
  "how you use LSP" recipe); full parity benchmark through the whole symbol set (wasted
  effort given the mismatch below).
- **Architectural mismatch to record in the findings:** loomyard's agent-execution model is
  the **file contract** — the Go orchestrator invokes a capability, gets a bounded
  structured result, and hands the LLM a digest. A native CC LSP tool is an
  *interactive-LLM-chooses-to-call-it* capability, not something the orchestrator
  (`builder`/`webster`/`burler`) can invoke deterministically and fold into a digest. So it
  likely solves a different problem than the deterministic, orchestrator-driven,
  per-card impact search this spike targets. Measure it as a baseline; note the mismatch as
  a mark against adopting it as *the* answer.

### warm-host-model

- Decision: Assume and measure a **run-scoped, orchestrator-held** warm model — the warm
  package graph's owner is a single orchestrator **run** (`lyx builder run` /
  `lyx webster run` / a card's implementer session), warmed once at run start and torn down
  at run end. **Explicitly reject** a machine-wide always-on daemon. Do **not** measure
  "cold" as a per-query number; measure it as the **once-per-run warm-up tax**, and measure
  per-query **steady-state** separately.
- Rationale: loomyard is one-shot / daemonless (`docs/overview.md` principle 3) — no
  machine-wide resident server. But `go/packages`/`gopls` must load and type-check the
  whole module (seconds on this repo) before answering anything, so a per-query cold load is
  meaningless for the real consumer, which is an orchestrator **run**, not a human at a
  prompt. The run is *already* a long-lived process; it is the natural amortization
  boundary and the natural warm-graph owner. Crucially, the two mechanisms differ in whether
  a separate process even exists:
  - **`go/packages` in-process** → the warmth is just **memory the orchestrator's own Go
    process holds**. No new process, no daemon, nothing external. This dissolves the daemon
    question entirely and is the cleanest fit with principle 3.
  - **`gopls`-subprocess** → the orchestrator **spawns and supervises a held-open gopls
    child** for the run's duration and tears it down at run end. A subprocess + external
    binary, but still run-scoped, **not** a machine daemon. (The `gopls` *CLI* form,
    `gopls references …`, starts fresh each call = cold every time = do not use as the warm
    path; only the held-open LSP-server mode is warm.)
- Rejected: (a) a machine-wide/persistent server (external always-on gopls or a
  `lyx codeintel serve` daemon) — reopens the daemon question principle 3 closes, needs
  lifecycle/staleness management across worktree churn, worst fit; (b) cold-only —
  meaningless per-query, the user's own objection; (c) warm-only without the warm-up number
  — the one-time tax is exactly what the adopt/defer verdict hinges on.

### transitive-impact-in-scope

- Decision: Transitive impact **is in scope**. Run a `golang.org/x/tools/go/callgraph`
  **CHA vs RTA vs VTA** precision/cost comparison on real symbols, and grade the transitive
  verdict **separately** from the direct-reference verdict.
- Rationale: The dominant use case is "who *directly* calls the thing I'm about to edit"
  (answered by `references` / `callHierarchy` cheaply and precisely), but the user wants the
  transitive story characterized too. CHA is cheap/over-broad, RTA is mid, VTA is
  precise/costly — the spike must characterize which (if any) lands in a usable
  precision/cost spot on this repo rather than assuming CHA is good enough.
- Rejected: direct-references-only (the user chose to include the callgraph comparison).

### benchmark-symbols

- Decision: The spike author picks a **fixed handful** of real loomyard symbols targeting
  the brief's stress cases; finalize the exact set during the spike. **Starter candidates
  found during exploration:**
  - **Interface satisfaction:** `shuttleengine.Engine` (the provider seam;
    `internal/shuttleengine/engine.go:142`) with its real implementer under
    `internal/shuttleengine/claudeengine/` plus test doubles; also `builderengine.Starter`
    / `OrchestratorHandle` and the `clock` test-double interfaces. Exercises "find
    implementations" and interface-dispatch call-hierarchy — where CHA/RTA/VTA diverge.
  - **Generics:** `state.WriteJSON[T any]` / `state.ReadJSON[T any]`
    (`internal/state/state.go:23,49`) — instantiated at many concrete types; the classic
    "find references across instantiations" stressor.
  - **High fan-in plain function:** a widely-called exported `internal/hubgeometry`
    function (the Hub Geometry Invariant makes that package pervasive) — pick the actual
    highest-fan-in one during the spike.
  - **Method with many call sites** on a concrete type (e.g. an `output`/logger method).
  - **Reflection-adjacent / negative case:** yaml/json struct-tag-driven code or the
    `internal/output` envelope, to honestly characterize false-negatives/positives at
    static-analysis limits.
- Rationale: Grounds precision claims in this repo's real stressors rather than toy code.
- Rejected: user-named symbols only (user delegated selection); a random sample (misses the
  deliberate stress cases).

### recommendation-rubric

- Decision: Two-dimensional verdict — **cost** (does warm-up + steady-state fit a run) and
  **precision** (can an implementer trust it), with a separate callgraph sub-verdict:
  - **Adopt now** — *both* hold: **Cost** = one-time warm-up tolerable at run start (order
    of a few seconds on this repo, paid once) **and** per-query steady-state effectively
    free relative to an LLM turn (sub-100ms-ish, in-process or over a warm LSP socket);
    **Precision** = on ordinary (non-reflection) benchmark symbols, direct-reference /
    call-hierarchy results have **no false negatives** (every real caller found), so an
    implementer can rely on it *instead of* grep, not merely alongside it. Bounded,
    explainable false *positives* (e.g. interface over-approximation the LLM can filter) are
    tolerable; false *negatives* are disqualifying (they silently hide a ripple site).
    → ship as a Go verb `builder`'s implementers can call, per extension-doc §6.
  - **Defer** — precision clears the bar but **only the warm path is fast enough** and it
    cannot be cleanly run-scoped until `loom`/`webster` owns the run process. Don't stand up
    infrastructure solely for this; revisit when that orchestrator exists.
  - **Drop** — precision misses real callers on ordinary code (false negatives on
    non-exotic symbols), **or** even warm steady-state + warm-up can't beat grep+LLM in
    practical wall-clock/token terms.
  - **Callgraph (CHA/RTA/VTA) sub-verdict** — graded separately; direct references may be
    Adopt while transitive is Defer/Drop. Axis is precision-vs-cost per algorithm; the
    finding names which algorithm (if any) lands in a usable spot on this repo, or concludes
    direct call-hierarchy is the sweet spot and transitive isn't worth it.
- Rationale: Separates the cheap-and-precise direct-reference case (the dominant use) from
  the expensive-and-approximate transitive case, so a good direct-reference result isn't
  sunk by a poor transitive one.
- Rejected: a stricter single bar requiring transitive precision too (would sink the likely
  main win); a looser "merely competitive with grep" bar (precision is the whole point —
  competitiveness without trust is not adoptable).

## Technical context

What mill-plan needs to know about this repo to write the plan:

- **Module system:** `lyx` is one binary with a namespaced Cobra subcommand tree
  (`cmd/lyx/main.go` → `newRoot()`); production modules follow the `<module>cli` +
  `<module>engine` split (`docs/overview.md`, CONSTRAINTS.md CLI/Cobra Invariant). **The
  spike deliberately does NOT use this** — it is a standalone `tools/` program, exactly
  like `tools/sandbox/` and `tools/deploy/` (plain `main.go`, no Cobra, no registration),
  so it never trips the helptree / registration / longlist / drift / tierpurity guards.
- **Precedent to mirror:** `docs/research/session-fork-spike.md` (findings doc) + its
  throwaway `tools/fork-poc/` harness (committed on branch, removed before merge, referenced
  via archive tag). Follow this shape precisely.
- **Findings doc home:** `docs/research/` (existing dir; siblings include
  `session-fork-spike.md`). Cost numbers go **inline** in the findings doc, not split into
  `docs/benchmarks/` (that dir is for ongoing tracked benchmarks, not one-shot spikes).
- **Dependencies the harness needs:**
  - `golang.org/x/tools/go/packages`, `.../go/types` (stdlib), `.../go/callgraph` (+
    `callgraph/cha`, `.../rta`, `.../vta`), and `.../go/ssa` for RTA/VTA. `x/tools` is
    **not currently in `go.mod`** — the harness adds it on the branch and it is **reverted
    before merge**.
  - `gopls` is **not currently installed** in this environment — the gopls-subprocess and
    CC-native-LSP arms require `go install golang.org/x/tools/gopls@latest` first. Record
    the version tested in the findings doc.
- **CC-native-LSP wiring:** `.lsp.json` at repo root with
  `{"go": {"command": "gopls", "transport": "stdio", "maxRestarts": 3, ...}}` and
  `ENABLE_LSP_TOOL=1` (per the `zircote/go-lsp` recipe). Any `.lsp.json` added for the
  baseline is throwaway and removed before merge with the rest of the harness.
- **Repo shape for load-cost realism:** ~47 `internal/` packages plus `cmd/` and `tools/` —
  a real multi-package module, so warm-up cost is non-trivial and worth measuring rather
  than assuming.

## Constraints

From `CONSTRAINTS.md` (hub root) and `CLAUDE.md`, those that bear on this task:

- **CLI/Cobra Invariant** — the spike avoids it entirely by being a `tools/` program, not a
  registered module. If any code were ever promoted to a real verb (the follow-up task, not
  this one), it must take the `<module>cli`+`<module>engine` split, `Short` on every
  command, and update the pinned helptree/registration/longlist sets.
- **Test Tier Purity Invariant** — any `_test.go` that spawns a process (gopls) must be
  `//go:build integration` (Tier 2). The spike sidesteps this by running the harness via
  `go run` **manually**, not as `go test`; if any harness test is added it must be Tier-2
  tagged.
- **Hub Geometry Invariant** — irrelevant to the harness (it doesn't resolve `_lyx`/config
  paths); noted only because `hubgeometry` is a benchmark-symbol source.
- **Documentation Lifecycle** (`CLAUDE.md`): this task produces `docs/research/codeintel-spike.md`.
  It is a spike findings doc, **not** a module doc, so `docs/modules/` and the module table in
  `docs/overview.md` are untouched. **`docs/roadmap.md` is NOT updated** — a spike is not a
  planned milestone (roadmap is milestones only). If the verdict is adopt, the *follow-up*
  adoption task adds the roadmap/module entries, not this one.
- **Worktree isolation** (`CLAUDE.md`): all work stays in this `codeintel-spike` worktree.
  The `websterv2_extension.md` design doc lives uncommitted in the `loomyard`/main worktree
  and is **read-only** here; do not edit or commit it from this worktree.
- **Persistent notes, not file-memory** (`CLAUDE.md`): durable output is the versioned
  findings doc, not `memory/`.

## Testing

The spike is measurement, not a shippable feature, so "testing" here means **the
measurement rig and its validity**, not a production test suite:

- **Harness (`tools/codeintel-poc/`):** run manually via `go run ./tools/codeintel-poc ...`.
  No Tier-1 unit tests required; it is disposable. Correctness of the *measurement* is what
  matters, established by:
  - **Precision ground-truth check:** for each benchmark symbol, cross-check the tool's
    reference/caller set against a hand-verified set (grep + manual reading) so
    false-negatives/positives are counted honestly, not assumed. This cross-check IS the
    precision result and must be recorded per symbol in the findings doc.
  - **Cost measurement hygiene:** report warm-up tax and steady-state as separate numbers;
    warm-up measured once-per-process, steady-state as the marginal cost of the Nth query in
    the same warm process. Numbers are wall-clock and noisy — report as order-of-magnitude
    with the machine/toolchain noted (matching `docs/benchmarks/` house style).
  - **CHA/RTA/VTA comparison:** for at least the interface-satisfaction symbol, report each
    algorithm's caller set size plus build/analysis time. **Transitive precision method
    (a full transitive caller set cannot be hand-enumerated at scale):** treat **VTA as the
    reference/gold** set and report how much **CHA and RTA over-approximate relative to VTA**
    — this inter-algorithm divergence is the transitive finding. Additionally, hand-verify
    the **complete transitive caller set for exactly one deliberately small, shallow symbol**
    (few callers, ≤2–3 hops) as an **absolute-truth anchor** confirming even VTA misses no
    real caller on this repo. So: divergence is graded VTA-relative; soundness is spot-checked
    once against hand-built truth. Direct-reference precision is unaffected — it is still
    graded against grep+manual ground truth per the bullet above.
- **If the verdict is adopt**, the findings doc's how-to section must contain a **runnable**
  minimal example (exact imports + call sequence, or exact gopls LSP request) that a
  follow-up implementer can lift directly — verified by actually running it during the spike,
  not written from memory.
- **No production test-tier obligations** are incurred because nothing ships to `main`
  except the doc.
- **Final revert-and-verify step (mandatory plan step):** after measurement is done and the
  findings doc is written, revert the throwaway harness (`tools/codeintel-poc/`), the
  `golang.org/x/tools` entries in **both `go.mod` and `go.sum`**, and any throwaway
  `.lsp.json`; then verify `git diff main...HEAD --name-only` lists **only**
  `docs/research/codeintel-spike.md`. A non-doc-only diff at this point is a task failure to
  fix before handoff — this is the machine-checkable guard the "deleted before merge"
  assertion needs.

## Q&A log

- **Q:** Prototype form — throwaway `tools/` program, throwaway registered module, or real module? **A:** Throwaway `tools/codeintel-poc/` program (deleted before merge); the findings-and-how-to **doc is the primary deliverable** per the user.
- **Q:** Which mechanisms to measure? **A:** All three — `go/packages` in-process (primary), gopls-held-open-subprocess (comparison), CC-native LSP `ENABLE_LSP_TOOL` (baseline).
- **Q:** Is `zircote/go-lsp` a usable library? **A:** No — it's a thin Claude Code *plugin* (empty hooks, only a test .go file); its value is surfacing that CC has a native LSP tool. Measured as the baseline arm; architectural mismatch with the file-contract model recorded.
- **Q:** "Daemonless" vs. needing a warm process — does the warm graph live outside lyx, or become a lyx daemon? **A:** Neither a machine-wide daemon nor an external always-on server — **run-scoped, held by the orchestrator run**. With in-process `go/packages` there is no extra process at all (memory the run holds); with gopls it's a run-supervised child. Cold is measured as the **once-per-run warm-up tax**, not per-query.
- **Q:** Measure cold and warm both? **A:** Yes — warm-up-once-per-run tax + per-query steady-state; the delta is the argument for/against any persistence.
- **Q:** Transitive impact (callgraph) in scope? **A:** Yes — CHA vs RTA vs VTA precision/cost comparison, graded separately from direct references.
- **Q:** Who picks the precision benchmark symbols? **A:** The spike author picks a fixed handful from loomyard targeting interface-satisfaction, generics, high-fan-in, a method, and a reflection-adjacent negative case.
- **Q:** How deeply to measure the CC-native LSP tool? **A:** Actually wire `ENABLE_LSP_TOOL` + `.lsp.json` and drive one real query, timeboxed; fall back to docs-characterization if it misbehaves. Document the recipe.
- **Q:** Adopt/defer/drop rubric? **A:** Two-dimensional (cost + precision), no-false-negatives-on-ordinary-code required for adopt, separate CHA/RTA/VTA sub-verdict; author sets exact thresholds per the elaborated rubric.
- **Q:** [r1 gap] How is *transitive* (CHA/RTA/VTA) precision measured, given a full transitive caller set can't be hand-enumerated? **A:** VTA-as-reference for the CHA/RTA over-approximation divergence (the finding), **plus** one deliberately small, shallow symbol hand-verified end-to-end as an absolute-truth soundness anchor. Author's call (user delegated).
