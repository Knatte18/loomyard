MILL_REVIEW_BEGIN
# Review: fabric: accumulate the result envelope from mutations, not control flow (slice 14)

```yaml
verdict: APPROVE
reviewer_model: opus
reviewer_self_id: Claude (Anthropic), Opus-class model
reviewed_file: _mill/discussion.md
date: 2026-08-11
```

## Findings

### [NIT:decision] fabriccli cannot errors.As an unexported refusal type
**Demoted-from:** BLOCKING
**Section:** `structured-refusal` + Technical context ("the refusal type")
**Issue:** `destructiveRefusal` is unexported (`destroy.go:62`) — the discussion's own text notes `fabrictest` matches by substring "because the gate's one error type ... is unexported and therefore unreachable" (`fabrictest/refusal.go:46-49`) — yet the decision has `internal/fabriccli` find it "via `errors.As`", which is impossible across packages; only the check *enum* is given an export disposition.
**Fix:** decide the refusal type's disposition explicitly — export the struct, or add an exported accessor in `fabricengine` (e.g. `RefusalOf(err) (Refusal, bool)`) — and say which fields cross the boundary.

### [NIT:scope] CLI-layer mutations and zero-result returns are unaccounted for
**Demoted-from:** BLOCKING
**Section:** Scope ("In"), `record-survives-the-error-return`, push/sync composition
**Issue:** the model is engine-verb-owned, but two verbs mutate in `internal/fabriccli`: `CloneAndWire` (`clone.go:29-67`) runs `configsync.ReconcileFabricAt`, `Bolt.Commit`, `Bolt.Push`, `WireJunctions`, `configsync.ReconcileAll` after `CloneHub` and returns `fabricengine.CloneResult{}` at five failure sites; `runReconcile` (`fabric.go:568,588-621`) does a config rewrite plus `Bolt.Commit`/`Bolt.Push`. `Bolt` is explicitly scoped out, so those mutations are recorded nowhere — and the `CloneHubReset/RealHub` cell (`verbs.go:1162`) drives `CloneAndWire`, so its unfiltered honesty diff contains junctions/config/commits no record entry covers.
**Fix:** state a composition rule for `clone` and `reconcile` as done for `push`/`sync` — which layer owns the recorder, whether the CLI-layer `Bolt` calls record, and that the CLI's own zero-result returns change too.

### [NIT:scope] No Kind covers a pull/fetch advance; the partial rationale is false
**Demoted-from:** BLOCKING
**Section:** `mutation-entry-shape` Kind table, `ok-semantics-and-error-path-fields`
**Issue:** `Pull` sets `WeftPulled = true` after the weft ff-pull (`pull.go:190`) and can then return `&PartialPullError{Stage:"fetch"}` (`:197`) having created no commit and executed no gate primitive — the record is empty, so `partial` is `false` while the weft worktree was really advanced. The stated rationale that a `PartialPullError` always has a non-empty record "because a landed commit is a recorded `commit_created`" is wrong for this type; the enum has no kind for a repo advanced by pull/fetch, so the omission direction also fires on the Pull cell on correct behaviour.
**Fix:** add a kind for the pull/fetch advance (or state why the weft advance needs no entry) and correct the `partial`-derivation rationale for `PartialPullError`.

### [NIT:scope] Non-verb result types the harness reads carry no stated record
**Section:** `permitted-roots-and-the-oracle` (the `Mutated()` seam)
**Issue:** the matrix cell drives `fabricengine.UnwireJunctions` returning `UnwireResult` (`junction.go:368`, `verbs.go:960`), not the `Unwire` verb's `UnwireVerbResult`, so a type outside the twelve-verb list must also embed the record for the cross-check to compile.
**Fix:** name the inner result types the harness reads as also carrying the record.

## Verdict

APPROVE
Refusal access, CLI-layer mutations, and pull's unrecorded advance are undecided.
MILL_REVIEW_END
