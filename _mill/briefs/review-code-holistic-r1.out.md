MILL_REVIEW_BEGIN
# Review: Rename loom CLI run/drive/step for verb/engine symmetry, plus rename ly-supervise — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-09-19
```

## Findings

### [BLOCKING:scope] Retired-verb hits survive in two fully-swept design docs
**Location:** `manifest/designs/loom.md:440`, `docs/overview.md:244`
**Issue:** `loom.md:440` still reads "The verbs that genuinely build producers (`run`, `drive`) keep failing early and loudly on a bad config" — contrast prose naming both verbs that must read `start`, `run`. `overview.md:244`'s module-table row still reads "the aggregation-and-reflection step internal/loomcli's drive verb calls once per run" — `drive` is the retired verb; card 10/11 rewired the same call site's comment in `run.go` itself to name `run`. Both files are named whole in card 15/14's `Edits:` list, and the batch's own per-file sweep rule requires every hit classified, not only the landmarks each card enumerates.
**Fix:** Rewrite `loom.md:440` as a sentence naming `start`/`run` (keeping both sides distinct, per the contrast-prose decision), and `overview.md:244` to name the `run` verb.

### [BLOCKING:scope] A bare backtick `drive` verb reference survives beside an edited sentence
**Location:** `manifest/designs/self-report-tier2.md:42`
**Issue:** Card 16 lists exactly four references to fix in this file; the bootstrap-lifecycle bullet's *first* sentence (line 41, "so `lyx loom start` and `lyx loom step` behave identically") was correctly rewritten, but its very next sentence in the same bullet — "It is deliberately not `drive`'s job alone — `step` spawns no driver..." — still names the retired verb `drive` in exactly the bare-backtick form the per-file sweep rule calls out (`bare backticked \`run\`/\`drive\` naming a verb`).
**Fix:** Rewrite `` `drive`'s job alone `` to `` `run`'s job alone ``, consistent with the rest of the file's rename.

### [BLOCKING:scope] Files never enumerated in any batch's Edits: still carry live pre-move filename citations and a retired-verb test case
**Location:** `internal/loomcli/landingdeps.go:2,21`, `internal/loomcli/wiring_test.go:350,625`, `internal/loomcli/step_test.go:288`
**Issue:** `landingdeps.go`'s header and its `landingDeps` doc comment both still say "every value already resolved by drive.go" / "the caller (drive.go)" — a stale filename citation, since card 2 moved `drive.go` to `run.go`. `wiring_test.go:350` repeats the same stale citation ("the three fields drive.go passes to landingDeps"), and `wiring_test.go:625` still exercises a `{"Drive", "drive", false}` table row for the retired verb name. `step_test.go:288` says "the in-process capture idiom TestVerbRefusals (cli_test.go) already uses for drive/pause" — `TestVerbRefusals` covers `Run_SeedMissing`, not `drive`, post-rename. None of these five files appear in any card's `Context:`/`Edits:`/`Creates:` list or in the overview's "All Files Touched" enumeration, yet all five carry a live hit of the retired name. This violates the `no-back-compat-aliases` Shared Decision ("The old names survive nowhere in the tree, including in historical prose"), which is stated to apply to all batches, and reflects an incomplete work-inventory enumeration rather than a per-card execution slip.
**Fix:** Add these five files (or at minimum `landingdeps.go` and `wiring_test.go`) to a batch's `Edits:` list and rewrite the stale `drive.go` citations to `run.go`, the `Drive`/`drive` test row to a covering `Run`/non-verb case, and the `step_test.go` comment to name `run`/`pause`.

## Verdict

REQUEST_CHANGES
Three concrete leftover "drive" hits (two in fully-swept batch-5 docs, one cluster in files never enumerated for the sweep at all).
MILL_REVIEW_END
