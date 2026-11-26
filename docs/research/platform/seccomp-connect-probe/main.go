//go:build ignore

// seccomp-connect-probe measures whether a supervisor can decide every connect() of an unprivileged process tree.
// The supervisor uses seccomp user notification.
// It runs a child twice: once unfiltered, then under a seccomp filter that hands every connect() to this process.
// The supervisor reads the address from the child's memory and checks a Unix socket path against an allow list.
// It makes each allowed connect itself on the child's own socket, which it takes with pidfd_getfd,
// so the child cannot change the address between the check and the connect.
// The filter also refuses io_uring_setup and every system call of a foreign ABI.
// The child tries socket connects, io_uring, a peer-credential check and a connect timing.
// It also races a connect against a thread that swaps the address.
// It also connects from an orphaned process and runs shell commands.
// The filtered child ends by asking the supervisor to close its listener, as a crashed supervisor would.
// It then connects once more.
// With -landlock-rw the filtered child also restricts itself with Landlock before it installs the filter.
// It runs on x86-64 only.
// Run it from the repository root with cgo off, since the all-threads syscall that sets no_new_privs refuses cgo:
//
//	CGO_ENABLED=0 go run docs/research/platform/seccomp-connect-probe/main.go -scratch <dir> -allow-socket <path> ...
//
// It connects to each named socket and closes it without sending a byte, and removes every file it creates.
package main

import (
	"bufio"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// Re-exec flags: the probe child, a process that orphans its own child, and that orphaned child.
const (
	childFlag       = "-seccomp-probe-child"
	orphanMakerFlag = "-seccomp-probe-orphan-maker"
	lateConnectFlag = "-seccomp-probe-late-connect"
)

// pidfdThread is PIDFD_THREAD, which lets pidfd_open take the thread id a seccomp notification carries.
const pidfdThread = unix.O_EXCL

// x32SyscallBit marks a system call of the x32 ABI, which shares the x86-64 audit arch.
const x32SyscallBit = 0x40000000

// testSocketName is the supervisor's own listener in the scratch directory, always on the allow list.
const testSocketName = "sup.sock"

// seccompData, seccompNotif and seccompNotifResp mirror the kernel's seccomp structs of the same names.
type seccompData struct {
	Nr   int32
	Arch uint32
	IP   uint64
	Args [6]uint64
}

type seccompNotif struct {
	ID    uint64
	Pid   uint32
	Flags uint32
	Data  seccompData
}

type seccompNotifResp struct {
	ID    uint64
	Val   int64
	Error int32
	Flags uint32
}

// stringList is a repeatable string flag.
type stringList []string

func (s *stringList) String() string     { return strings.Join(*s, ",") }
func (s *stringList) Set(v string) error { *s = append(*s, v); return nil }

type probeConfig struct {
	mode         string
	scratch      string
	allowSockets stringList
	connects     stringList
	commands     stringList
	landlockRW   stringList
	writeChecks  stringList
	timingCount  int
}

func main() {
	if runtime.GOARCH != "amd64" {
		fmt.Println("the filter is written for x86-64 only")
		os.Exit(2)
	}
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case childFlag:
			runChild(parseFlags(os.Args[2:]))
			return
		case orphanMakerFlag:
			makeOrphan(os.Args[2:])
			return
		case lateConnectFlag:
			lateConnect(os.Args[2], os.Args[3])
			return
		}
	}
	runSupervisor(parseFlags(os.Args[1:]), os.Args[1:])
}

func parseFlags(args []string) probeConfig {
	var cfg probeConfig
	fs := flag.NewFlagSet("seccomp-connect-probe", flag.ExitOnError)
	fs.StringVar(&cfg.mode, "mode", "", "set by the supervisor for its child: baseline or filtered")
	fs.StringVar(&cfg.scratch, "scratch", "", "directory for the supervisor's test socket and the orphan result files")
	fs.Var(&cfg.allowSockets, "allow-socket", "socket path the supervisor connects for the child (repeatable)")
	fs.Var(&cfg.connects, "connect", "socket path to connect to, @name for an abstract socket (repeatable)")
	fs.Var(&cfg.commands, "cmd", "shell command the child runs (repeatable)")
	fs.Var(&cfg.landlockRW, "landlock-rw", "path the filtered child's Landlock ruleset grants every right on (repeatable)")
	fs.Var(&cfg.writeChecks, "write", "directory the child creates and removes a file in (repeatable)")
	fs.IntVar(&cfg.timingCount, "timing", 200, "connects to the test socket the child times")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if cfg.scratch == "" {
		fmt.Println("-scratch is required")
		os.Exit(2)
	}
	return cfg
}

