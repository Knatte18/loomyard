# Batch: tierpurity-guard-generalization

```yaml
task: Formalize the Tier 1/2 substrate rule and re-tier mis-tagged tests
batch: tierpurity-guard-generalization
number: 1
cards: 5
verify: go test ./cmd/lyx/... -run TestTierPurity -count=1 && go test ./cmd/lyx/... -run TestIsTierTagged -count=1 && go test ./cmd/lyx/... -run TestFindLongLiteralSleep -count=1
depends-on: []
```

## Batch Scope

This batch generalizes `cmd/lyx/tierpurity_test.go`'s tag recognition from two hardcoded tags to a known-tags list (`integration`, `smoke`, `scout`), fixes the two stale doc comments in that file that this generalization would otherwise leave inaccurate, and adds a new, independent check to the same guard: an untagged test file containing a literal `time.Sleep(...)` call whose duration is a compile-time constant ≥ 1 second is now flagged, allowlist-backed exactly like the existing banned-token check. The `scout` tag itself does not exist anywhere in the repo yet after this batch — introducing it into `internal/scoutengine` is the next batch's job (`scoutengine-scout-tag`, `depends-on: [1]`) — this batch only teaches the guard machinery to recognize it once it appears. `internal/reedengine/testmain_test.go` and `internal/reedcli/testmain_test.go` are allowlisted against the new Sleep check in the same commit that introduces it, since both already contain a real, legitimate `for { time.Sleep(time.Hour) }` header-pane-keepalive stand-in that would otherwise regress the build the moment this check lands.

Cards are ordered so `cmd/lyx/tiersleep_test.go` exists (Card 3) before any later card's Requirements references it by name (Card 5) — Card execution is sequential within a batch, so a Context/Requirements reference to a file must never precede the card that creates it.

This batch is the sole owner of `cmd/lyx/tiersleep_test.go` (a new file) and makes coordinated edits to `cmd/lyx/tierpurity_test.go` and `CONSTRAINTS.md`; later batches also touch these same two files in different sections, which is expected — `depends-on: [1]` on batch 2 orders those edits after this batch's commits land.

## Cards

### Card 1: Generalize `isTierTagged()` to a known-tags list

- **Context:** none
- **Edits:**
  - `cmd/lyx/tierpurity_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Add a new package-level `var knownTierTags = []string{"integration", "smoke", "scout"}` in `cmd/lyx/tierpurity_test.go`. Replace `isTierTagged`'s final line — currently `return strings.Contains(trimmed, "integration") || strings.Contains(trimmed, "smoke")` — with a loop over `knownTierTags` that returns `true` as soon as `strings.Contains(trimmed, tag)` matches any entry, `false` if none match. Do not change `isTierTagged`'s signature, its early-return-on-non-`//go:build`-first-line behavior, or `TestTierPurity_UntaggedTestsSpawnNothing`'s own call site. Add a new test function `TestIsTierTagged_RecognizesKnownTagsList` in the same file: table-driven, covering `//go:build integration` (true), `//go:build smoke` (true), `//go:build scout` (true), `//go:build windows` (false — platform-only constraint stays untagged per the existing doc comment's own rule), `//go:build linux && scout` (true — substring match still fires on a compound constraint), and empty input (false).
- **Commit:** `test(cmd/lyx): generalize isTierTagged to a known-tags list (integration, smoke, scout)`

### Card 2: Fix `tierpurity_test.go`'s two stale doc comments

- **Context:** none
- **Edits:**
  - `cmd/lyx/tierpurity_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In the file-header comment block (lines 1-6), change the phrase "without `-tags integration`/`smoke`)" to "without `-tags integration`/`smoke`/`scout`)" so it no longer hardcodes only two of the three now-recognized tags. In `isTierTagged`'s own doc comment (the block directly above its function definition), change the phrase 'constraint mentioning "integration" or "smoke".' to 'constraint mentioning any entry of `knownTierTags` ("integration", "smoke", "scout").' so the doc comment names the same list Card 1 introduced instead of two hardcoded literals.
- **Commit:** `docs(cmd/lyx): update tierpurity_test.go's stale integration/smoke-only doc comments for the scout tag`

### Card 3: Add the literal `time.Sleep(>=1s)` parsing helper

- **Context:**
  - `internal/reedengine/testmain_test.go`
  - `internal/reedcli/testmain_test.go`
