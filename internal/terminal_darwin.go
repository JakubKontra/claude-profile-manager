package internal

import (
	"syscall"
	"unsafe"
)

func IsTerminal(fd uintptr) bool {
	var term syscall.Termios
	_, _, err := syscall.Syscall(syscall.SYS_IOCTL, fd, syscall.TIOCGETA, uintptr(unsafe.Pointer(&term)))
	return err == 0
}
