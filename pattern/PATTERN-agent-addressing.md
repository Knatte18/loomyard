# PATTERN-agent-addressing

Agent names are formed only via `internal/agentname`, and messages to another session go only through creel.

## Agent messaging

Rule: Messaging another session from an agent: only through creel, never an engine's own tool, bar a fork addressed in-session under ForkSubagents. (test)

## Across hubs

Rule: Messaging a strand of another hub of the machine: only a strand posts and replies across hubs, and only to a strand; the receiving daemon confirms each delivery with the sending daemon before anything reaches an agent, a confirm that answers no drops the message and journals it, and authority goes only to the exact orch names of a hub whose repository name, read from its origin remote, `trusted_hubs` lists, on Linux.

Held by the tests in `internal/creelengine` and by review.

## Agent name

Rule: Forming, parsing or matching an agent name: only `internal/agentname`, a strand's full name formed once by reed; a spawned role is its module's constant.

Held by review alone; no test enforces it.

## Slack boundary

Rule: Calling Slack: only `internal/slackengine` calls Slack, over the Web API and Socket Mode, reading its bot and app-level tokens only from `slack.json` in githubclient's credential directory, never from a config layer, `lym config env`, an environment variable or a repository; operator Slack text reaches an agent only as creel mail from `<shortname>:SLACK` of what Slack returned, and an agent reaches Slack only by creel mail to that program.

Held by review alone; no test enforces it.
