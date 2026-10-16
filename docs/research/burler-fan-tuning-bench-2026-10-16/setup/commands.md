# Bench commands

Pair: `lyx fabric add twb-fan-rv` from the prime, then `git -C /home/knatte/Code/loomyard-LYXHUB/twb-fan-rv reset --hard fd5b0f58e` (main at reed-view's pair creation, 2026-10-08 08:06).

Reference inputs: weft `f078e6da6` (`_lyx/discussion/decision-record.md`, `support-log.md`: round 1's target), weft tag `archive/reed-view/e3737041815f` (rounds 1-5), Bolt `6fcfd0d7d` (`design-reed-view.md`: the board entry at round 1).

Per arm, in the pair, `.scratch/fan/arm/` holds fresh copies of the two target files, `profile.yaml` (`profile-solo.yaml` or `profile-fan.yaml`) and `stencils/`: a full copy of `/home/knatte/Code/loomyard-LYXHUB/_board/_lyx/stencils` with the arm's overlay (`solo-explore.diff`; or `iter1/burler-step-1-explore.md` and `iter1/burler-step-review.md`).
The pair-layer `_lyx/config/burler.yaml` (`iter1/burler.yaml`) defines the fan `fan-bench`; no hub file was changed.
Shared inputs in `.scratch/fan/inputs/`: `board-entry.md`, `focus.md` (round 1's focus file).

```
cd /home/knatte/Code/loomyard-LYXHUB/twb-fan-rv && lyx burler run --profile .scratch/fan/arm/profile.yaml \
  --stencils-dir /home/knatte/Code/loomyard-LYXHUB/twb-fan-rv/.scratch/fan/arm/stencils \
  --review-model opus --review-effort medium --fix-model haiku --fix-effort low
```

After each run the arm directory was moved out of the pair into `runs/<arm>/`, so no later arm could see it.
Fork final messages were taken from `~/.claude/projects/-home-knatte-Code-loomyard-LYXHUB-twb-fan-rv/<review session>/subagents/*.jsonl`.
Cost: `jq -s -f cost.jq <transcript>.jsonl` per transcript file.
Graders: `lyx reed add --anchor own-window --role fan-grader1 --cmd "claude --model opus --effort high --permission-mode bypassPermissions 'Read <prompt> and do exactly what it says.'"` from the pair.
