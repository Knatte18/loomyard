# Codex as assistant Code-Review reviewer: verdict from 14 landed rounds

Read of the archived records of 12 landed darn runs (2026-11-06 to 2026-11-17), from the weft repository's tags `refs/tags/archive/<slug>/<sha>`.
Each Codex review (`astra[medium]`, `gpt-6-astra`) was read beside the same round's Chair review, lens reviews and fixer report, and each finding was checked against the pre-fix and landed trees.
No review was rerun and nothing was launched.

## Verdict proposal

Keep Codex on every darn that runs on Neve, and keep it non-gating.
The verdict itself is the operator's.

- Over 14 rounds it filed 29 real findings and 2 false ones, and 14 of the real ones no Claude seat had first.
  Two of those were BLOCKING-grade finds the Chair kept at their level: the detached hub stop started outside the prime's anchor (`hub-stop-all`), and a repeated flag-name literal against `PATTERN-code-conventions` (`machine-start-launchers`, round 2).
- The records show no added wait: Codex ran in parallel with the Chair, and no round shows the Chair waiting on it.
  The one wait on record is on OSL-1046, outside this sample, where its sandbox failed.
- It is no stand-in for the Claude fan: it missed 34 Chair-kept real findings that Claude seats filed, among them four MEDIUM findings that were the round's main defects.
  It is a second opinion beside the fan, not a replacement.
- Its severity runs high: it filed BLOCKING for five findings the Chair lowered to MEDIUM or LOW, and the Chair rejected two of its redesign proposals.
  The Chair's judgement, not Codex's label, decides the round, which is how the seat is already wired.

## Per round

Real and false are Codex's findings checked against the tree.
Unique is real and raised by no Claude seat before it.
Missed is Chair-kept real findings of Claude seats that Codex did not file.

| Round | Codex verdict | Real | False | Unique | Missed | Chair on Codex's findings | Fan and seat |
|---|---|---|---|---|---|---|---|
| `assist-fan-seat-work-alone` | BLOCKING | 2 | 0 | 0 | 4 | F1 lowered BLOCKING to MEDIUM and its fix rejected; F2 lowered MEDIUM to LOW; both fixed | fan asked, seat worked alone (the conflict this run fixed) |
| `begin-batch-wrong-conflict-kind` | BLOCKING | 1 | 0 | 1 | 2 | lowered BLOCKING to LOW; fixed in the landing | no fan asked; gate lock read-only, no code finding lost |
| `codex-assist-fan-trial` | APPROVED, 2 LOW | 2 | 0 | 2 | 1 | both kept LOW and fixed; no lens seats, so the Chair alone confirmed | fan asked, seat worked alone |
| `motif-before-tapestry` | APPROVED | 0 | 0 | 0 | 1 | nothing to judge | no fan asked |
| `reed-assist-pane-width` | BLOCKING | 1 | 0 | 1 | 2 | kept BLOCKING (a test registered on Windows against `PATTERN-test-speed`); fixed | no fan recorded; gate not runnable |
| `slack-forward-prefix` | APPROVED | 0 | 0 | 0 | 1 | nothing to judge; it missed the empty-body refusal | no fan recorded; gate slot not acquired |
| `board-prints-root-beam` | MEDIUM | 2 | 0 | 0 | 2 | both kept and fixed, both also found by a lens | fan recorded; no tests run |
| `codex-edit-loop-recorder` | MEDIUM | 1 | 0 | 1 | 6 | kept, lowered to LOW; test added | fan recorded; gate slot not acquired |
| `hub-stop-all` r1 | BLOCKING | 4 | 1 | 2 | 5 | F1 kept BLOCKING and fixed; F2 BLOCKING redesign rejected but its false doc claim kept MEDIUM and fixed; scope finding dropped | fan recorded; gate slot read-only |
| `hub-stop-all` r2 | APPROVED | 0 | 0 | 0 | 0 | all nine prior statuses matched the Chair's | solo Chair; gate slot read-only |
| `machine-start-launchers` r1 | BLOCKING | 9 | 0 | 4 | 6 | three BLOCKING kept (one lowered to LOW); MEDIUMs kept or lowered; one half dropped | fan recorded; built both targets, ran no tests |
| `machine-start-launchers` r2 | BLOCKING | 1 | 1 | 1 | 0 | F15 kept BLOCKING and fixed; F16 rejected | solo Chair; gate slot read-only |
| `redundant-literal-clause-mutants` | BLOCKING | 6 | 0 | 2 | 3 | two BLOCKING lowered to MEDIUM; one dropped as a deliberate trade-off; others kept | fan ran; no coverage map for its redundancy gate |
| `stale-driver-new-stencils` | APPROVED | 0 | 0 | 0 | 1 | nothing to judge; it called the docs correct where the Chair found them stale | fan recorded; gate not runnable |

