# facts.awk reads one review perch's newest round facts file and prints its figures, tab-separated:
#   F  round column value   one row per cell of the rounds table, the Round cell left out
#   K  key rounds           one row per ledger key listed under the recurring keys heading
# The rounds table's header row opens with "| Round |", and every data row opens with a round number.

/^\| *Round *\|/ {
  ncols = split($0, header, "|")
  for (i = 2; i < ncols; i++) gsub(/^ +| +$/, "", header[i])
  next
}
/^\| *[0-9]+ *\|/ && ncols > 0 {
  n = split($0, cell, "|")
  round = cell[2]; gsub(/ /, "", round)
  for (i = 3; i < n && i < ncols; i++) {
    value = cell[i]; gsub(/^ +| +$/, "", value)
    print "F\t" round "\t" header[i] "\t" value
  }
  next
}
/^## / { recurring = ($0 ~ /^## Recurring keys/); next }
recurring && /^- `[^`]+`: rounds / {
  key = $0; sub(/^- `/, "", key); sub(/`.*$/, "", key)
  rounds = $0; sub(/^.*: rounds /, "", rounds)
  print "K\t" key "\t" rounds
}
