# A closed area for lym: sandboxing options, 2026-10-30

How to run lym's whole process tree inside a closed area on the operator's Linux machines, so that agents keep bypass permissions inside it and reach the rest of the machine only through a short list of exits.
The report is a snapshot: it reads the lym source at the commit that adds it, Claude Code 2.1.314 as installed on OSL-1046, and each tool's documentation as fetched on 2026-10-30.
It changes no lym code and no machine setting.

## Answer in brief

Build the closed area on `bwrap`, launched and supervised by lym itself, with reed's tmux server as the area's console and a host-side lym process as its only door: an egress proxy with a host allowlist and a broker that serves a fixed set of verbs.
The operator's `lym reed attach` keeps working from outside, because a tmux client outside can drive a tmux server inside through a bound socket directory; a probe on OSL-1046 shows it.
On Ubuntu 24.04 `bwrap` needs a one-time AppArmor profile; Ubuntu 25.04 and later ship one.
The work is about five board tasks and eighteen Webster cards.
Leave out the dedicated user, podman and systemd user-scope sandboxing; use Claude Code's own `/sandbox` nowhere in lym runs, since it cannot nest inside the area and does not close the area by itself.

## Threat model

The attacker is an agent session that runs hostile commands, through a prompt injection in something it read or through a bad change it was asked to review.
The area holds when that session cannot:

- act outside the area: start a process, type into a window or a pane, or send a D-Bus call outside it;
- read a secret outside the area: SSH keys, the operator's GitHub token, the operator's own Claude credentials, other hubs;
- persist code that later runs outside the area without the operator's say: a git hook, a Claude Code hook or plugin, a shell rc file, a binary on `PATH`, a poisoned Go cache.

Out of scope: a kernel exploit, which every method on this list shares the host kernel with, and leaking data the area is allowed to hold through a host the allowlist lets through.
A proxy that checks host names only cannot tell the operator's GitHub account from an attacker's account on the same host, and Claude Code's own documentation says the same of its proxy.

## What lym's tree needs

Every agent lym starts carries `--dangerously-skip-permissions`: `validatePermissionMode` in `internal/shuttleengine/claudeengine/command.go` resolves the empty mode and `bypass` to the flag, and refuses `prompt` on an autonomous run because a prompt with nobody to answer stalls the run.
So the area has to make bypass safe, never replace it with prompts.

| Part | Where it lives today | What it needs from the area |
|---|---|---|
| reed's tmux server | one per hub, named from the hub path (`ServerName` in `internal/reedengine/server.go`), socket under `$TMUX_TMPDIR` else `/tmp/tmux-<uid>` | a socket directory the operator's attach can reach from outside |
| Agent sessions | Claude Code panes in that server, launched by shuttle | `~/.claude`, `~/.claude.json`, the Claude binary, `api.anthropic.com` and OAuth hosts |
| creel daemon | one per hub, socket `creel-<shortname>-<h>.sock` in lym's temp root `/tmp/lym-<uid>` (`internal/creelengine/doc.go`, `internal/lymtemp/doc.go`) | its socket inside the area; it identifies a strand by `SO_PEERCRED` and the process ancestry, which a shared PID namespace keeps true |
| dobby | one per beam worktree, spawned detached (`internal/dobbyengine/doc.go`) | the hub's worktrees and `.lym` state |
| Gate runs | `lym gate test`, Go builds with cgo, tmux-tier tests | the Go toolchain, a module cache, a build cache, `gcc`, `proxy.golang.org` and `sum.golang.org` |
| Fabric | in-process git on both sides of each pair (`internal/fabricengine`) | the whole hub: worktrees, records worktrees, the prime's `.git`, `_board`; pushes and fetches to GitHub |
| GitHub API | `internal/githubclient` resolves `GH_TOKEN`, `GITHUB_TOKEN`, a cache, then `gh auth token` | a token and `api.github.com` |

lym also reaches the host desktop and system from inside its tree today:

