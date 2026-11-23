# A lym-started Landlock area instead of bwrap, 2026-11-23

Whether lym can close the two exposures in note `sandbox-closed-area-build` with no `bwrap`, no root and no AppArmor profile.
The idea is that lym applies Landlock to its own process before it starts reed's tmux server, so every pane and agent inherits the restriction.
The two exposures are an agent reaching `/run/user/<uid>/bus`, and through it `systemd-run --user`, and an agent typing into another hub's tmux panes through its socket in `/tmp/tmux-<uid>`.
The report is a snapshot: three runs of the probe in [`landlock-probe/main.go`](landlock-probe/main.go) on Neve, and the lym source at the commit that adds it.
It changes no lym code and no machine setting.
It builds on [sandboxing-options-2026-10-30.md](sandboxing-options-2026-10-30.md), which weighed Landlock through a wrapper without measuring it.

## Answer in brief

No, not on the kernels both machines run today.
Neve's kernel 7.0.0-31 has Landlock ABI 8, and the kernel refuses both proposed forms of pathname Unix-socket control.
A process restricted with every right ABI 8 offers still connected to the session bus and to another hub's tmux socket, listed that hub's sessions, and started a process outside lym's tree with `systemd-run --user`.
That last step defeats the filesystem rules too, because the process systemd starts carries no Landlock domain.
On ABI 8 a Landlock area is therefore a guardrail against an agent's mistakes, never a barrier against a hostile agent.

Landlock does deliver the rest of the mechanism, measured:
`LANDLOCK_RESTRICT_SELF_TSYNC` restricts every thread of a Go process, and children inherit the domain across `exec`.
Writes and reads outside the allow list fail, SSH keys and the `gh` token among them.
Signals and abstract sockets outside the domain are refused, `sudo` stops working under `no_new_privs`, and a nested ruleset cannot widen the first.
git, a Go build, `lym board list`, `claude --version` and the hub's own tmux all worked under the allow list below.
So the area becomes worth building when a kernel with pathname-socket control reaches both machines; until then, add PreToolUse rows for the bus and other hubs' tmux servers as a cheap guardrail.

## Landlock ABI per machine

| Machine | Kernel | ABI | Source |
|---|---|---|---|
| Neve | 7.0.0-31-generic | 8 | measured by the probe; `landlock` is in `/sys/kernel/security/lsm` |
| OSL-1046 | 7.0.0-42 (Ubuntu 24.04 HWE) | 8, unmeasured | kernel version as recorded in the sandboxing report; same upstream series as Neve |

ABI 8 holds the filesystem rights through `IOCTL_DEV` (ABI 5), TCP bind and connect by port (ABI 4), scoping of abstract Unix sockets and signals (ABI 6), audit-log flags (ABI 7) and `TSYNC` (ABI 8).
The probe asked the kernel to create a ruleset with each candidate pathname-socket bit, the next free filesystem right (`1<<16`) and the next free scope (`1<<2`), and both were refused with `EINVAL`.
The sandboxing report places pathname Unix-socket control at ABI 9, newer than 7.0; which kernel ships it, and with which bit, is not checked here.

## The probe

[`landlock-probe/main.go`](landlock-probe/main.go) is one Go file with `//go:build ignore`, so no package build sees it; it runs with `CGO_ENABLED=0 go run` from the repository root.
It tries socket connects, file reads and writes, a signal and commands unrestricted;
then it restricts itself on every thread and tries them again in the same process, on a thread that existed before the restriction, and in a re-executed child.
The child can then apply a second, wider ruleset and try a write it grants.
It connects and closes without sending a byte, opens an existing file for writing without writing to it, and removes every file it creates.

