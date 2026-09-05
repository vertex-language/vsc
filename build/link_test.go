package build_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/vertex-language/vsc"
	"github.com/vertex-language/vsc/build"
)

// The link, with nothing installed.
//
// Every other program in this package is handed to clang to link,
// which proves the object is right and leaves the claim this compiler
// actually makes untested: that it needs no host toolchain. These
// tests link with vertex-language's own Mach-O linker and run what
// comes out. A machine with no clang passes them; a machine with no
// SDK does not, and cannot, since the SDK is where libSystem's stub
// is.

// linkProgram compiles src as a program and links it into an
// executable, returning the path it was written to.
func linkProgram(t *testing.T, src string, opts build.LinkOptions) string {
	t.Helper()
	if runtime.GOARCH != "arm64" || runtime.GOOS != "darwin" {
		t.Skip("not on Apple Silicon; skipping the link-and-run check")
	}
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
	path := filepath.Join(t.TempDir(), "prog")
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

// TestExecutableIsSigned: an arm64 executable macOS will not run is
// not an executable. The linker signs ad hoc, and the test that it
// did is that the process started at all — an unsigned one is killed
// before main.
func TestExecutableIsSigned(t *testing.T) {
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
