#!/usr/bin/env bash
# Re-runnable pipeline: find transcripts, prefilter on the fork prompt heading, extract one record per fork.
# Usage: run-extract.sh   (run from anywhere; writes into this script's own directory)
set -u
here="$(cd "$(dirname "$0")" && pwd)"
cd "$HOME/.claude/projects" || exit 1

# All fork transcripts last modified since 2026-10-12 00:00 local (covers the raf window and everything after 1e39afc10).
find . -path './-home-knatte-Code-loomyard-LYXHUB-*/*/subagents/agent-*.jsonl' -newermt '2026-10-12 00:00:00' | sort > "$here/cand-all.txt"

# Prefilter: the transcript mentions the Webster implementer job heading.
xargs -a "$here/cand-all.txt" grep -l '# Webster implementer job' | sort > "$here/webster-files.txt"

# Extract: one JSON record per transcript whose first tool call reads a Webster prompt file.
: > "$here/raw.jsonl"
while IFS= read -r f; do
  jq -s -c -f "$here/extract-pf.jq" "$f" >> "$here/raw.jsonl"
done < "$here/webster-files.txt"
wc -l "$here/cand-all.txt" "$here/webster-files.txt" "$here/raw.jsonl"