- `lym reed window` opens Konsole running `lym reed attach` (`internal/termwindow/termwindow.go`), and needs `DISPLAY` or `WAYLAND_DISPLAY`.
- `lym ide` runs `code <worktree>` (`internal/vscode/launch_linux.go`).
- `lym hibernate request` runs `systemctl hibernate` through the system bus (`internal/hibernateengine/verbs.go`).
- `lym report timing` reads `journalctl` for sleeps (`internal/reportcli/timing.go`); it degrades when the journal is unreadable.
- Batten's Land row runs the hub's landing `deploy` command in the prime (`internal/battenshed/land.go`), which on this hub is `tools/deploy`: it writes `lym` into the Go bin dir and the plugins into the Claude Code plugin cache (`tools/deploy/main.go`).

Two facts constrain every method:

- **Absolute paths are load-bearing.**
  The tmux server name, the creel socket name, git's worktree links and Claude Code's transcript directory (`<home>/.claude/projects/<worktree path>`, read by `TranscriptReader` in `internal/shuttleengine/claudeengine`) all derive from absolute paths, so the area must show the hub at the same path it has on the host.
- **lym writes git hooks and git config itself.**
  `InstallPostCheckoutHook` in `internal/fabricengine/hook.go` writes the post-checkout hook at clone and add, a non-fatal step that finds an installed hook by its sentinel and writes nothing then.
  `internal/gitrepo/branchmark.go` writes `branch.<b>.lymLocal` into the repository config, and `git worktree add` and tracking setup write it too.
  So `.git/hooks` can be read-only inside the area once the hook is installed, but `.git/config` cannot be.

## Probes on OSL-1046

OSL-1046 runs Ubuntu 24.04.5 with kernel 7.0.0-42 (HWE), bubblewrap 0.9.0, AppArmor 4.0.1, systemd 255, tmux 3.4, and `kernel.apparmor_restrict_unprivileged_userns = 1`.
`landrun`, `podman`, `slirp4netns` and `pasta` are not installed, `socat` is.
No `bwrap` AppArmor profile is loaded, and the `apparmor-profiles` package that carries `bwrap-userns-restrict` is not installed.

This session runs under the AppArmor label `vscode (unconfined)`, and so does the hub's tmux server: the server inherited the label of the VS Code terminal it was booted from, and `/etc/apparmor.d/code` grants `userns`.
A fresh login shell or a Konsole window is plain `unconfined` instead, so each namespace probe was also run from an unconfined `systemd-run --user` service.

| Probe | Label | Result |
|---|---|---|
| `bwrap --unshare-all ... true` | `vscode` | works |
| `unshare --user --map-root-user --mount`, then `mount -t tmpfs` | `vscode` | works |
| the same `unshare` | `unconfined` | `write failed /proc/self/uid_map: Operation not permitted` |
| `bwrap --unshare-all ... true` | `unconfined` | `loopback: Failed RTM_NEWADDR: Operation not permitted` |
| `bwrap --unshare-user --unshare-pid ...` (no network namespace) | `unconfined` | `setting up uid map: Permission denied` |
| `systemd-run --user -p ProtectSystem=strict -p ProtectHome=read-only -p PrivateTmp=yes` | `unprivileged_userns` | starts; `/etc` is unwritable only through file permissions, `$HOME` stays writable and `/tmp` stays shared, so the namespace properties were silently dropped |
| `systemd-run --user -p IPAddressDeny=any`, then TCP to `1.1.1.1:443` | `unconfined` | connects: the user manager does not apply IP filtering |
| tmux server run with `tmux -S <sock> -D` inside `bwrap --unshare-all`, hub bound read-write, `/tmp` and `/run/user` as tmpfs; session created and listed by a tmux client outside | `vscode` | works; inside, `/run/user` and `/tmp` are empty, the only interface is `lo`, TCP to `1.1.1.1:443` fails, 7 processes are visible, `$HOME` is read-only and the session bus is unreachable |

