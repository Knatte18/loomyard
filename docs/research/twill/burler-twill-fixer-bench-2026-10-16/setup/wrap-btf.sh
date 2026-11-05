#!/usr/bin/env bash
# Burler twill-fixer bench wrapper, adapted from twill-discussion-fork0-bench's wrap-d0.sh.
# The pair name's last letter picks the arm:
#   s: today's fixer (nothing spliced)
#   t: today's fixer plus the self-check-by-forks block, spliced into the rendered fix step before "## Fixer-report rule"
# Only a fixer session is touched: its prompt.md (beside the --settings file) carries "# Burler — fix"
# and names the fix-step file in its "Read and execute `<path>`" line; the block goes into that file.
# Every session (reviewer, fixer, judge) gets the code-base notice appended to its system prompt.
# Fails closed: a fixer prompt whose markers are missing or repeated leaves claude unstarted.
# Test mode: wrap-btf.sh --btf-test <arm> <in-fix-step> <out-fix-step> <pairdir> splices a copy and exits.
S=/home/knatte/Code/loomyard-LYXHUB/loomyard/.scratch/burler-twill-fixer-bench/setup

m_fix_title='# Burler — fix'
m_report='## Fixer-report rule'

# once <file> <marker>: succeeds when the marker occurs exactly once in the file.
once() { [ "$(grep -c -F -- "$2" "$1")" = 1 ]; }

# splice <arm> <in> <out> <pairdir>: writes the arm's fix step to <out>; returns 1 on a marker problem.
splice() {
  local arm=$1 in=$2 out=$3 pd=$4 content pre rest text
  once "$in" "$m_report" || { echo "marker not exactly once: $m_report"; return 1; }
  content="$(cat "$in")"
  if [ "$arm" = t ]; then
    pre="${content%%"$m_report"*}"
    rest="${content#*"$m_report"}"
    text="$(cat "$S/selfcheck-block.md")"
    text="${text//PAIRDIR/$pd}"
    content="$pre$text"$'\n\n'"$m_report$rest"
  fi
  printf '%s\n' "$content" > "$out"
}

if [ "$1" = "--btf-test" ]; then
  splice "$2" "$3" "$4" "$5"
  exit $?
fi

pairdir="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
pair="$(basename "$pairdir")"
arm="${pair: -1}"
log="$S/wrap-$pair.log"
settings=""
prev=""
for a in "$@"; do
  if [ "$prev" = "--settings" ]; then settings="$a"; fi
  prev="$a"
done
case "$arm" in s|t) ;; *) echo "$(date +%T) unknown arm '$arm' for $pair" >> "$log"; exit 1 ;; esac
if [ -n "$settings" ]; then
  prompt="$(dirname "$settings")/prompt.md"
  if [ ! -f "$prompt" ]; then
    echo "$(date +%T) no prompt at $prompt" >> "$log"
  elif grep -q -F "$m_fix_title" "$prompt"; then
    line="$(grep -F 'Read and execute `' "$prompt" | head -1)"
    fixstep="${line#*Read and execute \`}"
    fixstep="${fixstep%%\`*}"
    tag="$(basename "$(dirname "$settings")")"
    if [ -z "$fixstep" ] || [ ! -f "$fixstep" ]; then
      echo "$(date +%T) FAIL fixer prompt $prompt names no readable fix step ('$fixstep'); claude not started" >> "$log"
      exit 1
    elif grep -q -F 'btf-bench-arm:' "$fixstep"; then
      echo "$(date +%T) $fixstep already spliced" >> "$log"
    else
      cp "$fixstep" "$S/fixstep-$pair-$tag.orig.md"
      if ! msg="$(splice "$arm" "$S/fixstep-$pair-$tag.orig.md" "$fixstep" "$pairdir")"; then
        cp "$S/fixstep-$pair-$tag.orig.md" "$fixstep"
        echo "$(date +%T) FAIL splice arm $arm into $fixstep: $msg; claude not started" >> "$log"
        exit 1
      fi
      mkdir -p "$pairdir/.lyx/btf"
      cp "$S/fix-check-rubric.md" "$pairdir/.lyx/btf/fix-check-rubric.md"
      printf '\n<!-- btf-bench-arm: %s -->\n' "$arm" >> "$fixstep"
      cp "$fixstep" "$S/fixstep-$pair-$tag.md"
      cp "$prompt" "$S/prompt-$pair-$tag.md"
      echo "$(date +%T) spliced arm $arm into $fixstep (prompt $prompt)" >> "$log"
    fi
  else
    echo "$(date +%T) not a fixer prompt: $prompt" >> "$log"
  fi
fi
extra="$(cat "$S/notice.md")"
args=()
merged=0
while [ $# -gt 0 ]; do
  if [ "$1" = "--append-system-prompt" ] && [ $# -ge 2 ]; then
    args+=("$1" "$2"$'\n\n'"$extra")
    shift 2
    merged=1
    continue
  fi
  args+=("$1")
  shift
done
if [ "$merged" = 0 ]; then
  args+=(--append-system-prompt "$extra")
fi
# Both arms get the fork-subagent capability, so their settings are equal; only the spliced block asks for forks.
export CLAUDE_CODE_FORK_SUBAGENT=1
exec /home/knatte/.local/bin/claude "${args[@]}"
