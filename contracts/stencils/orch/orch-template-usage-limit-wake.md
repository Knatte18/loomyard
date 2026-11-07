<!-- This is the wake line the `lym orch` watcher types into the orch once a usage limit that stopped it should have reset.
     RenderUsageLimitWake (internal/orchengine/prompt.go) renders it.
     It must render to ONE line: shuttle's Send refuses multi-line text.
     Its one marker is {{.reset}}, when the limit reset, or that its reset time could not be read. -->
[lym orch watcher] The usage limit that stopped this session {{.reset}}. Run `lym tender revive`, then wait for the notices of the runs that stopped; read a filtered `lym batten status <slug>` only for a stop a notice names, and handle that stop as your role says.