// runSupervisor serves the test socket and runs the baseline child.
// It then runs the filtered child while it answers its notifications.
func runSupervisor(cfg probeConfig, args []string) {
	fmt.Printf("supervisor pid %d\n", os.Getpid())
	fmt.Printf("kernel.yama.ptrace_scope: %s\n", readTrimmed("/proc/sys/kernel/yama/ptrace_scope"))
	fmt.Printf("kernel.io_uring_disabled: %s\n", readTrimmed("/proc/sys/kernel/io_uring_disabled"))

	testSocket := filepath.Join(cfg.scratch, testSocketName)
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: testSocket, Net: "unix"})
	if err != nil {
		fmt.Printf("listen %s: %v\n", testSocket, err)
		os.Exit(1)
	}
	defer listener.Close()
	go serveTestSocket(listener)

	policy, err := newSocketPolicy(append([]string{testSocket}, cfg.allowSockets...))
	if err != nil {
		fmt.Printf("allow list: %v\n", err)
		os.Exit(1)
	}

	baseline := exec.Command("/proc/self/exe", append([]string{childFlag, "-mode", "baseline"}, args...)...)
	baseline.Stdout, baseline.Stderr = os.Stdout, os.Stderr
	if err := baseline.Run(); err != nil {
		fmt.Printf("baseline child: %v\n", err)
	}

	log := &connectLog{counts: map[string]int{}}
	runFilteredChild(args, policy, log)
	log.print()
	reapOrphans()
}

func runFilteredChild(args []string, policy *socketPolicy, log *connectLog) {
	pair, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		fmt.Printf("socketpair: %v\n", err)
		return
	}
	control := pair[0]
	childEnd := os.NewFile(uintptr(pair[1]), "control-child")
	child := exec.Command("/proc/self/exe", append([]string{childFlag, "-mode", "filtered"}, args...)...)
	child.Stdout, child.Stderr = os.Stdout, os.Stderr
	child.ExtraFiles = []*os.File{childEnd}
	if err := child.Start(); err != nil {
		fmt.Printf("filtered child: %v\n", err)
		return
	}
	childEnd.Close()

	listenerFD, err := receiveFD(control)
	if err != nil {
		fmt.Printf("receive the filter's listener: %v\n", err)
		child.Wait()
		return
	}
	var closeOnce sync.Once
	closeListener := func() { closeOnce.Do(func() { unix.Close(listenerFD) }) }
	supervised := make(chan int)
	go func() { supervised <- superviseNotifications(listenerFD, policy, log) }()
	unix.Write(control, []byte{'A'})
	go serveControl(control, closeListener)

	if err := child.Wait(); err != nil {
		fmt.Printf("filtered child: %v\n", err)
	}
	select {
	case n := <-supervised:
		fmt.Printf("supervisor: %d notifications\n", n)
	case <-time.After(3 * time.Second):
		fmt.Println("supervisor: listener still in use 3 s after the child exited")
	}
	closeListener()
}

func receiveFD(socket int) (int, error) {
	buf := make([]byte, 1)
	oob := make([]byte, unix.CmsgSpace(4))
	_, oobn, _, _, err := unix.Recvmsg(socket, buf, oob, 0)
	if err != nil {
		return -1, err
	}
	msgs, err := unix.ParseSocketControlMessage(oob[:oobn])
	if err != nil || len(msgs) == 0 {
		return -1, fmt.Errorf("no control message: %v", err)
	}
	fds, err := unix.ParseUnixRights(&msgs[0])
	if err != nil || len(fds) == 0 {
		return -1, fmt.Errorf("no descriptor: %v", err)
	}
	return fds[0], nil
}

