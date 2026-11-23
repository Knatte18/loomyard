//go:build ignore

// landlock-probe measures what Landlock on the running kernel can deny an unprivileged process tree.
// It reports the kernel's Landlock ABI and whether the kernel accepts a pathname Unix-socket right or scope.
// It then tries socket connects, file writes, a signal and commands once unrestricted.
// It restricts itself with Landlock on every thread.
// It tries them again in this process and in a re-executed child.
// Run it from the repository root with cgo off, since the all-threads syscall the restriction needs refuses cgo:
//
//	CGO_ENABLED=0 go run docs/research/platform/landlock-probe/main.go -write-allow <dir> -allow-socket <path> ...
//
// It connects to each socket and closes it without sending a byte, and removes every file it creates.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

// Candidate bits for pathname Unix-socket control, which kernels before Landlock ABI 9 refuse with EINVAL.
// candidateResolveUnixRight is the next free filesystem right, the form of the "resolve unix" patch series;
// candidatePathnameScope is the next free scope bit, the form of the "scope pathname sockets" series.
const (
	candidateResolveUnixRight = uint64(1) << 16
	candidatePathnameScope    = uint64(1) << 2
)

// childFlag marks the re-executed child, which runs the checks only, inside the parent's Landlock domain.
const childFlag = "-landlock-probe-child"

// stringList is a repeatable string flag.
type stringList []string

func (s *stringList) String() string     { return strings.Join(*s, ",") }
func (s *stringList) Set(v string) error { *s = append(*s, v); return nil }

type probeConfig struct {
	writeAllow   string
	writeDeny    stringList
	writeFiles   stringList
	readWrite    stringList
	readOnly     stringList
	readChecks   stringList
	allowSockets stringList
	connects     stringList
	commands     stringList
	signalPID    int
	widen        string
}

