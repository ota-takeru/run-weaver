//go:build !windows

package cli

import (
	"os"
	"syscall"
)

func restartCurrentProcess(binary string, args []string) error {
	argv := append([]string{binary}, args...)
	if err := syscall.Exec(binary, argv, os.Environ()); err != nil {
		return err
	}
	return errUpdateRestarting
}
