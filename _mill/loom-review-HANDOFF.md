# loom crucible campaign — handoff note

Campaign, thread A: confirm live that `centralize-glyph-shape-enum` and
`quarry-bump-v0-2-0-status-helpers` are genuinely behavior-preserving when driven through loom's
real built binary. **CONVERGED (rounds 1-2).**
Campaign, thread B/C: fix two pre-existing smoke-test failures, then independently review that fix
work plus a wider adversarial pass over loom's bootstrap/crash-recovery machinery. **STILL NOT
CONVERGED after round 5** — three consecutive safety-pass attempts (rounds 3, 4, 5) have each found
real defects. See `_mill/loom-crucible-orchestrator-kickoff.md` for the original (thread-A-only)
brief.

## Current state
**Thread A: CONVERGED**, unchanged since round 2, re-confirmed by round 5's light-touch pass.
**Thread B/C: NOT CONVERGED, round 6 needed.** Models tried on this thread: Sonnet (r3), Opus (r4),
Fable (r5) — all three have now found real defects here. The campaign's core defect class
("a terminal/negative classification answers a question using only a proxy fact, ignoring the fact
that actually owns the answer, one line away") has now been found in FOUR call sites across two
rounds (r4: two deadline paths; r5: two mechanism-failure caps) all in `internal/shuttleengine/wait.go`.
Round 6 should treat this as a strong signal to sweep `Wait` (and its neighbors) for a fifth instance
rather than assume the last two closed it.

## CLOSED-AND-VERIFIED

### Thread A (rounds 1-2) — unchanged; re-confirmed by round 5's live `validate-plan` driving
(handle canonicalization + on-disk rewrite, Create inversion both directions, ambiguous-with-file,
file-rename — no regression).

### Thread B, the three original production fixes (commit range `c0adce527..69886823e`)
`d0e5a0e7b`, `aba2c270a`, `69886823e` — independently re-verified correct by rounds 3, 4, 5, and the
orchestrator each time (never trusted from a prior account).

### Round 3 (`sonnet5-xhigh-r3`) — closed the `Started`-gating coverage gap; documented the
`AddStrand`/`run.json` crash-mid-registration race as an accepted residual.

### Round 4 (`opus5-high-r4`) — found F1/F2 (MEDIUM): both of `wait.go`'s deadline-expiry paths
(`classifyStartupWindow`, `Wait`'s run-deadline branch) finalized a negative outcome without
consulting the file contract. Fixed via `classifyDeadlineExpiry`. Also extended the
crash-mid-registration residual's documentation (F3, NIT) with why the obvious detection mitigation
isn't free either.

### Round 5 (`fable5-high-r5`, commit range `5f378a79f..d8a681d9b`) — found the SAME defect shape in
two MORE places:
- **F1 (MEDIUM)** — `Wait`'s two mechanism-failure exits (`maxEventsReadRetries` cap,
  `maxStatusRetries` cap) finalized a mechanism-failure error without checking the file contract
  first, unlike every neighboring branch. **Reproduced live**: a real `lyx shuttle run` with its
  output file written, then `reed.json` truncated mid-run — the mechanism failure won over a
  satisfied file contract. Fixed via a new `finishedDespiteMechanismFailure` helper (same shape as
  round 4's `classifyDeadlineExpiry`).
- **F3 (MEDIUM)** — `manifest/designs/loom.md` and commit `69886823e` promise a poisoned status file
  (malformed JSON OR an unknown field) never looks like bootstrap's own gate. The unknown-field half
  held; the malformed-JSON half did not on the `lyx loom run` path — `loomshed.Seed` read the file
  through a lenient decoder that aborted on malformed JSON with a raw error instead of the tolerant
  `ErrSeedExists` path. **Reproduced live**: a truly non-JSON status file made `lyx loom run` refuse
  before spawning a driver. Fixed by wrapping the lenient decode error in `state.ErrDecode` and
  mapping it to `ErrSeedExists` in `Seed`.
- **F2 (NIT)** — three doc/comment sites mis-attributed the decode-failure diagnosis to the
  Loom-Preflight producer; it's actually `Shed.Run`'s step-1 read gate (the producer never runs on a
  decode failure). Corrected.
- Round 5 also re-confirmed round 4's two fixes are still intact and re-confirmed the
  `AddStrand`/`run.json` residual's "genuinely unclosable" judgment with no new angle.

**Orchestrator's independent verification of round 5**: file-scope diff matched exactly (10 files,
none unexpected), cold-state hermetic gates green repo-wide (`go build`, `go vet`, `go test ./...`),
**all three new/changed guards independently sabotage-proved**: F1's two call sites (each reverted to
bare pre-fix behavior, each failed exactly as claimed), F3's two layers (`state.go`'s `ErrDecode`
wrap AND `seed.go`'s mapping to `ErrSeedExists`, each sabotaged separately — confirmed the
unknown-field case stays correctly unaffected by the `state.go` layer alone), full smoke suite now
12/12 green (new `TestSmokeBootstrap_MalformedStatusProceedsToHandoverAndLogsWhy` included).

## Incidental finding, OUT OF loom's scope, not fixed — surface to the operator when convenient
Round 5 hit a real bug in **fabric**, not loom: `lyx fabric clone` names the weft primary branch
after the weft bare repo's own HEAD (e.g. `master` from git's init default) rather than after the
warp's primary branch name, when the two differ — contradicting fabric's stated "named after the
paired warp branch" scheme. Caused a subsequent `lyx fabric add` to fail with `invalid reference:
main-weft` in round 5's own fixture setup (worked around by pointing the bare's HEAD at `main` before
cloning). Not a loom finding, not this campaign's job to fix. Worth a separate ticket/task through
the normal mill flow if the operator wants it tracked.

## RESIDUAL currently seeded
None specific — round 6 should be seeded as another genuine safety pass (no assigned residual), but
with an explicit note to sweep `Wait` (and structurally similar functions elsewhere) for a possible
FIFTH instance of the same defect shape, given four have now been found in two consecutive rounds.

## DEFERRED list
- The `AddStrand`/`run.json` crash-mid-registration race — operator-decision item, now re-confirmed
  by three independent rounds (3, 4, 5) as genuinely unclosable by reordering, with the detection
  tradeoff fully documented in `manifest/designs/loom.md`.
- The fabric weft-branch-naming bug above — not loom's scope; a separate task if the operator wants
  it tracked.

## Next action
Get the operator's model + effort pick for round 6. All three rotation models (Opus, Fable, Sonnet)
have now each found real defects on this thread at least once — there is no "untried" model left, so
picking any of them again is fine; consider whichever has the highest effort tier available if the
operator wants to push toward a decisive final pass (max), given the defect density found so far.
Re-seed `_mill/loom-review-prompt.md`'s "Round context" folding round 5's findings into
CLOSED-AND-VERIFIED (WITH file/test names, not just round-relative "F1" labels, since IDs collide
across rounds), then spawn `subagent_type: crucible-reviewer-<effort>`, `model: <pick>`, tagged
`<model>-<effort>-r6`.
**Do not call thread B/C converged until a round with no assigned residual comes back with nothing
new.** Three attempts in a row have each found real defects — this is still within the range the
method's own worked examples show as normal (reed: 7 rounds, fabric: 6), but it does mean the area
is genuinely defect-dense, not nearly clean.
