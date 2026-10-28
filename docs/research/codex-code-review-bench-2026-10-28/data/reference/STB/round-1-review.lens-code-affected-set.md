```yaml
lens: code-affected-set
outcome: reported
```

Task: the code-affected-set lens, covering what the diff forces outside the lines it touches.

**Per change, the set found**

- **`LoopSpec.LoopTraceFile` (new field).** Two production constructors exist, `internal/loomcli/looparm.go:41` and `internal/battencli/arm.go:527`. Both set it, so this is covered. The only other readers are `loop.go` and the new test. A nil field is guarded.
- **Cause string `start-failed: <error>` plus the ETXTBSY suffix.**
  - Readers and asserters: `shedverbs/loop_test.go:702` and `:710`, and `shedverbs/doc.go:41`. These agree with the code.
  - The `beamcli` "start-failed" hits at `dobbytrace_integration_test.go:83` and `:119` are an unrelated prime-log line.
  - I found no stencil, spec or other doc that reads the cause string.
  - Result: the set is consistent with the code, but the code departs from decision D4. See F1.
- **`proc.IsSpawnBusy` (new).**
  - Documented in `proc/doc.go:22`.
  - Callers: `loopchild.go` and `loop.go`.
  - `internal/beamcli/dobbyreexec_integration_test.go:62` has its own inline `errors.Is(err, syscall.ETXTBSY)` retry. That is a test fixture, and the decision record puts every other spawn site out of scope. I raise nothing.
  - `syscall.ETXTBSY` is defined on Linux, Darwin and Windows in Go's `syscall` package, so I see no platform build break. I did not build it on each platform.
- **`tools/deploy` `installBinary`.**
  - The only consumer of the build is `run()`, which stats `dest` afterwards.
  - `update-plugins.sh`, `deploy-dev` and the Land row all go through `tools/deploy`. Only `tools/deploy/main.go:83` builds `lym` to a target. Covered.
  - The `doc.go` for `tools/deploy` is the header comment in `main.go`, and it was updated.
- **`tools/deploy` Windows path.** Per the decision record, a rename over a running `lym.exe` fails as `go build -o` did before. This is recorded and accepted.
- **Docs.**
  - `shedverbs/doc.go` and `proc/doc.go` are updated in the same commits.
  - PATTERN: `PATTERN-spawn-observability` is met. Each retry logs at Warn, and the attempt count is capped.
  - The Warn is not in a state-less loop, because it is bounded at four.
  - `PATTERN-test-speed` is met: the waits go through `LoopSpec.Sleep`, and I found no `Sleep` real-wait path in the test.
  - `PATTERN-lym-temp-root` is not triggered, because the temporary binary sits in the install directory as the record states.
  - No declared PATTERN changes exist, and no `Pattern-Departure:` trailers are used. Both commits carry the `Co-Authored-By: Claude Opus 5.5` trailer, but the system attribution reminder asks for Sonnet 5.5. I treat that as outside this lens.

**Findings**

- **F1 — MEDIUM, design.** The code does not make the cause D4 asks for. Location: `internal/shedverbs/loop.go:435-439`, `internal/shedverbs/doc.go:40-41`, `internal/shedverbs/loop_test.go:702` and `:710`.
  - Decision D4 and its acceptance criterion require the cause `start-failed after <n> tries: <error>`, with the ETXTBSY meaning added.
  - The code writes `start-failed: <error>`, and appends `; the executable stayed open ... across N tries` only for ETXTBSY. An ETXTBSY that succeeds on a later try never reaches this path.
  - A non-ETXTBSY failure never names tries. That is correct for a single try, but the form differs from the decided `after <n> tries` prefix.
  - The doc and the tests encode the code's form, not D4's.
  - Fix: make the cause read `start-failed after <n> tries: <error>`, passing the actual attempts used from `startChild`, and then append the ETXTBSY meaning. Update the doc and both test expectations to match.
- **F2 — LOW, consistency.** The `doc.go` sentence at `shedverbs/doc.go:40` says the retry applies to "the lym binary", but the retry in `startChild` applies to any child executable. This is minor wording drift and I raise no fix beyond rewording.

**Tool calls (1)**
1. Grepped the repo for `start-failed`, `LoopSpec{` constructors and `go build -o` sites, because my context lacked other readers of the cause string, other `LoopSpec` constructors and other build sites.
