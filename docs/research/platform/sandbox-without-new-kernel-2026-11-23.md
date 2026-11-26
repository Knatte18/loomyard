# Closing the bus and tmux exposures on kernel 7.0, 2026-11-23

What closes the two exposures [landlock-area-2026-11-23.md](landlock-area-2026-11-23.md) left open on Landlock ABI 8,
on Neve's kernel 7.0 and on OSL-1046, with no kernel upgrade, no user namespace and no root at run time.
The first exposure: an agent reaches the session bus at `/run/user/<uid>/bus`,
and through `systemd-run --user` leaves lym's tree.
The second: an agent reaches other hubs' tmux servers in `/tmp/tmux-<uid>` and types into their panes.
The report is a snapshot of the lym source at the commit that adds it, and of three probe runs on Neve:
two of [`seccomp-connect-probe/main.go`](seccomp-connect-probe/main.go)
and one of [`same-uid-probe/main.go`](same-uid-probe/main.go).
Routes not probed are judged from documentation and the source, and marked unmeasured.
It changes no lym code and no machine setting.

## Answer in brief

One route closes both exposures today with no root:
a seccomp filter that hands every `connect()` to a supervisor in lym.
The supervisor checks the address against an allow list and makes each allowed connect itself,
on the agent's own socket.
On Neve, under that filter, the session bus, `systemd-run --user`, another hub's tmux server and creel daemon,
the SSH agent, VS Code's IPC socket and the X11 socket were all refused.
The hub's own tmux server and creel daemon, git, a Go build, `lym board list` and `claude --version` worked.
A thread that kept swapping the address between an allowed socket and the bus never reached the bus in 200 connects;
unfiltered, the same race reached it 42 times.
Landlock ABI 8 stacks under the filter and keeps closing the files, signals and abstract sockets it closed before.

The route costs lym a security-critical supervisor, and it changes what a server sees of its client.
`SO_PEERCRED` names the supervisor, so creel's sender identity has to move to `SCM_CREDENTIALS`,
which still names the agent.
The supervisor must be every agent's ancestor, so it runs as a subreaper;
if it dies, every connect in the tree fails with `ENOSYS`.
Every kernel feature it uses predates 7.0, and none needs a user namespace or AppArmor,
so it should work on OSL-1046; unmeasured.

Taking the sockets away instead does not work between processes of one user.
An environment without the bus address is undone by a hard-coded one,
which started a unit with `systemd-run --user` (measured).
A socket or directory at mode 000 is set back by its owner, inside a Landlock domain too (measured),
and `/proc/net/unix` lists every listening socket's path to any process.
A separate Unix user or an AppArmor profile per hub would close both,
but each needs root at setup and IT's consent on OSL-1046.

## The supervisor probe

[`seccomp-connect-probe/main.go`](seccomp-connect-probe/main.go) runs a child twice, unfiltered and then filtered.
The filtered child sets `no_new_privs` and installs a filter on every thread with `TSYNC` and `NEW_LISTENER`.
It sends the filter's listener to the supervisor, its parent, over an inherited socket pair.
The filter refuses every system call of a foreign ABI, i386 and x32, with `ENOSYS`,
and `io_uring_setup` with `EPERM`;
it hands `connect` to the supervisor and allows the rest.
For each `connect` the supervisor:

1. reads the address from `/proc/<tid>/mem`, and checks that the notification is still valid;
2. lets an address of another family through, and refuses every abstract Unix address;
3. opens a pathname with `O_PATH`, through `/proc/<tid>/cwd` when it is relative, and reads its canonical path;
4. refuses a canonical path not on its allow list with `EACCES`;
5. takes the agent's socket with `pidfd_getfd` and connects it to `/proc/self/fd/<the O_PATH descriptor>`,
   so the check and the connect see one inode, and the agent cannot change the address in between;
6. answers the notification with the connect's result.

Run 1 allowed the hub's own tmux and creel sockets and the system bus.
Run 2 added a Landlock ruleset under the filter, an address race, a tmux server started inside,
and a supervisor that closes its listener.

