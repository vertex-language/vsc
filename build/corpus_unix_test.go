//go:build unix

package build_test

import (
	"os"
	"strconv"
	"syscall"
)

// crashOf reports the signal that stopped a process, as the corpus
// compares it: a trap is SIGTRAP or SIGILL, and both compilers have to
// raise the same one.
func crashOf(ps *os.ProcessState) (string, bool) {
	ws, ok := ps.Sys().(syscall.WaitStatus)
	if !ok || !ws.Signaled() {
		return "", false
	}
	sig := ws.Signal()
	return "killed by " + sig.String() + " (signal " + strconv.Itoa(int(sig)) + ")", true
}
