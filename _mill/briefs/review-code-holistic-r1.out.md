No test even exercises the shuttle-suite path, confirming this gap slipped through untested. Now composing the final review.

MILL_REVIEW_BEGIN
# Review: Build internal/shuttle: one LLM agent via a swappable engine — holistic

```yaml
verdict: REQUEST_CHANGES
reviewer_model: sonnethigh
reviewed_file: plan/ + source
date: 2026-07-05
```

## Findings

### [BLOCKING] sandbox-shuttle-suite.cmd invokes a subcommand that does not exist
**Location:** `sandbox-shuttle-suite.cmd:8` (`go run ./tools/sandbox -parent C:\Code shuttle-suite %*`)
**Issue:** `tools/sandbox/main.go`'s subcommand switch has no `"shuttle-suite"` case (only `""`, `"build"`, `"suite"`, `"mux-suite"`, `"fetch"`), and `tools/sandbox/suite.go` defines no `shuttleSuite` spec or `//go:embed` for `SANDBOX-SHUTTLE-SUITE.md` — the mirror of `mainSuite`/`muxSuite` that `runSuite` needs. Running the launcher hits the `default:` branch and exits 1 with "unknown subcommand" before ever touching the new suite file; `SANDBOX-SHUTTLE-SUITE.md` is unreachable through any wired path. `tools/sandbox/suite_test.go` also has no case for it, so this gap is untested. Root cause: card 21 (`05-cli-and-registration.md`) lists `tools/sandbox/main.go`/`suite.go` only as unedited **Context**, never as **Edits**, so the plan itself never asked for the wiring `mux-suite`'s precedent required.
**Fix:** Add a `shuttleSuite` spec (embed `SANDBOX-SHUTTLE-SUITE.md`, default instruction) to `suite.go` and a `"shuttle-suite"` case to `main.go`'s switch (mirroring the `mux-suite` block exactly), plus a `main_test.go`/`suite_test.go` case proving the subcommand dispatches — otherwise the whole point of card 21 (an operator-runnable sandbox suite for shuttle) is a dead launcher.

### [NIT] ParseEvents' Raw carries the trimmed line, not the exact original bytes
**Location:** `internal/shuttleengine/claudeengine/events.go:51` (`Raw: []byte(trimmed)`)
**Issue:** Card 12 specifies `Raw: &lt;line bytes&gt;`; the implementation trims leading/trailing whitespace before storing, so a line with incidental surrounding whitespace would not round-trip byte-for-byte. Harmless in practice (events.jsonl lines are hook-generated JSON with no such whitespace) and the fixture test only asserts against already-trimmed fixture content.
**Fix:** If byte-exact round-trip ever matters to a caller, store the raw (untrimmed) line slice and use a separate trimmed copy only for the blank/parse checks.

## Verdict

REQUEST_CHANGES
Wire `tools/sandbox`'s `shuttle-suite` subcommand; the new sandbox launcher is otherwise non-functional.
MILL_REVIEW_END
