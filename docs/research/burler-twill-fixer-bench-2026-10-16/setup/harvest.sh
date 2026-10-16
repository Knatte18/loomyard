#!/usr/bin/env bash
# harvest.sh <ref-slug> <pair>...: copies each pair's evidence into runs/<pair>/:
# transcripts, the artifact after the fix, the round files, the fork reports, the spliced fix step,
# a diff of the artifact against the reference's pre-fix state, and the per-session usage (sessions.sh).
B=/home/knatte/Code/loomyard-LYXHUB/loomyard/.scratch/burler-twill-fixer-bench; S=$B/setup; H=/home/knatte/Code/loomyard-LYXHUB
ref=$1; shift
ph=$(sed -n 's/^phase: //p' "$B/keys/$ref/source.yaml")
case "$ph" in discussion) art=_lyx/discussion ;; plan) art=_lyx/plan ;; esac
for p in "$@"; do
  R=$B/runs/$p; rm -rf "$R"; mkdir -p "$R/forks" "$R/artifact" "$R/reviews"
  cp -r "$HOME/.claude/projects/-home-knatte-Code-loomyard-LYXHUB-$p" "$R/transcripts"
  cp "$H/$p/$art"/*.md "$R/artifact/"
  cp "$H/$p/_lyx/reviews/$ph"/*.md "$H/$p/_lyx/reviews/$ph"/*.yaml "$R/reviews/" 2>/dev/null
  cp "$H/$p"/.lyx/logs/trace-*.log "$R/" 2>/dev/null
  cp "$S"/fixstep-$p-*.md "$S"/prompt-$p-*.md "$R/" 2>/dev/null
  diff -u "$B/keys/$ref/before" "$R/artifact" > "$R/fix.diff"
  for m in "$R"/transcripts/*/subagents/*.meta.json; do
    [ -f "$m" ] || continue
    g=${m%.meta.json}.jsonl
    d=$(jq -r .description "$m" | tr -c 'a-zA-Z0-9-\n' '_')
    jq -s -r '[.[] | select(.type=="assistant") | .message.content[]? | select(.type=="text") | .text] | last' "$g" > "$R/forks/$d.md"
  done
  "$S/sessions.sh" "$p" > "$R/usage.tsv"
  echo "== $p"; cat "$R/usage.tsv"; wc -l < "$R/fix.diff"
done
