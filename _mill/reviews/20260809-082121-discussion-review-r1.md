MILL_REVIEW_BEGIN
# Review: fabric: store the warp-URL binding in weft:main; fold bootstrap into clone (slice 10)

```yaml
verdict: REQUEST_CHANGES
reviewer_model: opus
reviewer_self_id: Claude, Opus-class model (exact version not self-verifiable)
reviewed_file: _mill/discussion.md
date: 2026-08-09
```

## Findings

### [BLOCKING:design] Probe failure taxonomy is unspecified
**Section:** Decisions → pre-hub-probe **Issue:** The probe's outcome is treated as binary (record present / absent), but `git clone` and `git show HEAD:.lyx-warp` fail nonzero for unreachable remote, auth failure, wrong weft URL, and — critically — an empty weft remote with an unborn HEAD, which `CloneHub` supports today (`internal/fabricengine/clone.go:161-168`, the "genuinely empty weft remote" orphan-create path). Collapsing all of these into "absent" turns a network/auth failure into the misleading `unbound weft` error in the one-argument form, and into a silent bootstrap-and-write in the two-argument form. **Fix:** State which probe exit conditions map to "record absent" (missing path in an existing commit; empty/unborn remote) and which are hard errors surfaced verbatim.

### [BLOCKING:design] Reconcile backfill sits at the wrong result level
**Section:** Decisions → reconcile-backfill **Issue:** `Topology.Reconcile` (`internal/fabricengine/reconcile.go:95-176`) is a per-warp-worktree loop with no repo-wide phase, yet the backfill is a once-per-hub write to `BoardDir` reported on a top-level `ReconcileResult` field while the divergence case is reported on "the pair result" — which pair is unspecified, and with N worktrees the check would run N times. **Fix:** Say explicitly whether the binding check runs once before/after the pair loop, and put both the backfill field and the divergence report at the same (repo-wide) level.

### [BLOCKING:design] Reconcile push failure semantics unstated
**Section:** Decisions → reconcile-backfill **Issue:** `runReconcile` (`internal/fabriccli/fabric.go:436-461`) is entirely local today; adding a `Bolt` commit + push makes the first post-upgrade reconcile of every hub network-dependent, and the discussion never says whether a push failure fails the whole reconcile — the same "repair verb must not be blocked" reasoning it applies to divergence. **Fix:** State the disposition of a failed backfill commit/push (fatal, or reported as a detail while reconcile still succeeds).

### [NIT:consistency] Reconcile divergence test's normalization unstated
**Demoted-from:** BLOCKING
**Section:** Decisions → reconcile-backfill vs conflict-rule **Issue:** Clone compares normalized URLs and treats a transport swap as differing; reconcile's "present but differs from the warp `origin`" says nothing about normalization, so a hub recorded as `https://…` with an ssh `origin` (or a `.git` suffix difference) would emit a divergence detail line on every single reconcile forever. **Fix:** Say reconcile compares via the same `normalizeWarpURL`, and whether a transport-only difference is reportable or ignored.

### [NIT:decision] CloneResult's binding-written field left as two options
**Section:** Decisions → clonehub-signature **Issue:** "a boolean or string recording whether the binding was newly written" leaves the field's type and the CLI's JSON key undecided. **Fix:** Pick one and name the JSON key, since the clone envelope's keys are asserted in `internal/fabriccli/cli_test.go`.

### [NIT:consistency] Design-doc claim about its own examples is inverted
**Section:** Technical context → docs list **Issue:** `manifest/designs/fabric-unified-view.md:158-159` already shows the new weft-first order; it is line 163's prose ("this file's own examples above, which still show today's order") that is wrong — the discussion leaves this as "verify both". **Fix:** Record it as: examples stay, line 163's stale claim is deleted.

### [NIT:design] Probe temp dir placed in cwd without rationale
**Section:** Decisions → pre-hub-probe **Issue:** `os.MkdirTemp(cwd, ".lyx-clone-probe-")` puts a full weft clone inside the operator's cwd (which for local-fixture and hub-parent cases may be inside another repo) rather than `os.TempDir()`; the deferred `RemoveAll` does not cover SIGKILL. Also note the geometry-literal guard matches tokens by exact equality, so the prefix is safe and that open "check and maybe rename" item can be closed. **Fix:** State why cwd over `os.TempDir()`, or switch.

## Verdict

REQUEST_CHANGES
Probe failure semantics and the reconcile-backfill contract need resolving before plan writing.
MILL_REVIEW_END
