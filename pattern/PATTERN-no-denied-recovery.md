# PATTERN-no-denied-recovery

A way forward is performed by the agent that receives the refusal, and that agent's settings deny some commands.
A refusal, stencil, spec or skill that offers one of them as a step strands the agent: it either breaks its settings or stops.
When a recovery needs a denied step, lyx performs the step itself behind a guarded verb, such as `lyx webster reset --to start|pre-fix`, and the text names that verb.

- The closed list of denied forms is `git reset --hard`, `git push --force`, `git push -f`, `rm -rf`, `python` and `python3`, the deny set today.
  `claudeengine`'s deny table in `internal/shuttleengine/claudeengine/settings.go` owns lyx's own denies, and the operator's permission set owns the rest; the list in `cmd/lyx/norecoverydeny_test.go` follows both.
- The raw `go` deny in `claudeengine`'s table is pattern-shaped and stays outside this closed list and its token scan, because specs legitimately describe the plan verify commands Go runs.
  No refusal, stencil or spec names a raw module-wide `go` run as a way forward; the way forward is `lyx gate test`.
- A form matches only as a whole command token sequence, so `git push --force-with-lease` is not `git push --force`.
- The scanned surface is `contracts/specs/`, `contracts/stencils/`, `plugins/ly/skills/` and the string literals of the non-test Go files under `internal/webster*`, `internal/shed*` and `internal/loom*`, the modules with a refusal-spec section.
  `pattern/` itself is unscanned, because these background files name the denied forms to explain the rule.
  Test files are unscanned too, since they plant the forms on purpose.
- The one exemption marker is `never ` immediately before the form, case-insensitive, with an optional backtick between: it marks the form as forbidden.
  The marker covers the form only as the direct object of "never", a prohibition;
  a denied form offered later in the same sentence, or preceded by anything else, still fails.
- `git reset --keep <sha>`, `git checkout <head> -- <path>` and a plain `rm <path>` are not denied and stay valid steps.
- The scan reports each hit with file and line, and a self-test plants the shapes above to pin it.
