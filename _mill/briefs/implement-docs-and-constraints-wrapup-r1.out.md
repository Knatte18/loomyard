No dirty tracked files. Both card commits (45, 46) present in the log, matching the batch's 2 declared cards. Card-count self-check: 2 of 2 cards committed — all complete.

{"status":"success","commit_sha":"2011b10917577c5e13d1ca051a969f3bac7595b5","session_id":"5fd9e06a-7765-49ec-8ab0-88a6ee4fce60","cards_done":[45,46]}

Summary: 2 of 2 cards committed for batch 14 (docs-and-constraints-wrapup). `verify: null` for this batch, so no test command was run.

- `/home/knatte/Code/loomyard/wts/trace-logging/docs/shared-libs/README.md` — rewrote the `internal/logger` bullet (Card 45, commit `dbaf6d6e`) to name the trace ID / span surface and the durable Info+ sink (`.lyx/logs`, 8 MiB cap, age+count retention) plus `LYX_TRACE=1`.
- `/home/knatte/Code/loomyard/wts/trace-logging/CONSTRAINTS.md` — extended the "Live-Substrate Spawn Observability" entry (Card 46, commit `2011b109`) noting the durable sink removes the `LYX_LOG_LEVEL`/`LYX_LOG_FILE` precondition for spawn/teardown events, pointing to `internal/logger`'s package doc for the level policy, and adding `internal/scoutengine/ensureserver.go` to the known call-sites list (explicitly did not add `internal/perchengine/engine.go`, per the card's instruction).
- Confirmed `docs/overview.md:190` needed no edit (plain package-name list, no behavioral description).

{"status":"success","commit_sha":"2011b10917577c5e13d1ca051a969f3bac7595b5","session_id":"5fd9e06a-7765-49ec-8ab0-88a6ee4fce60","cards_done":[45,46]}
