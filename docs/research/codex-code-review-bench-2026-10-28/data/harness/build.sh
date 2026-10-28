#!/usr/bin/env bash
# Stage one bench input and the forked-fan handler prompt for a Code-Review round.
# usage: build.sh <id> <weft-commit> <code-commit> <base-commit>
#   weft-commit: the weft commit whose tree holds the round's _lym/discussion, _lym/landing and round-1 focus files.
#   code-commit: the code commit the Claude round reviewed.
#   base-commit: the merge base the rubric's review range starts from.
# Writes .scratch/bench/inputs/<id>/{work,meta.txt,fan/}; the handler prompt is fan/handler-fork.prompt.md.
# The work tree is a shared clone checked out at code-commit, so `git diff <base>..HEAD` works as in the production worktree.
# Prompts are assembled as internal/burlerengine's composeReviewPrompt does for a fanned Code-Review round, from the stencils of code-commit.
set -euo pipefail
ROOT=/home/knatte/Code/loomyard-LYMHUB/codex-code-review-bench
WEFT=/home/knatte/Code/loomyard-LYMHUB/loomyard-weft
HARNESS_DOCS=$ROOT/docs/research/codex-code-review-bench-2026-10-28/data
id=$1 wc=$2 code=$3 base=$4
in=$ROOT/.scratch/bench/inputs/$id
rm -rf "$in"; mkdir -p "$in"
W=$in/work
fan=$in/fan; mkdir -p "$fan"
echo "$id weft=$wc code=$code base=$base" > "$in/meta.txt"

git clone -q --shared --no-checkout "$ROOT" "$W"
git -C "$W" checkout -q -b bench "$code"
git -C "$W" config user.email bench@example.invalid
git -C "$W" config user.name bench
printf '%s\n' '.bench/' 'AGENTS.md' '_lym/' >> "$W/.git/info/exclude"

mkdir -p "$W/_lym/discussion" "$W/_lym/landing"
for f in board-entry decision-record support-log; do git -C $WEFT show $wc:_lym/discussion/$f.md > "$W/_lym/discussion/$f.md"; done
git -C $WEFT show $wc:_lym/landing/summary.md > "$W/_lym/landing/summary.md"

