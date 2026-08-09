# Batch: wordswap-tool

```yaml
task: "Rename the fabric host vocabulary to warp, and name the composite repo Fabric"
batch: "wordswap-tool"
number: 1
cards: 5
verify: go test ./tools/wordswap/...
depends-on: []
```

## Batch Scope

Build `tools/wordswap/` — a language-agnostic, case-preserving, whole-token word-substitution tool, committed with unit tests, modelled on the existing `tools/mdreflow/` and `tools/godocreflow/` house pattern.
This is the only genuinely new code in the task and the only batch that writes an implementation rather than applying one, so it is test-driven: card 2 writes the full test file first, against the API cards 3–5 then implement.
Every later batch consumes this tool through the command line `go run ./tools/wordswap -from host -to warp [-skip <regexp>]... [-dry-run] <path>...`, so the exported behaviour pinned here — the token-boundary rule, the two report buckets, the exit code, and the reversibility invariant — is the external interface the rest of the plan depends on.

**Batch-local decision, resolving a contradiction in `_mill/discussion.md`.**
The discussion states the boundary rule twice in mutually inconsistent forms: its Testing section lists `conhost` among the tokens that "must all be classified AMBIGUOUS", while its Decisions section requires that `ghost` and `localhost` "must **not** match (lowercase precedes)" and rejects a permissive rule precisely because it "would rewrite `hostname`, `localhost`, and `conhost`, destroying the tool for reuse".
`conhost` and `localhost` are the same shape — `host` at a token *end*, with a lowercase letter preceding — so they cannot be classified differently.
This plan resolves it in favour of the lowercase-precedes rule, which is stated twice and carries the reuse rationale: a lowercase letter immediately before `host` means **no match and no report at all**.
The AMBIGUOUS bucket is reserved for `host` at a token *start* followed by a lowercase letter.
The resolution is risk-free for this task either way — no file in the sweep set contains `conhost` or `localhost`.

## Cards

### Card 1: package skeleton and package doc

- **Context:**
  - `tools/mdreflow/main.go`
  - `tools/godocreflow/main.go`
  - `_mill/discussion.md`
