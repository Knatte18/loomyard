# prowler

prowler is a Go-native replacement for Millhouse's `weblens` skill: it fetches pages the built-in `WebFetch` tool cannot read — bot-blocked sites, paywalls, JS-rendered content, and Reddit posts — by driving a real headless browser plus Mozilla-Readability-style extraction, and returns the result as readable markdown. It keeps the same any-repo/any-session reach as weblens because it ships as an installable Claude Code plugin, not a repo-scoped project skill.

## Install

1. `/plugin marketplace add <this LoomYard repo>`
2. `/plugin install prowler@loomyard`

Once installed, the `prowler` skill is available in every Claude Code session, the same way weblens is today.

## Build-on-first-run

prowler ships no compiled binary — LoomYard's `.gitignore` bans committing binaries. Instead, `scripts/run.sh` builds the nested Go module into `bin/prowler` (or `bin/prowler.exe` on Windows) the first time it is invoked, under a lock, and reuses that binary on every later call. This means:

- A Go toolchain must be on `PATH` at first use. The operator's Linux and Windows machines are Go dev boxes, so this is normally already satisfied.
- The first invocation is slower (it compiles chromedp, go-readability, and goquery); every later invocation just execs the cached binary.
- If a build is ever interrupted in a way that leaves a stuck lock (e.g. a hard-killed process), the wrapper reclaims a lock older than ~300 seconds automatically. If you ever need to clear one manually: `rm -rf plugins/prowler/bin/.build.lock`.

## Runtime prerequisite: Chrome/Chromium

The headless-browser fallback (used when a page is bot-blocked or JS-rendered and static extraction alone isn't enough) needs a local Chrome or Chromium install. prowler discovers it via the `CHROME_PATH` environment variable first, then a platform-specific candidate list (matching weblens' own discovery). If no browser is found, the browser fallback is simply skipped — the run still returns whatever the static-extraction path produced, with a note; it is never a hard failure of the whole invocation.

## License

Apache-2.0, per the `license` field in `.claude-plugin/plugin.json`. See the repository-root `LICENSE` file, which covers this plugin along with the rest of LoomYard.
