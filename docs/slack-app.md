# The Slack app

Each machine that runs a Slack relay has its own Slack app, `lym-<machine>`.
The app holds a bot token and an app-level token, and no user token.
The bot makes and joins the channels itself, so the operator never adds it to a channel by hand.

## Manifest

Create the app from this manifest, with `<machine>` replaced by the machine's name:

```yaml
display_information:
  name: lym-<machine>
features:
  bot_user:
    display_name: lym-<machine>
    always_online: true
oauth_config:
  scopes:
    bot:
      - channels:history
      - channels:join
      - channels:manage
      - channels:read
      - chat:write
      - chat:write.customize
      - users:read
settings:
  event_subscriptions:
    bot_events:
      - message.channels
  org_deploy_enabled: false
  socket_mode_enabled: true
  token_rotation_enabled: false
```

The manifest has no user scope.
The app-level token's scope is not part of a manifest; it is chosen when the token is made.

## Steps

1. Create the app from the manifest above at `https://api.slack.com/apps`, under Create New App, From a manifest.
2. Install the app to the workspace.
3. Copy the bot token (`xoxb-…`) from the app's OAuth & Permissions page.
4. Make an app-level token on the app's Basic Information page, under App-Level Tokens, with the scope `connections:write`, and copy it (`xapp-…`).
5. Run `lym slack login` from the operator's terminal and give both tokens in one run.
   On a terminal it prompts for each token with echo off; otherwise it reads both from standard input, split at white space.
6. Run `lym slack setup --operator <user-id>` in each hub with the operator's Slack member id.

`lym slack login` refuses a token of any other prefix, and tokens of two different apps.
Replacing a stored token ends the relay of each live hub of the machine; run `lym slack start` in each to bring it back.
