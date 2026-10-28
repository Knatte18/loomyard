<!-- This is the wake line the `lym orch` watcher types into the orch once a usage limit that stopped it should have reset, rendered by RenderUsageLimitWake (internal/orchengine/prompt.go).
     It must render to ONE line: shuttle's Send refuses multi-line text.
     Its one marker is {{.reset}}, when the limit reset, or that its reset time could not be read. -->
[lym orch watcher] The usage limit that stopped this session {{.reset}}. Run `lym tender revive`, then check each run with `lym batten status <slug>` and restart what stopped.