The last row is the design's load-bearing fact: a tmux client outside drives a server inside, so the operator watches and types into the area without being in it.
The rows marked `unconfined` are what the area launcher meets when it starts from a Konsole window or a systemd user unit on Ubuntu 24.04 without a `bwrap` profile.

Two side findings, true today with no area:

- Every agent of a hub booted from a VS Code terminal may create user namespaces, an attack surface the Ubuntu restriction exists to remove.
- An agent today can reach `/run/user/<uid>/bus` and, through it, `systemd-run --user`, which starts any process outside lym's tree; it can also reach other hubs' tmux servers in `/tmp/tmux-<uid>` and type into their panes.
  On this machine that directory holds the sockets of three hubs, the Models hub among them.

## The six methods

### bwrap

**Requirements.**
Ubuntu 24.04 needs an AppArmor profile that lets `/usr/bin/bwrap` create user namespaces, loaded once by root.
The cleanest is AppArmor's own `bwrap-userns-restrict` from the `apparmor-profiles` package's extra profiles, copied to `/etc/apparmor.d/` and loaded with `apparmor_parser -r`.
It lets `bwrap` itself hold capabilities in its namespace and stacks every child under `unpriv_bwrap`, which denies all capabilities, so `bwrap` cannot be used to lift the restriction for anything else.
Ubuntu 25.04 and later ship that profile in the `apparmor` package, so Neve on 26.04 should need nothing; that is documented by secondary sources and not verified on Neve.
Turning `kernel.apparmor_restrict_unprivileged_userns` off works too, but undoes the restriction for every program.
bubblewrap is in both releases' archives; kernel 7.0 on both machines has every namespace bwrap uses.

**How the tree starts.**
A new host-side verb, `lym area start`, runs `bwrap` with `--unshare-all --die-with-parent --clearenv`, a fixed environment and a fixed bind set, and inside it a supervisor, `lym area supervise`, that boots the hub's tmux server and stays alive as long as it runs.
The PID namespace ends, and every process in it with it, when the supervisor or the host process exits, so the area cannot outlive its door.
Running `lym area start` as a systemd user unit gives the area one cgroup for `MemoryMax=` and `TasksMax=` and one `systemctl --user stop` to end it; that is the outline's cgroup scope, and it is resource control, not confinement.

The bind set:

- `/usr`, `/etc`, `/opt` read-only; `/proc` new, `/dev` minimal, `/tmp` and `/run/user` tmpfs.
- The hub, read-write, at its own path; the prime's and the records repository's `.git/hooks` read-only on top.
- `$HOME` as a tmpfs, with read-only binds of the Go toolchain, the Claude Code binary directory and the installed `lym`.
- The area's own Claude config directory, Go module cache and Go build cache, bound at `~/.claude`, `~/.claude.json`, `~/go/pkg/mod` and `~/.cache/go-build`, so paths stay the same and no lym code changes for them; the host's plugin cache read-only on top of the area's `~/.claude/plugins`.
- One host directory, for example `$XDG_RUNTIME_DIR/lym-area/<hub hash>/`, bound at the same path, holding the tmux socket directory (`TMUX_TMPDIR`), the proxy socket and the broker socket.

Inside, reed, creel, dobby, the orch and every run start as they do now.
Agents hold no capabilities, see only the area's processes and share the area's creel daemon, whose `SO_PEERCRED` reading stays true because the daemon and its clients share the PID namespace.
The operator attaches from outside with `lym reed attach`, given the area's `TMUX_TMPDIR`, and types lym commands into a shell window of that server, which runs inside the area.

**What stops working.**

- Anything that needs the host's desktop or system bus, until the broker serves it: `lym reed window`, `lym ide`, `lym hibernate request`.
- `lym report timing` reads no journal, and degrades as it does today.
- WebFetch, and so the `prowler` skill, reaches only allowlisted hosts; research runs that fetch arbitrary pages need a wider allowlist or run outside the area.
  WebSearch keeps working, since it runs on Anthropic's side.
