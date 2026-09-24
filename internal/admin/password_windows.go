//go:build windows

package admin

import (
	"os"

	"golang.org/x/sys/windows"
)

// hideInput silences console echo while a password is typed. When stdin is not
// a console (pipes, CI) the call fails and the caller reads normally.
func hideInput(file *os.File) (func(), error) {
	handle := windows.Handle(file.Fd())
	var mode uint32
	if err := windows.GetConsoleMode(handle, &mode); err != nil {
		return nil, err
	}
	if err := windows.SetConsoleMode(handle, mode&^windows.ENABLE_ECHO_INPUT); err != nil {
		return nil, err
	}
	return func() { _ = windows.SetConsoleMode(handle, mode) }, nil
}
