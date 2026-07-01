# Batch: sandbox-suite-doc-hardening

```yaml
task: Fix lyx CLI defects + host-commit gap from the sandbox run
batch: sandbox-suite-doc-hardening
number: 3
cards: 1
verify: null
depends-on: []
```

## Batch Scope

Hardens `tools/sandbox/SANDBOX-SUITE.md` so future sandbox runs do not re-file the two
declined findings from this task (#36 points 1 & 2, and #38). Scenario S2 already
carried language acknowledging that raw-git host commits are acceptable, but that
didn't stop #38 from being filed as an enhancement suggestion — S2 needs to say
explicitly that the absence of host-commit tooling is intentional and should not be
re-filed. Scenario S6's current wording ("Does lyx say what to do, or just fail?")
invited the #36 points 1/2 framing (JSON-shaped errors read as "not saying what to
do") — S6 needs to say explicitly that the JSON error envelope is the deliberate
machine-parseable contract, while still preserving genuine findings (a leaked raw
subprocess line, which Batch 1 fixes, remains a legitimate finding if it ever
regresses). This is a pure documentation batch — no Go code, no runnable verification
surface, `verify: null`.

External interface for later batches: none. Independent of Batch 1 and Batch 2.

## Cards

### Card 9: Tighten SANDBOX-SUITE.md scenarios S2 and S6

- **Context:** none
- **Edits:**
  - `tools/sandbox/SANDBOX-SUITE.md`
- **Creates:** none
- **Deletes:** none
- **Moves:** none
- **Requirements:**
  - In scenario **S2 — First real work in the host**, the `**Watch:**` line currently
    reads:
    ```
    **Watch:** The host is an ordinary git repo — committing host changes with plain
    `git` is acceptable and **not** a finding. Watch lyx's actual responsibility:
    host/weft coordination (junctions wired correctly, weft mirroring behaves).
    ```
    Extend it with an explicit, unambiguous statement that the absence of a lyx-owned
    host-commit command is an intentional design choice, not a gap — do not file it as
    an enhancement suggestion. Preserve the existing sentence about lyx's actual
    responsibility (junction wiring, weft mirroring) unchanged; add the new sentence
    after it.
  - In scenario **S6 — Wrong-directory and error ergonomics**, the `**Watch:**` line
    currently reads:
    ```
    **Watch:** Are errors legible? Does lyx say what to do, or just fail? This is where
    standalone usability lives or dies. A legible `not initialized` / "run from the
    initialized root"-style message is the `OK` (ergonomics-pass) outcome — not a
    `FAIL`. Do not file it as a finding.
    ```
    Add an explicit statement that `lyx`'s error output is a JSON envelope
    (`{"ok":false,"error":"..."}`) on every error path by design — that is the
    deliberate machine-parseable contract, not a defect, and "legible" means the
    `error` field's message text clearly identifies the problem, not that the output
    reads as human prose with a hint or usage suggestion. In the same edit, explicitly
    preserve the counter-case: a raw subprocess/tool string leaking unwrapped into the
    `error` field (e.g. a bare git `fatal:` line, or any other tool's raw stderr) is
    still a legitimate `WARN`/`FAIL` finding — the JSON-shape acceptance does not cover
    unwrapped internals leaking through it. Leave the existing sentences about
    `not initialized` / "run from the initialized root" being the `OK` outcome
    unchanged.
  - Do not edit any other scenario (S0, S1, S3, S4) or any other section of the file.
- **Commit:** `docs(sandbox): harden S2/S6 against re-filing declined findings`

## Batch Tests

`verify: null` — this is a pure documentation batch with no runnable surface. The
sandbox suite itself is agent-driven, not scripted (per the file's own "What this is"
section), so there is no automated test to run. Correctness is reviewed by reading the
diff against the two `**Watch:**` blocks specified above.
