# Commands of the 2026-10-15 pack-file audit

All paths are under `/home/knatte/Code/loomyard-LYXHUB/loomyard/.scratch/glyph-audit-3/` (written `$A`) unless stated.
Transcripts live in `~/.claude/projects/`.
Scripts were written with the Write tool; none was edited with `sed -i` or a heredoc.
Run the numbered steps in order to reproduce every table in `report.md`.
Orientation commands that only read (cat of the previous audit's README and report, `git show` of the three commits, `ls` of the projects directory, ad-hoc `jq` looks at single transcripts) are listed at the end and feed no number.

## 0. Setup

```
mkdir -p $A
cp /home/knatte/Code/loomyard-LYXHUB/loomyard/docs/research/glyph-audit-2026-10-13/{extract2.jq,stats2.jq,stats3.jq,stats-base.jq} $A/
```

## 1. Find the transcripts and extract one record per fork (arms pf and raf)

First find, used to scope pf (forks after `1e39afc10`, 2026-10-13 16:42 local):

```
cd ~/.claude/projects && find . -path './-home-knatte-Code-loomyard-LYXHUB-*/*/subagents/agent-*.jsonl' -newermt '2026-10-13 16:42:00' | sort
```

That gave 305 files, 27 of which mention `# Webster implementer job`.
The final pipeline widens the window to 2026-10-12 00:00 so the same pass also yields the raf forks, and decides the arm from the prompt text, not from dates:

```
$A/run-extract.sh
```

It runs `find . -path './-home-knatte-Code-loomyard-LYXHUB-*/*/subagents/agent-*.jsonl' -newermt '2026-10-12 00:00:00' | sort > cand-all.txt`, then `xargs -a cand-all.txt grep -l '# Webster implementer job' | sort > webster-files.txt`, then `jq -s -c -f extract-pf.jq <file>` per file into `raw.jsonl`.
Result: 794 candidates, 101 with the heading, 95 records.
pf = prompt holds `## Start here` (24 forks, all start 2026-10-13T14:46Z or later; `shed-tackler` batches 1, 4, 7, 13 included).
raf = prompt holds the inline `## Kick-start pack` and the read-all-first phrase (31 forks, 2026-10-12T13:11Z to 2026-10-13T14:08Z).
The other 40 records are the older base_on and mid arms, unused here.
The 6 files with the heading but no record are my own audit transcripts and one unrelated fork.

Cross-check of the old extractor over the same files (31 raf forks, as the new script finds; the old report said 29):

```
$A/run-old-extract.sh
jq -s -r -f $A/stats2.jq $A/raw-old.jsonl
```

## 2. Arm table, per-fork table, quartiles

```
cd $A
jq -s -r -f stats-pf.jq raw.jsonl > table.tsv
jq -s -r -f perfork-pf.jq raw.jsonl > perfork-pf.tsv
jq -s -r -f quartiles.jq raw.jsonl > quartiles.tsv
```

## 3. Pack correctness

```
cd $A
jq -r 'select(.layout=="file")|[.sid, .start, .proj]|@tsv' raw.jsonl | awk -F'\t' '{split($1,a,"/"); print a[2]"\t"$2"\t"$3"\t"a[4]}' | sort -k3,3 -k2,2 > pf-forks.tsv
./resolve-batches.sh            # batches.tsv: fork -> batch, startSha, weft commit (records repo)
./check-pack.sh                 # pack-check.tsv, pack-unres.tsv
BATCHES=$A/control-batches.tsv OUT=$A/control-check.tsv ./check-pack.sh   # negative control: batch 4, 7, 13 packs checked against batch 1's tree
./check-pack.sh                 # re-run to restore the real pack-unres.tsv (the control run overwrote it)
./missed-files.sh               # missed-files.tsv
```

Then the per-fork and pooled counts in the report came from:

```
awk -F'\t' '{k=$2"\t"substr($1,7,8); n[k]++; c[k,$8]++} END{for(k in n) printf "%s\tentries=%d\tcorrect=%d\tcorrect-decl=%d\tstale=%d\twrong=%d\tnofile=%d\tnotree=%d\n",k,n[k],c[k,"correct"],c[k,"correct-decl"],c[k,"stale"],c[k,"wrong"],c[k,"nofile"],c[k,"notree"]}' pack-check.tsv | sort > pack-check-perfork.tsv
awk -F'\t' '{print $8}' pack-check.tsv | sort | uniq -c
awk -F'\t' '{split($4,g,"#"); f=$5; sub(/\/[^\/]*$/,"",f); if (f!=g[1]) print $2, $4, $5}' pack-check.tsv      # glyph package vs file directory
awk -F'\t' '{print $5}' pack-unres.tsv | sed 's/ —.*//' | sed 's/^names a member.*/plan-handle/' | cut -c1-70 | sort | uniq -c | sort -rn
awk -F'\t' '$4>0 && $7=="yes"' missed-files.tsv                                                    # existing files edited, no span
```

The pooled counts, the missed-file sums, the extension split, the control and the plan-glyph join are all in one script:

```
$A/summaries.sh        # writes summaries.txt and plan-writers-summary.tsv
```

State of the records repo used by `resolve-batches.sh`: `/home/knatte/Code/loomyard-LYXHUB/loomyard-weft`, `git log --all -S<spawnedAt second> -- _lyx/webster/state.json`; the batch start tree is `startSha`, read with `git -C /home/knatte/Code/loomyard-LYXHUB/loomyard show <startSha>:<file>`.

## 4. Plan reference census and plan-writer tool use

```
cd $A
./plan-refs.sh                                           # plan-runs.tsv, plan-refs.tsv
jq -R -s -r -f plan-stats.jq plan-refs.tsv > plan-stats.tsv
./plan-writer.sh > /dev/null                             # plan-writer.tsv (every session that wrote or touched plan cards)
sort -t$'\t' -k1,1 -k2,2 -k5,5nr plan-writer.tsv | awk -F'\t' '$5>0 && !seen[$1"|"$2]++' > plan-writers.tsv
```

Then run `$A/summaries.sh` (section 3) for `plan-writers-summary.tsv` and the glyph-resolution join.
The plan-writer session of a run is the session with the most Writes of `_lyx/plan/NN-*.md` cards.
Final extra commands: `jq -s '[...]|add' raw.jsonl` sums of edits over the 21 checkable pf forks (edits 1774, edits to a file with no span 444).

## Orientation (reads only)

```
cat docs/research/glyph-audit-2026-10-13/README.md report.md extract2.jq stats2.jq stats3.jq
git -C /home/knatte/Code/loomyard-LYXHUB/loomyard show d8d1c0956 1e39afc10 da58615cd   (stencil and commit message)
git -C /home/knatte/Code/loomyard-LYXHUB/loomyard log --first-parent --format='%h %cI %s' ...
git -C /home/knatte/Code/loomyard-LYXHUB/loomyard-weft show 9155bb97a:_lyx/webster/state.json
git -C /home/knatte/Code/loomyard-LYXHUB/loomyard-weft show <commit>:_lyx/plan/<card>.md
```
