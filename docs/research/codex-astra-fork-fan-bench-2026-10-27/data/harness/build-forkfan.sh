#!/usr/bin/env bash
# Stage the fork-fan handler prompt over an input that build.sh and build-fan.sh already built: build-forkfan.sh <id>
# Writes work/.bench/instr/step-2-review-forkfan.md and fan/handler-fork.prompt.md.
# The cluster rules are burler-cluster-fan.md with its Agent-tool wording turned into Codex's spawn_agent with fork_turns="all";
# the boilerplate keeps the inherited-context sentence and the tool-call cap.
set -euo pipefail
ROOT=/home/knatte/Code/loomyard-LYMHUB/codex-astra-fork-fan-bench
id=$1
code=$(grep -o 'code=[0-9a-f]*' "$ROOT/.scratch/bench/inputs/$id/meta.txt" | cut -d= -f2)
in=$ROOT/.scratch/bench/inputs/$id
W=$in/work
fan=$in/fan
LENSES="affected-set guard-reach way-forward"

strip() { local s; s=$(<"$1"); [ "${s:0:4}" = '<!--' ] && s=${s#*-->}; printf '%s\n' "${s#"${s%%[![:space:]]*}"}"; }
fill() { local t=$1; shift; for kv in "$@"; do t=${t//"{{.${kv%%=*}}}"/"${kv#*=}"}; done; printf '%s' "$t"; }

git -C $ROOT show $code:contracts/stencils/burler/burler-cluster-fan.md > "$fan/cluster-fan.raw"
cluster=$(strip "$fan/cluster-fan.raw")
lens_list=""
for l in $LENSES; do lens_list+="- $l: $(cat "$fan/lens-$l.text")"$'\n'; done
cluster=$(fill "$cluster" "lens_list=$lens_list")
# Claude's spawn mechanics become Codex's.
cluster=${cluster//'via the Agent tool with `subagent_type: "fork"`, one per lens listed here — never pass a `name` (named forks silently lose inherited context).'/'by calling your `spawn_agent` tool once per lens listed here, so the forks run in parallel, each with `fork_turns` set to `"all"` so it inherits your context; never pass a name, a model or an effort for a fork, so each fork keeps your own.'}
cluster=${cluster//'and never call the Agent tool.'/'and never spawn another agent.'}
cluster=${cluster//'run only git commands that read, such as `git diff`, `git log` or `git show`, and never one that changes the repository, its index, its refs or its working tree,'/'run only commands that read,'}
cluster=${cluster//'The consolidated review is the ONE review file'/'Wait for every fork with your wait tool before you consolidate.'$'\n''The consolidated review is the ONE review file'}
if grep -q 'Agent tool\|subagent_type' <<<"$cluster"; then echo "unconverted Agent wording" >&2; exit 1; fi

git -C $ROOT show $code:contracts/stencils/burler/burler-step-review.md > "$fan/step-review.raw"
review=$(strip "$fan/step-review.raw")
review_path=$W/.bench/out/review.md
review=$(fill "$review" "cluster_rules=$cluster" "review_path=$review_path" \
  "review_format_path=$W/.bench/instr/review-format.md" "prior_rounds=This is the first round — no prior round files exist.")
printf '%s\n' "$review" > "$W/.bench/instr/step-2-review-forkfan.md"

git -C $ROOT show $code:contracts/stencils/burler/burler-template-reviewer.md > "$fan/reviewer.raw"
prompt=$(strip "$fan/reviewer.raw")
prompt=$(fill "$prompt" "parent_directive=" "edit_directive=" "review_path=$review_path" \
  "instruction_1_path=$W/.bench/instr/step-1-explore-fan.md" "instruction_2_path=$W/.bench/instr/step-2-review-forkfan.md")
printf '%s\n' "$prompt" > "$fan/handler-fork.prompt.md"
if grep -l '{{' "$fan/handler-fork.prompt.md" "$W/.bench/instr/step-2-review-forkfan.md"; then echo "leftover placeholder" >&2; exit 1; fi
echo ok