- **Edits:** none
- **Creates:**
  - `cmd/lyx/tiersleep_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `cmd/lyx/tiersleep_test.go`, `package main`, importing `go/ast`, `go/parser`, `go/token`, and the standard library packages its logic needs (`strconv`, `strings`). Define:
  - `var allowedLongSleepers = map[string]string{...}` with exactly two entries. The first key is `"internal/reedengine/testmain_test.go"`; its value describes the file's untagged header-pane-keepalive `TestMain` stand-in whose body loops calling `time.Sleep(time.Hour)` at line 31 — intentional and safe because `TestMain` itself intercepts this exact invocation shape (`os.Args[1] == "reed"`) as a deliberate stand-in for the real `lyx reed header --blocking` keepalive, not an accidental recursive re-exec; it is not reachable during a normal `go test` run (no test invokes the binary with a leading "reed" argument). The second key is `"internal/reedcli/testmain_test.go"`; its value states it has the same shape and the same `TestMain`-intercept rationale at its own matching loop on line 35 — note this file's `TestMain` also calls `lyxtest.HermeticGitEnv()` later in the same function, but only on the code path reached when the `"reed"` branch above is NOT taken; the sleep loop itself is an infinite `for` loop that never returns, so `HermeticGitEnv()` never executes on the sleep-loop path and plays no part in why that path is safe.
  - `func findLongLiteralSleep(fset *token.FileSet, filename string, data []byte) (evidence string, found bool)`: parses `data` via `parser.ParseFile(fset, filename, data, 0)`; on a parse error, return `("", false)` (a file that fails to parse as Go is not this check's concern). Walk the resulting `*ast.File` with `ast.Inspect`, looking for `*ast.CallExpr` nodes whose `Fun` is a `*ast.SelectorExpr` with `X` a `*ast.Ident` named `"time"` and `Sel.Name == "Sleep"`. For each match, evaluate its single argument via `sleepDurationAtLeastOneSecond` (below); on the first argument found to be `isLong`, return `evidence` naming the call's position (`fset.Position(call.Pos()).String()`) and `true`; on the first argument found `unresolvable`, also return `found = true` with an evidence string noting it could not be resolved (per the conservative-flag decision below); otherwise continue the walk.
  - `func sleepDurationAtLeastOneSecond(file *ast.File, arg ast.Expr) (isLong bool, unresolvable bool)`, handling exactly three argument shapes: (a) a bare `*ast.SelectorExpr` with `X` a `*ast.Ident` named `"time"` — `Sel.Name` of `"Second"`, `"Minute"`, or `"Hour"` is `isLong = true`; `"Millisecond"`, `"Microsecond"`, or `"Nanosecond"` is `isLong = false`; any other `Sel.Name` is `unresolvable = true`. (b) a `*ast.BinaryExpr` with `Op == token.MUL` where one operand is a `*ast.BasicLit` of `Kind == token.INT` or `token.FLOAT` and the other operand recurses into case (a)'s scale-selector check — compute `literal value * scale-in-nanoseconds >= time.Second` (1e9) using `strconv.ParseFloat` on the literal's text; if the non-literal operand does not resolve to a known `time.*` scale, `unresolvable = true`; if `strconv.ParseFloat` itself returns an error (e.g. a non-decimal literal it cannot parse), also `unresolvable = true`, consistent with every other unrecognized shape in this function. (c) a bare `*ast.Ident` — search `file.Decls` for a top-level `*ast.GenDecl` (`Tok == token.CONST` or `token.VAR`) containing a `*ast.ValueSpec` whose `Names` includes this identifier, and recurse into case (a) or (b) on that spec's corresponding `Values` expression (one level of indirection only — do not chase a further identifier-to-identifier chain); if no matching declaration is found in `file.Decls`, `unresolvable = true`. Any other argument shape not covered by (a)/(b)/(c) is also `unresolvable = true`. Document in a comment directly above `sleepDurationAtLeastOneSecond` that "unresolvable" is deliberately treated as "flag it" (forcing an explicit `allowedLongSleepers` entry or a rename), per this task's own discussion decision — not a bug to be silently swallowed.
  - Add a table-driven test `TestFindLongLiteralSleep_DetectsAllArgumentForms` in the same file, constructing small in-memory Go source snippets (as `[]byte`, parsed via the function under test with a fresh `token.NewFileSet()` each time — no real files on disk) covering: a bare scaled literal ≥1s (`time.Sleep(time.Hour)` — flagged), a bare scaled literal <1s (`time.Sleep(time.Millisecond)` — not flagged), a multiplication ≥1s (`time.Sleep(2 * time.Second)` — flagged), a multiplication <1s (`time.Sleep(500 * time.Millisecond)` — not flagged), a named same-file const ≥1s resolved by identifier (flagged), and an identifier with no resolvable declaration in the file (flagged as unresolvable, per the conservative-flag rule).
- **Commit:** `feat(cmd/lyx): add findLongLiteralSleep, an AST-based literal time.Sleep(>=1s) detector for the tier-purity guard`

### Card 4: Wire the Sleep guard into the tier-purity walk and generalize the allowlist lookup

- **Context:**
  - `cmd/lyx/tiersleep_test.go`
- **Edits:**
  - `cmd/lyx/tierpurity_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rename `spawnerAllowed(relPath string) bool` to `pathAllowlisted(relPath string, allowlist map[string]string) bool`, adding the `allowlist` parameter to its existing prefix-match loop body (`for prefix := range allowlist`), and update its one call site inside `TestTierPurity_UntaggedTestsSpawnNothing` to `pathAllowlisted(relPath, allowedSpawners)`. That walk callback's existing `token, bad := firstBannedToken(data)` local variable must be renamed to `bannedTok, bad := firstBannedToken(data)` (and its one other use, the trailing argument of the `failures` append's `fmt.Sprintf` call, updated from `token` to `bannedTok`), because this card also adds a `"go/token"` import and calls `token.NewFileSet()` in this same closure — leaving the local variable named `token` would shadow the `go/token` package identifier and fail to compile (`token.NewFileSet` undefined on a `string`). The existing banned-token check block in that function's walk callback returns `nil` on every one of its three branches (`!bad` → `return nil`; `pathAllowlisted(relPath, allowedSpawners)` true → `return nil`; the flagged path appends to `failures` then `return nil`) — placing new code after this block as written today would make it unreachable. Restructure the block so it falls through instead of returning early: change the `if !bad { return nil }` guard and the `if pathAllowlisted(relPath, allowedSpawners) { return nil }` guard so that when either condition is true, control simply skips past the `failures` append (e.g. wrap the append in an `if bad && !pathAllowlisted(...) { failures = append(...) }` shape, or equivalent `else`-free early-continue restructuring) rather than returning from the closure, and remove the unconditional `return nil` that currently follows the append so the closure reaches a single trailing `return nil` after both checks. Immediately after that restructured banned-token check, in the same walk callback, add a second, independent check that now genuinely runs for every file regardless of whether the banned-token check already flagged it: if `!pathAllowlisted(relPath, allowedLongSleepers)` (the map Card 3 defined in `cmd/lyx/tiersleep_test.go`), call `findLongLiteralSleep(token.NewFileSet(), path, data)` — do not also re-check `isTierTagged(data)` here, since the walk callback already `return nil`s earlier in the same function whenever `isTierTagged(data)` is true, so by the time this new check runs it is always false; re-testing it here would be dead weight; on `found`, append `fmt.Sprintf("%s: contains a literal time.Sleep(...) of >= 1s in an untagged test file (%s) — move it behind a build tag, shrink the duration, or add an allowedLongSleepers entry in cmd/lyx/tiersleep_test.go with a reason", relPath, evidence)` to `failures`. End the closure with the single trailing `return nil` mentioned above, after both checks have had a chance to append to `failures`. Add `"go/token"` to this file's import block (needed for `token.NewFileSet()`). Update the file-header comment (lines 1-6, already edited by Card 2 for the known-tags list) to add one sentence noting the guard now also flags untagged files containing a long literal `time.Sleep` (see `cmd/lyx/tiersleep_test.go`).
