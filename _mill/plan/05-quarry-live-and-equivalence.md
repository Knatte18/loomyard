# Batch: quarry-live-and-equivalence

```yaml
task: "Extract scout into its own standalone repo"
batch: "quarry-live-and-equivalence"
number: 5
cards: 4
verify: go -C /home/knatte/Code/quarry/wts/quarry test -tags lsp ./... -count=1
depends-on: [4]
```

## Batch Scope

This batch proves quarry does what `lyx scout` does.
It runs the live tier that no earlier batch compiled, resolves a query set fresh and compares JSON envelopes verb by verb between the two binaries, removes the port program, and files the follow-ups the port deliberately deferred.
It is one batch because the live tier and the envelope comparison exercise the same running daemons and the same installed toolchain, and because deleting the port program is only safe once both have passed.

This is the batch the whole task's acceptance rests on.
Every earlier batch proves quarry compiles and its own tests pass;
only card 35 proves it behaves the same.
Batch 6 must not start until this batch is green.

Batch-local decision: the query set is resolved at this commit against the current Loomyard tree and recorded.
The five Go positions in the multi-language research document are all stale — one package was renamed, two symbols moved by five to ten lines — and the ground-truth file they were graded against was never committed.
Feeding stale positions to both binaries returns a not-found error from each, and the envelopes compare equal while proving nothing.

## Cards

### Card 34: run the live tier and fix the fallout

- **Context:**
  - `/home/knatte/Code/quarry/wts/quarry/quarry/toolchain.go`
  - `/home/knatte/Code/quarry/wts/quarry/quarry/ensureserver.go`