// serveControl answers the child's requests to turn this process's subreaper flag on or off.
// It also closes the filter's listener on request, as a crashed supervisor would.
func serveControl(control int, closeListener func()) {
	reader := bufio.NewReader(os.NewFile(uintptr(control), "control"))
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		switch strings.TrimSpace(line) {
		case "subreaper 0":
			unix.Prctl(unix.PR_SET_CHILD_SUBREAPER, 0, 0, 0, 0)
		case "subreaper 1":
			unix.Prctl(unix.PR_SET_CHILD_SUBREAPER, 1, 0, 0, 0)
		case "drop":
			closeListener()
		}
		unix.Write(control, []byte{'A'})
	}
}

// reapOrphans waits for orphans the subreaper flag adopted, so none is left a zombie.
func reapOrphans() {
	for {
		var status unix.WaitStatus
		pid, err := unix.Wait4(-1, &status, unix.WNOHANG, nil)
		if err != nil || pid <= 0 {
			return
		}
	}
}

// superviseNotifications answers every notification until no process uses the filter, and returns their count.
func superviseNotifications(listener int, policy *socketPolicy, log *connectLog) int {
	count := 0
	for {
		fds := []unix.PollFd{{Fd: int32(listener), Events: unix.POLLIN}}
		n, err := unix.Poll(fds, 500)
		if errors.Is(err, unix.EINTR) || n == 0 {
			continue
		}
		if err != nil {
			fmt.Printf("supervisor poll: %v\n", err)
			return count
		}
		if fds[0].Revents&unix.POLLNVAL != 0 {
			return count
		}
		if fds[0].Revents&unix.POLLIN == 0 && fds[0].Revents&unix.POLLHUP != 0 {
			return count
		}
		var notif seccompNotif
		if err := ioctlPointer(listener, unix.SECCOMP_IOCTL_NOTIF_RECV, unsafe.Pointer(&notif)); err != nil {
			continue
		}
		count++
		go handleConnect(listener, notif, policy, log)
	}
}

func handleConnect(listener int, notif seccompNotif, policy *socketPolicy, log *connectLog) {
	comm := readTrimmed(fmt.Sprintf("/proc/%d/comm", notif.Pid))
	errno, target := decideAndConnect(listener, notif, policy)
	resp := seccompNotifResp{ID: notif.ID, Error: -int32(errno)}
	if err := ioctlPointer(listener, unix.SECCOMP_IOCTL_NOTIF_SEND, unsafe.Pointer(&resp)); err != nil {
		target += " (answer: " + describe(err) + ")"
	}
	log.add(comm, target, errno)
}

// decideAndConnect reads the child's address, checks it, and makes an allowed connect on the child's socket.
// It returns the errno the child's connect() answers with and the target as the log shows it.
func decideAndConnect(listener int, notif seccompNotif, policy *socketPolicy) (unix.Errno, string) {
	tid := int(notif.Pid)
	addrLen := int(notif.Data.Args[2])
	if addrLen < 2 || addrLen > 128 {
		return unix.EINVAL, fmt.Sprintf("address length %d", addrLen)
	}
	addr := make([]byte, addrLen)
	if err := readTargetMemory(tid, notif.Data.Args[1], addr); err != nil {
		return unix.EPERM, "address unreadable: " + describe(err)
	}
	if !notificationValid(listener, notif.ID) {
		return unix.EPERM, "notification gone"
	}
	sockaddr, target, cleanup, errno := policy.resolve(tid, addr)
	defer cleanup()
	if errno != 0 {
		return errno, target
	}
	socketFD, err := takeTargetFD(tid, int(notif.Data.Args[0]))
	if err != nil {
		return errnoOf(err), target + " (pidfd_getfd: " + describe(err) + ")"
	}
	defer unix.Close(socketFD)
	return connectRaw(socketFD, sockaddr), target
}

