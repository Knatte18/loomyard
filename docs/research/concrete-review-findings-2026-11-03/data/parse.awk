# Prints one TSV row per finding of a review file's frontmatter: id, severity, location, fix.
# A fix or location value may be a folded or literal block; its lines are joined with spaces.
function flush() {
  if (id != "") printf "%s\t%s\t%s\t%s\n", id, sev, loc, fix
  id = ""; sev = ""; loc = ""; fix = ""; cur = ""
}
/^---$/ { fm++; if (fm == 2) { flush(); exit } next }
fm != 1 { next }
/^  - id: / { flush(); id = $3; next }
/^    severity: / { sev = $2; cur = ""; next }
/^    location: / { loc = substr($0, 15); cur = "loc"; next }
/^    fix: / { v = substr($0, 10); if (v ~ /^[>|][-+]?$/) v = ""; fix = v; cur = "fix"; next }
/^    [a-z_]+: / { cur = ""; next }
/^      / { s = $0; sub(/^ +/, "", s); if (cur == "fix") fix = fix " " s; else if (cur == "loc") loc = loc " " s; next }
END { flush() }