Round tallies are taken per round, so `redundant-literal-clause-mutants` counts its uncertain shadow-suppression finding as real, the same limit the Chair dropped as a trade-off; without it the totals are 28 real and 13 unique.

## Totals

- Unique real findings: 14, from 8 of the 14 rounds.
  In `codex-assist-fan-trial` there was no lens seat, so "unique" there means only the Chair had them too.
  In `begin-batch-wrong-conflict-kind` three lens seats saw the literals and chose not to file them.
- Real findings in all: 29.
- False findings: 2, both judged false by the Chair's reasoning, not by a plain error: the `EndRole` against `EndSubject` divergence in `hub-stop-all` round 1, and the lock outside `.lym` in `machine-start-launchers` round 2.
  Each points at a real text divergence the Chair held to be no defect.
- Claude real findings Codex missed: 34.
- Rounds where Codex approved clean: 4, three of which held a Chair-kept defect Codex did not see (`slack-forward-prefix`, `stale-driver-new-stencils`, `motif-before-tapestry`), one of which found none to see (`hub-stop-all` round 2).

## Added wait and tokens

- Wall clock: not recorded.
  `round-<n>-usage.yaml` holds the assistant's engine, model and effort only, and no archived session row has `engine: codex`.
  The Chair's review window ends 16 to 17 s before the round's stamp in every round read, so a Codex run of minutes ran inside that window or beside it, and no round shows the Chair waiting for the end marker.
  Earlier direct measures from the trial note stand: 212 s to 266 s per solo or fanned run, in parallel with the Chair.
- Tokens: not recorded for any round.
  The trial note's two measured runs are the only figures: about 126,000 fresh input plus output tokens fanned, and 1,326,656 in all with 1,223,680 cached, solo.
- Billing: the ChatGPT plan's weekly window, no API billing.

## Fan and seat failures

- Rounds before the fix of `assist-fan-seat-work-alone` (`assist-fan-seat-work-alone`, `codex-assist-fan-trial`): the seat was asked to fan and worked alone after reporting an instruction conflict.
- Rounds after it: every usage record that carries `assist_ending` has `done`, none carries `assist_lenses_missing`, and each assistant report says its lens reports were saved.
  The seat's own lens files are not in the archive, so the fan is inferred from the report text and the `lens:*` origins, not seen.
- Sandbox: in at least seven rounds the seat could not take the gate slot or run the gate (read-only runner lock under `_bolt/.lym`), and said so without claiming a test result.
  No round lost its review, and no end marker went missing.
  The seat never ran a test in the archive, so its findings are reading findings.
- Independence: in `board-prints-root-beam` and `codex-edit-loop-recorder` the Codex report carries lens `origin:` fields and says the four lenses found nothing else, so it may have read the Claude lens reports before it wrote.
  Uncertain; if so, its overlap with the lenses in fanned rounds says less about its independence.

## What the data cannot say

- Time and tokens per round, as above.
- Whether Codex finds more where Claude has no fan: the three rounds with a solo Chair (`hub-stop-all` round 2, `machine-start-launchers` round 2, `codex-assist-fan-trial`) give 3 real unique findings, too few to separate from chance.
- Real or false is a reading against the code by the session that wrote this report, with the Chair's call as a cross-check; the uncertain calls are named above.