- **Edits:** none
- **Creates:**
  - `tools/wordswap/main.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `tools/wordswap/main.go` as `package main` with a package doc comment following `tools/mdreflow/main.go`'s shape exactly: a prose statement of purpose, the safety invariant, the usage lines, and a `Last run:` note.
  The doc comment must state (a) that the tool performs a case-preserving whole-token substitution of one word for another across files of any language, (b) that its safety invariant is reversibility over recorded spans — reverting exactly the recorded substitution offsets must reproduce the input byte-for-byte, and a file failing that check is left untouched and reported, and (c) that `host` + lowercase at a token start is reported as AMBIGUOUS rather than guessed at.
  Usage lines, indented one tab so godoc renders them as a code block:

  ```
  	go run ./tools/wordswap -from host -to warp [-dry-run] [-skip <regexp>]... <path-or-glob> [...]
  	go run ./tools/wordswap -from host -to warp -skip 'pane hosting an idle agent' internal/fabricengine/*.go
  ```

  The `Last run:` line reads `// Last run: 2026-08-09, host->warp sweep of the fabric-host-to-warp-rename task.`
  Declare `func main()` with the flag set only — `-from` (string, required), `-to` (string, required), `-dry-run` (bool), and `-skip` (repeatable regexp) — plus the glob expansion and the usage/exit-2 path.
  Card 5 fills in the per-file driving loop.
  Do NOT add a `TestMain`: this package spawns no git and no subprocess, so neither the Hermetic Git Test Environment Invariant nor the Test Tier Purity Invariant requires one.
- **Commit:** `feat(wordswap): add tools/wordswap package skeleton and CLI flags`

### Card 2: the substitution unit tests, written first

- **Context:**
  - `tools/mdreflow/reflow_test.go`
  - `tools/wordswap/main.go`
  - `_mill/discussion.md`
  - `cmd/lyx/tierpurity_test.go`
- **Edits:** none
- **Creates:**
  - `tools/wordswap/swap_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Write `tools/wordswap/swap_test.go` as `package main`, before any of `swap.go` exists, pinning the API cards 3–5 must satisfy.
  The tests call `swapText(in, from, to string, skips []*regexp.Regexp) (Result, error)` and inspect `Result`'s fields `Out string`, `Ambiguous []Occurrence`, `Skipped []Occurrence`, `Mismatch bool`, where `Occurrence` has fields `Line int`, `Text string`.
  Cover, as table-driven sub-tests wherever the shape allows:
  - Case-preserving substitution with `from="host"`, `to="warp"`: `host`→`warp`, `Host`→`Warp`, `HOST`→`WARP`; embedded forms `hostBranch`→`warpBranch`, `HostJunctions`→`WarpJunctions`, `HOST_BRANCH`→`WARP_BRANCH`.
  - Token-boundary rejection: `ghost`, `localhost` and `conhost` must be left unchanged AND must not appear in `Ambiguous` or `Skipped` — a lowercase letter immediately before the match means no match at all.
  - Camel-start acceptance: `myHostPath` swaps to `myWarpPath` even though a lowercase letter precedes, because the matched form itself starts uppercase.
  - Mixed-case rejection: `hOst` and `HoSt` match neither the lower, Title, nor UPPER form and are left unchanged and unreported.
  - Ambiguity classification: `hostclean`, `hostlayout`, `hosthub` and `hostname` are each left byte-unchanged in `Out` **and** are reported in `Ambiguous` with the correct 1-based `Line`.
    Assert both halves.
  - Multiple occurrences on one line, and mixed forms (`hostBranch` and `HOST_BRANCH` and bare `host`) on one line, all swapped in a single pass.
  - `-skip` behaviour: an occurrence whose **line** matches a skip regexp is left unchanged and reported in `Skipped`, never in `Ambiguous`;
    a non-matching occurrence on another line still swaps.
    Cover a skip claiming an otherwise-AMBIGUOUS occurrence (`a live pane hosting an idle agent`), which is the case that lets a run reach exit zero.
  - Reversibility invariant, including the critical case where the target word already occurs in the input: an input containing both `warp` and `host` must transform correctly and pass the check.
  - Language-agnosticism: a shell fragment (`HOST_BRANCH="$(git rev-parse --abbrev-ref HEAD)"`) and a markdown fragment (`the **host repo** holds ...`) each substitute correctly.
  Add a separate `TestProcessFile_DryRunWritesNothing` and `TestProcessFile_MismatchLeavesFileUntouched` in this same file, driving `processFile(path string, from, to string, skips []*regexp.Regexp, dryRun bool) (string, Result, error)` against files written into `t.TempDir()`.
  For the mismatch case, inject the failure through the package-level `var revertSpans = ...` hook card 5 introduces rather than by contriving input.
  Use only `os.WriteFile`/`os.ReadFile` and `t.TempDir()` — never `exec.Command` and never `gitexec` — so the file stays Tier-1 pure under `cmd/lyx/tierpurity_test.go`.
- **Commit:** `test(wordswap): pin substitution, boundary, ambiguity and reversibility rules`

### Card 3: case-preserving substitution and the token-boundary rule

- **Context:**
  - `tools/wordswap/swap_test.go`
  - `tools/wordswap/main.go`
- **Edits:** none
- **Creates:**
  - `tools/wordswap/swap.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Create `tools/wordswap/swap.go` as `package main` and implement the matching and substitution core.
  Declare `type Occurrence struct { Line int; Text string }` and `type Result struct { Out string; Ambiguous []Occurrence; Skipped []Occurrence; Mismatch bool }`.
  Implement `func caseForm(s string) (string, bool)` returning `"lower"`, `"title"` or `"upper"` for a candidate slice equal to `from` under `strings.EqualFold`, and `false` for any mixed form such as `hOst` — a mixed form is not a match.
  Implement `func applyCase(form, to string) string` producing `warp`, `Warp` and `WARP` respectively.
  Implement `func boundaryBefore(s string, i int) bool`: true when `i == 0`, or when `s[i-1]` is neither an ASCII letter nor an ASCII digit, or when the matched form is `"title"` or `"upper"` (the camelCase/SCREAMING_CASE start case).
  False otherwise — this is what rejects `ghost`, `localhost` and `conhost`.
  Implement `func boundaryAfter(s string, i, n int) bool`: true when `i+n == len(s)`, or when `s[i+n]` is an ASCII uppercase letter, an ASCII digit, an underscore, or any non-identifier character.
  False when `s[i+n]` is an ASCII lowercase letter — that is the AMBIGUOUS case card 4 handles.
  Implement `func swapText(in, from, to string, skips []*regexp.Regexp) (Result, error)` scanning `in` once, left to right, and for each case-insensitive occurrence of `from`: skip it entirely when `caseForm` reports mixed or `boundaryBefore` is false;
  otherwise classify and act per cards 8 and 9.
  Substitution is single-pass over the input — never re-scan the output, so a `warp` produced by this run can never be re-matched.
  `swapText` returns an error only for a malformed argument (empty `from` or `to`);
  a failed safety check is reported through `Result.Mismatch`, not an error.
- **Commit:** `feat(wordswap): implement case-preserving substitution and token-boundary rule`

### Card 4: ambiguity classification, the two report buckets, and `-skip`

- **Context:**
  - `tools/wordswap/swap_test.go`
- **Edits:**
  - `tools/wordswap/swap.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  Extend `swapText` in `tools/wordswap/swap.go` with the classification and reporting behaviour.
  For every occurrence that clears `caseForm` and `boundaryBefore`, resolve the line it sits on (1-based, counted by `\n` in `in`) and its full line text.
  Test that line text against every regexp in `skips`;
  on a match, leave the occurrence unchanged, append an `Occurrence` to `Result.Skipped`, and continue.
  A skip claims the occurrence regardless of whether it would otherwise have been swapped or classified AMBIGUOUS — this is what makes a deliberate keep expressible.
  Otherwise, when `boundaryAfter` is false, leave the occurrence unchanged and append an `Occurrence` to `Result.Ambiguous`.
  When `boundaryAfter` is true, substitute `applyCase(form, to)` and record the span per card 5.
  Deduplicate nothing — each textual occurrence produces its own report entry, so a line with two ambiguous hits reports twice.
  The two buckets are never merged: `Ambiguous` is what an operator must still resolve, `Skipped` is the audit record of what they already decided to keep.
- **Commit:** `feat(wordswap): classify ambiguous compounds and honour -skip as a second report bucket`

### Card 5: reversibility invariant and the driving loop

- **Context:**
  - `tools/wordswap/swap_test.go`
  - `tools/mdreflow/main.go`
- **Edits:**
  - `tools/wordswap/swap.go`
  - `tools/wordswap/main.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `tools/wordswap/swap.go`, declare `type span struct { start, end int; orig string }` recording, for each substitution `swapText` performs, the byte offsets of the replacement **in the output** and the original text it displaced.
  Implement `func revertSpansImpl(out string, spans []span) string` rebuilding the input by replacing each recorded output span with its `orig`, walking spans in ascending `start` order.
  Expose it through a package-level `var revertSpans = revertSpansImpl` so card 2's mismatch test can inject a deliberately broken transform.
  At the end of `swapText`, set `Result.Mismatch = revertSpans(out, spans) != in` and, when `Mismatch` is true, set `Result.Out = in` so a mismatching file is never rewritten.
  This check must hold even though `to` already occurs in the input — it compares reconstructed bytes, not counts, which is why a count invariant was rejected.
  In `tools/wordswap/main.go`, add `func processFile(path string, from, to string, skips []*regexp.Regexp, dryRun bool) (string, Result, error)` reading the file, calling `swapText`, and returning the status string `"mismatch"`, `"changed"` or `"unchanged"`, preserving the file's existing mode via `os.Stat` exactly as `tools/mdreflow/main.go`'s `processFile` does, and writing nothing when `dryRun`.
  Complete `main()` to drive every expanded path through `processFile`, printing `MISMATCH (left untouched): <path>` for a mismatch, `changed: <path>` / `would change: <path>` for a change, and accumulating changed/unchanged counts.
  After the loop, print the two report buckets separately under distinct headings — an `AMBIGUOUS (unresolved):` list and a `SKIPPED (deliberate):` list — each entry as `<path>:<line>: <line text>`.
  Exit 1 when the mismatch list is non-empty OR the unresolved-AMBIGUOUS list is non-empty.
  The skipped list never affects the exit code.
  Exit 0 otherwise.
- **Commit:** `feat(wordswap): add reversibility safety invariant and the file-driving loop`

## Batch Tests

`verify: go test ./tools/wordswap/...` runs `tools/wordswap/swap_test.go`, the only test file this batch creates, scoped to the one package it touches.
The scope is deliberately narrow: this batch adds a new leaf package under `tools/` that nothing else imports, so no other package's tests can be affected by it.
The overview's module-wide `verify: go build ./...` catches any compile-level breakage beyond that scope at the batch boundary.

The test file is the batch's specification: it is written in card 2 before `swap.go` exists, and cards 3–5 are complete exactly when it passes.
The two subtle rules — the camel/snake-aware token boundary (which is *not* `\b`-anchored, since `hostBranch` has no word boundary after `host`) and the AMBIGUOUS classification — are pinned there rather than discovered during implementation, because ~94 files are rewritten by this tool in batch 3 and a wrong boundary rule would corrupt all of them silently.

The reversibility test must include the case where `warp` already occurs in the input, mirroring the 576 pre-existing occurrences in `internal/fabricengine` that a naive round-trip or count-based check could not handle.
