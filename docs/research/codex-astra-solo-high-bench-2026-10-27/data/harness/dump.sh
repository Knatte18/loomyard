#!/usr/bin/env bash
# dump.sh <file>: one line per finding from the frontmatter.
id= sev= cls= loc= 
while IFS= read -r line; do
  case "$line" in
    "  - id: "*) [ -n "$id" ] && echo "$id $sev $cls $loc | $sum"; id=${line#  - id: }; sev=;cls=;loc=;sum=;;
    "    severity: "*) sev=${line#    severity: };;
    "    class: "*) cls=${line#    class: };;
    "    location: "*) loc=${line#    location: };;
    "    summary: "*) sum=${line#    summary: };;
    "---") [ -n "$id" ] && [ -n "$sum" ] && { echo "$id $sev $cls $loc | $sum"; id=; }; ;;
  esac
done < "$1"
[ -n "$id" ] && echo "$id $sev $cls $loc | $sum"
