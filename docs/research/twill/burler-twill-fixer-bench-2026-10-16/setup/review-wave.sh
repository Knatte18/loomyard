#!/usr/bin/env bash
# review-wave.sh <phase> <pair>...: runs the fresh round-N+1 review of each pair in parallel, once wave.sh has halted it after the Bouncer.
# A run the Bouncer routed back to <phase>-Burler-Review steps on; a run it routed past the perch (approved)
# is first moved back onto <phase>-Burler-Review with lyx loom goto, so every fix gets its fresh next review.
# It pauses after <phase>-Burler-Review, which halts the run at the fix row without running the round-N+1 fixer.
S=/home/knatte/Code/loomyard-LYXHUB/loomyard/.scratch/burler-twill-fixer-bench/setup
H=/home/knatte/Code/loomyard-LYXHUB
phase=$1; shift
run() {
  local p=$1 log="$S/review-$1.log" err="$S/review-$1.err" cur
  cd "$H/$p" || return 1
  echo "$(date +%T) start" >> "$log"
  cur=$(jq -r .current_producer "_lyx/shed/$p/status.json")
  echo "routed to $cur" >> "$log"
  if [ "$cur" != "$phase-Burler-Review" ]; then
    lyx loom goto --to "$phase-Burler-Review" >> "$log" 2>> "$err"
    echo >> "$log"
  fi
  lyx loom pause --after "$phase-Burler-Review" >> "$log" 2>> "$err"
  echo >> "$log"
  lyx loom step --until-stop >> "$log" 2>> "$err"
  echo "$(date +%T) end rc=$?" >> "$log"
}
for p in "$@"; do
  run "$p" &
done
wait
for p in "$@"; do
  echo "== $p"
  tail -c 500 "$S/review-$p.log"
done
