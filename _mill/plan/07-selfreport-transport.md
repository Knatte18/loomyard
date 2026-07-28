# Batch: selfreport-transport

```yaml
task: 'native clients: migrate gitrepo to go-git + selfreport gh-CLI to go-github'
batch: selfreport-transport
number: 7
cards: 5
verify: go test -race -count=1 ./internal/selfreportengine/... ./internal/selfreportcli/...
depends-on: [6]
```

## Batch Scope

Swaps `internal/selfreportengine`'s transport from the `gh` CLI to a go-github call through `githubclient`, and rewrites `internal/selfreportcli`'s tests and help text to match. `CreateIssue`'s signature, behaviour, and JSON output envelope are unchanged; the CLI's flags and arg validation are unchanged.

The package is **not** extended — the wider GitHub surface lives in `githubclient` and its consumers. `selfreportengine` keeps its one job and its hardcoded `targetRepo`.

Two things make this batch larger than "replace one call". First, `selfreportengine` has **no test files of its own** — every existing test lives in `selfreportcli/cli_test.go` and drives the exported `RunGH` variable directly, so deleting `RunGH` without a stated successor would delete the package's only seam. Second, the CLI's help text describes the `gh` CLI as the transport and states that it must be installed and authenticated; that prerequisite becomes false the moment this batch lands, and the CLI/Cobra Invariant's help-accuracy obligation makes rewriting it a required part of the change rather than a follow-up.

## Cards

### Card 31: Swap CreateIssue's transport

- **Context:**
  - `internal/githubclient/githubclient.go`
  - `internal/selfreportcli/cli.go`
- **Edits:**
  - `internal/selfreportengine/selfreport.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Replace the `gh`-CLI transport under `CreateIssue` with a go-github `Issues.Create` call. Delete `RunGH`, `realRunGH`, `buildCreateArgs`, and `lastNonEmptyLine`; `RunGH` has no production consumer at all, so removing it touches no shipping code path. Add the named successor seam: an **exported** package-level factory variable `var NewGitHubClient = githubclient.New` returning `(*github.Client, error)`, which `CreateIssue` calls instead of `RunGH`. Keep the `targetRepo` constant and pass owner and repo as parameters — `githubclient` resolves neither. `CreateIssue` keeps its context-free signature (the public surface must not change) and derives its own `context.WithTimeout` internally from the same 30 s budget the client carries. Its three documented error cases must survive in spirit: binary-not-found becomes token-not-resolvable, generic exec failure becomes transport/network failure, and a non-zero `gh` exit becomes a non-2xx GitHub response — keep them distinguishable, since a network failure and an API rejection call for different operator action. **Keep the zero-issue-number convention and the `if number != 0` gate in the CLI**: scope promises the output envelope does not change and the gate is what guarantees that structurally. With go-github the number arrives typed, so in practice a 201 always carries one and the omission branch becomes unreachable — record in `CreateIssue`'s godoc that the zero case is now defensive rather than an expected parse outcome, so a later reader does not mistake it for live behaviour.
- **Commit:** `refactor(selfreportengine): replace gh CLI transport with go-github`

### Card 32: Rewrite selfreportcli tests against httptest

- **Context:**
  - `internal/selfreportengine/selfreport.go`
  - `internal/githubclient/githubclient.go`
  - `internal/selfreportcli/cli.go`
- **Edits:**
  - `internal/selfreportcli/cli_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Replace the `RunGH`-fake tests with tests against an `httptest` server, injecting through `selfreportengine.NewGitHubClient` a closure that returns a real go-github client pointed at the test server's base URL. Injecting the whole **authenticated client** rather than only a base URL is deliberate: it guarantees no test ever reaches token resolution, so the suite never reads `GH_TOKEN`, never shells out to `gh auth token`, and never touches the operator's real credential cache — which also keeps it runnable on a machine with no `gh` installed and no GitHub credentials at all. Assert the request shape, which is where the risk in a REST client actually lives and which the old argv-slice assertions cannot cover because that shape ceases to exist: the method and path (`POST /repos/Knatte18/loomyard/issues`), that the JSON body carries the title, that the body field is present only when supplied and absent otherwise, and that multiple labels survive in order. Then map responses to behaviour: a 201 returns URL and number from the typed response; a 4xx/5xx surfaces as an error through the `output.Err` envelope; a network failure surfaces distinctly from an API rejection. Keep the existing cobra-level tests that assert arg-count rejection reaches no transport at all.