func main() {
	isChild := len(os.Args) > 1 && os.Args[1] == childFlag
	args := os.Args[1:]
	if isChild {
		args = args[1:]
	}
	cfg := parseFlags(args)
	if isChild {
		runChecks("child", cfg)
		return
	}

	abi := landlockABI()
	fmt.Printf("kernel ABI: %d\n", abi)
	fmt.Printf("kernel errata: %s\n", landlockErrata())
	resolveUnix := acceptsRuleset(unix.LandlockRulesetAttr{Access_fs: unix.LANDLOCK_ACCESS_FS_READ_FILE | candidateResolveUnixRight})
	pathnameScope := acceptsRuleset(unix.LandlockRulesetAttr{Scoped: candidatePathnameScope})
	fmt.Printf("candidate fs right 1<<16 (pathname socket connect): %s\n", describe(resolveUnix))
	fmt.Printf("candidate scope 1<<2 (pathname socket scope): %s\n", describe(pathnameScope))

	runChecks("unrestricted", cfg)

	// A thread that exists before the restriction checks it after, to show the restriction reached every thread.
	threadReady := make(chan struct{})
	threadGo := make(chan struct{})
	threadDone := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		close(threadReady)
		<-threadGo
		fmt.Printf("restricted-other-thread tid=%d\n", unix.Gettid())
		checkWrites("restricted-other-thread", cfg)
		close(threadDone)
	}()
	<-threadReady

	if err := restrict(abi, cfg, resolveUnix == nil, pathnameScope == nil); err != nil {
		fmt.Printf("restrict: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("restricted tid=%d\n", unix.Gettid())
	runChecks("restricted", cfg)
	close(threadGo)
	<-threadDone

	child := exec.Command("/proc/self/exe", append([]string{childFlag}, args...)...)
	child.Stdout = os.Stdout
	child.Stderr = os.Stderr
	if err := child.Run(); err != nil {
		fmt.Printf("child: %v\n", err)
	}
}

func parseFlags(args []string) probeConfig {
	var cfg probeConfig
	fs := flag.NewFlagSet("landlock-probe", flag.ExitOnError)
	fs.StringVar(&cfg.writeAllow, "write-allow", "", "directory granted every filesystem right; the probe creates and removes a file in it")
	fs.Var(&cfg.writeDeny, "write-deny", "directory granted read only; the probe tries to create a file in it (repeatable)")
	fs.Var(&cfg.writeFiles, "write-file", "file the probe opens for append, or creates and removes when absent (repeatable)")
	fs.Var(&cfg.readWrite, "rw", "extra path granted every filesystem right, such as a cache or the prime's .git (repeatable)")
	fs.Var(&cfg.readOnly, "ro", "path granted read and execute, in place of the default / (repeatable)")
	fs.Var(&cfg.readChecks, "read", "path the probe opens read-only and closes (repeatable)")
	fs.StringVar(&cfg.widen, "widen", "", "directory the child grants itself every right on in a second ruleset, then tries to write")
	fs.Var(&cfg.allowSockets, "allow-socket", "socket path granted the candidate connect right when the kernel has it (repeatable)")
	fs.Var(&cfg.connects, "connect", "socket path to connect to, @name for an abstract socket (repeatable)")
	fs.Var(&cfg.commands, "cmd", "shell command the restricted child runs (repeatable)")
	fs.IntVar(&cfg.signalPID, "signal-pid", 0, "process outside the domain to send signal 0 to")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	return cfg
}

// runChecks tries every connect, read, write and signal.
// In the child phase it also tries every command and then the widening ruleset.
func runChecks(phase string, cfg probeConfig) {
	for _, target := range cfg.connects {
		fmt.Printf("%s connect %s: %s\n", phase, target, describe(connectUnix(target)))
	}
	for _, path := range cfg.readChecks {
		fmt.Printf("%s read %s: %s\n", phase, path, describe(openAndClose(path, os.O_RDONLY)))
	}
	checkWrites(phase, cfg)
	if cfg.signalPID > 0 {
		fmt.Printf("%s signal-0 pid %d: %s\n", phase, cfg.signalPID, describe(unix.Kill(cfg.signalPID, 0)))
	}
	if phase != "child" {
		return
	}
	for _, command := range cfg.commands {
		out, err := exec.Command("/bin/sh", "-c", command).CombinedOutput()
		firstLine, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
		fmt.Printf("%s cmd %q: %s | %s\n", phase, command, describe(err), firstLine)
	}
	if cfg.widen == "" {
		return
	}
	// A second ruleset granting more than the first: Landlock intersects nested domains, so the write stays denied.
	if err := restrict(landlockABI(), probeConfig{writeAllow: cfg.widen, readOnly: cfg.readOnly}, false, false); err != nil {
		fmt.Printf("%s widen: %v\n", phase, err)
		return
	}
	fmt.Printf("%s widened write %s: %s\n", phase, cfg.widen, describe(createAndRemove(cfg.widen, phase)))
}

func checkWrites(phase string, cfg probeConfig) {
	if cfg.writeAllow != "" {
		fmt.Printf("%s write %s: %s\n", phase, cfg.writeAllow, describe(createAndRemove(cfg.writeAllow, phase)))
	}
	for _, dir := range cfg.writeDeny {
		fmt.Printf("%s write %s: %s\n", phase, dir, describe(createAndRemove(dir, phase)))
	}
	for _, path := range cfg.writeFiles {
		fmt.Printf("%s write-file %s: %s\n", phase, path, describe(openForWrite(path)))
	}
}

func openAndClose(path string, flags int) error {
	f, err := os.OpenFile(path, flags, 0)
	if err != nil {
		return err
	}
	return f.Close()
}

// openForWrite opens an existing file for append and closes it without writing.
// An absent file it creates and removes again.
func openForWrite(path string) error {
	err := openAndClose(path, os.O_WRONLY|os.O_APPEND)
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := openAndClose(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY); err != nil {
		return err
	}
	return os.Remove(path)
}

func createAndRemove(dir, phase string) error {
	path := filepath.Join(dir, fmt.Sprintf(".landlock-probe-%s-%d", phase, os.Getpid()))
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	f.Close()
	return os.Remove(path)
}

func connectUnix(target string) error {
	fd, err := unix.Socket(unix.AF_UNIX, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	return unix.Connect(fd, &unix.SockaddrUnix{Name: target})
}

func describe(err error) string {
	if err == nil {
		return "ok"
	}
	var errno syscall.Errno
	if errors.As(err, &errno) {
		return fmt.Sprintf("%s (%v)", unix.ErrnoName(errno), errno)
	}
	return err.Error()
}

func landlockABI() int {
	abi, _, errno := unix.Syscall(unix.SYS_LANDLOCK_CREATE_RULESET, 0, 0, unix.LANDLOCK_CREATE_RULESET_VERSION)
	if errno != 0 {
		return -int(errno)
	}
	return int(abi)
}

func landlockErrata() string {
	errata, _, errno := unix.Syscall(unix.SYS_LANDLOCK_CREATE_RULESET, 0, 0, unix.LANDLOCK_CREATE_RULESET_ERRATA)
	if errno != 0 {
		return describe(errno)
	}
	return fmt.Sprintf("%#x", errata)
}

// acceptsRuleset reports nil when the kernel creates a ruleset with attr, so it knows every bit attr sets.
func acceptsRuleset(attr unix.LandlockRulesetAttr) error {
	fd, err := createRuleset(attr)
	if err != nil {
		return err
	}
	unix.Close(fd)
	return nil
}

func createRuleset(attr unix.LandlockRulesetAttr) (int, error) {
	fd, _, errno := unix.Syscall(unix.SYS_LANDLOCK_CREATE_RULESET, uintptr(unsafe.Pointer(&attr)), unsafe.Sizeof(attr), 0)
	if errno != 0 {
		return -1, errno
	}
	return int(fd), nil
}

// handledFilesystemRights returns every filesystem right the kernel's ABI knows.
func handledFilesystemRights(abi int) uint64 {
	rights := uint64(unix.LANDLOCK_ACCESS_FS_MAKE_SYM<<1 - 1)
	if abi >= 2 {
		rights |= unix.LANDLOCK_ACCESS_FS_REFER
	}
	if abi >= 3 {
		rights |= unix.LANDLOCK_ACCESS_FS_TRUNCATE
	}
	if abi >= 5 {
		rights |= unix.LANDLOCK_ACCESS_FS_IOCTL_DEV
	}
	return rights
}

// restrict builds the probe's ruleset and applies it to every thread of this process.
// It handles every filesystem right and every scope the ABI knows, plus the candidate socket bits the kernel accepted.
// It grants read and execute under each ro path, or under / when none is given.
// It grants every right under the write-allow and rw paths.
// It grants the candidate connect right on each allow-socket path.
func restrict(abi int, cfg probeConfig, hasResolveUnix, hasPathnameScope bool) error {
	if abi < 1 {
		return fmt.Errorf("landlock unavailable: ABI %d", abi)
	}
	handled := handledFilesystemRights(abi)
	if hasResolveUnix {
		handled |= candidateResolveUnixRight
	}
	attr := unix.LandlockRulesetAttr{Access_fs: handled}
	if abi >= 6 {
		attr.Scoped = unix.LANDLOCK_SCOPE_ABSTRACT_UNIX_SOCKET | unix.LANDLOCK_SCOPE_SIGNAL
	}
	if hasPathnameScope {
		attr.Scoped |= candidatePathnameScope
	}
	rulesetFD, err := createRuleset(attr)
	if err != nil {
		return fmt.Errorf("create ruleset: %w", err)
	}
	defer unix.Close(rulesetFD)

	readExecute := uint64(unix.LANDLOCK_ACCESS_FS_EXECUTE | unix.LANDLOCK_ACCESS_FS_READ_FILE | unix.LANDLOCK_ACCESS_FS_READ_DIR)
	readPaths := cfg.readOnly
	if len(readPaths) == 0 {
		readPaths = []string{"/"}
	}
	for _, path := range readPaths {
		if err := addPathRule(rulesetFD, path, readExecute); err != nil {
			return err
		}
	}
	fullRights := handled &^ candidateResolveUnixRight
	for _, path := range append([]string{cfg.writeAllow}, cfg.readWrite...) {
		if path == "" {
			continue
		}
		if err := addPathRule(rulesetFD, path, fullRights); err != nil {
			return err
		}
	}
	if hasResolveUnix {
		for _, path := range cfg.allowSockets {
			if err := addPathRule(rulesetFD, path, candidateResolveUnixRight); err != nil {
				fmt.Printf("allow-socket %s: %v\n", path, err)
			}
		}
	}

	// no_new_privs must hold on every thread before any of them restricts itself.
	if _, _, errno := syscall.AllThreadsSyscall(unix.SYS_PRCTL, unix.PR_SET_NO_NEW_PRIVS, 1, 0); errno != 0 {
		return fmt.Errorf("no_new_privs on all threads: %w", errno)
	}
	if abi >= 8 {
		if _, _, errno := unix.Syscall(unix.SYS_LANDLOCK_RESTRICT_SELF, uintptr(rulesetFD), unix.LANDLOCK_RESTRICT_SELF_TSYNC, 0); errno != 0 {
			return fmt.Errorf("restrict self with TSYNC: %w", errno)
		}
		fmt.Println("restricted with LANDLOCK_RESTRICT_SELF_TSYNC")
		return nil
	}
	if _, _, errno := syscall.AllThreadsSyscall(unix.SYS_LANDLOCK_RESTRICT_SELF, uintptr(rulesetFD), 0, 0); errno != 0 {
		return fmt.Errorf("restrict self on all threads: %w", errno)
	}
	fmt.Println("restricted through AllThreadsSyscall")
	return nil
}

func addPathRule(rulesetFD int, path string, access uint64) error {
	fd, err := unix.Open(path, unix.O_PATH|unix.O_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer unix.Close(fd)
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return fmt.Errorf("stat %s: %w", path, err)
	}
	if st.Mode&unix.S_IFMT != unix.S_IFDIR {
		access &= unix.LANDLOCK_ACCESS_FS_EXECUTE | unix.LANDLOCK_ACCESS_FS_WRITE_FILE | unix.LANDLOCK_ACCESS_FS_READ_FILE |
			unix.LANDLOCK_ACCESS_FS_TRUNCATE | unix.LANDLOCK_ACCESS_FS_IOCTL_DEV | candidateResolveUnixRight
	}
	rule := unix.LandlockPathBeneathAttr{Allowed_access: access, Parent_fd: int32(fd)}
	_, _, errno := unix.Syscall6(unix.SYS_LANDLOCK_ADD_RULE, uintptr(rulesetFD), unix.LANDLOCK_RULE_PATH_BENEATH,
		uintptr(unsafe.Pointer(&rule)), 0, 0, 0)
	if errno != 0 {
		return fmt.Errorf("add rule %s: %w", path, errno)
	}
	return nil
}
