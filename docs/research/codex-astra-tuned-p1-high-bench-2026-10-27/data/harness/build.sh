#!/usr/bin/env bash
# Stage one bench input: code snapshot, artifacts, era stencils and the assembled reviewer prompt.
# usage: build.sh <id> <kind: discussion|plan> <weft-commit> <code-commit>
set -euo pipefail
ROOT=/home/knatte/Code/loomyard-LYMHUB/codex-astra-tuned-p1-high-bench
WEFT=/home/knatte/Code/loomyard-LYMHUB/loomyard-weft
id=$1 kind=$2 wc=$3 code=$4
dir=$ROOT/.scratch/bench/inputs/$id
rm -rf "$dir"
mkdir -p "$dir/work"

start=$(git -C $WEFT show $wc:_lyx/reviews/$kind/round-1-usage.yaml | grep -m1 'start:' | tr -s ' ' | cut -d' ' -f3)
echo "$id $kind weft=$wc start=$start code=$code" > "$dir/meta.txt"

git -C $ROOT archive "$code" | tar -x -C "$dir/work"
mkdir -p "$dir/work/_lyx/discussion"
if [ "$kind" = discussion ]; then
  for f in $(git -C $WEFT ls-tree --name-only $wc _lyx/discussion/ | grep -vE 'support-log\.lens-'); do
    git -C $WEFT show $wc:$f > "$dir/work/$f"
  done
else
  git -C $WEFT show $wc:_lyx/discussion/decision-record.md > "$dir/work/_lyx/discussion/decision-record.md"
  mkdir -p "$dir/work/_lyx/plan"
  for f in $(git -C $WEFT ls-tree -r --name-only $wc _lyx/plan/ | grep -v '/archive-'); do
    mkdir -p "$dir/work/$(dirname $f)"
    git -C $WEFT show $wc:$f > "$dir/work/$f"
  done
fi

# Hub-style specs and stencils dirs, from the snapshot's own era.
bench=$dir/work/.bench
mkdir -p "$bench/specs/loom" "$bench/stencils" "$bench/instr" "$bench/out"
cp "$dir"/work/contracts/specs/*.md "$bench/specs/" 2>/dev/null || true
for f in "$dir"/work/contracts/specs/loom-*.md; do cp "$f" "$bench/specs/loom/${f##*/loom-}"; cp "$f" "$bench/specs/loom/$(basename $f)"; done
cp -r "$dir"/work/contracts/stencils/* "$bench/stencils/"

strip() { local s; s=$(<"$1"); [ "${s:0:4}" = '<!--' ] && s=${s#*-->}; printf '%s\n' "${s#"${s%%[![:space:]]*}"}"; }
fill() { # fill <text> key=value...
  local t=$1; shift
  for kv in "$@"; do t=${t//"{{.${kv%%=*}}}"/"${kv#*=}"}; done
  printf '%s' "$t"
}

W=$dir/work
review_path=$W/.bench/out/review.md
rubric=$(strip "$bench/stencils/loom/loom-rubric-$kind-review.md")
rubric=$(fill "$rubric" "specs_dir=$bench/specs" "stencils_dir=$bench/stencils" "amendment=none" "webster_record=none" "gate_tiers=none")

if [ "$kind" = discussion ]; then
  target="- \`$W/_lyx/discussion/decision-record.md\`
- \`$W/_lyx/discussion/support-log.md\`"
  fasit="The authority for this round is the rubric supplied in the prompt. The mechanical section contract over the discussion artifact pair is already enforced by this round's own gate over this round's own output, so re-deriving it here is duplicated work."
else
  target="- \`$W/_lyx/plan\` (a directory — everything under it)"
  fasit="- \`$W/_lyx/discussion/decision-record.md\`

The decision record named above is the answer key: the plan's job is to implement the decisions and constraints it settles. With no rework generation, the decision record alone is the answer key. The format authority is $bench/specs/loom/loom-plan-spec.md, and the mechanical checks over that format are already enforced by this round's own gate over this round's own output, so re-deriving them here is duplicated work. This round may never write the approved: key in the plan's overview file."
fi

focus=""
if git -C $WEFT cat-file -e $wc:_lyx/reviews/$kind/round-1-focus.md 2>/dev/null; then
  git -C $WEFT show $wc:_lyx/reviews/$kind/round-1-focus.md > "$W/.bench/instr/focus.md"
  focus=$(fill "$(strip "$bench/stencils/burler/burler-focus-directive.md")" "focus_path=$W/.bench/instr/focus.md")
  echo focus=yes >> "$dir/meta.txt"
else
  echo focus=no >> "$dir/meta.txt"
fi

# The PATTERN directive a production burler reviewer gets: the review-fix stencil filled with PATTERN.md and the record's declared changes.
declared=$(awk '/^## /{on=($0=="## PATTERN changes")} on && /^- `PATTERN-/' "$W/_lyx/discussion/decision-record.md")
changes=""
if [ -n "$declared" ]; then
  changes=$(fill "$(strip "$bench/stencils/pattern/pattern-fragment-changes.md")" "pattern_change_list=$declared")
fi
directive=$(fill "$(strip "$bench/stencils/pattern/pattern-directive-review-fix.md")" "pattern_overview=$(<"$W/PATTERN.md")" "pattern_changes=$changes")
# Codex loads AGENTS.md from the working directory as a Claude session loads CLAUDE.md.
cp "$W/CLAUDE.md" "$W/AGENTS.md"

explore=$(strip "$bench/stencils/burler/burler-step-1-explore.md")
explore=$(fill "$explore" "pattern_directive=$directive" "focus_directive=$focus" "target=$target" "fasit=$fasit" "rubric=$rubric" \
  "tool_use_rules=Read-only analysis in the explore and review steps: read files, run nothing." "fork_reading=")
printf '%s\n' "$explore" > "$W/.bench/instr/step-1-explore.md"

review=$(strip "$bench/stencils/burler/burler-step-review.md")
review=$(fill "$review" "cluster_rules=$(strip "$bench/stencils/burler/burler-cluster-solo.md")" "review_path=$review_path" \
  "review_format_path=$W/.bench/instr/review-format.md" "prior_rounds=This is the first round — no prior round files exist.")
printf '%s\n' "$review" > "$W/.bench/instr/step-2-review.md"
strip "$bench/stencils/burler/burler-review-format.md" > "$W/.bench/instr/review-format.md"

prompt=$(strip "$bench/stencils/burler/burler-template-reviewer.md")
prompt=$(fill "$prompt" "parent_directive=" "edit_directive=" "review_path=$review_path" \
  "instruction_1_path=$W/.bench/instr/step-1-explore.md" "instruction_2_path=$W/.bench/instr/step-2-review.md")
printf '%s\n' "$prompt" > "$dir/prompt.md"
cat "$dir/meta.txt"
if grep -l '{{' "$dir/prompt.md" "$W"/.bench/instr/*.md; then
  echo "leftover placeholder in the files above" >&2
  exit 1
fi
