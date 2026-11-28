# Gate tier runs on Septim: work and realism, 2026-11-28

How much work it is to run `lym gate test` tier runs on Septim while the hub and its agents stay where they are.
Septim is a Windows 11 desktop (Ryzen 7 9800X3D) with WSL2 installed, on a closed LAN.
The hub is on Neve (Linux).

Two modes are studied:

- **Stored:** the hub's code is checked out on Septim's WSL2 disk with a warm build cache (private code such as LoomYard).
- **Ephemeral:** the code is never checked out or stored on Septim.
  It is sent into a RAM disk for one tier run and deleted after (work code, hubs on the laptop OSL-1046).

The fixed requirements: a hub opts in and names its mode, a hub that names none never sends a run to Septim,
work code uses ephemeral mode only, and the service runs inside WSL2 only.

## Verdict

| Mode | Realistic | Size | Recommendation |
|---|---|---|---|
| Stored | Yes. No obstacle found in the gate code or the documentation read. | 4 code tasks and 1 setup and trial task | Measure Septim by hand first, then build it. Send `tier1` and `tier2`; keep `tier0` local. |
| Ephemeral | In part. "Not written to disk by the run" is reachable inside WSL2; "never on Septim's disk" also depends on Windows. | 2 more tasks, 3 with C# | Do not build it yet. Decide the three questions under [Ephemeral mode](#ephemeral-mode) first. |

Nothing here was run on Septim, and no tier or benchmark was run for this study.
**How much faster Septim is than Neve is unknown**, and it decides whether either mode is worth building.

## Method and confidence

