//go:build ignore

// same-uid-probe measures whether a pathname Unix socket can be taken away from a process of the same user.
// It needs no namespace, no root and no guard on connect().
// It listens on a socket in a 0700 directory of its own and tries to hide it three ways:
// the socket's mode, the directory's mode, and a Landlock ruleset that grants nothing on the directory.
// It reads /proc/net/unix for the named sockets.
// It runs bus and SSH agent clients with their environment stripped, and again with only a hard-coded address.
// The Landlock checks run in a re-executed child that restricts itself, so the parent can restore modes and clean up.
// Run it from the repository root with cgo off, since the all-threads syscall the restriction needs refuses cgo:
//
//	CGO_ENABLED=0 go run docs/research/platform/same-uid-probe/main.go -dir <scratch dir> -find <socket path> ...
//
// It connects to each named socket and closes it without sending a byte;
// ssh-add prints only its exit status, so no key is listed; every file it creates is removed.
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

// childFlag marks the re-executed child, which runs the Landlock checks.
const childFlag = "-same-uid-probe-child"

// systemReadPaths are the directories the child's Landlock ruleset grants read and execute on;
// neither /tmp nor /home is among them.
var systemReadPaths = []string{"/usr", "/etc", "/bin", "/lib", "/lib64", "/sbin", "/opt", "/proc", "/sys", "/dev", "/run"}

// stringList is a repeatable string flag.
type stringList []string

func (s *stringList) String() string     { return strings.Join(*s, ",") }
func (s *stringList) Set(v string) error { *s = append(*s, v); return nil }

type probeConfig struct {
	dir      string
	finds    stringList
	sshAgent string
}

func main() {
	isChild := len(os.Args) > 1 && os.Args[1] == childFlag
	args := os.Args[1:]
	if isChild {
		args = args[1:]
	}
	cfg := parseFlags(args)
	hidden := filepath.Join(cfg.dir, "hidden")
	socket := filepath.Join(hidden, "s.sock")
	granted := filepath.Join(cfg.dir, "granted")
	if isChild {
		runLandlockChecks(cfg, hidden, socket, granted)
		return
	}

	if err := os.Mkdir(hidden, 0o700); err != nil {
		fmt.Printf("mkdir %s: %v\n", hidden, err)
		os.Exit(1)
	}
	defer os.Remove(hidden)
	if err := os.Mkdir(granted, 0o700); err != nil {
		fmt.Printf("mkdir %s: %v\n", granted, err)
		os.Exit(1)
	}
	defer os.Remove(granted)
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: socket, Net: "unix"})
	if err != nil {
		fmt.Printf("listen %s: %v\n", socket, err)
		os.Exit(1)
	}
	defer listener.Close()
	go acceptAndClose(listener)

	fmt.Printf("connect %s: %s\n", socket, describe(connectUnix(socket)))
	fmt.Printf("socket mode 000, connect: %s\n", withMode(socket, 0, func() error { return connectUnix(socket) }))
	fmt.Printf("socket chmod back to 0700 by its owner: %s\n", describe(os.Chmod(socket, 0o700)))
	fmt.Printf("directory mode 000, connect: %s\n", withMode(hidden, 0, func() error { return connectUnix(socket) }))
	reportListed("unrestricted", append([]string{socket}, cfg.finds...))
	runEnvironmentChecks(cfg)

	// The child finds the socket at mode 000, as a hub would leave another hub's socket.
	if err := os.Chmod(socket, 0); err != nil {
		fmt.Printf("chmod %s: %v\n", socket, err)
	}
	child := exec.Command("/proc/self/exe", append([]string{childFlag}, args...)...)
	child.Stdout, child.Stderr = os.Stdout, os.Stderr
	if err := child.Run(); err != nil {
		fmt.Printf("child: %v\n", err)
	}
	os.Chmod(hidden, 0o700)
	os.Chmod(socket, 0o700)
}

func parseFlags(args []string) probeConfig {
	var cfg probeConfig
	fs := flag.NewFlagSet("same-uid-probe", flag.ExitOnError)
	fs.StringVar(&cfg.dir, "dir", "", "scratch directory the probe creates its hidden and granted directories in")
	fs.Var(&cfg.finds, "find", "socket path to look up in /proc/net/unix and connect to from the child (repeatable)")
	fs.StringVar(&cfg.sshAgent, "ssh-agent", "", "SSH agent socket ssh-add is pointed at with a stripped environment")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if cfg.dir == "" {
		fmt.Println("-dir is required")
		os.Exit(2)
	}
	return cfg
}

