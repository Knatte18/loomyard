<!-- This is the orch role's on-demand file for the hub's Slack relay, rendered beside the role by RenderRoleFile (internal/orchengine/prompt.go). It carries no markers. -->
# Slack

The hub's Slack relay connects you to the operator's channel while the operator is away:
its mail comes from `<shortname>:SLACK`,
and mail you post to `<shortname>:SLACK` is posted to the channel.

- Mail from `<shortname>:SLACK` whose body opens `[slack]` carries the operator's own words.
  It has the same authority as a line typed in your pane.
  Answer it with `lym creel post --reply <id> "<text>"`, which reaches Slack.
- Mail from `<shortname>:SLACK` that opens `[slack presence]` tells you the new presence and is not answered.
- Mail from `<shortname>:SLACK` that opens `[slack relay]` is a notice about the relay itself.
  It carries no operator authority and is not answered.

You learn the presence from `[slack presence]` mail and from the presence line your resume pointer carries,
and `lym slack status` prints it.

While the presence is `slack`, also post each operator-bound message to `<shortname>:SLACK` with `lym creel post`:
a design question, a go-ahead you need, a status report, a relayed owner halt and an answer to the operator.
Leave out the attach command, which needs a terminal.
While the presence is `terminal`, post to `<shortname>:SLACK` only a reply to mail that came from it.
A beam orch's post to `<shortname>:SLACK` reaches the channel with an `@<slug>` answer hint,
and the operator's answer comes back to that beam's orch.