- Run 1 granted read and execute under `/` and every right under one scratch directory, and asked for the connect right on the hub's own tmux and creel sockets.
- Run 2 added the hub, the Go caches, `/tmp` and the terminal devices, and ran the toolchain commands.
- Run 3 replaced `/` with the enumerated read list in [The allow list](#the-allow-list), checked the secrets and the allow list's own file, and tried to widen.

| Check | Unrestricted | Restricted, ABI 8 |
|---|---|---|
| connect `/run/user/1000/bus` | ok | ok |
| connect `/run/dbus/system_bus_socket` | ok | ok |
| connect `/run/user/1000/wayland-0` | ok | ok |
| connect another hub's tmux socket | ok | ok |
| connect another hub's creel socket | ok | ok |
| connect abstract `@/tmp/.X11-unix/X0` | ok | `EPERM` |
| signal 0 to the shell that started the probe | ok | `EPERM` |
| create a file in `$HOME`, `~/.config`, `~/go/bin`, `~/.config/lym` | ok | `EACCES` |
| create `~/.config/lym/area-loomyard-LYMHUB-92073c9b.yaml`, the allow list's proposed file | ok, removed again | `EACCES` |
| read `~/.ssh`, `~/.config/gh/hosts.yml`, `~/.config/lym/credentials.json` | ok | `EACCES` (run 3) |
| create a file in the granted directory | ok | ok |
| the same reads and writes on the pre-existing thread | | same as the restricted rows: `TSYNC` reached it |
| child: a second ruleset granting `$HOME`, then a write there | | `EACCES`: the nested domain cannot widen the first |
| child: `tmux -S <other hub> list-sessions` | | ok, listed `scribe: 3 windows` |
| child: `busctl --user status` | | ok |
| child: `systemd-run --user --quiet --collect true` | | ok |
| child: `sudo -n true` | | refused: `no new privileges` flag is set |
| child: `git update-index --refresh`, `git status` | | ok |
| child: `go build ./internal/lymtemp` | | ok |
| child: `lym board list` | | ok |
| child: `claude --version` | | ok |
| child: `tmux -S <own hub> display-message` | | ok |

Landlock's documented ptrace rule also keeps a restricted process from reading `/proc/<pid>/environ` of a process outside its domain; the probe did not measure it.

## The allow list

Every path run 3 granted, with why, and the paths a running agent needs that the probe could not show.
Landlock rules only add access, so the list names what to allow; anything it leaves out, such as `~/.ssh`, is denied without a rule.
"Derived" paths come from the hub's geometry and lym computes them; "file" paths belong in the operator's allow-list file.

| Path | Rights | Kind | Why | Measured |
|---|---|---|---|---|
| `/usr`, `/etc`, `/opt`, `/bin`, `/lib`, `/lib64`, `/sbin` | read, execute | file | binaries, libraries, configuration and the Go toolchain under `/usr/lib/go-1.26` | yes, run 3's commands |
| `/proc`, `/sys`, `/dev`, `/run` | read | file | process and device files the tools read; `/etc/resolv.conf` links into `/run` | yes |
| `~/.gitconfig`, `~/.config/git` | read | file | git's identity and settings | yes, git |
| `~/.local/bin`, `~/.local/share/claude` | read, execute | file | the Claude Code launcher and its versions | yes, `claude --version` |
| `~/go/bin` | read, execute | file | the installed `lym` | yes, `lym board list` |
| `~/.config/lym/machine.json` | read | file | lym's machine name | yes, `lym board list` |
| `~/.claude`, `~/.claude.json` | read | file | Claude Code's settings and login | read only; a session writes them, which the probe did not run |
| `~/.cache/go-build` | all | file | Go's build cache | yes, `go build` |
| `~/go/pkg/mod` | all | file | Go's module cache, written on a module download | granted; the build hit the cache only |
| `/tmp` | all | file | Go's build and test temp dirs while `GOTMPDIR` is unset | yes; narrower with `GOTMPDIR` and `TMPDIR` under the hub's `.lym` |
| the hub root | all | derived | worktrees; the prime's `.git`, which holds each worktree's git dir and the objects; `_bolt` and `.lym` state | yes, git and `lym board list` |
| `/tmp/tmux-<uid>` or `$TMUX_TMPDIR` | all | derived | the server creates its socket there | through `/tmp` |
| `/tmp/lym-<uid>` | all | derived | lym's temp root: creel's socket and lym's temp dirs | through `/tmp` |
| `/dev/null`, `/dev/pts` | all | derived | shells and tools write to `/dev/null`; panes are pseudo-terminals | yes |
| `/dev/ptmx` | write, ioctl | derived | a tmux server started inside allocates panes through it | no: the probe never started a server inside |

Denied, measured: writes to `$HOME`, `~/.config`, `~/go/bin` and `~/.config/lym`, and reads of `~/.ssh`, `~/.config/gh/hosts.yml` and `~/.config/lym/credentials.json`.
A running session writes `~/.claude`, whose `settings.json` hooks and plugins run in the operator's own sessions: the persistence exit the sandboxing report names.
So the area needs its own Claude config directory through `CLAUDE_CONFIG_DIR`, with `TranscriptReader` in `internal/shuttleengine/claudeengine` following it, as that report says.
Claude Code also writes `~/.claude.json` through a file beside it, so writing it in place would need rights on `$HOME` itself, which reopens `~/.bashrc`; that is one more reason for the separate directory.

**Where the list lives.**
One YAML file per hub, `~/.config/lym/area-<hub key>.yaml`, in the per-user lym config directory `standalonestate.UserConfigDir` already names, where `<hub key>` is the key reed's server name uses.
It holds `read`, `write` and `exec` path lists with `~` expanded, written in full the first time the area is set up, so the file alone shows every path the operator chose.
It lies outside the hub and outside every derived path, so no rule grants a write to it or its directory, and the start refuses a list that would.
Inside the area the file is read-only: run 3 failed both to create it and to create any file in its directory.
The derived paths are not in the file: lym computes them from the hub's geometry, and an operator cannot drop one without breaking the hub.
`PATTERN-config-layers` reads every config module through `configengine`'s layers under the hub, so the file needs either a new layer outside the hub or an exception in that entry; the build's PATTERN change decides which.

**When a change takes effect.**
The wrapper reads the file once, when the operator runs `lym area start` from a host shell, and the kernel holds the rules from then on.
A process inside cannot widen them: a ruleset applied inside nests in the first, and Landlock grants only what both allow, which run 3's widening check shows.
So an edited list takes effect only when the operator stops the area and starts it again from outside.
On ABI 8 this holds only against an agent that stays inside, since `systemd-run --user` starts a process outside that can rewrite the file.

**How lym shows the list in force.**
A verb `lym area show` prints the kernel's ABI, every path with its rights and its source, the file or derived, and the rights the kernel lacks, such as pathname-socket control on ABI 8.
The wrapper writes the same list to reed's boot log, so the log of a running server says what its agents may touch.
An `exec` list narrower than `read` limits only what `execve` loads: `sh script.sh` and an interpreter still run, and `go test` runs its binaries from `/tmp`, so the default `exec` equals `read`.

## Where lym would apply it

reed starts the hub's tmux server in `ensureServerAndSessionLocked` in `internal/reedengine/lifecycle.go`, with `exec.Command(e.cfg.Tmux, ...)` and `proc.Detach`.
Go has no hook between `fork` and `exec`, so the restriction goes in a hidden wrapper verb, `lym area exec -- <tmux argv>`.
The wrapper builds the ruleset, restricts itself with `TSYNC` and `syscall.Exec`s tmux; the domain carries across the `exec` to the server and every pane.
`TSYNC` matters because lym is built with cgo, under which `syscall.AllThreadsSyscall` refuses to run.

The tree must be one domain.
Landlock's signal and abstract-socket scopes admit only the same domain or one nested in it, so two processes restricted separately with equal rules are sibling domains and cannot signal each other.
dobby (`SpawnDetached` in `internal/dobbyengine/start.go`) and the creel daemon (`spawnDaemon` in `internal/creelcli/spawn.go`) inherit the domain when an agent or the orch starts them, and stay outside it when the operator's host shell does.
With the area on, those spawn sites refuse to start from outside the domain and name the way forward: run the command in a pane of the hub's server, or hand the spawn to the server as a new window.
A dobby left outside would also run the verify gate's agent-written tests outside the area.

What breaks, read from the code and the probe:

| Path | With the area on |
|---|---|
| `lym reed attach` from outside | works: Landlock limits only the domain's own actions, and a client outside connects to the server as before; windows opened through it run inside |
| Landing deploy | fails: Batten's Land row runs `tools/deploy` in the domain, and writes to `~/go/bin` are denied (measured) as are writes to the plugin cache; granting them reopens persistence, so the operator deploys from a host shell |
| `lym reed window` | Konsole starts inside the domain: its Wayland connect works (measured), its writes to `~/.config` and `~/.local/share` are denied; whether it starts anyway is unmeasured |
| `lym ide` | `code` reaches a running VS Code through its socket under `/run/user`, which ABI 8 cannot deny, so it works, and VS Code outside then runs git over the area's repositories; a fresh VS Code inside fails its writes to `~/.config/Code`, unmeasured |
| `lym hibernate request` | works: the system bus connect is allowed (measured) and `systemctl hibernate` goes through logind and polkit as today |
| Ending a process outside the domain | `EPERM` (measured for a signal): a lym inside cannot end a dobby or creel daemon started from a host shell |
| `sudo` in a pane | refused under `no_new_privs` (measured), a gain |

## PreToolUse hooks as a second line

lym already renders guards into PreToolUse hooks from one table, `internal/shellguard`.
Rows could refuse `systemd-run`, `busctl --user`, `dbus-send --session`, `gdbus` and `systemctl --user start` in command position, and `tmux -S` or `tmux -L` naming a server other than the hub's own.
They stop an agent that goes there by mistake or follows a planted instruction that spells the command out.

They cannot stop:

- a script file the agent writes and then runs, a shape the table's own documentation names as unguarded;
- a test or program the agent writes and the gate runs, since a test binary can open the bus socket like the probe does;
- an interpreter one-liner or a D-Bus library in any language;
- anything not run through the Bash tool, such as a hook the agent writes into a settings file it can reach.

So hooks are a guardrail on the same footing as an ABI 8 Landlock area, and neither is a barrier.

## A route not taken: seccomp user notification

A seccomp filter needs no root and no user namespace, and with `SECCOMP_RET_USER_NOTIF` a supervisor can decide each `connect()`.
To be sound the supervisor must make the connection itself and install it with `SECCOMP_IOCTL_NOTIF_ADDFD`, since the address in the agent's memory can change after the check.
The filter must also refuse `io_uring_setup`, whose connect bypasses seccomp, and the 32-bit system-call ABIs.
It would close both exposures on kernel 7.0, but it is a security-critical component lym would own alone; this report did not probe it.

## Recommendation

1. **Build no Landlock area on ABI 8.**
   It closes neither exposure, and `systemd-run --user` undoes its filesystem rules (measured).
2. **Add the PreToolUse rows now**, as one task in `internal/shellguard`: refuse the bus commands and another hub's tmux server, each row's comment stating what it lets through.
3. **Re-run the probe when a new kernel reaches either machine.**
   When the kernel accepts a pathname-socket bit, read the kernel's documentation for the bit's final value and build the area as below.
4. **Leave seccomp user notification** unless the operator wants a barrier on 7.0 enough to own its supervisor; then a probe of it comes first, as research.

The tasks the area takes once the kernel controls pathname sockets:

- **The allow-list file and its display.**
  `~/.config/lym/area-<hub key>.yaml` with the file paths above as its first content, read through `configengine` once `PATTERN-config-layers` admits it;
  `lym area show` with the derived paths, each path's source and the kernel's ABI.
- **The wrapper and reed's boot.**
  `lym area start` from a host shell and `lym area exec`, building the ruleset with the connect right on the hub's tmux and creel sockets and the system bus only, restricting with `TSYNC`;
  reed's server spawn through it, refusing to boot when the area is on and the kernel lacks a right the area needs, or when the list grants a write to its own file.
- **One domain per hub.**
  dobby's and creel's spawn sites refuse from outside the domain with their way forward, and `GOTMPDIR` and `TMPDIR` move under the hub's `.lym` so `/tmp` leaves the list.
- **The area's own Claude config directory.**
  Agents start with `CLAUDE_CONFIG_DIR`, and `TranscriptReader` follows it.
- **Docs and the PATTERN entries.**
  `README.md`, `docs/overview.md`, the `PATTERN-config-layers` change for the file outside the hub, and a new entry: an area's rules come only from that file and lym's derived paths, read at a start from outside.

Deploy, `lym reed window` and `lym ide` stay operator steps from a host shell; a broker for them is the bwrap design's stage 3 and applies here unchanged.

## Sources

- Landlock userspace API, kernel documentation: <https://docs.kernel.org/userspace-api/landlock.html>
- `landlock.h` as installed on Neve, holding `LANDLOCK_RESTRICT_SELF_TSYNC` and no pathname-socket right
- [sandboxing-options-2026-10-30.md](sandboxing-options-2026-10-30.md), for OSL-1046's kernel and the ABI 9 placement
- `seccomp_unotify(2)`: <https://man7.org/linux/man-pages/man2/seccomp_unotify.2.html>
