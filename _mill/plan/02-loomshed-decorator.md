# Batch: loomshed DiscussionWrite commit decorator

```yaml
task: 'loom: Discussion-Write producer'
batch: 'loomshed DiscussionWrite commit decorator'
number: 2
cards: 3
verify: go test ./internal/loomshed/...
depends-on: []
```

## Batch Scope

This batch delivers the one genuinely new behaviour in the task: a thin `shedengine.ShedProducer` decorator that delegates to a wrapped producer and, on a `Done` outcome with a nil error, invokes an injected commit closure.
It is written test-first — the decorator's whole contract is outcome mapping, so a table test against a fake inner producer covers it exhaustively without any real git, filesystem, or agent involvement.

The external interface batch 3 consumes is one exported constructor: `loomshed.NewDiscussionWrite(name string, inner shedengine.ShedProducer, commit func() error) shedengine.ShedProducer`.

This batch is independent of batch 1 and can run in parallel with it.
It adds no import to `internal/loomshed` beyond `context` and `github.com/Knatte18/loomyard/internal/shedengine`, both of which the package's own `seam_enforcement_test.go` allowlist already covers, so that allowlist needs no change.

Batch-local decision: the decorator does not consult `entryErr` itself.
The wrapped `SingleLLMProducer` already entry-checks the context as its first act, so a second check here would be duplicate work at the same seam;
what the decorator must guarantee instead is that it never invokes `commit` for any outcome other than `Done` with a nil error, which is what makes a cancelled or errored inner call leave the weft untouched.

## Cards

### Card 6: Write the DiscussionWrite decorator test

- **Context:**
  - `internal/loomshed/discussionvalidate_test.go`
  - `internal/loomshed/stub_test.go`
  - `internal/loomshed/ctx.go`
  - `internal/shedengine/shed.go`
- **Edits:** none
- **Creates:**
  - `internal/loomshed/discussionwrite_test.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/loomshed/discussionwrite_test.go` as an untagged Tier-1 table test for `NewDiscussionWrite`, written before the production file so the first run fails to compile.
  Declare a fake inner `shedengine.ShedProducer` in this file whose `Call` returns a caller-settable `shedengine.Outcome`, `shedengine.OutputPointer`, and `error`, and which records how many times it was called.
  Declare a commit-closure recorder that counts invocations and returns a caller-settable error.
  Cover these behaviours, each as its own table row or subtest:
  the inner producer reporting `Done` with a non-empty `OutputPointer` and a nil error invokes `commit` exactly once and returns that same outcome and pointer through unchanged;
  the inner producer reporting `Stuck` leaves `commit` uninvoked and returns `Stuck` with an empty pointer;
  the inner producer returning a non-nil error leaves `commit` uninvoked and surfaces that error unchanged;
  and a `commit` that returns an error surfaces as a returned non-nil error with an empty `shedengine.Outcome`, never as `Stuck`.
  Add one further subtest asserting the decorator wraps the commit error with enough context to name the producer, so an operator reading a `StateFailed` status can tell which row's commit failed.
  Every test in this file uses `context.Background()` and touches no filesystem.
- **Commit:** `test(loomshed): table test for the DiscussionWrite commit decorator`

### Card 7: Implement the DiscussionWrite decorator

- **Context:**
  - `internal/loomshed/discussionvalidate.go`
  - `internal/loomshed/webster.go`
  - `internal/loomshed/ctx.go`
  - `internal/shedengine/shed.go`
  - `internal/shedadapters/singlellm.go`
- **Edits:** none
- **Creates:**
  - `internal/loomshed/discussionwrite.go`
- **Deletes:** none
- **Moves:** none
- **Requirements:** Create `internal/loomshed/discussionwrite.go` declaring an unexported `discussionWrite` struct with three fields — `name string`, `inner shedengine.ShedProducer`, and `commit func() error` — plus a `var _ shedengine.ShedProducer = (*discussionWrite)(nil)` compile-time assertion, mirroring `discussionvalidate.go`'s own file shape.
  Export `NewDiscussionWrite(name string, inner shedengine.ShedProducer, commit func() error) shedengine.ShedProducer`, returning the seam interface so `internal/shedrecipe` can reach it while the concrete type stays unexported — the same reason `NewDiscussionValidate` states in its own doc comment.
  Implement `Call(ctx context.Context) (shedengine.Outcome, shedengine.OutputPointer, error)` as a delegation: call `p.inner.Call(ctx)` once, return its three results verbatim whenever the error is non-nil or the outcome is anything other than `shedengine.Done`, and otherwise invoke `p.commit()` before returning.
  A non-nil `commit` error returns an empty `shedengine.Outcome`, an empty `shedengine.OutputPointer`, and the wrapped error — never `shedengine.Stuck`.
  Do not call `entryErr` or `cancelErr` here; the wrapped producer owns the cancellation obligation and this file's doc comment must say so explicitly.
  Write the file-header comment and the doc comments in the style the surrounding package uses: state what the decorator adds and, for the commit-error mapping, why an error rather than `Stuck` — a git fault is not something re-writing the discussion can fix, the same reasoning `discussionvalidate.go` applies to a non-not-exist read failure.
  Also record in the doc comment that the commit fires before `Discussion-Validate` has judged the output, and that this is intentional: the commit keeps the weft clean and the artifact durable, it does not certify it.
- **Commit:** `feat(loomshed): add the DiscussionWrite commit decorator`

### Card 8: Correct the stub and package doc comments

- **Context:**
  - `internal/loomshed/loomshed.go`
  - `internal/loomshed/discussionwrite.go`
- **Edits:**
  - `internal/loomshed/stub.go`
  - `internal/loomshed/doc.go`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:** In `internal/loomshed/stub.go`, correct `stubProducer`'s doc comment, which currently claims the stub backs five rows and names them: `Discussion-Write`, `Discussion-Review`, `Plan-Write`, `Plan-Review`, and `Webster-Review`.
  Once this task lands, `Discussion-Write` is a real producer, so the count becomes four and `Discussion-Write` drops out of the named list.
  Also correct the file-header comment's "every row of loom's 13-row producer list this task does not build for real" phrasing so it no longer implies `Discussion-Write` is among them.
  Do not change `NewStub`, `stubProducer`'s fields, or its `Call` — four rows still use it.
  In `internal/loomshed/doc.go`, correct the package doc's opening claim that the package owns "six producer constructors": `NewDiscussionWrite` makes seven.
  Leave the rest of the package doc — the thirteen durable row names, the status seeder, the told-paths statement, and the whole cancellation-helper-duplication paragraph — unchanged.
- **Commit:** `docs(loomshed): correct the stub's row list and the package's constructor count`

## Batch Tests

`verify: go test ./internal/loomshed/...` runs the whole `internal/loomshed` package, which is the only package this batch touches.
The new `discussionwrite_test.go` is the batch's own coverage: it drives `NewDiscussionWrite` against a fake inner producer across every outcome the `shedengine.ShedProducer` contract admits, plus the commit-failure case, so the decorator's whole behaviour is pinned without a real git repo, a real shuttle, or a real filesystem.
The same run also re-executes `seam_enforcement_test.go`, whose import allowlist proves the new file drags no new dependency into the package, and `stub_test.go`, which card 8's doc-comment edits must leave passing.
Card 6 is written and run before card 7 so the first execution genuinely fails to compile, per the batch's test-first framing.
