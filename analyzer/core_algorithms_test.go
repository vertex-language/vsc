package analyzer

import "testing"

// TestCoreAlgorithmsCheckClean: the core's algorithms are source, and
// loading the core keeps what is wrong with them from a program's
// diagnostics -- so this is where a mistake in them is caught.
func TestCoreAlgorithmsCheckClean(t *testing.T) {
	for _, d := range CoreDiagnostics() {
		t.Errorf("core: %s", d.Message)
	}
}