- Claude Code's auto-updater and `/sandbox` inside the area: `unpriv_bwrap` denies the capabilities a nested `bwrap` needs, so `sandbox.enabled` would fail, or with `failIfUnavailable` refuse to start.
- `lym creel` from a host shell, since the creel socket lives in the area's `/tmp`; the operator posts from the attach shell instead.

**Existing implementation to borrow from.**
Anthropic's `sandbox-runtime` (`srt`) wraps a process in the same `bwrap` with a host-side proxy reached through a socket bound into a network namespace with no interfaces, which is the network half of this design.
It is a beta with a changing configuration format, and on Linux its Unix-socket control is all or nothing (`allowAllUnixSockets`), which tmux and creel force on; the area would then rest on filesystem hiding alone, which is the design above anyway.
A launcher in lym keeps the bind set, the supervisor and the fail-closed checks under lym's own tests.

**lym changes.**

- New `internal/areaengine` and `internal/areacli`: the bind set, the environment, the `bwrap` argument list, a start probe that refuses on any failure, the supervisor, and the `lym area start|status|stop` verbs wired in `cmd/lym/main.go`.
- `internal/reedengine` and `internal/hubgeom`: the area's `TMUX_TMPDIR` for boot and attach, and a refusal to boot a hub's server outside an area when the hub's config requires one.
- `internal/configengine` template: an `area` config module with `required` and the egress allowlist.
- The egress proxy, host side, and a loopback relay inside the area that forwards `127.0.0.1:<port>` to the proxy socket.
- The broker, host side, and its client verbs, routed from `internal/termwindow` callers, `internal/ideengine`, `internal/hibernateengine` and `internal/battenshed`'s Land row.
- `README.md`, `docs/overview.md`, the new packages' `doc.go`, and a `PATTERN.md` entry: reaching the host from inside an area goes only through the broker's verbs.

**Estimate.**
Five board tasks, about eighteen cards: launcher and supervisor (five), area-aware boot (three), egress proxy and relay (three), broker and its routed verbs (five), docs and the PATTERN entry (two).

**Verdict.**
Closes every exit in the threat model on both releases, fails closed, keeps lym's tree and the operator's attach as they are, and costs one root step on 24.04 only.

### Landlock through a wrapper

**Requirements.**
Landlock is unprivileged, needs no user namespace and so no AppArmor profile; it is in `/sys/kernel/security/lsm` on OSL-1046.
`landrun` packages it as a CLI; the `landrun` README lists it in `apt` for Ubuntu 26.04, and on 24.04 it installs with `go install`.
Kernel 7.0 gives Landlock ABI 8 by the `landrun` README's table: filesystem rules, TCP bind and connect by port (ABI 4), and scoping of abstract Unix sockets and signals (ABI 6).
Pathname Unix-socket control is ABI 9 and UDP control ABI 10 (kernel 7.2), both newer than 7.0.

**How the tree starts.**
`landrun` with `--rw` on the hub and the caches, `--rox` on the toolchain and Claude, `--connect-tcp` on the proxy port only, wraps the supervisor; every child inherits the domain.
Paths stay where they are, so the operator's attach works through the shared socket directory with no change.
Landlock cannot remount, so the area's own Claude config directory, Go module cache and Go build cache need `CLAUDE_CONFIG_DIR`, `GOMODCACHE` and `GOCACHE`, and `TranscriptReader` in `internal/shuttleengine/claudeengine`, which reads `<home>/.claude/projects` today, must follow `CLAUDE_CONFIG_DIR`.

**What stops working, and what stays open.**
On kernel 7.0 the area does not close:

- A process inside can still `connect()` to `/run/user/<uid>/bus` and to other hubs' tmux sockets, since connecting to a pathname socket is outside Landlock's rights before ABI 9; `systemd-run --user` through that bus is a full escape.
- UDP is unrestricted, so DNS and any UDP protocol leave the area.
- A TCP connect rule is per port, not per host, so allowing the proxy's port allows that port on every host.
- Go 1.24 and later listen with Multipath TCP by default, which Landlock's TCP rules do not cover.

