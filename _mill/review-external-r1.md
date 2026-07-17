# External review (r1) — loom-preflight discussion.md

Reviewed from the `loomyard` worktree, after the internal discussion (through
`discussion-fix-r5`) had already landed. Three findings below; the rest of the doc
(the check-collection model, the strict-parse sentinel scheme, the field-presence
handling, `at-worktree-root`) is well-reasoned and I'd leave it as-is.

## 1. `seed-unreadable "see check 3"` doesn't actually fire for the common broken-junction case

`seed-read-path` and `check-ordering-and-collection` both claim that a broken junction
gets attributed to check 4 as `seed-unreadable, "see check 3"` (never `seed-missing`),
based on classifying `os.Stat` on the seed path into `IsNotExist → seed-missing` /
`other → seed-unreadable`. But the two junction failure modes `PairInSync` actually
detects — `"junction missing"` and `"junction points elsewhere"` — both resolve, via a
plain `os.Stat(l.LoomStatusFile())`, to `ENOENT`/`IsNotExist` in the realistic case: a
missing junction means the `_lyx` path component itself doesn't resolve (ENOENT), and
"points elsewhere" means the wrong target directory exists but has no `status.json` in
it (also ENOENT — the file just isn't there). Only rarer errors (permission-denied,
`_lyx` existing as a plain file instead of a directory, some Windows reparse-point
failure) would actually produce a non-`IsNotExist` stat error.

So in the common case, when check 3 reports a `junction` failure, check 4 will *also*
independently produce `seed-missing` — not the intended `seed-unreadable, "see check 3"`
— because the syscall genuinely returns "not found" regardless of *why*. The two
failures would still both appear in `Report.Failures`, so nothing is silently dropped,
but the doc's stated guarantee ("the seed-unreadable, see check 3 attribution is exact")
doesn't hold for the case it's actually meant to cover, and the operator sees a
confusingly-labeled `seed-missing` next to a `junction` failure that's really the same
root cause. The Testing section's own scenario for this ("Seed unreadable via broken
junction (non-IsNotExist)") is written in a way that only exercises the rare
non-`IsNotExist` sub-case, which likely isn't how a real broken junction fails on either
Linux or Windows.

**Suggested fix (plan-time, cheap):** don't rely on the stat error's *shape* alone —
gate check 4's classification on check 3's *outcome*: if check 3 already produced a
`junction` failure, classify any resulting `seed-missing`-shaped read on check 4
(`IsNotExist` included) as `seed-unreadable, "see check 3"` instead. That delivers the
attribution the decision actually intends, rather than one that only happens to work for
uncommon stat-error types.

## 2. Stale "five" language after the fold-into-check-4 resolution

Scope → In (line 36) still says Preflight "validates five preconditions", but the
`the-five-checks` decision itself enumerates exactly four checks and explicitly notes
"(No fifth ... — it is folded into the coherence half of check 4)". The decision's own
heading (`the-five-checks`) wasn't renamed either. Cosmetic, but worth fixing before it
lands: per the Documentation Lifecycle, this discussion is what the package godoc is
supposed to distill from — if "five" leaks into the actual `Preflight` doc comment, it
will misstate the check count to every future reader who doesn't have this discussion's
history. Cheap fix: reword Scope to "four checks (one absorbing the no-half-finished
condition as an internal coherence rule)" and rename the decision heading to match.

## 3. The invocation-only-at-fresh-stage contract isn't visible at the call site

`preflight-invocation-model` and `no-half-finished-prior-run` both depend on a real but
entirely implicit invariant: Preflight is only ever called when the task is genuinely at
the fresh/preflight stage — that's what makes "non-empty history ⇒ half-finished"
correct rather than a false positive on legitimate cross-machine resume. That invariant
is owned by a *different, not-yet-built* module (the phase machine, #004), and nothing
in `Preflight`'s own contract states it — a future phase-machine implementer has to have
read this discussion doc (not the code) to know not to call `Preflight` on an advanced
task. Per the Documentation Lifecycle, this discussion file is deleted once loom-preflight
lands and only the durable parts fold into `loom.md`/package godoc — if this specific
precondition isn't explicitly carried into the `Preflight` godoc comment itself
("Callers MUST NOT invoke Preflight except at the fresh/preflight stage — doing so on an
advanced task will be misreported as half-finished, not rejected as a caller error"),
the constraint becomes invisible at the one place (`internal/loomengine`'s package doc)
someone building #004 would actually be looking when they wire the call in.
