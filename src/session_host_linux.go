package src

import "syscall"

// disableDumpable sets PR_SET_DUMPABLE to 0, which also keeps other
// processes of the same user from attaching to the host with ptrace or
// reading its memory through /proc.
func disableDumpable() {
	const prSetDumpable = 4
	_, _, _ = syscall.RawSyscall(syscall.SYS_PRCTL, prSetDumpable, 0, 0)
}

func dupFD(oldfd, newfd int) error { return syscall.Dup3(oldfd, newfd, 0) }
