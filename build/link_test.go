package build_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/vertex-language/vsc"
	"github.com/vertex-language/vsc/build"
)

// The link, with nothing installed.
//
// Every other program in this package is handed to clang to link,
// which proves the object is right and leaves the claim this compiler
// actually makes untested: that it needs no host toolchain. These
// tests link with vertex-language's own linker and run what comes
// out. A machine with no clang passes them; a machine with no
// platform libraries does not, and cannot, since that is where the
// startup and malloc are.
//
// They run wherever this package has a backend, which is the only
// condition they have: no clang, no swiftc, no oracle of any kind.
// That makes them the tests that follow a new target — the check that
// a machine which can compile can also link and run is the whole of
// what "supports a target" means here, and it is the same check on
// every one.

// linkProgram compiles src as a program and links it into an
// executable, returning the path it was written to.
func linkProgram(t *testing.T, src string, opts build.LinkOptions) string {
	t.Helper()
	target, ok := build.Host()
	if !ok {
		t.Skip("no backend for this machine")
	}
	u, diags := vsc.Compile([]vsc.Source{{Name: "main.swift", Text: []byte(src)}},
		vsc.Options{Module: "main", Target: target})
	for _, d := range diags {
		t.Fatalf("%s", d)
	}
	obj, err := build.Object(u.VIR, build.Options{})
	if err != nil {
		t.Fatal(err)
	}
	opts.Target = target
	exe, err := build.Executable([]build.Input{{Name: "main.o", Data: obj}}, opts)
	if err != nil {
		t.Fatal(err)
	}
	// The extension the platform gives a program, because this one is
	// started and not merely written: Windows resolves an
	// extensionless path through %PATHEXT% and finds nothing there.
	path := vsc.ImageName(target, filepath.Join(t.TempDir(), "prog"))
	if err := os.WriteFile(path, exe, 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestExecutableRuns is the whole claim in one test: Swift in, a
// running process out, and no cc, as or ld on the path at any point.
func TestExecutableRuns(t *testing.T) {
	path := linkProgram(t, `
func helper(_ n: Int32) -> Int32 { return n + 39 }
func main() -> Int32 { return helper(3) }
`, build.LinkOptions{})

	cmd := exec.Command(path)
	if err := cmd.Run(); err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			t.Fatalf("run: %v", err)
		}
	}
	if got := cmd.ProcessState.ExitCode(); got != 42 {
		t.Errorf("exit status = %d, want 42", got)
	}
}

// TestExecutableExitsZero: a main that returns nothing exits zero,
// through the linker as well as through the compiler.
func TestExecutableExitsZero(t *testing.T) {
	path := linkProgram(t, `func main() {}`, build.LinkOptions{})
	if err := exec.Command(path).Run(); err != nil {
		t.Errorf("run: %v", err)
	}
}

// TestExecutableStarts: an image the loader refuses is not an
// executable, and the test that it is one is that the process ran at
// all rather than being killed before main.
//
// It is macOS this catches. An arm64 binary that is not signed will
// not execute there, the linker signs ad hoc for that reason, and a
// regression in the signature shows up here and nowhere else. The
// same assertion on Windows costs nothing and says the CRT startup
// reached main.
func TestExecutableStarts(t *testing.T) {
	path := linkProgram(t, `func main() -> Int32 { return 7 }`, build.LinkOptions{})
	cmd := exec.Command(path)
	err := cmd.Run()
	if _, ok := err.(*exec.ExitError); err != nil && !ok {
		t.Fatalf("the process did not start: %v", err)
	}
	if got := cmd.ProcessState.ExitCode(); got != 7 {
		t.Errorf("exit status = %d, want 7", got)
	}
}

// TestLinkRefusesNothing: a link with no objects is a mistake worth a
// message rather than an empty executable.
func TestLinkRefusesNothing(t *testing.T) {
	target, ok := build.Host()
	if !ok {
		t.Skip("no backend for this machine")
	}
	if _, err := build.Executable(nil, build.LinkOptions{Target: target}); err == nil {
		t.Error("linked nothing into something")
	}
}