Landlock does block abstract sockets such as X11's `@/tmp/.X11-unix/X0`, signals into the host, and ptrace and `/proc/<pid>/environ` of processes outside the domain.

**lym changes.**
The same launcher, broker and proxy as `bwrap`, with a Landlock rule set in place of the bind set, plus the `CLAUDE_CONFIG_DIR` change in `internal/shuttleengine/claudeengine`; about five tasks and seventeen cards.

**Verdict.**
Not a closed area on these kernels.
A good inner layer later, around each agent session, once both machines run a kernel with ABI 9 and 10, because it nests under `bwrap` without a user namespace.

### A dedicated Linux user

**Requirements.**
Root, once: a local user (OSL-1046 resolves users from AD, and a local account beside them works), an nftables rule matching `meta skuid` that lets the user reach only the loopback proxy, and a sudo or polkit rule that lets the operator start the area as that user.
Nothing release-specific: it works the same on 24.04 and 26.04.

**How the tree starts.**
`sudo -u lymarea lym area start`, with the tree running as that user; ordinary file permissions keep it out of the operator's home, `/run/user/<operator uid>` and `/tmp/tmux-<operator uid>`.
The operator attaches through tmux's `server-access` (tmux 3.3 and later) on a socket in a group-shared directory.
The area user has its own `~/.claude`, so it logs in to Claude once on its own, and its own Go caches.

**What stops working.**

- The hub must belong to the area user, or carry ACLs for it; files the agents write then belong to another user in the operator's tree.
- Git run by the operator refuses repositories owned by another user (`safe.directory`), which protects the host from the area's hooks but breaks VS Code's git and the operator's own git until the operator lists the hub as safe, which reopens the hook exit.
- `internal/lymtemp` refuses a temp root not owned by the current user, and reed's socket handling assumes one user per server; both need changes for a shared directory.
- With no namespaces, nothing hides a host path beyond file permissions: `/usr`, `/etc` and every world-readable file under `/home` stay readable to the area.

**lym changes.**
The launcher as user switch instead of namespaces, the broker on a group socket, reed attach to another user's server, `internal/lymtemp` and file modes for a shared group; about five tasks and fifteen cards, plus the root setup.

**Verdict.**
Strong separation through the oldest Unix mechanism, but the hub's ownership churns, and the host git exit cannot be both safe and usable.
Leave it out.

### systemd sandboxing properties on a user scope

**Requirements and probes.**
A transient scope (`systemd-run --user --scope`) takes only resource-control properties, since it adopts a process that already exists.
A transient user service takes the sandboxing properties, but the user manager applies them through an unprivileged user namespace, which Ubuntu's restriction strips of capabilities: the probe above started with `ProtectHome=read-only` and `PrivateTmp=yes` and applied neither, and `IPAddressDeny=any` was ignored.
So on both releases the method fails open: the unit starts and the area is not there.

A system unit run as the operator's user (`User=`), installed by root once with a polkit rule to start it, does apply them: `ProtectHome=tmpfs` with `BindPaths=`, `PrivateTmp=yes`, `InaccessiblePaths=/run/user`, `PrivateNetwork=yes` with a socket-activated `systemd-socket-proxyd` or lym's proxy for egress.
A PID namespace (`PrivatePIDs=`) needs systemd 257 or later, so Neve's systemd 259 has it and OSL-1046's 255 does not; without it, the area sees the operator's processes and can read their environment, `GH_TOKEN` included, since Yama's ptrace scope 1 limits attaching, not reading.

**lym changes.**
The system-unit variant needs the launcher as a unit template, the broker and the proxy, and a second code path per systemd version; about five tasks and fourteen cards, plus root.

**Verdict.**
The user-scope method fails open, so leave it out.
The system-unit variant is viable on 26.04 only and duplicates what `bwrap` does on both.

### A rootless container (podman)

