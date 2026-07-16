#!/usr/bin/env python3
"""Harvest a session's post-marker output from its transcript.

Usage: harvest.py <transcript.jsonl> <marker-substring>

Forked transcripts copy the parent's full history, so measurement must start
at the fork point: the first user message containing <marker-substring>.
Prints a one-line JSON summary (post-marker usage, tool calls, models),
then the final assistant text. Pass a marker matching the session's own
first prompt for cold (non-fork) sessions too — then it measures the whole
session.
"""
import json
import sys


def main(argv):
    if len(argv) != 3:
        print(__doc__, file=sys.stderr)
        return 2
    path, marker = argv[1], argv[2]
    after = False
    tools = 0
    last = ""
    models = set()
    u = {"in": 0, "cache_cr": 0, "cache_rd": 0, "out": 0, "turns": 0}
    with open(path, encoding="utf-8") as f:
        for line in f:
            try:
                d = json.loads(line)
            except json.JSONDecodeError:
                continue
            if d.get("type") == "user":
                m = d.get("message", {}).get("content")
                txt = m if isinstance(m, str) else " ".join(
                    c.get("text", "") for c in (m or []) if isinstance(c, dict))
                if marker in txt:
                    after = True
            if not after or d.get("type") != "assistant":
                continue
            msg = d.get("message", {})
            if msg.get("model"):
                models.add(msg["model"])
            for c in msg.get("content", []):
                if c.get("type") == "text":
                    last = c["text"]
                elif c.get("type") == "tool_use":
                    tools += 1
            usage = msg.get("usage")
            if usage:
                u["in"] += usage.get("input_tokens", 0)
                u["cache_cr"] += usage.get("cache_creation_input_tokens", 0)
                u["cache_rd"] += usage.get("cache_read_input_tokens", 0)
                u["out"] += usage.get("output_tokens", 0)
                u["turns"] += 1
    u["compute"] = u["in"] + u["cache_cr"] + u["out"]
    u["tool_calls"] = tools
    u["models"] = sorted(models)
    u["found_marker"] = after
    print(json.dumps(u))
    print(last)
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
