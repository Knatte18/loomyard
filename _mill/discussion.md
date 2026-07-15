# Discussion: lyx mux remove errors when it empties the last session

```yaml
task: lyx mux remove errors when it empties the last session
slug: mux-remove-last-pane-error
status: discussing
parent: cluster-fork-spike
```

## Problem

`lyx mux remove <guid>` returns a hard error when the removed strand was the
session's last live strand, even though the removal fully succeeded:

```json
{"error":"list panes: exit status 1: no server running on <socket>","ok":false}
```

100% reproducible: `lyx mux up` → `lyx mux add --cmd "sleep 300"` (one strand,
no children) → `lyx mux remove <that-guid>`. The pane, the session, and the
(now sessionless) server are all gone exactly as intended — the removal worked
— but the caller is told it failed.

**Root cause (already diagnosed via `debug_log` forensic logging).**
`internal/muxengine/strand.go`'s `RemoveStrand` (~L422–481) kills the removed
strand's pane via `kill-pane`, then calls `reconcileApplyPersistLocked`, which
re-lists panes (`internal/muxengine/spawn.go:181`) to repair the layout. The
`kill-pane` call site's comment (strand.go:453–456) rests on the assumption
that killing a session's *last* pane does not remove it — that under
`remain-on-exit` psmux corpses it as `pane_dead=1`, keeping the session alive
for the follow-up reconcile. **That assumption does not hold on tmux:** killing
a session's true last pane destroys the session outright, and since it was the
server's only session, the server then exits. The subsequent `listPanes` hits
"no server running", `reconcileApplyPersistLocked` returns
`fmt.Errorf("list panes: %w", err)`, and `RemoveStrand` returns that as
`applyErr` — a hard error for an operation that already completed correctly.

**Why now:** caught during interactive `lyx mux` layout-model exploration with
Fable's newly-landed `debug_log` logging (`mux-server-crash`, done) turned on,
which made the exact mechanism visible instead of a bare "no server running".

## Scope

**In:**

- `RemoveStrand` (`internal/muxengine/strand.go`): treat "the removal emptied
  the session/server entirely" as an expected terminal success, not a failure.
- A pure decision helper capturing that classification, unit-tested hermetically.
- **Persist the emptied state** on the success-swallow path (see the
  persistence-gap decision below) so `mux.json` no longer lists the removed
  strand.
- Correct the load-bearing behavioral assumption: fix the `kill-pane`
  comment in `strand.go` and promote+correct it into `internal/muxengine/doc.go`'s
  "Load-bearing behavioral assumptions" list (same commit — CLAUDE.md docs rule).
- Tests: a hermetic unit test for the new pure helper, plus an
  `//go:build integration` end-to-end regression (up → add one → remove →
  assert success and empty `mux.json`) that self-skips when tmux is absent.

**Out:**

- **Windows/psmux verification.** Whether real psmux on Windows actually
  corpses a session's last pane (as the old comment claimed) or was wrong there
  too is *unverified* — this was found and reproduced only on Linux/tmux, and
  no Windows box is available. Documented as an unverified note; not blocking.
  The chosen fix is backend-agnostic (it keys off psmux confirming the session
  is gone, not off which backend is running), so it is correct either way.
- **The `mux-server-crash` connection.** Whether this same "tmux destroys a
  session's true last pane, contradicting the remain-on-exit assumption"
  behavior explains any of the earlier, still-unexplained whole-server deaths
  is left as an open note. Those crashes happened with no `lyx mux remove`
  issued, so it is not obviously the same mechanism; ruling it in/out is a
  separate investigation, not this task.
- No refactor of `Engine.psmux` from a concrete `PsmuxCmd` into an interface
  (considered for testability — rejected, see testing decision).
- No change to `reconcileApplyPersistLocked`, `removeStrandLocked`, or the
  reap logic beyond what the swallow branch needs.

## Decisions

### classify-via-hassession-reprobe

- Decision: After `reconcileApplyPersistLocked` returns `applyErr`, re-probe
  `e.psmux.hasSession(e.SessionName())`. Only when psmux *confirms* the session
  is gone (`up == false`, `err == nil`) do we consider swallowing the error.