- **Commit:** `test(selfreportcli): drive tests through httptest instead of RunGH fake`

### Card 33: Rewrite selfreportcli help text

- **Context:**
  - `internal/selfreportengine/selfreport.go`
  - `CONSTRAINTS.md`
- **Edits:**
  - `internal/selfreportcli/cli.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Rewrite every piece of prose that names the `gh` CLI as the transport or states it as a prerequisite. The sites, exhaustively: the package doc; `Command`'s godoc, which says the command "talks to a hardcoded external service (gh CLI + Knatte18/loomyard)"; the `selfreport` parent `Short`; the `create` `Short` and `Long`, which state "The gh CLI must be installed and authenticated before running this command." That prerequisite becomes false — after this task the binary is needed only as a *fallback* token source, and not at all when `GH_TOKEN` or `GITHUB_TOKEN` is set. Also fix the two implementation comments that name deleted machinery: the one referencing `buildCreateArgs`, and the one describing "the gh output URL's trailing path segment", which no longer exists now that the number arrives typed. Any error message the CLI prints that names `gh` falls under the same obligation. Flags, arg validation, and the output envelope do not change. This is the CLI/Cobra Invariant's help-accuracy obligation, which makes stale help a review-blocking defect rather than a cosmetic one.
- **Commit:** `docs(selfreportcli): rewrite help text for the go-github transport`

### Card 34: Add selfreportengine's own tests

- **Context:**
  - `internal/selfreportengine/selfreport.go`
  - `internal/selfreportcli/cli_test.go`
  - `internal/githubclient/githubclient.go`
- **Edits:** none
- **Creates:**
  - `internal/selfreportengine/selfreport_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Give the package its own tests, which it has never had — the whole reason `RunGH` had to be exported was that every test lived one layer up in `selfreportcli`. Drive `CreateIssue` directly through the `NewGitHubClient` seam against an `httptest` server, covering the three error cases the transport swap remaps (token-not-resolvable, transport/network failure, non-2xx response) and asserting each stays distinguishable from the others. Add a case where the factory itself returns an error, since that is the new token-not-resolvable path and it must surface as a typed error rather than as a nil-client panic. Untagged Tier 1 — no git spawn, no process, no fixture tree.
- **Commit:** `test(selfreportengine): cover CreateIssue error contract directly`

### Card 35: Confirm the sandbox allowlist still holds

- **Context:**
  - `CONSTRAINTS.md`
  - `cmd/lyx/sandbox_coverage_test.go`
  - `internal/selfreportcli/cli.go`
- **Edits:** none
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** Verification-only card, no diff. Confirm that `selfreport`'s entry on the Sandbox Suite Coverage `excludedModules` allowlist still reads correctly after the transport swap. Its stated reason — "`create` files a real GitHub issue" — survives unchanged, because the module still files a real issue; only the transport moved. No allowlist edit is needed, and adding one would be wrong. Confirm also that no cobra module was added or removed by this batch, so the CLI/Cobra Invariant's registration, help-tree, and `root.Long` requirements are untouched — `githubclient` is a library package, not a module.
- **Commit:** none

## Batch Tests

`verify:` runs `go test -race -count=1 ./internal/selfreportengine/... ./internal/selfreportcli/...` — the two packages this batch touches, both untagged Tier 1.

Coverage moves rather than merely changing shape. The old tests asserted on an argv slice, which is a shape that no longer exists; the new ones run a real go-github client against an `httptest` server, so the request path, method, JSON body, and title/body/label encoding are actually exercised. That is the right trade: in a REST client the risk is request construction, so that is what the tests must cover.

Card 35 is verification-only and produces no diff — it exists because the temptation to "update" the sandbox allowlist after a transport change is real, and the correct action here is to leave it alone.