| Check | Unfiltered | Filtered |
|---|---|---|
| connect `/run/user/1000/bus` | ok | `EACCES` |
| connect a symlink to the bus | ok | `EACCES`: the check follows the link |
| `busctl --user status` | ok | refused: `Operation not permitted` |
| `systemd-run --user --quiet --collect true` | ok | refused; it tried `/run/user/1000/systemd/private` first, also refused |
| connect another hub's tmux socket; `tmux -S <it> list-sessions` | ok, listed `scribe: 3 windows` | `EACCES`; `Permission denied` |
| connect another hub's creel socket | ok | `EACCES` |
| connect the SSH agent, VS Code's IPC socket | ok | `EACCES` |
| connect abstract `@/tmp/.X11-unix/X0` | ok | `EACCES` |
| connect the hub's own tmux socket; `tmux -S <own> display-message` | ok | ok |
| connect the hub's own creel socket, the system bus | ok | ok |
| connect the test socket by a relative path, and through a symlink | ok | ok |
| 200 connects from a buffer another thread swaps between the test socket and the bus (run 2) | 142 test socket, 42 bus, 16 `ENOENT` | 150 test socket, 0 bus, 31 `EACCES`, 19 `ENOENT` |
| `io_uring_setup` | ok | `EPERM` |
| peer as the test socket's server sees it | `SO_PEERCRED` and `SCM_CREDENTIALS` both the child | `SO_PEERCRED` the supervisor, `SCM_CREDENTIALS` the child |
| connect time, 200 connects, runs 1 and 2 | 4.0 µs, 2.8 µs | 26.6 µs, 29.9 µs |
| connect from an orphan, reparented to `systemd --user` | ok | `EPERM`: the supervisor cannot read the orphan's memory |
| the same with the supervisor as subreaper | ok | ok: the orphan's parent is the supervisor |
| a tmux server started inside; its pane runs `busctl --user status`, then `tmux display-message` on its own server (run 2) | both ok | bus refused, own server ok |
| `git status`, `go build ./internal/lymtemp`, `claude --version` | ok | ok, with no connect at all |
| `lym board list`, `lym creel hubs` | ok | ok; `creel hubs` reports the other hub's daemon as not answering |
| under Landlock as well (run 2): create a file in `$HOME`, `~/.config` | ok | `EACCES` |
| connect after the supervisor closed its listener (run 2) | | `ENOSYS`: the filter fails closed |

Yama's `ptrace_scope` is 1 on Neve, Ubuntu's default.
Under it a process may read another's memory or take its descriptors only as that process's ancestor.
An agent's orphan, such as a double-forked daemon, is reparented to `systemd --user` and leaves the supervisor's tree;
the supervisor cannot read its address and refuses its connect.
As a subreaper, the supervisor adopts orphans and decides their connects.

The supervisor's log shows what lym's own tree connects to:
the hub's tmux and creel sockets, and, for `lym creel hubs`, the creel socket of every hub on the machine.
The tmux server started inside tried the session bus too,
which tmux uses to put each new pane in a systemd scope of its own;
refused, it kept the pane in its own cgroup and went on.

## The take-away probe

[`same-uid-probe/main.go`](same-uid-probe/main.go) listens on a socket in a 0700 directory of its own,
and tries to hide that socket from its own user.
A re-executed child then restricts itself with Landlock ABI 8:
read and execute on `/usr`, `/etc`, `/proc`, `/dev`, `/run` and the other system directories,
every right on one granted directory, and nothing on the socket's directory, `/tmp` or `/home`.

| Check | Same user, unrestricted | Same user, Landlock ABI 8 |
|---|---|---|
| socket at mode 000: connect | `EACCES` | `EACCES` |
| the socket's owner sets it back to 0700 | ok | ok: Landlock has no right over `chmod` |
| connect after that | ok | ok |
| directory at mode 000: connect | `EACCES` | `EACCES`, and the owner set it back as above |
| list the socket's directory; create or remove a file in it | | `EACCES`: Landlock holds its files |
| connect to a socket in a directory the ruleset grants nothing on | | ok |
| connect another hub's tmux socket and the bus, neither under a granted path | | ok |
| `/proc/net/unix` lists the socket, another hub's tmux socket, the bus and the SSH agent | yes | yes |
| `busctl --user status` with `DBUS_SESSION_BUS_ADDRESS` unset | ok: it falls back to `$XDG_RUNTIME_DIR/bus` | |
| the same with `XDG_RUNTIME_DIR` unset too | refused: no address | |
| `systemd-run --user` with only `PATH`, `HOME` and a hard-coded bus address | ok | |
| `ssh-add -l` with `SSH_AUTH_SOCK` unset, then with a hard-coded agent path | cannot connect; then connects (no keys loaded) | |

## The routes

### seccomp user notification on `connect()`

**Closes the bus:** yes, measured on Neve.
Every pathname socket not on the allow list is refused,
which takes in the SSH agent, the desktop's sockets and VS Code's IPC socket as well.

**Closes other hubs' tmux:** yes, measured, for agents inside a filtered hub.
An agent of a hub that runs without the filter, or a host shell, still reaches every server.