- Rationale: The bug was caused by *predicting* psmux behavior (the
  remain-on-exit-corpses-last-pane assumption) and being wrong. The fix must
  not replace one wrong prediction with another — it grounds the decision in an
  observed fact ("psmux says the session is gone"). `hasSession` is the exact
  round-trip `requireSessionLocked` already uses to classify "no session", so
  this reuses the module's existing, backend-agnostic classification rather
  than inventing a new one.
- Rejected:
  - **String-matching the psmux error text** for "no server running"/"no
    session" — fragile across tmux vs psmux (Windows) wording, and the Windows
    behavior is explicitly unverified.
  - **Proactively skipping `reconcileApplyPersistLocked`** when we predict the
    session will empty — avoids error handling but re-introduces a behavioral
    prediction of exactly the kind that caused this bug.

### expected-terminal-state-condition

- Decision: The swallow applies only when, in addition to the session being
  confirmed gone, **no remaining strand is non-hidden** — i.e. every strand
  left in `st.Strands` (after `removeStrandLocked`) has
  `Display.Anchor == render.AnchorHidden`. The zero-strands case is the common
  instance of this.
- Rationale: A hidden (`anchor:hidden`) strand never holds a live pane, so when
  the sole *visible* strand is removed the session legitimately dies even
  though hidden strands remain — that is still an expected terminal state, not
  a failure. If a non-hidden strand remained, it *should* still own a live
  pane, so the session dying would be genuinely unexpected (a real error worth
  surfacing, and closer to the `mux-server-crash` shape). This filter mirrors
  the existing `anyPlacedStrand` predicate (`apply.go:90`), which also treats
  `Anchor != render.AnchorHidden && PaneID != ""` as "expected to be placed".
- Rejected: Gating strictly on `len(st.Strands) == 0` — simpler, but wrongly
  surfaces an error when hidden strands remain after removing the last visible
  one.

### persist-emptied-state

- Decision: On the success-swallow path, call `SaveState(e.layout.DotLyxDir(),
  st)` before returning success. Fail the op only if `SaveState` itself errors.
- Rationale: **Persistence gap found during exploration.** `removeStrandLocked`
  only mutates `st.Strands` *in memory*; the sole `SaveState` for a remove is
  at the *end* of `reconcileApplyPersistLocked` (`spawn.go:203`), which is
  never reached when `listPanes` fails first. So simply swallowing the error
  and returning success would leave `mux.json` still listing the removed
  strand — and a later `lyx mux resume` (which replays every persisted
  non-hidden strand) would resurrect a strand the user explicitly removed.
  Swallowing the error without persisting would ship a data-integrity bug.
- Rejected: Leaving persistence as-is / out of scope — reintroduces the removed
  strand into persisted state.

### test-pure-helper-plus-integration

- Decision: Extract the classification into a **pure decision helper** (given
  the post-removal remaining strands and a "session gone" boolean, return
  whether this is an expected empty terminal state) and unit-test it
  hermetically in the `_test.go` alongside the existing `removeStrandLocked`
  tests. Additionally add an `//go:build integration` end-to-end regression in
  `contract_integration_test.go` (or a sibling integration file) that drives a
  real Engine: `Up` → `AddStrand` (one strand) → `RemoveStrand` → assert no
  error and that the persisted `mux.json` has zero strands. It self-skips when
  the configured multiplexer binary is absent (same pattern as
  `TestMultiplexerContract`, contract_integration_test.go:95).
- Rationale: `Engine.psmux` is a concrete `PsmuxCmd` (`lock.go:38`), not an
  interface, and `doc.go` names `reconcileApplyPersistLocked` "the one seam
  every public op's hermetic unit test must not cross." So the full
  `RemoveStrand` path can only be exercised end-to-end with a real tmux. The
  pure helper keeps the *decision logic* hermetically tested (runs everywhere,
  including CI/Windows without tmux); the integration test pins the real wire
  behavior that this bug is actually about (tmux destroying the last-pane
  session).
