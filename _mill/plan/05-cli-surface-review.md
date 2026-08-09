# Batch: cli-surface-review

```yaml
task: "Rename the fabric host vocabulary to warp, and name the composite repo Fabric"
batch: "cli-surface-review"
number: 5
cards: 2
verify: go test ./cmd/lyx/... ./internal/fabriccli/... ./tools/sandbox/...
depends-on: [4]
```

## Prior failure

- Round 1: implementer reported `status: success` with `cards_done: [13, 14]` and zero content commits (both cards found nothing to correct, per their explicit "make no commit" instruction) — finalize's completeness recount classified this as `stuck_type: logic, reason: "success reported but no content commit (only batch-start commit since start_sha)"`. The two per-card "make no commit" instructions did not say the required "record that explicitly in the batch notes" step itself must land in a commit, so the implementer's batch-notes record (if any) was never captured in git history for finalize to see. Clarified below: the record is now an explicit committed artifact.

## Batch Scope

The judgment pass over everything a **user** sees, applied to text that batch 3 already rewrote mechanically.

`wordswap` is a word-level substitution: it cannot tell the difference between a sentence that means *the composite repo* (which the vocabulary rule says must read "Fabric") and one that genuinely distinguishes the two sides (which must read "warp" or "weft").
Batch 3 swept the CLI help strings, the git-hook stderr output and the sandbox error strings along with everything else, producing "warp" uniformly.
This batch re-reads each of those surfaces and asks the vocabulary question per occurrence, correcting the ones the tool got wrong.

`CONSTRAINTS.md`'s CLI/Cobra Invariant makes this batch mandatory rather than optional: "Help accuracy is a review obligation.
When a change alters observable behaviour, the reviewer must re-check every affected `Short`/`Long`."
This task alters observable CLI text, so the re-check is owed.

The expected diff is small — most of this surface is `lyx fabric`'s own two-sided vocabulary, where "warp" is exactly right.
A small or empty diff is a valid outcome;
the deliverable is the confirmation, and any correction it turns up.

## Cards

### Card 13: re-check the `lyx fabric` help surface against the vocabulary rule

- **Context:**
  - `CONSTRAINTS.md`
  - `cmd/lyx/helptree_test.go`
  - `cmd/lyx/longlist_test.go`
  - `internal/fabriccli/clone.go`
  - `internal/fabriccli/unwire.go`
  - `cmd/lyx/jsonhelp_test.go`
- **Edits:**
  - `internal/fabriccli/fabric.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Read every `Short` and `Long` string in `internal/fabriccli/fabric.go`, plus the package doc comment, in their post-sweep state.
  For each occurrence of `warp` or `weft` that batch 3 produced from `host`, apply the vocabulary rule: does the sentence mean *the composite repo* (→ "Fabric", capital F), or does it genuinely need to tell the two sides apart (→ warp/weft)?
  Correct only the former.
  Never substitute a bare "repo" for warp — the rule rejects it as too vague.

  Expect most occurrences to survive as `warp`, because `lyx fabric` is the one command surface whose entire job is coordinating the two sides.
  The clone argument placeholders in particular are correct as `<warp-url>`, `<warp-name>` and `<weft-url>`: a `clone` verb taking two URLs cannot avoid naming which is which.

  Then confirm the CLI/Cobra Invariant still holds on this subtree:
  - Every command (parent and sub) still has a non-empty `Short`.
  - The root `Long` module list still names `fabric`.
  - No help string still contains `host` in any case, in any sense.

  `cmd/lyx/helptree_test.go`, `cmd/lyx/longlist_test.go` and `cmd/lyx/jsonhelp_test.go` were verified to contain zero `host` assertions before this task began, so they should pass unchanged;
  confirm that rather than assume it, since the invariant names them as the machine half of this check.
  If this card's review finds nothing to correct, make no source-file commit — but the confirmation itself must still be committed: append a one-line dated note to this batch file's `## Batch Notes` section (create the section at the end of this file, after `## Batch Tests`, if it does not exist) stating the review found nothing to correct, and commit that note with a trivial message (e.g. `docs(fabriccli): record card 13 review — no correction needed`).
- **Commit:** `docs(fabriccli): apply the Fabric vocabulary rule to the fabric help surface` (or the no-correction record commit above, if the review found nothing to change)

### Card 14: re-check the user-visible runtime strings

- **Context:**
  - `CONSTRAINTS.md`
  - `tools/sandbox/resolve.go`
  - `_mill/discussion.md`