**How lym would run it.**
It replaces the bare spawn of reed's tmux server in `ensureServerAndSessionLocked`,
in `internal/reedengine/lifecycle.go`, where the Landlock report puts its wrapper.
A hidden verb `lym area exec -- <tmux argv>` applies the Landlock ruleset and installs the filter with `TSYNC`.
It sends the listener over an inherited socket pair and `exec`s tmux, so the server and every pane carry both.
`TSYNC` sets the filter on every thread in one call and copies `no_new_privs` to each,
so it should need no all-threads syscall in lym's cgo build;
unmeasured, since the probe sets `no_new_privs` with one.
The wrapper's parent, `lym area supervise`, holds the listener and is the subreaper of the whole tree;
it reaps orphans and outlives the server.
The operator attaches from outside, since a tmux client outside the filter connects to the server like any other.
dobby, the creel daemon and the orch inherit the filter when a pane starts them.
Spawn sites a host shell reaches refuse from outside the filter, as in the Landlock design,
and `Seccomp_filters` in `/proc/self/status` tells them whether they are inside.

**What lym owns.**
A notification loop, address parsing, path resolution through `O_PATH`, the descriptor takeover and the filter program.
The probe's supervisor is about 200 lines, and every one of them is on the security boundary.
A wrong line opens the area without a symptom, so the supervisor needs tests on a real kernel,
among them the race above and the foreign-ABI and `io_uring` refusals.

**What breaks or changes.**

- **creel's sender identity.**
  `peerPID` in `internal/creelengine/peer_linux.go` reads `SO_PEERCRED`,
  which names the supervisor for every connect the supervisor makes (measured).
  Every post from a filtered pane would come from the supervisor's process, outside every pane,
  so creel would read it as `EXTERNAL`.
  `SCM_CREDENTIALS` names the sending process (measured),
  and the kernel refuses an unprivileged sender any pid but its own.
  The client would send its credentials on its first message, and the daemon would set `SO_PASSCRED` and read them.
- **Cross-hub mail.**
  Trusted hubs' creel sockets go on the allow list.
  How a remote daemon confirms a sender whose connect the supervisor made is unmeasured.
- **The gate's memory cap.**
  `RunMemoryCapped` in `internal/proc` starts a capped scope with `systemd-run --user --scope`,
  which goes through the bus and is refused.
  Its documented fallback, `RLIMIT_DATA` per process, takes over; read from the code, unmeasured.
  A capped scope would need a supervisor verb that asks the user manager to adopt the caller's pid.
  The caller keeps the filter, since a scope moves a process between cgroups without starting a new one.
- **tmux's per-pane scopes.**
  The server's bus connect is refused (measured),
  so panes stay in the server's cgroup and `systemctl --user` no longer lists them.
- **Desktop and editor reach.**
  From inside, `lym reed window` cannot reach Wayland or X11,
  `lym ide` cannot reach VS Code's IPC socket (measured refused), and desktop notifications fail.
  These stay operator steps from a host shell, as the Landlock report says.
  This hub pushes over HTTPS, so the refused SSH agent breaks nothing here.
- **A supervisor crash.**
  Every connect in the tree fails with `ENOSYS` (measured), the hub's own tmux and creel included.
  The filter cannot be handed to a new listener, so the hub's server restarts with a new supervisor.
- **Name lookups.**
  A glibc program may ask `systemd-resolved` over its Unix socket;
  refused, it falls back to DNS, or the socket goes on the allow list. Unmeasured.

**What it leaves open.**
A filter on `connect` alone does not cover `sendto` or `sendmsg` to a named datagram socket, such as the journal's.
Neither exposure uses one: the bus and tmux are stream sockets,
and a stream socket answers a send with an address with an error.
Covering datagrams would hand every `sendmsg` to the supervisor, since a filter program cannot read the message header.
TCP and UDP connects are allowed, but the supervisor makes those too,
since the family it checked could otherwise change before the connect.
Without Landlock, an agent can kill the supervisor, which fails closed,
and can rewrite the `lym` binary the next supervisor runs.
So the route needs Landlock's filesystem rules and signal scope under it.

**On OSL-1046.**
User notification (Linux 5.0), `pidfd_getfd` (5.6), `TSYNC_ESRCH` (5.7) and `PIDFD_THREAD` (6.9)
are all in its kernel 7.0.0-42.
None needs a user namespace, so the AppArmor restriction that stops `bwrap` there does not apply.
Ubuntu ships Yama's `ptrace_scope` at 1 there too, so the subreaper is needed the same way.
Whether Cortex XDR, which runs on OSL-1046, interferes with a seccomp listener or `pidfd_getfd` is unknown.
All unmeasured until the probe runs there.