func acceptAndClose(listener *net.UnixListener) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		conn.Close()
	}
}

// withMode sets path's mode, runs check, restores mode 0700 and describes the check's result.
func withMode(path string, mode os.FileMode, check func() error) string {
	if err := os.Chmod(path, mode); err != nil {
		return "chmod: " + describe(err)
	}
	result := describe(check())
	os.Chmod(path, 0o700)
	return result
}

// reportListed says, for each socket path, whether /proc/net/unix names it.
func reportListed(phase string, paths []string) {
	listed := map[string]bool{}
	file, err := os.Open("/proc/net/unix")
	if err != nil {
		fmt.Printf("%s /proc/net/unix: %s\n", phase, describe(err))
		return
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 8 {
			listed[fields[7]] = true
		}
	}
	for _, path := range paths {
		fmt.Printf("%s /proc/net/unix lists %s: %v\n", phase, path, listed[path])
	}
}

// runEnvironmentChecks runs the bus and SSH agent clients with their addresses removed from the environment.
// It then runs them with only a hard-coded address.
func runEnvironmentChecks(cfg probeConfig) {
	busAddress := fmt.Sprintf("DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/%d/bus", os.Getuid())
	minimal := []string{"PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME")}
	runWithEnv("busctl --user status, DBUS_SESSION_BUS_ADDRESS unset",
		environWithout("DBUS_SESSION_BUS_ADDRESS"), "busctl", "--user", "status")
	runWithEnv("busctl --user status, DBUS_SESSION_BUS_ADDRESS and XDG_RUNTIME_DIR unset",
		environWithout("DBUS_SESSION_BUS_ADDRESS", "XDG_RUNTIME_DIR"), "busctl", "--user", "status")
	runWithEnv("systemd-run --user, DBUS_SESSION_BUS_ADDRESS and XDG_RUNTIME_DIR unset",
		environWithout("DBUS_SESSION_BUS_ADDRESS", "XDG_RUNTIME_DIR"), "systemd-run", "--user", "--quiet", "--collect", "true")
	runWithEnv("systemd-run --user, only PATH, HOME and a hard-coded bus address",
		append(minimal, busAddress), "systemd-run", "--user", "--quiet", "--collect", "true")
	runWithEnv("ssh-add -l, SSH_AUTH_SOCK unset", environWithout("SSH_AUTH_SOCK"), "ssh-add", "-l")
	if cfg.sshAgent != "" {
		runWithEnv("ssh-add -l, only PATH, HOME and a hard-coded SSH_AUTH_SOCK",
			append(minimal, "SSH_AUTH_SOCK="+cfg.sshAgent), "ssh-add", "-l")
	}
}

func environWithout(names ...string) []string {
	var env []string
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		keep := true
		for _, drop := range names {
			if name == drop {
				keep = false
			}
		}
		if keep {
			env = append(env, entry)
		}
	}
	return env
}

// runWithEnv runs a command with env and prints its exit status; it prints output only for a client other than ssh-add.
func runWithEnv(label string, env []string, name string, args ...string) {
	cmd := exec.Command(name, args...)
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	firstLine, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
	if name == "ssh-add" {
		firstLine = "(output withheld)"
	}
	status := "exit 0"
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		status = fmt.Sprintf("exit %d", exitErr.ExitCode())
	} else if err != nil {
		status = describe(err)
	}
	fmt.Printf("%s: %s | %s\n", label, status, firstLine)
}

