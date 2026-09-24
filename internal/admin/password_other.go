//go:build !windows && !linux && !darwin

package admin

import (
	"errors"
	"os"
)

// hideInput has no implementation on this platform; passwords are read with
// the terminal's normal echo behaviour.
func hideInput(*os.File) (func(), error) {
	return nil, errors.New("terminal echo control is not supported on this platform")
}
