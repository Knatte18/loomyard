# Discussion: Refine SANDBOX-SUITE.md from the 2026-06-30 sandbox run

```yaml
task: Refine SANDBOX-SUITE.md from the 2026-06-30 sandbox run
slug: sandbox-suite-refinements
status: discussing
parent: main
```

## Problem

A full S0–S6 pass of the lyx sandbox suite on 2026-06-30 surfaced a batch of
meta/doc defects in the suite instructions themselves (GitHub issues #39, #40, #41,
plus the false-positive bug #35 that #41 traces back to suite vagueness). None are lyx
code bugs — they are gaps and contradictions in the test scheme that an agent reads and
follows. Left unfixed they keep producing friction and false positives in every future
session:

- **S5** ("Re-clone and reset idempotency") asks the in-hub black-box agent to run
  `sandbox.cmd -reset`, which lives outside the hub and tears down the very worktree the
  agent's shell is `cwd`-ed into. It violates the black-box rule and is unrunnable by the
  agent regardless of outcome (#39).
- The suite **never states lyx's operating model** — that lyx resolves against the
  current directory's own `_lyx/` and does not walk up — so an agent reads S4's "config
  scope from cwd" as an invitation to probe subdirectories, then reports the expected
  "not initialized" failure as a defect (#41, which is exactly what produced #35,
  closed invalid).
- Smaller friction: PowerShell users backslash-escape JSON args and hit a cryptic
  parse error (#40.1); S2 wrongly calls a raw-`git` host commit a "gap" (#40.2); the
  board is durable across sessions but the scheme's S3 board wording implicitly assumes
  a fresh board (#40.3).

**Why now:** the suite is the spine for hardening lyx; every session pays these costs
until the scheme is corrected, and the 2026-06-30 pass already wasted effort on a false
positive (#35).

## Scope

**In:**

- Edit **`tools/sandbox/test-scheme.md`** (this is the source of the `SANDBOX-SUITE.md`
  the issues name; the launcher generates `SANDBOX-SUITE.md` = fingerprint header +
  `test-scheme.md` body at run time — see `tools/sandbox/suite.go`). All seven
  refinements land here:
  1. Add an **Operating-model** paragraph to Pre-conditions (cwd-local resolution;
     subdir failure is expected, not a finding).
  2. Add a **PowerShell JSON-quoting** note to Pre-conditions, with a one-line pointer
     in S3.
  3. Reword **S2** so a raw-`git` host commit is not framed as a gap.
  4. Reword **S4**'s Watch line to remove the directory-varying reading.
  5. **Reframe S5** out of the agent's numbered scenarios into an Operator-steps section;
     mark its session-log line operator-supplied.
  6. Add a **board-durability** note to S3 (board persists across sessions; clean up
     test tasks you create).
  7. Add a short suite note to the **Notes** section clarifying "cwd-relpath mirroring"
     = weft path mirroring.

**Out:**

- **No lyx code changes.** This is meta/doc only. The cwd-local gating in
  `internal/boardengine/config.go` / `internal/warpengine/config.go` is correct and
  intended behaviour, not something this task alters.
- **No renumbering of scenarios.** S5 is removed from the agent list but S6 stays S6
  (numbering gap is intentional) to preserve continuity with prior issue references and
  the session-log format. See Decisions.
- **No edit to S1 (Hub orientation).** Although #40.3 names "S1/S3," S1's current text
  makes no fresh-board assumption, so it is intentionally left untouched; the
  board-durability note lands in S3 and covers durability for the whole session. See
  Decisions → `board-durability-note`.
- **No "run from a subdir" behavior probe in S6.** The brief's wording ("promote 'run
  from a subdir' to an explicit S6 watch bullet") is explicitly **not** implemented — it
  contradicts #41 and #40's own revision. See Decisions → `subdir-handling`.
- **No doc propagation to `docs/sandbox-howto.md` / `docs/sandbox-hub.md`.** They already
  document `sandbox.cmd -reset` as an operator action and do not enumerate S0–S6, so the
  S5 reframe needs no change there.
- **No CONSTRAINTS.md change** (no new cross-cutting invariant) and **no golden-test
  update** — `suite_test.go` only asserts the `"Sandbox test-scheme"` heading is present,
  not the scenario body.
- The external host README (`Knatte18/lyx-test` → `services/api` cwd-relpath wording)
  is **already fixed and pushed out-of-band** during discussion (commit `d9e5e66` on
  that repo); it is not part of this task branch's plan.

## Decisions

### edit-target-is-test-scheme-md

- Decision: All suite edits go in `tools/sandbox/test-scheme.md`, not a literal
  `SANDBOX-SUITE.md` file (none exists in-repo).
- Rationale: `suite.go` embeds `test-scheme.md` via `//go:embed` and renders
  `SANDBOX-SUITE.md = info.header() + "\n" + test-scheme.md` at launch
  (`renderScheme`). The fingerprint header is generated; only the body is authored.
- Rejected: creating a standalone `SANDBOX-SUITE.md` (would duplicate/diverge from the
  embedded source).

### s5-operator-only

- Decision: Remove S5 from the numbered scenario list entirely. Add an **Operator steps**
  subsection (outside the agent's scenarios) describing the `sandbox.cmd -reset`
  idempotency check as an operator-run step before/after the session, outside the agent
  transcript. In the session-log format, keep an `S5:` line but mark it
  **operator-supplied** (a value the agent transcribes, not an agent verdict).
- Rationale: #39 — `sandbox.cmd` lives outside the hub and `-reset` destroys the agent's
  own cwd; the agent cannot run it without violating the black-box rule or killing its
  session.
- Rejected: keeping S5 numbered with a stronger caveat (still reads as an agent
  scenario); deleting S5 entirely with no operator replacement (loses the idempotency
  check the operator genuinely wants run).

### keep-scenario-numbers-stable

- Decision: Do not renumber. After removing S5, the agent scenarios are S0, S1, S2, S3,
  S4, S6 — S6 keeps its number. The numbering gap is intentional and signals S5 moved to
  the operator section.
- Rationale: Prior GitHub issues and session logs reference scenarios by number;
  renumbering S6→S5 would silently break those references.
- Rejected: renumbering S6 to S5 (churns external references for no gain).

### operating-model-cwd-local

- Decision: Add an Operating-model paragraph to Pre-conditions stating: lyx resolves
  against the **current directory's own `_lyx/`** and does **not** walk up to a parent;
  the hub host repo is initialized at its root, so the agent runs the entire session from
  there; running lyx from a subdirectory that has not itself been initialized correctly
  reports `not initialized here; run "lyx init"` — **that is expected behaviour, not a
  finding.** Note (explanatory) that `lyx init` in a subdirectory would create `_lyx/`
  there and make lyx work in that subdir, but the agent should **not** scaffold nested
  `_lyx/` during a session.
- Rationale: #41/#35 — the model was never stated, so an agent treated the expected
  subdir failure as a bug. Verified against source: `boardengine.LoadConfig` /
  `warpengine` gate on `<baseDir>/_lyx/` existence with no upward walk.
- Rejected: stating "lyx works only from one fixed root, subdir always fails" (inaccurate
  — init-in-subdir genuinely works, per project owner).

### subdir-handling-explanatory-only

- Decision: Treat subdirs **explanatorily only** (via the Operating-model paragraph). Do
  **not** add an S6 (or any) bullet telling the agent to actively run `lyx init` in
  `services/api/` and probe it.
- Rationale: An active init-in-subdir probe would scaffold a nested `_lyx/` in the
  sandbox host, polluting it; the false-positive risk #41 warns about is best closed by
  explanation, not by inviting the experiment. This also resolves the brief's stale
  "promote run-from-subdir to an S6 watch bullet" item against #40's own revision (which
  removed a subdir proposal) and against #41.
- Rejected: active S6 init-in-subdir probe (Q8 option 2).

### s4-reword-no-dir-varying

- Decision: Reword S4's Watch from "Does it find the right config scope from cwd?" to a
  form that fixes cwd at the initialized root, e.g. "From the worktree root, does
  `lyx config` read/write the correct `_lyx/config/` and round-trip a value?"
- Rationale: #41.2 — "from cwd" reads as "vary the directory"; the intended meaning is
  the opposite.
- Rejected: leaving S4 as-is.

### s2-raw-git-is-fine-for-host

- Decision: Reword S2 so a raw-`git` host commit is **not** a finding. The host is an
  ordinary git repo; committing host changes with plain `git` is acceptable. Refocus
  S2's Watch on what lyx is actually responsible for — host/weft coordination: junctions
  wired correctly, weft mirroring behaves.
- Rationale: Project owner: "host commits do NOT need to go via lyx." The current
  "falling back to raw git is a gap in the lyx surface" line is wrong and would generate
  spurious WARN/FAIL findings.
- Rejected: asserting host commits must be lyx-owned (#40.2 option A — contradicts intent);
  leaving it as an "open question" (#40.2 option C — still invites a spurious finding).

### powershell-json-note

- Decision: Add a PowerShell JSON-quoting note to **Pre-conditions** (it is shell-wide),
  with a one-line pointer in S3 where `lyx board upsert '{…}'` appears. State the
  wrong-on-PowerShell form (backslash-escaped JSON → `invalid json: invalid character
  '\' …`) and the working form: a single-quoted string with literal inner double quotes,
  e.g. `lyx board upsert '{"slug":"s3-demo","title":"S3 demo"}'`.
- Rationale: #40.1. `lyx board upsert` takes a JSON positional arg
  (`internal/boardcli/cli.go`), so this bites every PowerShell-driven S3.
- Rejected: placing it only in S3 (misses other JSON-arg commands) or only in
  Pre-conditions (S3 is where it bites, so it gets a pointer).

### board-durability-note

- Decision: Add a note (in **S3** only) that the board is **durable across sessions** —
  it starts non-empty (e.g. a `T1 "Test task from S3"` persists from prior runs), so do
  not assume a fresh board — and instruct the agent to **clean up test tasks it creates**
  at session end. **S1 is not edited:** its current text ("Hub orientation") makes no
  fresh-board assumption, so the single S3 note suffices for the whole session.
- Rationale: #40.3 — the scheme's board wording implicitly assumed a fresh board, making
  verdicts non-reproducible. The issue cited "S1/S3," but on inspection only S3 carries
  the assumption; S1 needs no change.
- Rejected: leaving test tasks behind (option 2 — accumulates cruft); durability note
  with no cleanup guidance (option 3).

### external-readme-clarified-out-of-band

- Decision: Clarify the external host README's "cwd-relpath mirroring" wording **and add
  a short suite note in the Notes section** of test-scheme.md saying the same (that the
  phrase refers to weft path mirroring, not running lyx from subdirs). The README itself was edited and pushed
  to `Knatte18/lyx-test` (commit `d9e5e66`) so the fix is permanent and survives
  `sandbox.cmd -reset`.
- Rationale: #41.3 — the README's phrasing compounds the subdir confusion. The README
  lives in a separate repo (not the lyx-test task branch); committing upstream makes the
  fix durable, while the suite note keeps the clarification visible inside the scheme the
  agent reads.
- Rejected: suite note only (README keeps misleading future agents); local-only README
  edit (wiped on next `-reset`).

## Technical context

- **Suite generation:** `tools/sandbox/suite.go` — `//go:embed test-scheme.md` →
  `renderScheme(info) = info.header() + "\n" + testSchemeMD`. `suiteFileName =
  "SANDBOX-SUITE.md"`. The agent is launched with
  `"Read ./SANDBOX-SUITE.md and follow the instructions in it exactly."` So
  `test-scheme.md` is the single authored source for everything the agent reads.
- **Current scheme structure** (`tools/sandbox/test-scheme.md`): What this is →
  Pre-conditions → Black-box rule → Fingerprint header → How to run a scenario →
  Verdict key → Capturing findings → Scenarios (S0–S6) → Session log format → Notes.
  The capture path is `lyx ghissues create` only (no harvester, no board-upsert capture
  step) — preserve that.
- **cwd-local gating (verified):** `internal/boardengine/config.go` `LoadConfig` and
  `internal/warpengine/config.go` return `not initialized here; run "lyx init"` when
  `<baseDir>/_lyx/` is absent. No upward walk — confirms the operating-model wording.
- **board upsert JSON arg (verified):** `internal/boardcli/cli.go` `upsert [json-payload]`
  / `upsert-batch [json-payload]` `json.Unmarshal` the first positional arg; example in
  help is `lyx board upsert '{"slug":"my-task","title":"My Task","brief":"Short summary"}'`.
- **Tests:** `tools/sandbox/suite_test.go` only asserts `renderScheme` output contains
  the `"Sandbox test-scheme"` heading — keep that heading. No scenario-body golden test
  to update.
- **Source issues:** #39 (S5 reframe), #40 (PowerShell JSON note, S2 stance, board
  durability, operating model), #41 (operating model + S4 reword + README), #35 (the
  false positive #41 explains; closed invalid).

## Constraints

From `CONSTRAINTS.md` and `CLAUDE.md`:

- **Documentation Lifecycle:** a behaviour/doc change ships with its docs in the same
  commit. Here the "doc" *is* `test-scheme.md`; no module doc or `docs/overview.md`
  change is triggered (the suite content is not a named module's design and the module
  table/execution stack is unchanged). Confirmed `docs/sandbox-howto.md` /
  `docs/sandbox-hub.md` need no change (they don't enumerate scenarios and already treat
  `-reset` as operator-run).
- **`docs/roadmap.md`:** do **not** touch — this is delivered doc-refinement work, not a
  planned milestone.
- **Help-prose accuracy** (CONSTRAINTS review obligation): not engaged — no Cobra command
  `Short`/help text changes; only the suite scheme prose changes. Keep the scheme prose
  factually accurate against the verified lyx behaviour above.
- **Path Invariant / CLI / lyxtest Leaf invariants:** not engaged (no Go code change).

## Testing

- **No new automated tests.** `tools/sandbox/suite_test.go` already covers
  fingerprinting, `renderScheme`, git-exclude management, and `runSuite` orchestration;
  none asserts the scenario body. The only standing requirement is that
  `renderScheme` output still contains the `"Sandbox test-scheme"` heading — keep that
  heading line intact so `TestRenderScheme_ContainsHeaderAndBody` continues to pass.
- **Verification after edits:** run `go test ./tools/sandbox/...` to confirm the embed +
  render tests still pass; run `go build ./...` to confirm the `//go:embed` still
  resolves.
- **Manual smoke (optional, operator):** a future sandbox session is the real
  acceptance — the operating-model paragraph should prevent a subdir false positive, the
  PowerShell note should let an S3 board upsert succeed first try, and S5 should no longer
  appear as an agent scenario.

## Q&A log

- **Q:** Where do the edits land, given there's no literal `SANDBOX-SUITE.md`? **A:**
  `tools/sandbox/test-scheme.md` — `suite.go` generates `SANDBOX-SUITE.md` from it + a
  fingerprint header.
- **Q:** How far to take the S5 reframe (#39)? **A:** Fully — remove from the numbered
  agent scenarios, add an Operator-steps section, mark the session-log `S5:` line
  operator-supplied.
- **Q:** Follow the brief's "promote run-from-subdir to an S6 watch bullet"? **A:** No.
  lyx is cwd-local; running from an *uninitialized* subdir failing is expected, and
  `lyx init` in a subdir makes lyx work there. Handle subdirs explanatorily in the
  Operating-model paragraph; do not add an active S6 probe (would scaffold nested
  `_lyx/`). Contradicts the brief item but aligns #41 and #40's revision.
- **Q:** S2's stance on raw `git` for host commits (#40.2)? **A:** Host commits do **not**
  need to go through lyx (host is an ordinary repo); reword S2 so raw-git is not a gap.
- **Q:** The external README (#41.3) — note it's a clone, wiped on `-reset`? **A:** Edit
  it and **commit/push to the `Knatte18/lyx-test` repo** so it's permanent (done:
  `d9e5e66`), plus add a suite note in test-scheme.md.
- **Q:** PowerShell JSON-quoting note placement (#40.1)? **A:** Pre-conditions (shell-wide)
  with a one-line pointer in S3.
- **Q:** Board durability + cleanup (#40.3)? **A:** Note the board is durable (don't assume
  fresh) and clean up test tasks the agent creates.
