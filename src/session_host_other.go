//go:build !linux

package src

import "syscall"

// disableDumpable has no equivalent here; RLIMIT_CORE=0 still prevents
// core dumps.
func disableDumpable() {}

func dupFD(oldfd, newfd int) error { return syscall.Dup2(oldfd, newfd) }
