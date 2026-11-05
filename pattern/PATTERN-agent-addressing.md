# PATTERN-agent-addressing

Agent names are formed only via `internal/agentname`, and messages to another session go only through creel.

## Agent messaging

Rule: Messaging another session from an agent: only through creel, never an engine's own tool, bar a fork addressed in-session under ForkSubagents. (test)

## Agent name

Rule: Forming, parsing or matching an agent name: only `internal/agentname`, a strand's full name formed once by reed; a spawned role is its module's constant.

Held by review alone; no test enforces it.

## Slack boundary

Rule: Calling Slack: only `internal/slackengine` calls the Web API, reading the bot token only from its machine credential file (`slack.json` in githubclient's credential directory), never from a config layer, `lym config env`, an environment variable or a repository; operator Slack text reaches an agent only as creel mail from `<shortname>:SLACK`, and an agent reaches Slack only by creel mail to that program.

Held by review alone; no test enforces it.