### Taking the exposures away

**Closes the bus:** no, measured.
Leaving `DBUS_SESSION_BUS_ADDRESS` and `XDG_RUNTIME_DIR` out of the agents' environment
stops a client that reads them, and nothing else: a hard-coded `/run/user/<uid>/bus` started a unit.
The socket cannot be made unreachable for agents alone,
since the operator's desktop connects to the same path as the same user.

**Closes other hubs' tmux:** no, measured.
A per-hub directory for each hub's socket, through `TMUX_TMPDIR`, keeps nothing out.
A same-user process enters any directory whose mode lets the hub's own agents in,
and it sets back a mode of 000 on a directory or socket it owns, inside a Landlock domain too.
Landlock ABI 8 has no right over a directory walk, a `connect` or a `chmod`.
Hiding the paths does not help either: `/proc/net/unix` lists them all.

**Whether same-user processes can be kept apart this way at all:**
not without a kernel mechanism that tells them apart.
File modes cannot, because the owner may change them.
Namespaces can, and are out of bounds here.
What remains is a filter on the system calls themselves, as in the route above,
or a mandatory access control profile, as below.

**On OSL-1046:** the same, by the same file semantics; unmeasured.

### A separate Unix user for agents

**Closes the bus and other hubs' tmux:** yes, judged from file modes.
`/run/user/1000` and `/tmp/tmux-1000` are both 0700 on Neve,
so another user cannot reach the bus or any tmux socket in them.
An agent user has no user manager of its own unless it logs in or lingers,
so `systemd-run --user` has nothing to talk to.
One agent user for every hub separates agents from the operator, not hubs from each other;
that takes one user per hub.

**Root:** once per user, to create it.
Then each start needs a rule that lets the operator act as that user:
a `sudo -u` rule, or a polkit rule for a system unit.
On OSL-1046, with sudo tightly limited and accounts from AD, both need IT; unmeasured.

**What lym owns and what breaks:** as [sandboxing-options-2026-10-30.md](sandboxing-options-2026-10-30.md) lists.
The hub is owned by another user, and git's `safe.directory` refuses it to the operator;
`internal/lymtemp` refuses a temp root it does not own, and reed has to attach to another user's server;
each user needs its own Claude login and Go caches.
With one user per hub, all of that repeats per hub.

### Landlock ABI 8 with the others, and PreToolUse hooks

Landlock ABI 8 closes neither exposure alone, and with `systemd-run --user` open it is undone,
as the Landlock report measured.
Under the seccomp filter it is no longer undone: run 2 refused `systemd-run` and writes to `$HOME` in one tree.
The two divide the work.
Landlock holds files, signals and abstract sockets,
which keeps the supervisor's binary, its allow list and its process out of agents' reach;
the filter holds pathname connects.
The pane check answers a question the Landlock report left open:
a tmux server started inside a Landlock domain allocates panes when the ruleset grants `/dev/ptmx` and `/dev/pts`.
Taking the sockets away gains nothing from Landlock, as the take-away probe shows.

PreToolUse rows in `internal/shellguard` would refuse `systemd-run`, `busctl --user`, `dbus-send --session`, `gdbus`,
and a `tmux -S` or `-L` naming another hub's server.
They are a guardrail on every route:
a script file, a test binary or an interpreter one-liner gets past them, as the Landlock report lists.
Over the filter they still help:
an agent that tries a refused command meets a refusal with a way forward,
in place of an `EACCES` from deep inside a tool.

### Other routes

- **An AppArmor profile per hub.**
  Ubuntu's kernels carry AppArmor's Unix-socket mediation; Neve's lists `network/af_unix` among its AppArmor features.
  AppArmor mediates a pathname socket's connect through its file rules,
  so a profile can deny the bus, the SSH agent and other hubs' sockets, along with files, signals and ptrace.
  An unprivileged start enters it with `aa-exec -p <profile>`.
  Root loads each profile once, and a profile names its own hub's paths, so each new hub is one more root step.
  It closes both exposures, judged from AppArmor's documentation;
  unmeasured, including a start from a pane under the `vscode` label.
  It is Ubuntu-only, and OSL-1046 needs IT for each load.
- **A cgroup BPF program on Unix connects** (`BPF_CGROUP_UNIX_CONNECT`, Linux 6.7) can refuse a connect per cgroup.
  Attaching one needs root each time a hub's cgroup is made,
  and a process can move itself to another cgroup its user owns.
  Not a barrier; unmeasured.