func readTargetMemory(tid int, address uint64, buf []byte) error {
	mem, err := os.Open(fmt.Sprintf("/proc/%d/mem", tid))
	if err != nil {
		return err
	}
	defer mem.Close()
	_, err = mem.ReadAt(buf, int64(address))
	return err
}

func notificationValid(listener int, id uint64) bool {
	return ioctlPointer(listener, unix.SECCOMP_IOCTL_NOTIF_ID_VALID, unsafe.Pointer(&id)) == nil
}

// takeTargetFD duplicates the child's socket into this process; both descriptors share one open socket.
func takeTargetFD(tid, fd int) (int, error) {
	pidfd, err := unix.PidfdOpen(tid, pidfdThread)
	if err != nil {
		return -1, err
	}
	defer unix.Close(pidfd)
	return unix.PidfdGetfd(pidfd, fd, 0)
}

func connectRaw(fd int, sockaddr []byte) unix.Errno {
	_, _, errno := unix.Syscall(unix.SYS_CONNECT, uintptr(fd), uintptr(unsafe.Pointer(&sockaddr[0])), uintptr(len(sockaddr)))
	return errno
}

func ioctlPointer(fd int, request uintptr, arg unsafe.Pointer) error {
	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), request, uintptr(arg)); errno != 0 {
		return errno
	}
	return nil
}

// socketPolicy allows a pathname Unix socket by its canonical path, and every address of another family.
// It refuses every abstract Unix socket.
type socketPolicy struct {
	paths map[string]bool
}

// newSocketPolicy canonicalizes each allowed path; a socket not yet created is canonicalized through its directory.
func newSocketPolicy(allowed []string) (*socketPolicy, error) {
	policy := &socketPolicy{paths: map[string]bool{}}
	for _, path := range allowed {
		dir, err := filepath.EvalSymlinks(filepath.Dir(path))
		if err != nil {
			return nil, err
		}
		canonical := filepath.Join(dir, filepath.Base(path))
		if resolved, err := filepath.EvalSymlinks(path); err == nil {
			canonical = resolved
		}
		policy.paths[canonical] = true
	}
	return policy, nil
}

// resolve returns the address to connect to, the target as the log shows it, a cleanup, and an errno for a refusal.
// A pathname socket is opened with O_PATH and connected through /proc/self/fd,
// so the check and the connect see one inode even when the path changes in between.
func (p *socketPolicy) resolve(tid int, addr []byte) ([]byte, string, func(), unix.Errno) {
	noCleanup := func() {}
	family := binary.NativeEndian.Uint16(addr[:2])
	if family != unix.AF_UNIX {
		return addr, describeInet(family, addr), noCleanup, 0
	}
	path := addr[2:]
	if len(path) == 0 {
		return addr, "unnamed unix", noCleanup, 0
	}
	if path[0] == 0 {
		return nil, "@" + string(path[1:]), noCleanup, unix.EACCES
	}
	name := cString(path)
	lookup := name
	if !filepath.IsAbs(name) {
		lookup = fmt.Sprintf("/proc/%d/cwd/%s", tid, name)
	}
	pathFD, err := unix.Open(lookup, unix.O_PATH|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, name, noCleanup, errnoOf(err)
	}
	cleanup := func() { unix.Close(pathFD) }
	canonical, err := os.Readlink(fmt.Sprintf("/proc/self/fd/%d", pathFD))
	if err != nil {
		cleanup()
		return nil, name, noCleanup, unix.EACCES
	}
	target := name
	if canonical != name {
		target = name + " -> " + canonical
	}
	if !p.paths[canonical] {
		cleanup()
		return nil, target, noCleanup, unix.EACCES
	}
	return sockaddrUnix(fmt.Sprintf("/proc/self/fd/%d", pathFD)), target, cleanup, 0
}

func sockaddrUnix(path string) []byte {
	sockaddr := make([]byte, 2+len(path)+1)
	binary.NativeEndian.PutUint16(sockaddr, unix.AF_UNIX)
	copy(sockaddr[2:], path)
	return sockaddr
}