- Rejected:
  - **Integration test only** — matches the proposal's "regression test
    alongside the existing RemoveStrand tests" literally, but leaves the new
    branch logic untested where tmux is absent.
  - **Refactor `Engine.psmux` to an interface** for a scripted fake — most
    thorough, but a broad change touching every psmux call site, out of
    proportion to this fix, and against `doc.go`'s seam rule.

### doc-correction-in-scope

- Decision: In the same commit, (a) rewrite the `kill-pane` comment in
  `strand.go` (currently L449–456) so it no longer asserts the false
  "killing a session's LAST pane does not remove it" claim, and instead states
  the corrected behavior and how `RemoveStrand` now handles it; and (b) add a
  new bullet to `internal/muxengine/doc.go`'s "Load-bearing behavioral
  assumptions" list (L54+) recording that on tmux, killing a session's *true
  last* pane destroys the session (and, if it was the server's only session,
  the server exits) — refining the existing "Dead-pane adoption via
  remain-on-exit" bullet (L62–68), which currently implies the corpse behavior
  holds universally. Note the Windows/psmux side as unverified.
- Rationale: CLAUDE.md requires docs updated in the same commit as behavior
  changes, and this is precisely the cross-platform assumption `doc.go`'s list
  exists to track. Leaving the false comment in place would re-seed the same
  bug.
- Rejected: Deferring the doc update to a follow-up — violates the same-commit
  docs rule for a corrected load-bearing assumption.

## Technical context

- **Fix site:** `internal/muxengine/strand.go`, `RemoveStrand` (L422–481). The
  relevant tail:
  ```go
  _, applyErr := e.reconcileApplyPersistLocked(st)
  reapPaneChildren(reapPIDs, reapExitTimeout)   // must still run — panes are dying async
  if applyErr != nil {
      return applyErr
  }
  ```
  The new logic goes in the `applyErr != nil` branch: if the pure helper says
  this is an expected empty terminal state *and* `hasSession` confirms the
  session is gone, then `SaveState` and return `removed` (success); otherwise
  return `applyErr` unchanged. Keep `reapPaneChildren` before the check exactly
  as now (the removed pane's subtree is dying asynchronously either way — the
  never-skip-the-reap rule).
- **Ordering subtlety:** `reapPaneChildren(reapPIDs, ...)` already runs before
  the `applyErr` check today; preserve that. Do not move it.
- **Classification primitive:** `PsmuxCmd.hasSession(name)`
  (`overlay.go:90`) returns `(false, nil)` when the session is absent (psmux
  exit 1 is the normal "not there" case) and `(false, err)` only on other
  failures. `requireSessionLocked` (`lifecycle.go:831`) already uses it for the
  "no session" pre-flight — reuse the same call, not error-text matching.
- **"Non-hidden" filter:** `Strand.Display.Anchor != render.AnchorHidden`, as
  in `anyPlacedStrand` (`apply.go:90–97`). `Strand` is defined at
  `state.go:25`; `Display render.Display` carries the anchor.
- **Persistence:** `SaveState(dotLyxDir, st)` (`state.go`, ~L77) writes
  `mux.json` atomically; `dotLyxDir = e.layout.DotLyxDir()`. `removeStrandLocked`
  (`strand.go:269`) has already pruned `st.Strands` before any psmux call, so
  `st` is the correct post-removal state to persist.
- **Why `applyErr` can also be non-"no-server":** `reconcileApplyPersistLocked`
  wraps several distinct failures (`list panes`, `reconcile`, `list panes after
  reconcile`, `apply layout`, `save state` — spawn.go:179–207). Keying the
  swallow off `hasSession` + the non-hidden-strand check (not off the error
  string) means a genuine failure with the session still alive is still
  surfaced correctly.
- **Test harness:** `contract_integration_test.go` is `//go:build integration`,
  seeds a real config (`seedMuxConfig`), self-skips when the binary is absent
  (L95), and builds a scratch socket/session it owns (L99–108) with a
  `kill-server` cleanup. It currently pins wire-level `listPanes` behavior only;
  the new integration test drives full Engine ops (`Up`/`AddStrand`/
  `RemoveStrand`) on the same self-skip + scratch-socket + cleanup pattern.
  Hermetic `RemoveStrand`-family tests live in `strand_test.go` and use
  `newTestEngine(t)`; they exercise the `*Locked` helpers directly and never
  cross the psmux seam — the new pure helper's unit test belongs there.

