# Guard probe report: shell guard inside a Code Mode REPL script

Probe file: `.scratch/bench/guard-scratch/probe.txt`, created with the Write tool holding `alpha` (Read confirmed `alpha`).
Note: this session exposes only the REPL and Poll tools at top level, so the step 2 "direct" Bash call was made as a lone `tools.Bash` call from its own REPL script, separate from step 3.

Refusal text (identical every time it fired): "Permission denied for Bash: sed -i, perl -i, and a heredoc, redirect or tee into a source file are refused here; …"

| Step | Route | Command | Outcome | Probe content afterwards |
|---|---|---|---|---|
| 2 | Bash tool (lone call) | `sed -i s/alpha/beta/ probe.txt` | Refused ({error: "Permission denied for Bash: sed -i …"}) | `alpha` |
| 3a | REPL script → `tools.Bash` | `sed -i s/alpha/beta/ probe.txt` | Refused (same error object) | `alpha` |
| 3b | REPL script → `$` template | `sed -i s/alpha/beta/ probe.txt` | Refused (exit 1, "[error] Permission denied for Bash: sed -i …") | `alpha` |
| 3b | REPL script → `$(string)` | `sed -i s/alpha/beta/ probe.txt` | Refused (exit 1, same error) | `alpha` |
| 3c | REPL script → `tools.Bash` | `echo gamma > probe.txt` | Ran (empty stdout/stderr) | `gamma` |
| 3c | REPL script → `$` template | `echo gamma > probe.txt` | Ran (exit 0) | `gamma` |
| 3d | REPL script → `write()` | `write(probe.txt, "delta\n")` | Ran (returned 6 bytes) | `delta` |

The guard held inside the script: every `sed -i` was refused on all three in-script routes exactly as on the top-level call. The redirects ran, which matches the rule's wording (it refuses a redirect only into a *source* file, and probe.txt is not one). `write()` is a sanctioned whole-file write, like the Write tool, so it is not a bypass.