- **Commit:** `feat(cmd/lyx): wire the literal time.Sleep(>=1s) guard into TestTierPurity_UntaggedTestsSpawnNothing`

### Card 5: Update CONSTRAINTS.md's Test Tier Purity Invariant for the known-tags list and the new Sleep guard

- **Context:**
  - `cmd/lyx/tierpurity_test.go`
  - `cmd/lyx/tiersleep_test.go`
- **Edits:**
  - `CONSTRAINTS.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In the `## Test Tier Purity Invariant` section's **Statement** bullet, change the phrase "constraint mentioning `integration` or `smoke`" to "constraint mentioning any tag in `cmd/lyx/tierpurity_test.go`'s known-tags list (`integration`, `smoke`, `scout`)", so the invariant's authoritative prose matches Card 1's generalized guard instead of hardcoding two tags. Add one new bullet to the same section, after **Allowlist** and before **Enforced by**, titled "**Real-time-wait guard (additive).**", stating: an untagged test file containing a literal `time.Sleep(...)` call whose duration argument is a compile-time constant ≥ 1 second is also flagged, allowlisted the same way as the banned-spawn-token check (`allowedLongSleepers` in `cmd/lyx/tiersleep_test.go`); an argument shape the guard cannot resolve to a known constant (an unrecognized selector, an identifier with no in-file declaration, or a malformed numeric literal) is conservatively flagged too, forcing an explicit allowlist entry or a rename rather than silently passing; this check does not inspect `context.WithTimeout` ceilings or short bounded-retry constants, and is additive only — it would not have caught the historic `githubclient`/`webstercli` real-time-wait regressions (those were production-side hardcoded `const` timeouts a test could not override, not a test-side `Sleep` call).
- **Commit:** `docs(CONSTRAINTS): document the known-tags list and the new literal time.Sleep guard under Test Tier Purity Invariant`

## Batch Tests

`verify:` runs three focused `-run` filters against `cmd/lyx`: `TestTierPurity` (the full walk, including the new Sleep check, against the real repo tree — this is where the `internal/reedengine`/`internal/reedcli` allowlist entries get proven necessary, since without them this run would fail today), `TestIsTierTagged` (Card 1's new table-driven unit test), and `TestFindLongLiteralSleep` (Card 3's new table-driven unit test). All three are untagged, so no `-tags` flag is needed. This batch does not touch `internal/scoutengine`, so no scoutengine-scoped verification runs here.