## Constraints

From `CONSTRAINTS.md` and CLAUDE.md:

- **Documentation Lifecycle / same-commit docs:** the corrected assumption must
  land in `strand.go`'s comment and `doc.go`'s assumptions list in the same
  commit as the code fix. A `docs/modules/` update is warranted only if a muxengine
  module doc exists and describes this behavior; check `docs/modules/` and update if so.
- **Hub Geometry Invariant:** all `_lyx`/geometry/path access goes through
  `internal/hubgeometry`. The fix already uses `e.layout.DotLyxDir()` for the
  state path — do not hardcode `.lyx`/`mux.json` literals.
- **doc.go's own seam rule:** hermetic unit tests must not cross
  `reconcileApplyPersistLocked`. Honor it — the full-path assertion is the
  integration test's job; the unit test targets the pure helper.
- **Roadmap:** this is a bugfix, not a planned milestone — do **not** add a
  `docs/roadmap.md` entry (per CLAUDE.md's roadmap rule).

## Testing

- **Pure decision helper (hermetic, `strand_test.go` alongside existing
  `RemoveStrand`/`removeStrandLocked` tests):**
  - Session gone + zero remaining strands → expected terminal state (swallow).
  - Session gone + remaining strands all `AnchorHidden` → expected terminal
    state (swallow).
  - Session gone + at least one remaining non-hidden strand → NOT expected
    (surface the error).
  - Session *not* gone (any remaining strands) → NOT expected (surface the
    error) — a real reconcile/apply failure with a live session must still fail.
- **Integration regression (`//go:build integration`, self-skips without the
  configured binary):**
  - Reproduction path: `Up` → `AddStrand` with one non-hidden strand, no
    children → `RemoveStrand(thatGuid, false)`.
  - Assert: `RemoveStrand` returns nil error and the returned `Removed` names
    the strand.
  - Assert persistence: reload `mux.json` (`LoadState`) and confirm zero
    strands — this is what guards against the resurrect-on-resume regression.
  - Reuse the scratch-socket + `kill-server` cleanup pattern from
    `TestMultiplexerContract`.
- Run the hermetic suite normally (`go test ./internal/muxengine/...`) and the
  integration suite with `-tags integration` on a machine with tmux.

## Q&A log

- **Q:** How should the fix detect that the removal emptied the session/server?
  **A:** Re-probe `hasSession` after `applyErr` and act only when psmux
  confirms the session is gone — grounded in observation, not another
  behavioral prediction (the class of assumption that caused this bug). Not
  error-text matching (fragile across backends), not proactively skipping
  reconcile (another prediction).
- **Q:** Exact condition for "expected terminal state"? **A:** No remaining
  *non-hidden* strand (includes zero-strands), not strictly `len(Strands)==0` —
  so removing the last visible strand while `anchor:hidden` strands remain is
  still treated as success. Mirrors `anyPlacedStrand`'s filter.
- **Q:** Do we fix the persistence gap (in-memory-only `st.Strands` prune,
  `SaveState` skipped when `listPanes` fails)? **A:** Yes — `SaveState` on the
  swallow path, else `resume` would resurrect the removed strand.
- **Q:** Test strategy given `psmux` is a concrete struct (not mockable)?
  **A:** Extract a pure decision helper + hermetic unit test, plus an
  `//go:build integration` end-to-end regression that self-skips without tmux.
  Not integration-only (leaves logic untested off-tmux); not an interface
  refactor (too broad, against the seam rule).
- **Q:** Scope of the proposal's three follow-ups (Windows verification,
  `mux-server-crash` connection, `doc.go` update)? **A:** `doc.go` +
  `strand.go` comment correction in scope (same commit). Windows behavior and
  the crash connection are documented as unverified notes only — no Windows
  hardware, and the crash link is a separate investigation.