**Requirements.**
podman is not installed on OSL-1046.
Ubuntu 24.04's `apparmor` package already carries a `podman` profile granting `userns` (`/etc/apparmor.d/podman` is present here), so the namespace restriction is handled.
Rootless podman needs a subordinate ID range in `/etc/subuid` and `/etc/subgid`, which is empty on OSL-1046, so root must add one; AD users can get them through SSSD's `subid` plugin only on SSSD versions and providers the podman documentation does not list for AD.
Networking is pasta (from `passt`) in current podman.

**How the tree starts.**
An image with Go, gcc, git, tmux, Claude Code and `lym`, run with `--userns=keep-id` so files keep the operator's owner and Claude Code accepts `--dangerously-skip-permissions`, which it refuses as root.
The hub, the socket directory and the area's caches are mounted at their host paths; `--network=none` with the proxy socket mounted gives the same egress as `bwrap`, since podman has no host allowlist of its own.
The operator attaches through the mounted socket directory, as with `bwrap`.

**What stops working.**
The same as `bwrap`, plus the image: Go, Claude Code and `lym` inside it must track the host's versions, and a deploy inside the area has nowhere persistent to go.

**lym changes.**
The `bwrap` set with `podman run` in place of `bwrap`, plus a maintained image; about five tasks and twenty cards, plus root for subordinate IDs and the install.

**Verdict.**
Everything `bwrap` gives, plus a default seccomp profile, at the cost of an image to maintain and two root steps.
Leave it out.

### Claude Code's own `/sandbox`, permissions and managed settings

**What it is.**
`/sandbox` wraps each Bash, PowerShell and Monitor command, and its children, in `bwrap` with a host-side proxy and an optional seccomp filter.
Read, Edit, Write, WebFetch, hooks, MCP servers and the status line run outside it, governed by permission rules; deny rules hold in every mode, `bypassPermissions` included.
`sandbox.failIfUnavailable` refuses to start without the sandbox, `allowUnsandboxedCommands: false` removes the `dangerouslyDisableSandbox` escape, which bypass mode otherwise runs without a prompt, and managed settings, written by root, keep a user from widening the policy.
It has the same AppArmor requirement as `bwrap` on 24.04.

**How the tree runs under it.**
lym's own processes stay on the host: the tmux server, dobby, the creel daemon, the orch watcher and every hook lym installs.
Only the Bash commands of each agent are confined, so the method is not a closed area by construction.
It also cannot confine lym's agents usefully:

- The loom driver drives a run through Bash, with `lym shed step`, which spawns panes on the hub's tmux server, so the driver's Bash must reach that server's socket; any agent that can reach it can open a window outside the sandbox, which is the escape this report exists to close.
- On Linux the sandbox's Unix-socket control is all or nothing, and creel and tmux need sockets.
- Writes outside the working directory need `allowWrite` for the hub, the records worktree, `_board/.lym` and the prime's `.git`, and the sandbox's mandatory deny on `.git/config` breaks `internal/gitrepo/branchmark.go`.
- `allowedDomains` does not limit WebFetch, so in bypass mode WebFetch can carry data to any host unless it is denied outright.
- Auto mode, the documented middle ground between prompts and bypass, falls back to prompting after three blocks in a row or twenty in total, and a prompt with nobody to answer stalls an autonomous run.

**lym changes.**
Rendering `sandbox` keys and deny rules into the settings shuttle passes with `--settings` (`internal/shuttleengine/claudeengine/settings.go`), and a managed-settings file the operator installs: about one task and four cards.

**Verdict.**
Not a closed area, and it cannot run inside one, because the `bwrap` area denies a nested `bwrap` its capabilities.
Its permission half is still worth having inside the area: deny rules in shuttle's settings for paths the area should never be asked to touch cost little and hold in bypass.

## The exits

Each exit is the smallest grant that keeps the threat model, with the methods that can give it.

