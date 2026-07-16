#!/usr/bin/env python3
"""Sum token usage from a Claude Code session transcript (.jsonl).

Usage: usage_sum.py <transcript.jsonl> [...]

Prints one line per file plus a TOTAL line. Columns:
  in        raw input_tokens (uncached)
  cache_cr  cache_creation_input_tokens
  cache_rd  cache_read_input_tokens
  out       output_tokens
  compute   in + cache_cr + out  (headline: tokens actually processed fresh)
  turns     assistant messages carrying a usage block
"""
import json
import sys


def sum_file(path):
    tot = {"in": 0, "cache_cr": 0, "cache_rd": 0, "out": 0, "turns": 0}
    models = set()
    with open(path, encoding="utf-8") as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            try:
                d = json.loads(line)
            except json.JSONDecodeError:
                continue
            if d.get("type") != "assistant":
                continue
            msg = d.get("message", {})
            u = msg.get("usage")
            if not u:
                continue
            tot["in"] += u.get("input_tokens", 0)
            tot["cache_cr"] += u.get("cache_creation_input_tokens", 0)
            tot["cache_rd"] += u.get("cache_read_input_tokens", 0)
            tot["out"] += u.get("output_tokens", 0)
            tot["turns"] += 1
            if msg.get("model"):
                models.add(msg["model"])
    tot["compute"] = tot["in"] + tot["cache_cr"] + tot["out"]
    tot["models"] = ",".join(sorted(models))
    return tot


def main(argv):
    if len(argv) < 2:
        print(__doc__, file=sys.stderr)
        return 2
    cols = ["in", "cache_cr", "cache_rd", "out", "compute", "turns"]
    grand = dict.fromkeys(cols, 0)
    hdr = f"{'file':<44}" + "".join(f"{c:>10}" for c in cols) + "  models"
    print(hdr)
    for path in argv[1:]:
        t = sum_file(path)
        name = path.rsplit("/", 1)[-1]
        print(f"{name:<44}" + "".join(f"{t[c]:>10}" for c in cols) + f"  {t['models']}")
        for c in cols:
            grand[c] += t[c]
    if len(argv) > 2:
        print(f"{'TOTAL':<44}" + "".join(f"{grand[c]:>10}" for c in cols))
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
