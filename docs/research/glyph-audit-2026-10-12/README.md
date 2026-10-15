# Webster kick-start pack audit, 2026-10-12

Pack on: 35 implementer forks over 11 runs (150 cards); pack off: 113 forks over 43 runs (508 cards), 2026-10-08 to 2026-10-11; all on Sonnet.

- No measurable gain: calls before the first edit (median 6 on, 7 off), exploration calls per card (5.8 on, 5.9 off), turns and tokens are all within noise.
- The one consistent signal is about 20% fewer whole-file reads and greps, offset by more partial reads: pack spans replace whole-file reads rather than cutting exploration.
- Forks never used quarry lookups either way.
- Main confounder: forks inherit the Master's orientation, so a fork explores only about 6 calls before its first edit; most exploration is in the Master, which the pack does not reach.
  Pack quality also varies widely (1 to 144 resolved entries, up to 41 unresolved).

Verdict: the evidence did not justify building the Burler side of the pack.
The follow-up audits are `glyph-audit-2026-10-13` and `glyph-audit-2026-10-15`; `tools/forkaudit` makes the measurement repeatable.

## Files

`extract.jq` is the per-transcript extractor, `sessions.tsv` its collected output (one row per fork), and `stats.jq` the pack-on versus pack-off medians and means.
They are copies from the scratch directory `.scratch/kickstart-measure/`.

## Running

`jq -s -c -f extract.jq <transcript>` per fork transcript, concatenated; `jq -s -r --arg from <ISO timestamp> -f stats.jq <records>` for the comparison.
The transcript list and the `$from` value used are not recorded.