func describeInet(family uint16, addr []byte) string {
	switch {
	case family == unix.AF_INET && len(addr) >= 8:
		return fmt.Sprintf("inet %s:%d", net.IP(addr[4:8]), binary.BigEndian.Uint16(addr[2:4]))
	case family == unix.AF_INET6 && len(addr) >= 24:
		return fmt.Sprintf("inet6 [%s]:%d", net.IP(addr[8:24]), binary.BigEndian.Uint16(addr[2:4]))
	}
	return fmt.Sprintf("family %d", family)
}

func cString(b []byte) string {
	if i := strings.IndexByte(string(b), 0); i >= 0 {
		return string(b[:i])
	}
	return string(b)
}

// connectLog counts the supervisor's decisions by process name, target and result.
type connectLog struct {
	mu     sync.Mutex
	counts map[string]int
}

func (l *connectLog) add(comm, target string, errno unix.Errno) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.counts[fmt.Sprintf("%s -> %s: %s", comm, target, describe(errnoOrNil(errno)))]++
}

func (l *connectLog) print() {
	l.mu.Lock()
	defer l.mu.Unlock()
	keys := make([]string, 0, len(l.counts))
	for key := range l.counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		fmt.Printf("supervisor decided %dx %s\n", l.counts[key], key)
	}
}

// serveTestSocket answers each connection with its peer's SO_PEERCRED pid.
// It also answers with the SCM_CREDENTIALS pid of the connection's first message.
func serveTestSocket(listener *net.UnixListener) {
	for {
		conn, err := listener.AcceptUnix()
		if err != nil {
			return
		}
		go serveTestConnection(conn)
	}
}

func serveTestConnection(conn *net.UnixConn) {
	defer conn.Close()
	raw, err := conn.SyscallConn()
	if err != nil {
		return
	}
	var peer *unix.Ucred
	raw.Control(func(fd uintptr) {
		unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_PASSCRED, 1)
		peer, _ = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	})
	if peer == nil {
		return
	}
	if _, err := conn.Write([]byte("r")); err != nil {
		return
	}
	buf := make([]byte, 16)
	oob := make([]byte, unix.CmsgSpace(unix.SizeofUcred))
	n, oobn, _, _, err := conn.ReadMsgUnix(buf, oob)
	if err != nil || n == 0 {
		return
	}
	sender := "none"
	if msgs, err := unix.ParseSocketControlMessage(oob[:oobn]); err == nil && len(msgs) > 0 {
		if cred, err := unix.ParseUnixCredentials(&msgs[0]); err == nil {
			sender = strconv.Itoa(int(cred.Pid))
		}
	}
	fmt.Fprintf(conn, "SO_PEERCRED pid %d, SCM_CREDENTIALS pid %s\n", peer.Pid, sender)
}

// runChild runs every check; the filtered child first restricts itself and hands the filter's listener to the supervisor.
func runChild(cfg probeConfig) {
	phase := cfg.mode
	fmt.Printf("%s child pid %d\n", phase, os.Getpid())
	if phase == "filtered" {
		if len(cfg.landlockRW) > 0 {
			if err := restrictLandlock(cfg.landlockRW); err != nil {
				fmt.Printf("%s landlock: %v\n", phase, err)
				os.Exit(1)
			}
			fmt.Printf("%s restricted with Landlock\n", phase)
		}
		if err := installConnectFilter(); err != nil {
			fmt.Printf("%s seccomp: %v\n", phase, err)
			os.Exit(1)
		}
		fmt.Printf("%s seccomp filter installed, listener handed to the supervisor\n", phase)
	}
	testSocket := filepath.Join(cfg.scratch, testSocketName)

	for _, target := range cfg.connects {
		fmt.Printf("%s connect %s: %s\n", phase, target, describe(connectUnix(target)))
	}
	checkSymlinkAndRelative(phase, cfg.scratch, testSocket)
	fmt.Printf("%s io_uring_setup: %s\n", phase, describe(ioURingSetup()))
	peerCredentials(phase, testSocket)
	connectTiming(phase, testSocket, cfg.timingCount)
	raceConnect(phase, testSocket, cfg.timingCount)
	for _, dir := range cfg.writeChecks {
		fmt.Printf("%s write %s: %s\n", phase, dir, describe(createAndRemove(dir, phase)))
	}
	if phase == "filtered" {
		askSupervisor("subreaper 0")
		orphanConnect(phase+" no-subreaper", cfg.scratch, testSocket)
		askSupervisor("subreaper 1")
		orphanConnect(phase+" subreaper", cfg.scratch, testSocket)
	} else {
		orphanConnect(phase, cfg.scratch, testSocket)
	}
	for _, command := range cfg.commands {
		start := time.Now()
		out, err := exec.Command("/bin/sh", "-c", command).CombinedOutput()
		firstLine, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
		fmt.Printf("%s cmd %q (%d ms): %s | %s\n", phase, command, time.Since(start).Milliseconds(), describe(err), firstLine)
	}
	if phase == "filtered" {
		askSupervisor("drop")
		fmt.Printf("%s connect after the supervisor closed its listener: %s\n", phase, describe(connectUnix(testSocket)))
	}
}

