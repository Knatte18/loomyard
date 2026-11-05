#!/usr/bin/env bash
# Stage the fan prompts over an input that build.sh already built: build-fan.sh <id>
# Writes work/.bench/instr/step-1-explore-fan.md, step-2-review-fan.md, one lens-<name>.prompt.md per lens and handler.prompt.md.
set -euo pipefail
ROOT=/home/knatte/Code/loomyard-LYMHUB/codex-astra-tuned-prompt-bench
id=$1
code=$(grep -o 'code=[0-9a-f]*' "$ROOT/.scratch/bench/inputs/$id/meta.txt" | cut -d= -f2)
in=$ROOT/.scratch/bench/inputs/$id
W=$in/work
fan=$in/fan
rm -rf "$fan"; mkdir -p "$fan"
LENSES="affected-set guard-reach way-forward"

strip() { local s; s=$(<"$1"); [ "${s:0:4}" = '<!--' ] && s=${s#*-->}; printf '%s\n' "${s#"${s%%[![:space:]]*}"}"; }

# The pre-read the forks inherited from the handler in round 1.
git -C $ROOT show $code:contracts/stencils/burler/burler-fork-reading.md > "$fan/fork-reading.raw"
reading=$(strip "$fan/fork-reading.raw")
{ cat "$W/.bench/instr/step-1-explore.md"; printf '\n%s\n' "$reading"; } > "$W/.bench/instr/step-1-explore-fan.md"

# A lens's text is its block in the template at the code commit, 4-space indent removed.
git -C $ROOT show $code:internal/lenses/template.yaml > "$fan/template.yaml"
lens_text() {
  awk -v n="  $1: |" '$0==n {on=1; next} on && /^  [a-z-]+: \|/ {exit} on && /^[a-z]+:/ {exit} on {sub(/^    /,""); print}' "$fan/template.yaml"
}

for l in $LENSES; do
  lens_text "$l" > "$fan/lens-$l.text"
  [ -s "$fan/lens-$l.text" ] || { echo "no text for lens $l" >&2; exit 1; }
  cat > "$fan/lens-$l.prompt.md" <<PROMPT
# Burler — fork reviewer

You are one fork reviewer of a fanned plan-review round, and you own one lens; the other forks own the other lenses, and a handler reviews holistically afterwards.

Read \`$W/.bench/instr/step-1-explore-fan.md\` and do what it directs: it names the target, the fasit and the rubric, and ends with the reading to do.
Do that reading, then work your lens's procedure and start your final message with the list your lens asks for.
Report every finding your lens's procedure produces and nothing outside it, because the other lenses and the handler cover the rest.
Judge nothing while you read, and review only after the reading is done.
You are READ-ONLY: never write, edit or delete any file, run only commands that read, never touch \`$W/.bench/out\`, and never start another agent.
Return your findings ONLY as your final message, each with a severity (\`BLOCKING\`, \`MEDIUM\`, \`LOW\` or \`NIT\`), a class (\`design\`, \`scope\`, \`decision\` or \`consistency\`), a location, a one-line summary and the file and line of the evidence you read.

Your lens: $l
$(cat "$fan/lens-$l.text")
PROMPT
done

# The handler's review step: the fan cluster rules with the forks already returned.
lens_list=""
for l in $LENSES; do lens_list+="- $l: $(cat "$fan/lens-$l.text")"$'\n'; done
lens_files=""
for l in $LENSES; do lens_files+="- \`$W/.bench/lens/$l.md\`: the final message of the \`$l\` fork"$'\n'; done
cluster=$(cat <<CLUSTER
This is a cluster round. The fork reviewers have already run, one per lens listed here, and each returned its findings as its final message.

Lenses for this round:

$lens_list
The forks' final messages are these files, which you read once your reading in step 1 is done:

$lens_files
You do not spawn any fork: the results are in those files.

Do your own HOLISTIC review: what no lens owns, such as the target against its fasit and its board entry, the rubric's leanness and relocation rules, architecture and PATTERN-fit, reading further only where your review needs it.
Then prepare the ground truths and the severity rubric you will judge every finding against.
Write nothing to the review file until you have read all three fork reports and finished your own holistic review: consolidate with every finding in view.

Consolidation: judge every fork finding and your own holistic findings with EQUAL skepticism, dedup findings that describe the same defect across lenses, tag every kept finding's frontmatter with an \`origin:\` key (\`lens:<name>\` or \`handler\`), move false positives to a \`## Rejected\` prose section below the frontmatter with a one-line reason each (a rejected item never appears in \`findings:\`), and order kept findings by severity.
The consolidated review is the ONE review file, and the review is complete once it is fully written to disk — consolidation is part of the review, not a separate step after it.
CLUSTER
)
git -C $ROOT show $code:contracts/stencils/burler/burler-step-review.md > "$fan/step-review.raw"
review=$(strip "$fan/step-review.raw")
review_path=$W/.bench/out/review.md
fill() { local t=$1; shift; for kv in "$@"; do t=${t//"{{.${kv%%=*}}}"/"${kv#*=}"}; done; printf '%s' "$t"; }
review=$(fill "$review" "cluster_rules=$cluster" "review_path=$review_path" \
  "review_format_path=$W/.bench/instr/review-format.md" "prior_rounds=This is the first round — no prior round files exist.")
printf '%s\n' "$review" > "$W/.bench/instr/step-2-review-fan.md"

git -C $ROOT show $code:contracts/stencils/burler/burler-template-reviewer.md > "$fan/reviewer.raw"
prompt=$(strip "$fan/reviewer.raw")
prompt=$(fill "$prompt" "parent_directive=" "edit_directive=" "review_path=$review_path" \
  "instruction_1_path=$W/.bench/instr/step-1-explore-fan.md" "instruction_2_path=$W/.bench/instr/step-2-review-fan.md")
printf '%s\n' "$prompt" > "$fan/handler.prompt.md"
if grep -l '{{' "$fan"/*.prompt.md "$W"/.bench/instr/*.md; then echo "leftover placeholder" >&2; exit 1; fi
ls "$fan"
