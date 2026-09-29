//go:build unix

package build_test

import (
	"os/exec"
	"runtime"
	"syscall"
	"testing"
)

// runMeasured runs a program and answers its peak resident set in bytes
// and its exit status.
func runMeasured(t *testing.T, path string) (int64, int) {
	t.Helper()
	cmd := exec.Command(path)
	_ = cmd.Run()
	usage, ok := cmd.ProcessState.SysUsage().(*syscall.Rusage)
	if !ok {
		t.Skip("no resource usage for the child")
	}
	peak := int64(usage.Maxrss)
	// ru_maxrss is bytes on macOS and kilobytes everywhere else.
	if runtime.GOOS != "darwin" {
		peak *= 1024
	}
	return peak, cmd.ProcessState.ExitCode()
}