| Exit | Smallest controlled grant | Methods that can give it |
|---|---|---|
| Network | No network namespace route; a host-side proxy, reached through a bound socket, allows CONNECT to `api.anthropic.com`, `claude.ai`, `platform.claude.com`, `proxy.golang.org`, `sum.golang.org`, `github.com` and `api.github.com`; no DNS inside, so names resolve at the proxy; Claude Code's telemetry and auto-updater off by `DISABLE_TELEMETRY` and `DISABLE_AUTOUPDATER` | `bwrap`, podman, systemd system unit; a dedicated user through nftables; Landlock only by port and TCP |
| GitHub token | A fine-grained token limited to the hub's repositories, with contents, issues and pull requests, handed to the area as `GH_TOKEN`; the operator's `gh` login and `~/.config/gh` never enter | all |
| Push | `git push` over HTTPS through the proxy with that token; the alternative, a broker that pushes from the host, runs host git over a repository the area writes and so reopens the hook exit | all |
| Claude config dir | The area's own `~/.claude` and `~/.claude.json`, logged in once inside the area, with the host's plugin cache read-only on top; never the host's, whose `settings.json` hooks and plugins run in the operator's own sessions | `bwrap` and podman by remount; Landlock by `CLAUDE_CONFIG_DIR`; dedicated user by its own home |
| Landing deploy | A broker verb: the host side fetches `origin/main` into its own clone, checks the requested commit is that tip, builds with the host's Go and installs; nothing from the area's tree or caches reaches the build | any method with a broker |
| Desktop windows | A broker verb taking a worktree under the hub and a title, which runs Konsole with the host's `lym reed attach` against the area's socket directory; never a command line from the area | any method with a broker |
| VS Code | None that is both safe and full: VS Code on the host runs git over the area's repositories and reads workspace settings the area can write; the broker refuses `lym ide` with that reason, and a later option is `code serve-web` inside the area, served to a host browser | none today |
| Host git over a repository | None: a repository's `.git/config` can name `core.fsmonitor`, `core.hooksPath`, filters and more, and lym needs that file writable; the host runs no git over the area's repositories, and the deploy broker uses its own clone | none |
| Hibernate | A broker verb that runs `systemctl hibernate` after the area's pause is confirmed | any method with a broker |

Two exits stay open by design, and the report's claim must say so:

- **Landed code for this hub.**
  The loomyard hub develops lym, and a deploy runs agent-written, landed code on the host: as the operator's `lym`, as the broker, and as plugins in the operator's Claude sessions.
  The area protects the host during a run, not from a change the operator lands and deploys; the broker can require the operator's approval before each deploy, which is the only control left.
- **Data through allowed hosts.**
  `github.com` with an attacker's token and `api.anthropic.com` with an attacker's key both carry data out, and a host-name proxy cannot tell them apart from the operator's own.

## lym changes per method

| Method | Packages that change | Tasks | Cards | Root steps |
|---|---|---|---|---|
| `bwrap` | new `areaengine`, `areacli`; `reedengine`, `hubgeom`, `configengine`, `termwindow` callers, `ideengine`, `hibernateengine`, `battenshed` | 5 | 18 | AppArmor profile, 24.04 only |
| Landlock | the `bwrap` set without remounts, plus `claudeengine` for `CLAUDE_CONFIG_DIR` | 5 | 17 | none |
| Dedicated user | the `bwrap` set with a user switch, plus `lymtemp`, `reedengine` attach | 5 | 15 | user, nftables, sudo or polkit |
| systemd system unit | the `bwrap` set as a unit template, two systemd versions | 5 | 14 | unit, polkit |
| podman | the `bwrap` set with `podman run`, plus an image | 5 | 20 | install, subordinate IDs |
| Claude `/sandbox` and permissions | `claudeengine` settings rendering | 1 | 4 | managed settings |

The counts are estimates at the size of today's Webster cards, and each includes its tests at the lowest tier that fits.

## What the repository can state

