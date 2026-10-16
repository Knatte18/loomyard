#!/usr/bin/env bash
# wave.sh <phase> <pair>...: for each pair in parallel, commits the placed status, pauses after the perch's Bouncer
# and steps until the stop; phase is Plan or Discussion.
# The run thus runs the round-N fixer and the Bouncer's judgment of round N, and halts at whatever row the Bouncer routed to.
# Each pair's envelopes go to setup/step-<pair>.log, stderr to setup/step-<pair>.err.
S=/home/knatte/Code/loomyard-LYXHUB/loomyard/.scratch/burler-twill-fixer-bench/setup
H=/home/knatte/Code/loomyard-LYXHUB
phase=$1; shift
run() {
  local p=$1 log="$S/step-$1.log" err="$S/step-$1.err"
  cd "$H/$p" || return 1
  echo "$(date +%T) start" >> "$log"
  lyx fabric commit >> "$log" 2>> "$err"
  echo >> "$log"
  lyx loom pause --after "$phase-Bouncer" >> "$log" 2>> "$err"
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
  tail -c 600 "$S/step-$p.log"
done
