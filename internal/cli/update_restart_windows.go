//go:build windows

package cli

import (
	"os"
	"os/exec"
)

func restartCurrentProcess(binary string, args []string) error {
	cmd := exec.Command(binary, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Start(); err != nil {
		return err
	}
	return errUpdateRestarting
}