- The code was read at commit `4a19e81cf`: `internal/gatecli`, `internal/gateslot` and what they call.
- Load figures come from Neve's existing gate run records and from earlier reports in this directory tree.
- The public pages listed under [Sources](#sources) were read.
- A claim marked **(unverified)** rests on general knowledge that no page read here states.
  Check it on Septim before building on it.

## What there is to gain

Neve's gate run records dated 2026-11-22 to 2026-11-28 (none from the 24th and 25th),
leaving out carried, build and abandoned records:

| Tier | Runs | Median wall s | 90th percentile s | Wall, summed h | Slot wait, summed h |
|---|---|---|---|---|---|
| `tier0` | 3102 | 8 | 84 | 23.0 | 13.8 |
| `tier1` | 1829 | 28 | 265 | 46.1 | 4.5 |
| `tier2` | 899 | 62 | 119 | 15.3 | 0.7 |

- About 19 h of slot wait stood against 84 h of tier runs. Waits of parallel callers overlap each other.
- `tier0` waits the most and runs the shortest.
  A short run gains least from being sent away, since sync and connection are a fixed cost on top.
  Moving `tier1` and `tier2` off Neve frees local slots, which shortens the `tier0` wait without sending `tier0`.
- Neve has 12 threads and 30 GiB, and the hub now has 2 slots.
- Four `tier1` runs timed out.

## The Septim side

### Network and SSH

- **Mirrored networking** is the simpler choice. It needs Windows 11 22H2 or higher,
  is set as `networkingMode=mirrored` under `[wsl2]` in `.wslconfig`, and lets the LAN connect to WSL directly.
- Inbound connections need a Hyper-V firewall rule.
  Use `New-NetFirewallHyperVRule` for TCP on the SSH port alone,
  not `Set-NetFirewallHyperVVMSetting ... -DefaultInboundAction Allow`, which opens every port.
- The **NAT** alternative needs `netsh interface portproxy add v4tov4 ... connectaddress=(wsl hostname -I)`.
  The VM's address changes across restarts, so the rule must be refreshed **(unverified)**.
- `openssh-server` runs inside the distribution. The WSL FAQ says running it needs Windows administrator privileges.
- Give the gate its own Linux user and an `authorized_keys` line with `restrict,command="..."`,
  so the key can start the gate runner and nothing else.
  The runner still runs whatever test command it is sent, so the key amounts to code execution as that user.

### Starting and staying up

This is the least documented part and the first thing to try on the machine.

- `systemd=true` under `[boot]` in `/etc/wsl.conf` needs WSL 0.67.6 or newer.
- "systemd services will NOT keep your WSL instance alive."
- `instanceIdleTimeout` defaults to 15000 ms and takes `-1` to disable; `vmIdleTimeout` defaults to 60000 ms.
  Whether an open SSH session counts as activity is not stated.
- WSL does not start at Windows boot by itself.
  The usual way is a Task Scheduler task at startup, run whether or not the user is logged on,
  as the Windows account that owns the distribution, holding `wsl.exe -d <distro> --exec sleep infinity` **(unverified)**.

### Sleep and Wake-on-LAN

None of the pages read covers this. All of it is **(unverified)**:

- A sleeping or hibernating Windows host suspends the VM, so Septim is unreachable.
- Wake-on-LAN is a firmware, network card and Windows setting that WSL has no part in.
  Waking the host may not start a stopped WSL instance.
- The cheap choice is a desktop that does not sleep on mains power. Otherwise treat a sleeping Septim as absent.

### Disk, resources and Defender

- Keep the checkout and caches on the distribution's own filesystem.
  Microsoft recommends against working across the Windows and Linux filesystems and advises the WSL one for speed.
- WSL2 gets 50% of Windows memory and all logical processors by default.
  Septim's memory size was not recorded; it sets the slot count and what ephemeral mode can hold.
- Defender advice for WSL is not stated on the pages read.
  The common advice to exclude the distribution's VHDX or the WSL processes from scanning is **(unverified)**.
  Measure with and without before adding an exclusion, since an exclusion widens what the machine trusts.
- Set `[automount] enabled=false` and `[interop] enabled=false` in `wsl.conf`.
  This keeps a test from writing to Windows drives or starting Windows programs by accident.
  Neither setting is described as a security boundary.

### Toolchain

| Need | For | Note |
|---|---|---|
| Go, the same version as the caller | every tier | A closed LAN cannot download a toolchain or modules; copy the module cache or serve it from a file proxy. |
| `gcc` | every tier of LoomYard | The tree-sitter dependencies are cgo. |
| `git` 2.43.0 or newer | `tier1`, `tier2` | `toolfloor.GitFloor`; Neve has 2.53. |
| `tmux` 3.4 or newer | `tier2` | `toolfloor.TmuxFloor`; Neve has 3.6a. A newer tmux has behaviour an older one lacks. |
| `rsync`, `openssh-server` | sync and access | |
| systemd user manager | a tier with `memory_mb` | LoomYard's tiers set none today. |
| .NET SDK, restored NuGet packages | a C# hub | Linux builds only. A project that targets Windows or .NET Framework cannot run there **(unverified)**. |

If Septim has no internet access at all, installing and updating all of this is a recurring manual job.

## The LoomYard side

### Where to cut

Send **the tier's commands**, not the whole gate call.
The caller keeps everything that reads the hub or decides a verdict:
scope computation, `{test_targets}` narrowing, the green record, the counter, run records,
the failure report and the card fixer.
Septim runs the rendered shell commands and streams their output back.
Septim then needs no hub, board or config of its own.

The seam is `runInvocations` in `internal/gatecli/tierrun.go`,
where `exec.CommandContext` and `runTiedChild` start each invocation.
The `go-json` counter in `internal/gatecli/counters.go` is already a line-splitting `io.Writer`,
so remote output fed into it counts and reports as local output does.
Test paths are module-relative under `-trimpath`, so failures read the same from either machine.

### What must move or change

| Part | Today | Change |
|---|---|---|
| Command execution | `runInvocations` starts a local tied child | An executor with a local and a remote form. |
| Child cleanup | `runTiedChild` kills the group and every marked process | Runs on Septim, in the remote runner. A dropped connection alone must not be trusted to end the command. |
| CPU watchdog, memory guard, process cap | Sample the local process tree | Run on Septim; their stop lines and flags come back in the final message. |
| `prebuilt_lym` | `prebuildLym` builds `cmd/lym` and stores it under its content hash | Build and store on Septim, and set `LYM_PREBUILT_LYM` there. |
| Stray files | `checkStrayFiles` lists the local worktree before and after | **Must run on Septim.** Left local, a test that writes into the tree passes unnoticed. |
| Environment | The caller's environment, the hub `env` module, then the tier's `env` | Send the `env` module's and the tier's entries only, never the session's environment. |
| Timeout | `withTimeoutMin` on the caller | Keep it on the caller and give Septim the same bound, for a caller that vanishes. |
| Slot inheritance | `LYM_GATE_SLOT` names a local lock file | The remote runner sets it to its own slot's lock. |
| `vstest` with `results: trx` | A local temp directory read after the run | Create it on Septim and send the files back. |
| Run records, `gate status`, `gate report` | No host | Record and show the host; without it, timings of the two machines mix. |

The remote output needs its own framing.
`ssh` exits with the remote command's status or 255 on its own error,
so an exit code alone cannot tell a failed test from a lost connection.
Send merged output in frames and end with a result message; a stream with no result message judged nothing.

### Slots

`gateslot.Pool` is a set of file locks on one machine.
Septim needs **its own pool**, with its locks on Septim, because two hubs or two calling machines would share it.
The process that holds a Septim slot is the SSH session, so a crashed or cut caller releases it.

A tier's acquire then waits on two pools: take a free remote slot, else a free local one,
else queue on both and drop the loser.
Waiting on Septim through the session keeps the pool's priority tickets working there.
A tier with `slots: all` takes every slot of one machine.

The non-tier holders stay local at first: card `**Verify:**` lines, drift commands, the plan lint's type load
and `cmd/testtiming`. So do `lym gate red-first`, whose Go overlay names absolute local paths, and `--copies`.

### Sync

- Take the file list from git, tracked and untracked but not ignored, through `internal/gitrepo`,
  and send it with `rsync` so that after the first copy only changes travel.
  LoomYard's tracked tree is about 6500 files and 108 MB.
- Delete files on Septim that the list no longer holds, or a removed test goes on running.
- Keep one directory per hub and worktree, locked for the length of a run.
- A worktree's `.git` is a file pointing into the caller's disk, so it cannot be copied.
  `git init` the copy and commit nothing but what the stray-file listing needs.
  Whether any test needs the real history was not established; none was found by search.
- `-trimpath` is already on every gate build, so worktrees at different paths share Septim's build cache.

### Configuration

- `gate.yaml`, hub file: a new guard key naming the mode (`none`, `stored`, `ephemeral`), default `none`,
  and the tiers that may be sent.
- Machine override: the host, the SSH alias and the slot count, since reachability belongs to the calling machine.

### Finding a dead or sleeping Septim

- Probe before asking for a slot, with `BatchMode=yes` and a `ConnectTimeout` of a few seconds.
  Remember a failure for a short backoff, so a card gate does not pay the timeout each time.
- During a run set `ServerAliveInterval` and `ServerAliveCountMax` (both ends default to never probing;
  15 s with the default count of 3 gives up after about 45 s), and `ClientAliveInterval` on Septim's `sshd`.
- Without a terminal, a remote command is likely not signalled when the connection drops **(unverified)**.
  The runner must end its tree when its input closes, and its own timeout backs that up.
- **Before the run starts,** any failure means a local slot, with one line saying why.
- **During the run,** a stream without a result message is no verdict:
  rerun the tier once locally, as `retryTier` already does for a runner crash, and never record it as a failed test.
- A host that answers with another Go version, tool floor or `lym` protocol version counts as absent.
  The green record does not hold the toolchain, so a mismatch must be refused rather than recorded.
- `ControlMaster` with `ControlPersist` saves a handshake per run.
- Wake-on-LAN, if wanted, sends the packet and runs locally this time; no gate waits for a wake.

### Tests that behave differently there

A test that passes on Neve may fail under WSL2 through timing, `/proc`, tmux version or systemd differences.
Each such failure reaches an agent or a fixer as a real one.
Start with one tier, compare verdicts on identical trees,
and keep a switch that turns the remote off without editing the hub file.

## Ephemeral mode

### What can stay on Septim

| Where | During a run | After a run | Measure |
|---|---|---|---|
| Source and build output in tmpfs | RAM | Gone on unmount or delete; freed pages are not wiped **(unverified)** | One tmpfs per run, in a mount that dies with the run's unit. |
| tmpfs pages swapped out | WSL's swap file, `%Temp%\swap.vhdx`, 25% of memory by default | Stays in the file | `swap=0` in `.wslconfig`; mount with `noswap`. |
| VM memory paged or hibernated by Windows | `pagefile.sys`, `hiberfil.sys` **(unverified)** | May stay | Not controllable from WSL2. Turn off hibernation and fast startup; encrypt the disk; decide about the page file. |
| Go build cache | Compiled work code | On disk if `GOCACHE` is left at its default | `GOCACHE` in the run's tmpfs. |
| Test binaries, Go work directory | `GOTMPDIR`, `TMPDIR` | Left behind after a kill | Both in the tmpfs. |
| Test temp dirs, test hubs, tmux sockets, `.lym` logs | Under `TMPDIR` and the tree | Same | Same tmpfs. Keep its path short for tmux socket limits. |
| `HOME`, XDG caches, Go telemetry, MSBuild and NuGet scratch | Written by tools | On disk by default | Point `HOME` and the XDG variables into the tmpfs. |
| Module cache, NuGet packages | Dependencies | Kept | Public ones may stay on disk. A private work module must be fetched into RAM. |
| Process core dumps | WSL pipes them to a capture program **(unverified)** | WSL keeps up to 10 crash dumps in `%Temp%\wsl-crashes`, on the Windows disk | `ulimit -c 0` is not enough: the limit is not enforced for a piped core. Change `core_pattern` and set `maxCrashDumpCount`; whether 0 disables it is not stated. |
| journald, `sshd` logs | Command lines | Kept | Send everything about the tree on standard input, never in arguments; the runner logs no test output. |
| Run logs and records | Streamed to the caller | None on Septim | The caller writes them, as today. |

Pointing variables at the tmpfs is discipline that one forgetful tool breaks.
Enforce it: run each tier in its own systemd unit that sees the filesystem read-only
except the run's tmpfs, with swap denied **(unverified)**,
and test the guarantee by listing what changed on the distribution's disk across a run.
tmpfs defaults to half of the machine's memory, which inside WSL2's default is a quarter of Septim's;
size it explicitly. An oversized tmpfs can deadlock the machine, and a full one fails writes.

### No checkout

The same file list and `rsync` fill the tmpfs, in full on every run.
No git checkout is made, so the stray-file check compares a file manifest instead of asking git.
A tier whose tests need a git repository around the code would fail; that needs a trial run.
Uploading LoomYard's 108 MB takes about a second on a wired gigabit LAN, longer for a larger tree.

### What it costs without a warm cache

Septim was not measured. Neighbouring figures:

- The full integration suite with an empty `GOCACHE`, on OSL-1046 on 2026-10-20, took 184 to 220 s wall
  and 900 to 1040 CPU-seconds, against 688 CPU-seconds for an uncached test run over a warm build cache that day
  ([gate-test-xdr](../../benchmarks/gate-test-xdr.md)).
  The runs were not made alike, and the suite has grown since.
- Narrow runs lose far more: a coverage build of one package with 49 linked packages took 13 s on an empty cache
  and 0.3 s on a repeat ([testmap-miss-causes](../speed/testmap-miss-causes-2026-11-15.md)).
  A cold `tier0` run, whose warm median on Neve is 8 s, would mostly compile.

| Cache choice | Speed | Fits "stored nowhere" |
|---|---|---|
| Empty cache each run | Slowest: the standard library and every dependency recompile | Yes |
| A cache of the standard library and public dependencies only, built once, copied into the tmpfs per run | Only the work code recompiles; pays a copy per run | Yes: it holds no work code |
| One RAM cache kept between runs | Near stored mode until a restart | No: compiled work code outlives the run |

The second is the one to build. Its size, and so its copy time, is unmeasured.
For C#, `bin` and `obj` are lost after each run, so every run is a full build of the solution.

### Decide these first

1. **Is the guarantee enough?** Everything above stops the run from writing work code to disk.
   It cannot stop Windows from paging or hibernating VM memory, and Windows administrators can read the VM.
   Septim is inside the trust boundary, as a machine running a model server is.
2. **May the work code leave OSL-1046 at all?** For work hubs the caller is the laptop, not Neve,
   so the laptop must reach the closed LAN, and the employer's rules and the laptop's Cortex XDR apply.
3. **Does a kept RAM cache count as stored?** If not, the speed gain rests on the dependency-only cache.

## Size of the change

| # | Task | Mode |
|---|---|---|
| 1 | An executor seam under the tier run, local only, with no change in behaviour | both |
| 2 | The remote runner verb: framed protocol, tied child, guards, prebuild, stray check, its own slot pool | both |
| 3 | Transport and sync: one module that owns `ssh` and `rsync`, with recorded stand-ins and a contract test | both |
| 4 | Config keys, choosing between pools, dead-host detection and fallback, host in records and status | both |
| 5 | Septim setup document, then a trial that compares times and verdicts | both |
| 6 | The ephemeral run area: tmpfs unit, redirected environment, manifest check, sweep of leftovers | ephemeral |
| 7 | The dependency-only cache, and a test that shows what a run writes to disk | ephemeral |
| 8 | TRX return, offline NuGet and project scoping on the remote | a C# hub only |

Tasks 2 and 4 are the large ones.
Task 4 touches the slot pool every gate shares, so a fault there stalls landings.

## Risks that could make a mode not worth it

**Both modes**

- Septim is not much faster than Neve under WSL2, or sync and connection eat the gain on short runs.
- Septim is often asleep, restarted by Windows updates or in use by the operator,
  so runs fall back and Neve keeps its queue.
- WSL2 does not stay up reliably without a logged-in user.
- WSL-only test failures cost more agent and fixer time than the queue did.
- Keeping versions equal on an offline machine becomes a chore.
- A cheaper cut removes the queue first:
  [gate-time-2026-11-28](../webster/gate-time-2026-11-28.md) ranks cuts of the card and batch gates' scope
  worth about 9 to 15 min per implementer row each, and
  [gate-fixed-cost-2026-11-27](../speed/gate-fixed-cost-2026-11-27/report.md) lists fixed per-call costs to remove.

**Ephemeral only**

- Cold builds make Septim slower than the laptop for the runs that matter.
- The disk guarantee cannot be shown well enough for work code.
- Septim's memory is too small for the tree, cache and test temp files of parallel slots.
- Work hubs are C#, which adds task 8 and may need Windows to build.

## Recommendation

**Stored: build it, after a measurement.**
Set up WSL2 on Septim by hand, copy the tree and run `tier1` and `tier2` cold once and warm twice,
beside the same runs on Neve with the same load noted.
Include an unattended restart and a night left idle, to settle the keep-alive and sleep questions.
Go on if Septim's warm `tier1` is clearly faster than Neve's under normal load and no test fails there alone.
Then do tasks 1 to 5.

**Ephemeral: wait.**
Answer the three questions, then measure one run with an empty cache and one with the dependency-only cache by hand.
Build tasks 6 and 7 only if the second beats the laptop and the guarantee in question 1 is accepted.
Until then work hubs name no mode and never send a run.

## Sources

- <https://learn.microsoft.com/en-us/windows/wsl/networking>
- <https://learn.microsoft.com/en-us/windows/wsl/wsl-config>
- <https://learn.microsoft.com/en-us/windows/wsl/systemd>
- <https://learn.microsoft.com/en-us/windows/wsl/filesystems>
- <https://learn.microsoft.com/en-us/windows/wsl/faq>
- <https://devblogs.microsoft.com/commandline/background-task-support-in-wsl/>
- <https://man.openbsd.org/ssh.1>
- <https://man.openbsd.org/ssh_config> (read in two parts)
- <https://man.openbsd.org/sshd_config>
- <https://man.openbsd.org/sshd>
- <https://www.kernel.org/doc/html/latest/filesystems/tmpfs.html>
- <https://man7.org/linux/man-pages/man5/core.5.html>
