#!/usr/bin/env bash
# keys.sh <ref-slug> <records-commit> <phase-dir> <round> <archive-tag>: copies a reference's grading material into keys/<ref>/:
# the artifact before the round-N fix (at the records commit), the artifact after the reference's own fix
# (at the first "<P>-Bouncer -> running" commit after it), the round-N review and fixer report,
# and every later round's review and fixer report from the archive tag.
H=/home/knatte/Code/loomyard-LYXHUB
B=$H/loomyard/.scratch/burler-twill-fixer-bench
W=$H/loomyard-weft
ref=$1 rec=$2 ph=$3 n=$4 tag=$5
K=$B/keys/$ref
case "$ph" in discussion) art=_lyx/discussion; P=Discussion ;; plan) art=_lyx/plan; P=Plan ;; *) exit 2 ;; esac
after=$(git -C "$W" log --reverse --ancestry-path --format='%h %s' "$rec..$tag" | grep -m1 "loom: $P-Bouncer -> running" | cut -d' ' -f1)
echo "ref $ref: before $rec, after $after"
mkdir -p "$K/before" "$K/after-ref" "$K/reviews"
for f in $(git -C "$W" ls-tree --name-only "$rec" "$art/"); do git -C "$W" show "$rec:$f" > "$K/before/$(basename "$f")"; done
for f in $(git -C "$W" ls-tree --name-only "$after" "$art/"); do git -C "$W" show "$after:$f" > "$K/after-ref/$(basename "$f")"; done
for f in $(git -C "$W" ls-tree --name-only "$tag" "_lyx/reviews/$ph/" | grep -E 'round-[0-9]+-(review|fixer-report|bouncer-verdict)\.md$'); do
  r=$(basename "$f" | sed -E 's/round-([0-9]+)-.*/\1/')
  [ "$r" -ge "$n" ] && git -C "$W" show "$tag:$f" > "$K/reviews/$(basename "$f")"
done
printf 'ref: %s\nphase: %s\nround: %s\nbefore: %s\nafter: %s\ntag: %s\n' "$ref" "$ph" "$n" "$rec" "$after" "$tag" > "$K/source.yaml"
ls "$K/reviews"
