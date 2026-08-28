# Batch: github-caller-warn-lines

```yaml
task: "Audit internal/logger coverage across spawn/hard-error paths"
batch: "github-caller-warn-lines"
number: 4
cards: 2
verify: go test ./internal/selfreportengine/ ./internal/landingshed/
depends-on: [1]
```

## Batch Scope

This batch delivers the compensation the `githubclient-leaf` decision owes: `internal/githubclient` cannot import `internal/logger` — its own `leaf_enforcement_test.go` enforces a three-entry allowlist — so GitHub failures are logged one layer up, in both of its production callers.
`grep -rln 'internal/githubclient"'` over non-test files returns exactly `internal/selfreportengine/selfreport.go` and `internal/landingshed/publish.go`; neither is skipped, and `publish.go` is the one carrying Publish's own GitHub calls, which is the gap the brief originally named.
It is one batch because both cards add the same shape of line — a `logger.Warn` carrying the operation, the target repo, and the wrapped error — against the same three-class error taxonomy the two files already share.

Batches 3 and 5 consume nothing from this batch; it is parallel to them under the DAG root.
Neither file in this batch is a spawn site, so batch 5's guard never walks either for a `logger` import.

Batch-local decision differing from `## Shared Decisions`: none beyond the `publish-warn-overlaps-reportstuck` decision recorded in the overview, which applies to card 13 only.

## Cards

### Card 12: Warn on GitHub failures in selfreportengine

- **Context:**
  - `internal/githubclient/token.go`
  - `internal/githubclient/leaf_enforcement_test.go`
  - `internal/loomshed/gatefindings_test.go`
  - `internal/logger/logger.go`
- **Edits:**
  - `internal/selfreportengine/selfreport.go`
  - `internal/selfreportengine/selfreport_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/selfreportengine/selfreport.go`, add the import `github.com/Knatte18/loomyard/internal/logger` and add a `logger.Warn` to each of `CreateIssue`'s two failure sites, in both cases placed immediately BEFORE the existing `return "", 0, fmt.Errorf(...)`:

  - the `client, err := NewGitHubClient()` failure branch — carries `action` of `"new github client"` and `cause`;
  - the `client.Issues.Create` failure branch — one `Warn` covering all three of its classified returns (`githubclient.ErrTokenUnresolvable`, `*github.ErrorResponse`, and the catch-all), carrying `action` of `"create issue"`, `owner`, `repo`, and `cause`. Place it once, before the `errors.Is` classification, so a single line covers every class rather than three near-identical lines.

  Both messages are package-prefixed.
  Do not change `CreateIssue`'s signature, its `NewGitHubClient` seam, its `targetRepo` splitting, its `context.WithTimeout` bound, or any of its returned error texts — the three-class classification stays exactly as it is.

  In `internal/selfreportengine/selfreport_test.go`, extend the existing failure-path cases to assert that a failing `Issues.Create` produces a captured `WARN` line containing the `action`, `owner`, `repo`, and `cause` field keys, and that a failing `NewGitHubClient` seam produces one containing `action` and `cause`.
  Use the inline capture pattern from the `test-log-capture-pattern` shared decision; `Warn` is the default threshold, so no `logger.SetVerbosity` call is needed.
  Assert on field keys and the level token, never on an exact rendered line.
- **Commit:** `feat(selfreportengine): warn on github client and issue-create failures`

### Card 13: Warn on GitHub failures in landingshed Publish

- **Context:**
  - `internal/landingshed/stuck.go`
  - `internal/landingshed/seam_enforcement_test.go`
  - `internal/githubclient/token.go`
  - `internal/loomshed/gatefindings_test.go`
  - `internal/logger/logger.go`
- **Edits:**
  - `internal/landingshed/publish.go`
  - `internal/landingshed/publish_test.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  In `internal/landingshed/publish.go`, add the import `github.com/Knatte18/loomyard/internal/logger` — the package already imports it in `stuck.go`, and `landingshedAllowedImports` in `internal/landingshed/seam_enforcement_test.go` already admits it, so no allowlist edit is needed; confirm that by reading the map rather than assuming — and add a `logger.Warn` to each of `(*Publish).Call`'s three GitHub failure sites, in every case placed immediately BEFORE the existing `return p.stuckOrCancelled(...)`:

  - the `client, err := NewGitHubClient()` failure branch — carries `producer` (`publishName`), `action` of `"new github client"`, and `cause`;
  - the `client.PullRequests.List` failure branch — carries `producer`, `action` of `"query existing pull request"`, `owner`, `repo`, and `cause`;
  - the `client.PullRequests.Create` failure branch — carries `producer`, `action` of `"create pull request"`, `owner`, `repo`, and `cause`.

  Each message is package-prefixed.
  These lines are added even though `stuckOrCancelled` already reaches `reportStuck`, which emits its own `logger.Warn("landingshed: producer stuck", …)` in `stuck.go`: that line carries the classified reason as one prose string and no operation-level structure, whereas these carry `action`, `owner`, `repo`, and `cause` as separate greppable fields.
  The duplication is deliberate — see the `publish-warn-overlaps-reportstuck` shared decision.
  Do not fold the new lines into `reportStuck`, do not remove or reword its existing line, and do not change `publishGitHubErrorReason`'s three-class classification or any reason string it produces.
  Do not change `(*Publish).Call`'s signature, its step ordering, or any return value.

  In `internal/landingshed/publish_test.go`, extend the existing GitHub-failure cases to assert that each of the three sites produces a captured `WARN` line carrying its own `action` value together with the `cause` field key, and that the `List` and `Create` lines additionally carry `owner` and `repo`.
  Assert the reason-classification behaviour is unchanged by keeping the existing assertions on the recorded stuck reason intact.
  Use the inline capture pattern from the `test-log-capture-pattern` shared decision.
- **Commit:** `feat(landingshed): warn on github failures in publish`

## Batch Tests

`verify: go test ./internal/selfreportengine/ ./internal/landingshed/` runs the two affected packages' untagged suites — exactly the packages this batch's `Edits:` touch, no wider.

What each run covers:

- `./internal/selfreportengine/` runs card 12's extended cases in `internal/selfreportengine/selfreport_test.go`. The package's `NewGitHubClient` seam is a package-level `var`, so both failure sites are reachable from an in-package test by swapping it: a factory that returns an error drives the first, and a client whose `Issues.Create` fails drives the second. The package has no import-allowlist test of its own, so the new `logger` import carries no enforcement risk here.
- `./internal/landingshed/` runs card 13's extended cases in `internal/landingshed/publish_test.go` — which is `package landingshed`, in-package, so it can drive `NewGitHubClient` the same way — and, importantly, `TestToldGeometryInvariant_AllowlistOnly` in `internal/landingshed/seam_enforcement_test.go`. That test already admits `internal/logger`, so it is expected to stay green; it runs here so a mistaken import (of `githubclient`'s own internals, say, while chasing a field) fails at this batch rather than at the repo-wide gate.

Both cards are TDD candidates: every failure site is behind a swappable seam, so the `WARN` assertions can be written and observed to fail before the log calls exist.

Neither file in this batch contains an `exec.Command` call, so batch 5's guard is indifferent to both — this batch closes an observability gap the guard structurally cannot see, which is precisely why the audit document records it in prose under Structural blocks rather than as an enforced row.

The module-wide `verify:` in the overview (`go build ./... && GOOS=windows go build ./...`) runs at the batch boundary and catches cross-package fallout from the new `logger` import in `publish.go`.
