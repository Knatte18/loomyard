// memorycap.go runs a command with the memory of its process tree capped, by whatever means the platform has.

package proc

import "os/exec"

// RunMemoryCapped runs cmd as cmd.Run does, with a cap of limit bytes on the memory cmd and the processes it starts hold.
// It exists so a runaway command is ended alone, before it takes the memory of everything beside it.
// On Linux, cmd runs in a transient scope of the systemd user manager whose memory limit is limit, with no swap.
// The kernel kills a command that passes the limit, and killed reports that kill.
// After any kill at the cap, every process left in the scope is killed too,
// and the kill raises no desktop low-memory notice.
// cmd keeps its pid and process group there, so a cancel that kills the group still reaches it.
// On a Linux host whose user manager cannot cap a scope's memory, the data segment of each process is limited instead.
// On Windows, cmd runs in a job object whose memory limit is limit, and a cancel ends the whole job.
// cmd joins the job a moment after it starts, so a process it starts in that moment is outside the limit and the cancel.
// Under those two an allocation past the limit fails in the process that asks for it, and killed stays false.
// In a scope cmd.ExtraFiles is replaced, so cmd sets none.
// The error is cmd.Run's, or reports a cap that could not be set, which includes a scope that never ran cmd.
func RunMemoryCapped(cmd *exec.Cmd, limit int64) (killed bool, err error) {
	return runMemoryCappedPlatform(cmd, limit)
}

// MemoryCapCoversTree reports whether a memory cap bounds a command's whole process tree as one.
// It is false where the cap limits each process alone.
// The processes of one command can then hold the limit several times over, so a caller runs one capped command at a time.
func MemoryCapCoversTree() bool {
	return memoryCapCoversTreePlatform()
}