// runLandlockChecks restricts this process with a Landlock ruleset.
// The ruleset grants read and execute on the system directories and every right on granted.
// It then tries to reach the socket the parent left at mode 000 in a directory the ruleset grants nothing on.
func runLandlockChecks(cfg probeConfig, hidden, socket, granted string) {
	if err := restrictLandlock(granted); err != nil {
		fmt.Printf("landlock: %v\n", err)
		os.Exit(1)
	}
	phase := "landlock"
	_, err := os.ReadDir(hidden)
	fmt.Printf("%s list %s: %s\n", phase, hidden, describe(err))
	fmt.Printf("%s create a file in %s: %s\n", phase, hidden, describe(createAndRemove(hidden)))
	fmt.Printf("%s create a file in %s: %s\n", phase, granted, describe(createAndRemove(granted)))
	var st unix.Stat_t
	err = unix.Stat(socket, &st)
	fmt.Printf("%s stat %s: %s, mode %#o\n", phase, socket, describe(err), st.Mode&0o777)
	fmt.Printf("%s connect at mode 000: %s\n", phase, describe(connectUnix(socket)))
	fmt.Printf("%s chmod the socket to 0700: %s\n", phase, describe(os.Chmod(socket, 0o700)))
	fmt.Printf("%s connect after the chmod: %s\n", phase, describe(connectUnix(socket)))
	fmt.Printf("%s remove the socket: %s\n", phase, describe(os.Remove(socket)))
	fmt.Printf("%s directory mode 000, connect: %s\n", phase, withMode(hidden, 0, func() error { return connectUnix(socket) }))
	reportListed(phase, append([]string{socket}, cfg.finds...))
	for _, path := range cfg.finds {
		fmt.Printf("%s connect %s: %s\n", phase, path, describe(connectUnix(path)))
	}
}

func createAndRemove(dir string) error {
	path := filepath.Join(dir, fmt.Sprintf(".same-uid-probe-%d", os.Getpid()))
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

// restrictLandlock applies, on every thread, every filesystem right of ABI 8 and the abstract-socket and signal scopes;
// it grants read and execute on systemReadPaths and every right on granted.
func restrictLandlock(granted string) error {
	abi, _, errno := unix.Syscall(unix.SYS_LANDLOCK_CREATE_RULESET, 0, 0, unix.LANDLOCK_CREATE_RULESET_VERSION)
	if errno != 0 || abi < 8 {
		return fmt.Errorf("need Landlock ABI 8 for TSYNC, have %d (%v)", abi, errno)
	}
	handled := uint64(unix.LANDLOCK_ACCESS_FS_MAKE_SYM<<1-1) | unix.LANDLOCK_ACCESS_FS_REFER |
		unix.LANDLOCK_ACCESS_FS_TRUNCATE | unix.LANDLOCK_ACCESS_FS_IOCTL_DEV
	attr := unix.LandlockRulesetAttr{
		Access_fs: handled,
		Scoped:    unix.LANDLOCK_SCOPE_ABSTRACT_UNIX_SOCKET | unix.LANDLOCK_SCOPE_SIGNAL,
	}
	rulesetFD, _, errno := unix.Syscall(unix.SYS_LANDLOCK_CREATE_RULESET, uintptr(unsafe.Pointer(&attr)), unsafe.Sizeof(attr), 0)
	if errno != 0 {
		return fmt.Errorf("create ruleset: %w", errno)
	}
	defer unix.Close(int(rulesetFD))
	readExecute := uint64(unix.LANDLOCK_ACCESS_FS_EXECUTE | unix.LANDLOCK_ACCESS_FS_READ_FILE | unix.LANDLOCK_ACCESS_FS_READ_DIR)
	for _, path := range systemReadPaths {
		if _, err := os.Stat(path); err != nil {
			continue
		}
		if err := addPathRule(int(rulesetFD), path, readExecute); err != nil {
			return err
		}
	}
	if err := addPathRule(int(rulesetFD), granted, handled); err != nil {
		return err
	}
	if _, _, errno := syscall.AllThreadsSyscall(unix.SYS_PRCTL, unix.PR_SET_NO_NEW_PRIVS, 1, 0); errno != 0 {
		return fmt.Errorf("no_new_privs on all threads: %w", errno)
	}
	if _, _, errno := unix.Syscall(unix.SYS_LANDLOCK_RESTRICT_SELF, rulesetFD, unix.LANDLOCK_RESTRICT_SELF_TSYNC, 0); errno != 0 {
		return fmt.Errorf("restrict self with TSYNC: %w", errno)
	}
	return nil
}

func addPathRule(rulesetFD int, path string, access uint64) error {
	fd, err := unix.Open(path, unix.O_PATH|unix.O_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer unix.Close(fd)
	rule := unix.LandlockPathBeneathAttr{Allowed_access: access, Parent_fd: int32(fd)}
	_, _, errno := unix.Syscall6(unix.SYS_LANDLOCK_ADD_RULE, uintptr(rulesetFD), unix.LANDLOCK_RULE_PATH_BENEATH,
		uintptr(unsafe.Pointer(&rule)), 0, 0, 0)
	if errno != 0 {
		return fmt.Errorf("add rule %s: %w", path, errno)
	}
	return nil
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
