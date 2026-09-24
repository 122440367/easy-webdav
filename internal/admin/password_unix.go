//go:build linux || darwin

package admin

import (
	"os"

	"golang.org/x/sys/unix"
)

// hideInput silences terminal echo while a password is typed. Pipes and
// non-console input make the ioctl fail, in which case the caller reads
// normally.
func hideInput(file *os.File) (func(), error) {
	fd := int(file.Fd())
	original, err := unix.IoctlGetTermios(fd, ioctlReadTermios)
	if err != nil {
		return nil, err
	}
	updated := *original
	updated.Lflag &^= unix.ECHO
	if err := unix.IoctlSetTermios(fd, ioctlWriteTermios, &updated); err != nil {
		return nil, err
	}
	return func() { _ = unix.IoctlSetTermios(fd, ioctlWriteTermios, original) }, nil
}