// raceConnect connects count times from one address buffer.
// Another thread keeps flipping the buffer between the test socket and the session bus.
// It counts where each connection landed by its peer address.
func raceConnect(phase, testSocket string, count int) {
	allowed := sockaddrUnix(testSocket)
	denied := sockaddrUnix(fmt.Sprintf("/run/user/%d/bus", os.Getuid()))
	size := max(len(allowed), len(denied))
	allowedPadded := append(allowed, make([]byte, size-len(allowed))...)
	deniedPadded := append(denied, make([]byte, size-len(denied))...)
	shared := make([]byte, size)
	copy(shared, allowedPadded)
	stop := make(chan struct{})
	flipped := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		defer close(flipped)
		for {
			select {
			case <-stop:
				return
			default:
			}
			copy(shared, deniedPadded)
			copy(shared, allowedPadded)
		}
	}()
	landed := map[string]int{}
	for range count {
		fd, err := unix.Socket(unix.AF_UNIX, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
		if err != nil {
			continue
		}
		_, _, errno := unix.Syscall(unix.SYS_CONNECT, uintptr(fd), uintptr(unsafe.Pointer(&shared[0])), uintptr(size))
		switch {
		case errno != 0:
			landed[describe(errno)]++
		default:
			peer, err := unix.Getpeername(fd)
			if sockaddr, ok := peer.(*unix.SockaddrUnix); ok && err == nil {
				landed["peer "+sockaddr.Name]++
			} else {
				landed["peer unknown"]++
			}
		}
		unix.Close(fd)
	}
	close(stop)
	<-flipped
	keys := make([]string, 0, len(landed))
	for key := range landed {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		fmt.Printf("%s race: %dx %s\n", phase, landed[key], key)
	}
}

// installConnectFilter installs the filter on every thread and sends its listener to the supervisor on fd 3.
// It then waits for the supervisor's ack.
func installConnectFilter() error {
	if _, _, errno := syscall.AllThreadsSyscall(unix.SYS_PRCTL, unix.PR_SET_NO_NEW_PRIVS, 1, 0); errno != 0 {
		return fmt.Errorf("no_new_privs on all threads: %w", errno)
	}
	program := connectFilter()
	fprog := unix.SockFprog{Len: uint16(len(program)), Filter: &program[0]}
	flags := unix.SECCOMP_FILTER_FLAG_NEW_LISTENER | unix.SECCOMP_FILTER_FLAG_TSYNC | unix.SECCOMP_FILTER_FLAG_TSYNC_ESRCH
	listener, _, errno := unix.Syscall(unix.SYS_SECCOMP, unix.SECCOMP_SET_MODE_FILTER, uintptr(flags), uintptr(unsafe.Pointer(&fprog)))
	runtime.KeepAlive(program)
	if errno != 0 {
		return fmt.Errorf("install filter: %w", errno)
	}
	if err := unix.Sendmsg(3, []byte{'L'}, unix.UnixRights(int(listener)), nil, 0); err != nil {
		return fmt.Errorf("send listener: %w", err)
	}
	unix.Close(int(listener))
	ack := make([]byte, 1)
	if _, err := unix.Read(3, ack); err != nil {
		return fmt.Errorf("wait for the supervisor: %w", err)
	}
	return nil
}