Today, without an area, the honest statement is that every agent lym starts runs with `--dangerously-skip-permissions`, and that lym belongs on a machine or virtual machine the operator is willing to hand to its agents.
`README.md` does not say so yet.

Once the `bwrap` area ships, the repository can state, for that method only:

- On Linux, `lym area start` runs the whole lym process tree, agents included, inside a bubblewrap sandbox, and nothing starts when the sandbox cannot.
- Inside it, agents keep bypass permissions; outside it, they reach only the hosts on the hub's allowlist and the broker's named verbs.
- It is tested on Ubuntu 24.04 with the `bwrap` AppArmor profile, and on Ubuntu 26.04.

It cannot claim, for any method:

- isolation from a kernel exploit, which needs a virtual machine;
- that data in the area cannot leave through an allowed host;
- that a landed change of lym itself cannot run on the host once deployed;
- anything on Windows or macOS, or for a hub run outside an area.

## Recommendation

1. **Build the `bwrap` area first.**
   It is the one method that closes every exit in the threat model on both machines, it fails closed, it keeps paths and the operator's attach as they are, and its only root step is a profile that Ubuntu 25.04 and later already ship.
   Within it, build the launcher and supervisor first, then the area-aware boot with its fail-closed refusal, then the egress proxy, then the broker's verbs in the order deploy, windows, hibernate, and last the documentation and the repository's claim.
2. **Then add Claude Code deny rules inside the area,** rendered into shuttle's settings: cheap, held in bypass mode, and a second line for paths inside the area that no agent should touch.
3. **Revisit Landlock when both machines run a kernel with Landlock ABI 9 and 10,** as a per-session inner layer under the area, since it nests without user namespaces.
4. **Leave out** the dedicated user, for its ownership churn and the host-git trade-off; podman, for its image and root steps with nothing `bwrap` lacks for this threat model; systemd's user-scope sandboxing, which fails open on Ubuntu; and Claude Code's `/sandbox` in lym runs, which closes nothing lym needs closed and cannot nest inside the area.

Before the area exists, two cheap steps shrink today's exposure: boot hubs from a Konsole window rather than a VS Code terminal, so agents do not inherit the `vscode` profile's user-namespace grant, and add a `README.md` line stating that agents run with bypass permissions.

## Sources

- Claude Code, sandboxing: <https://code.claude.com/docs/en/sandboxing>
- Claude Code, sandbox environments and the sandbox runtime: <https://code.claude.com/docs/en/sandbox-environments>
- Claude Code, permission modes: <https://code.claude.com/docs/en/permission-modes>
- Claude Code, network configuration and required hosts: <https://code.claude.com/docs/en/network-config>
- Anthropic sandbox-runtime README: <https://github.com/anthropics/sandbox-runtime>
- AppArmor `bwrap-userns-restrict` profile: <https://gitlab.com/apparmor/apparmor/-/raw/master/profiles/apparmor/profiles/extras/bwrap-userns-restrict>
- Ubuntu, understanding AppArmor user namespace restriction: <https://discourse.ubuntu.com/t/understanding-apparmor-user-namespace-restriction/58007>
- VS Code issue on `bwrap` under Ubuntu 24.04 and 25.04: <https://github.com/microsoft/vscode/issues/316046>
- Launchpad, user namespace restrictions enabled by default: <https://bugs.launchpad.net/bugs/2046477>
- Ubuntu 26.04 LTS release notes summary: <https://www.debugpoint.com/ubuntu-26-04-lts/>
- systemd `PrivatePIDs=`, available since systemd 257: <https://linux-audit.com/systemd/settings/units/privatepids/>
- landrun README: <https://github.com/Zouuup/landrun>
- Landlock pathname Unix-socket scope patch series: <https://kernsec.org/pipermail/linux-security-module-archive/2026-February/055149.html>
- Podman rootless tutorial: <https://github.com/containers/podman/blob/main/docs/tutorials/rootless_tutorial.md>
- VS Code workspace trust: <https://code.visualstudio.com/docs/editing/workspaces/workspace-trust>
