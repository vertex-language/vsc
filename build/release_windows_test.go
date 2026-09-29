package build_test

import (
	"os/exec"
	"syscall"
	"testing"
	"unsafe"
)

var procGetProcessMemoryInfo = syscall.NewLazyDLL("kernel32.dll").NewProc("K32GetProcessMemoryInfo")

// processMemoryCounters is PROCESS_MEMORY_COUNTERS.
type processMemoryCounters struct {
	cb                         uint32
	pageFaultCount             uint32
	peakWorkingSetSize         uintptr
	workingSetSize             uintptr
	quotaPeakPagedPoolUsage    uintptr
	quotaPagedPoolUsage        uintptr
	quotaPeakNonPagedPoolUsage uintptr
	quotaNonPagedPoolUsage     uintptr
	pagefileUsage              uintptr
	peakPagefileUsage          uintptr
}

// runMeasured runs a program and answers its peak working set in bytes
// and its exit status.
//
// Windows keeps a process's counters for as long as a handle to it is
// open, and exec's own handle is closed by Wait, so a second one is
// opened while the program runs and read after it has exited.
func runMeasured(t *testing.T, path string) (int64, int) {
	t.Helper()
	cmd := exec.Command(path)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	h, err := syscall.OpenProcess(syscall.PROCESS_QUERY_INFORMATION, false, uint32(cmd.Process.Pid))
	if err != nil {
		_ = cmd.Wait()
		t.Skipf("no handle to the child: %v", err)
	}
	defer syscall.CloseHandle(h)
	_ = cmd.Wait()
	var pmc processMemoryCounters
	pmc.cb = uint32(unsafe.Sizeof(pmc))
	if r, _, err := procGetProcessMemoryInfo.Call(uintptr(h), uintptr(unsafe.Pointer(&pmc)), uintptr(pmc.cb)); r == 0 {
		t.Skipf("no memory counters for the child: %v", err)
	}
	return int64(pmc.peakWorkingSetSize), cmd.ProcessState.ExitCode()
}