// connectFilter refuses foreign ABIs and io_uring_setup, hands connect to the supervisor and allows everything else.
func connectFilter() []unix.SockFilter {
	statement := func(code uint16, k uint32) unix.SockFilter { return unix.SockFilter{Code: code, K: k} }
	jump := func(code uint16, k uint32, jt, jf uint8) unix.SockFilter {
		return unix.SockFilter{Code: code, Jt: jt, Jf: jf, K: k}
	}
	const (
		load      = unix.BPF_LD | unix.BPF_W | unix.BPF_ABS
		equals    = unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K
		bitsSet   = unix.BPF_JMP | unix.BPF_JSET | unix.BPF_K
		ret       = unix.BPF_RET | unix.BPF_K
		offsetNr  = 0
		offsetArc = 4
	)
	return []unix.SockFilter{
		statement(load, offsetArc),
		jump(equals, unix.AUDIT_ARCH_X86_64, 1, 0),
		statement(ret, unix.SECCOMP_RET_ERRNO|uint32(unix.ENOSYS)),
		statement(load, offsetNr),
		jump(bitsSet, x32SyscallBit, 0, 1),
		statement(ret, unix.SECCOMP_RET_ERRNO|uint32(unix.ENOSYS)),
		jump(equals, unix.SYS_CONNECT, 0, 1),
		statement(ret, unix.SECCOMP_RET_USER_NOTIF),
		jump(equals, unix.SYS_IO_URING_SETUP, 0, 1),
		statement(ret, unix.SECCOMP_RET_ERRNO|uint32(unix.EPERM)),
		statement(ret, unix.SECCOMP_RET_ALLOW),
	}
}

func askSupervisor(line string) {
	unix.Write(3, []byte(line+"\n"))
	ack := make([]byte, 1)
	unix.Read(3, ack)
}

// checkSymlinkAndRelative connects through a symlink to the session bus and a symlink to the test socket.
// It also connects to the test socket by a path relative to the working directory.
func checkSymlinkAndRelative(phase, scratch, testSocket string) {
	busLink := filepath.Join(scratch, fmt.Sprintf("bus-link-%d", os.Getpid()))
	if err := os.Symlink(fmt.Sprintf("/run/user/%d/bus", os.Getuid()), busLink); err == nil {
		fmt.Printf("%s connect symlink to the session bus: %s\n", phase, describe(connectUnix(busLink)))
		os.Remove(busLink)
	}
	testLink := filepath.Join(scratch, fmt.Sprintf("test-link-%d", os.Getpid()))
	if err := os.Symlink(testSocket, testLink); err == nil {
		fmt.Printf("%s connect symlink to the test socket: %s\n", phase, describe(connectUnix(testLink)))
		os.Remove(testLink)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return
	}
	if err := os.Chdir(scratch); err != nil {
		return
	}
	fmt.Printf("%s connect relative %s: %s\n", phase, testSocketName, describe(connectUnix(testSocketName)))
	os.Chdir(cwd)
}

func ioURingSetup() error {
	var params [120]byte
	fd, _, errno := unix.Syscall(unix.SYS_IO_URING_SETUP, 4, uintptr(unsafe.Pointer(&params[0])), 0)
	if errno != 0 {
		return errno
	}
	return unix.Close(int(fd))
}

func peerCredentials(phase, testSocket string) {
	fd, err := unix.Socket(unix.AF_UNIX, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return
	}
	defer unix.Close(fd)
	if err := unix.Connect(fd, &unix.SockaddrUnix{Name: testSocket}); err != nil {
		fmt.Printf("%s peer credentials: connect %s\n", phase, describe(err))
		return
	}
	buf := make([]byte, 96)
	unix.Read(fd, buf[:1])
	unix.Write(fd, []byte("hi"))
	n, _ := unix.Read(fd, buf)
	if n < 0 {
		n = 0
	}
	fmt.Printf("%s peer credentials: own pid %d, server saw %s\n", phase, os.Getpid(), strings.TrimSpace(string(buf[:n])))
}

