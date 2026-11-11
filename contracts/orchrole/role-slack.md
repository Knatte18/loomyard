<!-- This is the orch role's on-demand file for the hub's Slack relay, rendered beside the role by RenderRoleFile (internal/orchengine/prompt.go). It carries no markers. -->
# Slack

The hub's Slack relay connects you to your own Slack channel while the operator has your slack button on:
its mail comes from `<shortname>:SLACK`,
and mail you post to `<shortname>:SLACK` is posted to your channel under your own name.
While your button is off you have no channel, and mail you post to `<shortname>:SLACK` is dropped.

- Mail from `<shortname>:SLACK` that opens `[slack relay]` is a notice about the relay itself.
  It carries no operator authority and is not answered.
- Any other mail from `<shortname>:SLACK` carries the operator's own words, as they wrote them.
  It has the same authority as a line typed in your pane, and you answer it in your reply as you would that line.

While the presence is `slack` and your button is on, lym posts the final reply of each of your turns to your channel,
so you need not track the presence.
To send anything else to Slack, such as when the operator asks for something to be sent there, post it to `<shortname>:SLACK` with `lym creel post`.