- **Edits:**
  - `internal/fabricengine/post-checkout.sh`
  - `tools/sandbox/main.go`
  - `tools/sandbox/report.go`
  - `tools/sandbox/suite.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Apply the same vocabulary question to every string a user or operator actually reads at runtime, in its post-sweep state.

  `internal/fabricengine/post-checkout.sh` — the two stderr lines the hook prints, currently reading (post-sweep) `fabric: warp/weft out of sync — run \`lyx fabric checkout <branch>\` to re-sync, or \`lyx fabric reconcile\` to inspect` at line 62 and `  warp: $WARP_BRANCH (expects weft: $EXPECTED_WEFT_BRANCH)` at line 63.
  Both are genuine two-sided distinctions and should survive as warp/weft;
  confirm the shell variable rename from `HOST_BRANCH` to `WARP_BRANCH` is internally consistent across lines 43, 44, 54, 55 and 58, since a half-renamed shell variable expands to the empty string silently rather than failing to compile.

  `tools/sandbox` — six user-visible error strings carry the vocabulary, not the four `_mill/discussion.md` enumerates;
  it missed `suite.go`'s pair, which duplicates `report.go`'s wording.
  Check all six:
  - `tools/sandbox/main.go` line 139 — `fabric hub warp repo not found at %s -- run sandbox/fabric-suite.cmd, which clones it first`
  - `tools/sandbox/main.go` line 141 — `stat fabric warp repo %s: %w`
  - `tools/sandbox/report.go` line 57 — `hub warp repo not found at %s -- run sandbox/build.cmd first`
  - `tools/sandbox/report.go` line 59 — `stat warp repo %s: %w`
  - `tools/sandbox/suite.go` line 323 — `hub warp repo not found at %s -- run sandbox/build.cmd first`
  - `tools/sandbox/suite.go` line 325 — `stat warp repo %s: %w`

  Each names the warp side of a hub that also contains a weft side, so "warp" is the right word in all six and no change is expected;
  the deliverable is the confirmation and the correction of the undercount.
  Line numbers will have shifted slightly from batch 3's edits — locate each string by its text, not by line number.

  Do not touch `tools/sandbox/*.md` here;
  the eight `SANDBOX-*-SUITE.md` agent prompt templates are consumer-facing prose and belong to batch 6.
  If this card's review finds nothing to correct, make no source-file commit — but the confirmation itself must still be committed: append a one-line dated note to this batch file's `## Batch Notes` section (create the section at the end of this file, after `## Batch Tests`, if it does not exist) stating the review found nothing to correct (including the undercount confirmation), and commit that note with a trivial message (e.g. `docs(fabric,sandbox): record card 14 review — no correction needed`).
- **Commit:** `docs(fabric,sandbox): apply the Fabric vocabulary rule to user-visible runtime strings` (or the no-correction record commit above, if the review found nothing to change)

## Batch Tests

`verify: go test ./cmd/lyx/... ./internal/fabriccli/... ./tools/sandbox/...` scopes to the three packages that own or machine-check the edited surfaces.

`cmd/lyx` carries the CLI/Cobra Invariant's machine half — `helptree_test.go`, `longlist_test.go`, `jsonhelp_test.go`, `drift_test.go` and `registration_test.go` — which is precisely what a botched help-string edit breaks, and it is also where `sandbox_coverage_test.go` lives, so a `tools/sandbox` change that disturbed a `**Covers:**` line surfaces here too.
`internal/fabriccli` and `tools/sandbox` run their own suites, including `tools/sandbox/pathresolve_guard_test.go`, which enforces the Dev/Prod Binary Separation invariant over the same files card 14 edits.

`internal/fabricengine/post-checkout.sh` has no Go test of its own — it is an embedded shell hook — so its gate is the shell-variable consistency check written into card 14 plus `internal/fabricengine`'s hook tests, which batch 3's unbounded run already proved green and which the overview's module-wide `go build ./...` keeps compiling.

## Batch Notes

- 2026-08-09: Card 13 review — no correction needed. Read every `Short`/`Long` string and the package doc comment in `internal/fabriccli/fabric.go` in its post-sweep state; every `warp`/`weft` occurrence, including the `<warp-url>`/`<warp-name>`/`<weft-url>` clone placeholders, genuinely distinguishes the two sides, so none is substituted with "Fabric". Confirmed zero `host` occurrences (any case) in `fabric.go`, `clone.go`, and `unwire.go`, and zero `host` assertions in `cmd/lyx/helptree_test.go`, `cmd/lyx/longlist_test.go`, and `cmd/lyx/jsonhelp_test.go`. Every command retains a non-empty `Short`, and the root `Long` names `fabric` throughout.
- 2026-08-09: Card 14 review — no correction needed (including the undercount confirmation). `internal/fabricengine/post-checkout.sh` lines 62–63 read `fabric: warp/weft out of sync — run \`lyx fabric checkout <branch>\` to re-sync, or \`lyx fabric reconcile\` to inspect` and `  warp: $WARP_BRANCH (expects weft: $EXPECTED_WEFT_BRANCH)`; both are genuine two-sided distinctions and survive unchanged. The `HOST_BRANCH`→`WARP_BRANCH` shell-variable rename is internally consistent across lines 43, 44, 54, 55, and 58 — no half-renamed variable. Confirmed all six `tools/sandbox` user-visible error strings (two each in `main.go` lines 139/141, `report.go` lines 57/59, and `suite.go` lines 323/325) — the undercount `_mill/discussion.md` missed `suite.go`'s pair, which duplicates `report.go`'s wording; each names the warp side of a hub that also contains a weft side, so "warp" is correct in all six and none is changed.