bench=$W/.bench
mkdir -p "$bench/specs/loom" "$bench/stencils" "$bench/instr" "$bench/out"
git -C $ROOT archive "$code" contracts/specs contracts/stencils | tar -x -C "$in"
cp "$in"/contracts/specs/*.md "$bench/specs/"
for f in "$in"/contracts/specs/loom-*.md; do cp "$f" "$bench/specs/loom/$(basename $f | sed 's/^loom-//')"; cp "$f" "$bench/specs/loom/$(basename $f)"; done
cp -r "$in"/contracts/stencils/* "$bench/stencils/"
rm -rf "$in/contracts"

strip() { local s; s=$(<"$1"); [ "${s:0:4}" = '<!--' ] && s=${s#*-->}; printf '%s\n' "${s#"${s%%[![:space:]]*}"}"; }
fill() { local t=$1; shift; for kv in "$@"; do t=${t//"{{.${kv%%=*}}}"/"${kv#*=}"}; done; printf '%s' "$t"; }
st=$bench/stencils

# Rubric, as shedadapters.ReadRubric fills it, with the review range base.
rubric=$(strip "$st/loom/loom-rubric-code-review.md")
rubric=$(fill "$rubric" "specs_dir=$bench/specs" "stencils_dir=$st" "amendment=none" "webster_record=none" "gate_tiers=none" "review_base=$base")

# Target and fasit, as darn-recipe.yaml's Code-Burler-Review profile gives them.
target="The subject under review is the committed diff. The rubric supplied in this prompt is
the single definition of the review range: read its \"Determining the review range\"
section and derive the range exactly as written there."
fasit="- \`$W/_lym/discussion/board-entry.md\`
- \`$W/_lym/discussion/decision-record.md\`

The board entry snapshot named above is the task's scope, as amended by the
\`Added after Discussion\` entries of the decision record named above, which win where
they differ. The record's other sections are the writer's own claim, checked against
the snapshot and never the answer key. The diff's job is to make the change the
amended entry asks for."

git -C $WEFT show $wc:_lym/reviews/code/round-1-focus.md > "$W/.bench/instr/focus.md"
focus=$(fill "$(strip "$st/burler/burler-focus-directive.md")" "focus_path=$W/.bench/instr/focus.md")

declared=$(awk '/^## /{on=($0=="## PATTERN changes")} on && /^- `PATTERN-/' "$W/_lym/discussion/decision-record.md")
changes=""
if [ -n "$declared" ]; then
  changes=$(fill "$(strip "$st/pattern/pattern-fragment-changes.md")" "pattern_change_list=$declared")
fi
directive=$(fill "$(strip "$st/pattern/pattern-directive-review-fix.md")" "pattern_overview=$(<"$W/PATTERN.md")" "pattern_changes=$changes")
cp "$W/CLAUDE.md" "$W/AGENTS.md"

# Fork reading: the board entry is a file in the snapshot, as lym is not on the Codex run's path.
reading=$(strip "$st/burler/burler-fork-reading.md")
reading=${reading//'the board entry of the task the target belongs to, through `lym board get`;'/"the board entry snapshot at \`$W/_lym/discussion/board-entry.md\`;"}
if grep -q 'lym board get' <<<"$reading"; then echo "board-entry anchor missing" >&2; exit 1; fi
fork_reading=$'\n\n'"$reading"

# Read-only tool-use rules, as the plan benches ran: no build, no test, no lym in the Codex run.
explore=$(strip "$st/burler/burler-step-1-explore.md")
explore=$(fill "$explore" "pattern_directive=$directive" "focus_directive=$focus" "target=$target" "fasit=$fasit" "rubric=$rubric" \
  "tool_use_rules=Read-only analysis in the explore and review steps: read files, run nothing." "fork_reading=$fork_reading")
printf '%s\n' "$explore" > "$W/.bench/instr/step-1-explore-fan.md"

# Lens texts of the code commit's template.
git -C $ROOT show $code:internal/lenses/template.yaml > "$fan/template.yaml"
lens_text() {
  awk -v n="  $1: |" '$0==n {on=1; next} on && /^  [a-z-]+: \|/ {exit} on && /^[a-z]+:/ {exit} on {sub(/^    /,""); print}' "$fan/template.yaml"
}
LENSES="code-correctness code-affected-set code-tests"
lens_list=""
for l in $LENSES; do
  lens_text "$l" > "$fan/lens-$l.text"
  [ -s "$fan/lens-$l.text" ] || { echo "no text for lens $l" >&2; exit 1; }
  lens_list+="- $l: $(cat "$fan/lens-$l.text")"$'\n'
done

# The fanned cluster rules; Claude's spawn mechanics become Codex's.
cluster=$(strip "$st/burler/burler-cluster-fan.md")
cluster=$(fill "$cluster" "lens_list=$lens_list" "lens_report_path=$W/.bench/out/review.lens-<lens>.md" "lens_report_spec_path=$bench/specs/twill-lens-report-spec.md")
cluster=${cluster//'via the Agent tool with `subagent_type: "fork"`, one per lens listed here — never pass a `name` (named forks silently lose inherited context).'/'by calling your `spawn_agent` tool once per lens listed here, so the forks run in parallel, each with `fork_turns` set to `"all"` so it inherits your context; never pass a name, a model or an effort for a fork, so each fork keeps your own.'}
cluster=${cluster//'and never call the Agent tool.'/'and never spawn another agent.'}
cluster=${cluster//'run only git commands that read, such as `git diff`, `git log` or `git show`, and never one that changes the repository, its index, its refs or its working tree,'/'run only commands that read, such as `git diff`, `git log` or `git show`, and never one that changes the repository, its index, its refs or its working tree,'}
cluster=${cluster//'While the forks run, YOU (the handler) do your own HOLISTIC review'/'Wait for every fork with your wait tool; while the forks run, YOU (the handler) do your own HOLISTIC review'}
# The tuned block goes into the fork boilerplate as one line and onto the handler prompt.
BLOCK=$HARNESS_DOCS/prompts/block-v2-code.md
block_line=$(tail -n +3 "$BLOCK" | sed 's/^- //' | tr '\n' ' ' | sed 's/ $//')
cluster=${cluster//"Work your lens's procedure, and start"/"Reading for this review: $block_line Work your lens's procedure, and start"}
cluster=${cluster//"at most 6 tool calls"/"at most 20 tool calls"}
for anchor in 'at most 20 tool calls' 'Reading for this review' 'spawn_agent' 'Wait for every fork'; do
  grep -q "$anchor" <<<"$cluster" || { echo "anchor missing: $anchor" >&2; exit 1; }
done
if grep -q 'Agent tool\|subagent_type\|{{' <<<"$cluster"; then echo "unconverted wording" >&2; exit 1; fi

review_path=$W/.bench/out/review.md
strip "$st/burler/burler-review-format.md" > "$W/.bench/instr/review-format.md"
review=$(fill "$(strip "$st/burler/burler-step-review.md")" "cluster_rules=$cluster" "review_path=$review_path" \
  "review_format_path=$W/.bench/instr/review-format.md" "prior_rounds=This is the first round — no prior round files exist.")
printf '%s\n' "$review" > "$W/.bench/instr/step-2-review-fan.md"

prompt=$(strip "$st/burler/burler-template-reviewer.md")
prompt=$(fill "$prompt" "parent_directive=" "edit_directive=" "review_path=$review_path" \
  "instruction_1_path=$W/.bench/instr/step-1-explore-fan.md" "instruction_2_path=$W/.bench/instr/step-2-review-fan.md")
{ printf '%s\n' "$prompt"; printf '\n'; cat "$BLOCK"; } > "$fan/handler-fork.prompt.md"
if grep -l '{{' "$fan/handler-fork.prompt.md" "$W"/.bench/instr/*.md; then echo "leftover placeholder" >&2; exit 1; fi
echo "built $id"; cat "$in/meta.txt"
