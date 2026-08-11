MILL_REVIEW_BEGIN
# Review: fabric: accumulate the result envelope from mutations, not control flow (slice 14) — holistic

```yaml
verdict: APPROVE
reviewer_model: fablehigh
reviewer_self_id: Fable 5 (claude-fable-5)
reviewed_file: plan/
date: 2026-08-11
```

## Verdict

APPROVE
Every sampled source claim verified; structure, DAG, numbering, verify scopes, and decisions are all sound.

Verification notes (sampled against source, all confirmed): `checkout.go:206` `refusal.Check.String()`; the eight unparameterised executors and `(*Fabric).ResetHard` in `destroy.go`; `unwire.go:69`; `weft_verbs.go:179/:192` and the bypass branch returning before `resolveWarpLocation()` (line 73 vs 76); `gitrepo` `HasUnpushed`/`CurrentBranch`/`CurrentSHA`; `mutateGitExclude`'s current two-value return; `configsync.ReconcileAll` `[]Result` with `Applied bool` and `configengine.ConfigFile`; `pathAtOrBelowRoot`/`pathPermitted`, `DiffManifest(before, after, permitted)`, `AssertNoUnpermittedChange`, and the two prime-admin-root helpers in fabrictest; `newFabricFixture` in the `integration`-tagged external `package fabricengine_test`; `Bolt.Commit`'s `(sha, committed, err)`; `remove.go:74`'s bare pre-flight error; the exactly-four production `WireJunctions` callers (`add.go:176`, `checkout.go:113`, `reconcile.go:342`, `fabriccli/clone.go:55`); `CloneHub`'s two `hubPath = HubPath(cwd, name)` branches; `spawn_test.go:50`; `BoardDir` in `junctionnames.go` (present in card 25's Context). Cards 1–36 are globally sequential; the Batch Index DAG is acyclic with all files present; every card carries Context/Edits-or-Creates/Moves/Requirements/Commit; all `Moves:` are `none`; every batch creating tagged tests has a verify that compiles or runs them; All Files Touched matches the Edits/Creates union.
MILL_REVIEW_END