- **Edits:**
  - `/home/knatte/Code/quarry/wts/quarry/quarry/ensureserver_integration_test.go`
  - `/home/knatte/Code/quarry/wts/quarry/quarry/refs_integration_test.go`
  - `/home/knatte/Code/quarry/wts/quarry/quarry/supervised_integration_test.go`
  - `/home/knatte/Code/quarry/wts/quarry/quarry/supervised_lsp_test.go`
  - `/home/knatte/Code/quarry/wts/quarry/quarry/toolchain_integration_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Run `go -C /home/knatte/Code/quarry/wts/quarry test -tags lsp ./... -count=1` and make it pass.
  These five files have been type-checked by `go vet -tags lsp` since batch 3 but never executed, so this is their first real run.
  Expect fallout in three places and fix only that: a fixture that still assumes the engine derives its own state path, now that `DaemonStateFile` is told a leaf directory;
  a `gopls` install that lands under the renamed `quarry` cache segment rather than the old one, which is correct and may need an assertion updated;
  and a comment or fixture name the vocabulary sweep could not reach because it lives behind the build tag.
  The other four languages' arms skip when their servers are absent, exactly as they do in Loomyard, so a skip is not a failure.
  Do not weaken an assertion to make a test pass.
  If a test fails because behaviour genuinely differs from Loomyard's, stop and report it — that is a port defect, and it invalidates card 35's premise rather than being something to fix by editing the test.
  Record the wall-clock duration of the run and whether `gopls` was installed fresh or reused, for the port log.
- **Commit:** `test(quarry): make the lsp live tier green`

### Card 35: prove behavioural equivalence against lyx scout

- **Context:**
  - `/home/knatte/Code/quarry/wts/quarry/internal/cli/cli.go`
  - `internal/scoutcli/cli.go`
- **Edits:** none
- **Creates:**
  - `/home/knatte/Code/quarry/wts/quarry/docs/port-equivalence.md`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Build both binaries — `go -C /home/knatte/Code/quarry/wts/quarry build -o /home/knatte/Code/quarry/wts/quarry/.scratch/quarry ./cmd/quarry` and `go build -o .scratch/lyx ./cmd/lyx` in this worktree — and compare their JSON envelopes for the same queries against this worktree as the target directory.
  Resolve the query set fresh at this commit rather than taking positions from any existing document: pick roughly ten Go symbols in this repo spanning the categories the earlier benchmark used — a high-fan-in plain function, a method with many call sites, a generic function, an interface method, and one symbol that does not exist, for the error path — and resolve each position with `lyx scout symbol` on the current tree.
  Record the resolved query set verbatim in the new document, so a later reader can re-run it.
  Then, for every query, run all four verbs on both binaries and compare.
  Assert on the `lyx scout` side **first** that the envelope is `ok` and carries at least one result, before comparing it to quarry's — a pair of identical `ErrSymbolNotFound` envelopes compares equal and proves nothing, and that is the failure mode this rule exists to prevent.
  The deliberate not-found query is the one exception: there, matching errors *are* the assertion, and the document must mark it as such so it is not mistaken for one of the others.
  The `assert-no-callers` arm additionally compares process exit codes, not only envelope bodies.
  Envelopes must otherwise match byte for byte, including error-message text — the `"scoutengine: "` prefixes are still verbatim on both sides, which is exactly why this comparison can be strict.
  The only permitted difference is an absolute path that legitimately changed, and every such difference must be listed individually in the document with the reason it changed.
  Write the result to the new quarry document: the query set, the per-query per-verb verdict, every permitted path difference, and a plain statement of whether the port is proven equivalent.
  If any comparison fails, stop and report rather than adjusting the criterion.
- **Commit:** `docs(quarry): record the lyx scout behavioural-equivalence comparison`

### Card 36: remove the port program and file the deferred follow-ups

- **Context:**
  - `/home/knatte/Code/quarry/wts/quarry/docs/port-equivalence.md`
- **Edits:** none
- **Creates:** none
- **Deletes:**
  - `/home/knatte/Code/quarry/wts/quarry/tools/port/main.go`
- **Moves:** none
- **Requirements:** Delete the port program and its `tools/` directory from quarry now that the port is proven;
  it was written for one run and keeping it would leave a Loomyard-shaped rewriter in a repo that has no Loomyard to rewrite.
  Confirm afterwards that `go -C /home/knatte/Code/quarry/wts/quarry build ./...` still succeeds and that `go.mod` is unchanged, since the program was stdlib-only.
  Then file two issues on the quarry repository with `gh issue create --repo Knatte18/quarry`, each linking the equivalence document by path:
  one for renaming the 59 `"scoutengine: "` error prefixes to `quarry:`, stating that they were deliberately left verbatim so the equivalence comparison could be strict and that the rename is now unblocked;
  and one for darwin support, stating that `internal/proc` has only linux and windows implementations so a darwin build fails, and naming `KillPID`, `IsAlive`, and `DetachBreakaway` as the three functions a darwin sibling must provide.
  Record both issue numbers for the port log.
  Do not fix either one in this task.
- **Commit:** `chore(quarry): remove the port program after the equivalence proof`

### Card 37: record batch 5 in the port log

- **Context:**
  - `/home/knatte/Code/quarry/wts/quarry/docs/port-equivalence.md`
- **Edits:**
  - `docs/research/quarry-port-log.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Append a `## Batch 5 — quarry-live-and-equivalence` section to the port log in this worktree.
  Copy the equivalence document's verdict table into it verbatim, so this worktree carries the evidence for the deletion batch 6 performs without a reader having to open the other repo.
  Also record the live tier's wall-clock duration and which language arms skipped, the two quarry issue numbers card 36 filed, and the quarry commit SHAs from cards 34 through 36.
  End the section with an explicit go or no-go line for batch 6: the deletion is authorized only if every verb compared equal on every query and the live tier is green.
- **Commit:** `docs: record the equivalence proof in the quarry port log`

## Batch Tests

`verify:` runs `go -C /home/knatte/Code/quarry/wts/quarry test -tags lsp ./... -count=1`, the live tier.
`-tags lsp` is what compiles the five files the earlier batches' gates never built, and the whole point of the `test-tier-tags` decision is that this one tag covers all of them — a command spelled `-tags integration` would compile one file out of six and appear green.
The command deliberately does not narrow to `./quarry/`: the tag selects files, not packages, and running the hermetic suites again alongside them costs seconds and catches a tag-guard mistake that hid a file from both tiers.

`-count=1` disables the test cache, which matters here because these tests spawn real language servers and install a real toolchain — a cached pass would prove nothing about the current tree.

Card 35's comparison is not a Go test and is not covered by `verify:`.
It is verified by its own recorded output: the equivalence document must show, for every query, a non-empty `ok` envelope on the `lyx scout` side and a byte-identical envelope on quarry's.
That asymmetry is deliberate — the document is the evidence a human reads before authorizing batch 6's irreversible deletion, and a green exit code would not carry it.
