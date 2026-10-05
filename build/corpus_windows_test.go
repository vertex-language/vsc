package build_test

import (
	"fmt"
	"os"
)

// crashOf reports the exception that stopped a process, as the corpus
// compares it. Windows has no signals: a process that faults ends with
// the exception's NTSTATUS as its exit code -- 0xC000001D for an illegal
// instruction, 0x80000003 for a breakpoint -- which a program cannot
// return from main by accident, since those are error and warning
// severities rather than anything exit() is handed. A trap is one of
// them, and both compilers have to raise the same one.
func crashOf(ps *os.ProcessState) (string, bool) {
	code := uint32(ps.ExitCode())
	if code>>30 == 0 {
		return "", false
	}
	return fmt.Sprintf("crashed with exception 0x%08X", code), true
}