func connectTiming(phase, testSocket string, count int) {
	if count <= 0 {
		return
	}
	start := time.Now()
	for range count {
		if err := connectUnix(testSocket); err != nil {
			fmt.Printf("%s timing: %s\n", phase, describe(err))
			return
		}
	}
	fmt.Printf("%s timing: %d connects, %.1f µs each\n", phase, count, float64(time.Since(start).Microseconds())/float64(count))
}

// orphanConnect starts a process whose parent exits at once, and reports that orphan's connect to the test socket.
func orphanConnect(label, scratch, testSocket string) {
	result := filepath.Join(scratch, fmt.Sprintf("orphan-%d-%s", os.Getpid(), strings.ReplaceAll(label, " ", "-")))
	if err := exec.Command("/proc/self/exe", orphanMakerFlag, testSocket, result).Run(); err != nil {
		fmt.Printf("%s orphan: %v\n", label, err)
		return
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(result)
		if err == nil && strings.HasSuffix(string(data), "\n") {
			fmt.Printf("%s orphan: %s", label, data)
			os.Remove(result)
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	fmt.Printf("%s orphan: no result within 5 s\n", label)
}

func makeOrphan(args []string) {
	orphan := exec.Command("/proc/self/exe", append([]string{lateConnectFlag}, args...)...)
	if err := orphan.Start(); err != nil {
		fmt.Printf("orphan: %v\n", err)
	}
}

func lateConnect(testSocket, result string) {
	time.Sleep(300 * time.Millisecond)
	parent := os.Getppid()
	err := connectUnix(testSocket)
	os.WriteFile(result, fmt.Appendf(nil, "pid %d, parent %d once orphaned, connect %s\n", os.Getpid(), parent, describe(err)), 0o600)
}

func connectUnix(target string) error {
	fd, err := unix.Socket(unix.AF_UNIX, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	return unix.Connect(fd, &unix.SockaddrUnix{Name: target})
}

func createAndRemove(dir, phase string) error {
	path := filepath.Join(dir, fmt.Sprintf(".seccomp-probe-%s-%d", phase, os.Getpid()))
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	f.Close()
	return os.Remove(path)
}

// restrictLandlock applies a Landlock ruleset on every thread: read and execute under /, every right under rwPaths,
// and the abstract-socket and signal scopes.
func restrictLandlock(rwPaths []string) error {
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
	if err := addPathRule(int(rulesetFD), "/", readExecute); err != nil {
		return err
	}
	for _, path := range rwPaths {
		if err := addPathRule(int(rulesetFD), path, handled); err != nil {
			return err
		}
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
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return fmt.Errorf("stat %s: %w", path, err)
	}
	if st.Mode&unix.S_IFMT != unix.S_IFDIR {
		access &= unix.LANDLOCK_ACCESS_FS_EXECUTE | unix.LANDLOCK_ACCESS_FS_WRITE_FILE | unix.LANDLOCK_ACCESS_FS_READ_FILE |
			unix.LANDLOCK_ACCESS_FS_TRUNCATE | unix.LANDLOCK_ACCESS_FS_IOCTL_DEV
	}
	rule := unix.LandlockPathBeneathAttr{Allowed_access: access, Parent_fd: int32(fd)}
	_, _, errno := unix.Syscall6(unix.SYS_LANDLOCK_ADD_RULE, uintptr(rulesetFD), unix.LANDLOCK_RULE_PATH_BENEATH,
		uintptr(unsafe.Pointer(&rule)), 0, 0, 0)
	if errno != 0 {
		return fmt.Errorf("add rule %s: %w", path, errno)
	}
	return nil
}

func readTrimmed(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return describe(err)
	}
	return strings.TrimSpace(string(data))
}

func errnoOf(err error) unix.Errno {
	var errno unix.Errno
	if errors.As(err, &errno) {
		return errno
	}
	return unix.EPERM
}

func errnoOrNil(errno unix.Errno) error {
	if errno == 0 {
		return nil
	}
	return errno
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