- **An `LD_PRELOAD` shim** over `connect` misses Go's static binaries and any raw system call. Not a barrier.

## The routes side by side

| Route | Bus | Other hubs' tmux | Root | OSL-1046 | Measured |
|---|---|---|---|---|---|
| seccomp supervisor, with Landlock ABI 8 | closed | closed, for filtered hubs | none | expected to work | yes, on Neve |
| Taking the sockets away | open | open | none | the same | yes, on Neve |
| Separate Unix user per hub | closed | closed | once per user, and a sudo or polkit rule | needs IT | no |
| AppArmor profile per hub | closed | closed | once per hub | needs IT | no |
| Landlock ABI 8 alone | open | open | none | the same | yes, in the Landlock report |
| PreToolUse hooks | guardrail | guardrail | none | the same | no |

## Recommendation

1. **Add the PreToolUse rows now**, one task in `internal/shellguard`, as the Landlock report proposed.
   They are cheap, useful on every route, and the refusal agents meet first once a barrier exists.
2. **Build the area on Landlock ABI 8 with the seccomp connect supervisor** when a barrier on kernel 7.0 is wanted.
   It is the one route that closes both exposures with no root and no user namespace, and it is measured.
   Run the supervisor probe on OSL-1046 first, under Cortex XDR, before the first task starts.
3. **Keep the AppArmor profile per hub as the fallback**,
   for when the supervisor probe fails on OSL-1046 and IT will load profiles.
4. **Leave out** taking the sockets away, which closes nothing (measured),
   and a separate user, whose cost per hub is the highest.
5. **Re-run the Landlock probe when a kernel with ABI 9 reaches both machines.**
   Landlock's pathname-socket control would then take over the supervisor's job, and the supervisor could go.

The tasks for item 2, on top of the Landlock report's allow-list file and the area's own `CLAUDE_CONFIG_DIR`:

- **The supervisor.**
  A new `internal/areaengine` with the filter program, the notification loop, take-and-connect,
  the allow list by canonical path, the subreaper and the reaping.
  It is tested on a real kernel at the lowest tier that runs one;
  the address race, the foreign-ABI and `io_uring` refusals, an orphan and a closed listener are among the cases.
- **The area's boot.**
  `lym area exec` and `lym area supervise`, and reed's server spawn through them.
  reed refuses to boot an area hub without its supervisor,
  and a supervisor that dies ends the server with a way forward.
- **creel's sender identity from `SCM_CREDENTIALS`**, landed before the boot task.
  The client sends credentials on its first message, and the daemon sets `SO_PASSCRED` and reads them;
  `SO_PEERCRED` stays for a peer outside an area.
- **The allow list.**
  Derived entries: the hub's own tmux and creel sockets, trusted hubs' creel sockets,
  and the system bus for `lym hibernate request`.
  The operator's file adds more, and `lym area show` prints both with their sources.
- **The memory cap inside an area.**
  Decide between `RLIMIT_DATA`, which `internal/proc` already falls back to,
  and a supervisor verb that starts a capped scope for its caller.
- **One domain per hub, docs and PATTERN.**
  Spawn sites refuse from outside the filter with their way forward.
  `README.md` and `docs/overview.md` describe the area,
  and a PATTERN entry states that within an area only the supervisor decides a connect, against the area's allow list.

## Sources

- `seccomp_unotify(2)`: <https://man7.org/linux/man-pages/man2/seccomp_unotify.2.html>
- `seccomp(2)`, for `TSYNC_ESRCH` and `NEW_LISTENER`: <https://man7.org/linux/man-pages/man2/seccomp.2.html>
- `pidfd_getfd(2)`: <https://man7.org/linux/man-pages/man2/pidfd_getfd.2.html>
- `pidfd_open(2)`, for `PIDFD_THREAD`: <https://man7.org/linux/man-pages/man2/pidfd_open.2.html>
- `unix(7)`, for `SO_PEERCRED`, `SO_PASSCRED` and `SCM_CREDENTIALS`: <https://man7.org/linux/man-pages/man7/unix.7.html>
- Yama, kernel documentation: <https://docs.kernel.org/admin-guide/LSM/Yama.html>
- Landlock userspace API, kernel documentation: <https://docs.kernel.org/userspace-api/landlock.html>
- `apparmor.d(5)`, for Unix-socket and file rules: <https://manpages.ubuntu.com/manpages/noble/man5/apparmor.d.5.html>
- [landlock-area-2026-11-23.md](landlock-area-2026-11-23.md)
  and [sandboxing-options-2026-10-30.md](sandboxing-options-2026-10-30.md)
