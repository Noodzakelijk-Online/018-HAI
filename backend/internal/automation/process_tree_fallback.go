//go:build !unix && !windows

package automation

import (
	"os/exec"
	"time"
)

func configureControlledProcess(cmd *exec.Cmd) {
	cmd.WaitDelay = time.Second
}
