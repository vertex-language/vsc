package build_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"

	"github.com/vertex-language/ir"

	vsc "github.com/vertex-language/vsc"
	"github.com/vertex-language/vsc/build"
)

// The compiler corpus: whole programs, compiled and run.
//
// tests/syntax asks whether a file parses and checks. This asks the
// only question after that one -- whether the program does what it
// says -- and it asks it the way the rest of this repo asks every
// question, by putting the same source through swiftc and comparing.
//
// Nothing here states an expected value. A number written down beside
// a program is a claim about Swift that has to be maintained by hand
// and is wrong the moment it drifts; swiftc's answer cannot drift,
// because it is Swift's answer. So each file is compiled twice and run
// twice, and the two outcomes have to agree -- exit status for a
// program that returns, and the same signal for one that traps.
//
// Each file is a whole program with `func main() -> Int32`, which is
// this compiler's entry point. swiftc has no such convention, so for
// its half the function is renamed and called from top-level code;
// that rewrite is the only difference between what the two compilers
// are given.
//
// The files are numbered in the order they get harder. The early ones
// are deliberately one idea each -- a loop, a struct, an override --
// so that a failure names the thing that broke rather than the last
// thing added.

// outcome is how a process ended.
type outcome struct {
	status   int
	signal   syscall.Signal
	signaled bool
}

func (o outcome) String() string {
	if o.signaled {
		return "killed by " + o.signal.String() + " (signal " + itoa(int(o.signal)) + ")"
	}
	return "exit " + itoa(o.status)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		return "-" + string(b)
	}
	return string(b)
}

func TestCompilerCorpus(t *testing.T) {
	if runtime.GOARCH != "arm64" || runtime.GOOS != "darwin" {
		t.Skip("not on Apple Silicon; skipping the compile-and-run corpus")
	}
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("no clang on PATH")
	}
	swiftc, err := exec.LookPath("swiftc")
	if err != nil {
		t.Skip("no swiftc on PATH; there is no oracle to compare against")
	}
	target, ok := build.Host()
	if !ok {
		t.Skip("no backend for this machine")
	}

	files, err := filepath.Glob("../tests/compiler/*.swift")
	if err != nil || len(files) == 0 {
		t.Fatal("no programs found in tests/compiler/*.swift")
	}

	for _, file := range files {
		name := strings.TrimSuffix(filepath.Base(file), ".swift")
		t.Run(name, func(t *testing.T) {
			src, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			want := runSwiftc(t, swiftc, string(src))
			got := runVsc(t, target, string(src))
			if got != want {
				t.Errorf("vsc gave %s, swiftc gave %s", got, want)
			}
		})
	}
}

// runVsc compiles the program with this compiler and runs it.
func runVsc(t *testing.T, target ir.Target, src string) outcome {
	t.Helper()
	u, diags := vsc.Compile([]vsc.Source{{Name: "main.swift", Text: []byte(src)}},
		vsc.Options{Module: "main", Target: target})
	if len(diags) > 0 {
		// A refusal is a failure here. Every program in this corpus is
		// one this compiler is expected to handle; a diagnostic means
		// it no longer does, or never did and the file was added too
		// early.
		var b strings.Builder
		for _, d := range diags {
			b.WriteString("\n  ")
			b.WriteString(d.String())
		}
		t.Fatalf("vsc refused the program:%s", b.String())
	}
	obj, err := build.Object(u.VIR, build.Options{})
	if err != nil {
		t.Fatalf("vsc: %v", err)
	}
	dir := t.TempDir()
	objPath := filepath.Join(dir, "main.o")
	if err := os.WriteFile(objPath, obj, 0o644); err != nil {
		t.Fatal(err)
	}
	// The runtime alongside it: a program that makes a class calls the
	// allocator, and the allocator is not in the program's own object.
	rt, err := build.Runtime(target)
	if err != nil {
		t.Fatal(err)
	}
	rtPath := filepath.Join(dir, "vertex_runtime.o")
	if err := os.WriteFile(rtPath, rt.Data, 0o644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "main")
	if out, err := exec.Command("clang", "-o", bin, objPath, rtPath).CombinedOutput(); err != nil {
		t.Fatalf("link: %v\n%s", err, out)
	}
	return run(t, bin)
}

// runSwiftc compiles the same program with the real compiler.
//
// The entry point is the one thing that has to be rewritten: this
// compiler runs `func main() -> Int32`, and swiftc has no such rule.
// Renaming it and calling it from top-level code gives swiftc a
// program with the same body and the same result.
func runSwiftc(t *testing.T, swiftc, src string) outcome {
	t.Helper()
	const entry = "func main() -> Int32 {"
	if !strings.Contains(src, entry) {
		t.Fatalf("the program has no %q to rewrite for swiftc", entry)
	}
	rewritten := "import Darwin\n" +
		strings.Replace(src, entry, "func vsMain() -> Int32 {", 1) +
		"\nexit(vsMain())\n"

	dir := t.TempDir()
	srcPath := filepath.Join(dir, "main.swift")
	if err := os.WriteFile(srcPath, []byte(rewritten), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "oracle")
	cmd := exec.Command(swiftc, "-o", bin, srcPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("swiftc rejected the program: %v\n%s", err, out)
	}
	return run(t, bin)
}

// run executes a built program and reports how it ended. A trap is an
// outcome like any other: `Int32(bigValue)` is supposed to kill the
// process, and a compiler that returned a number instead would be
// wrong in a way an exit status alone would hide.
func run(t *testing.T, bin string) outcome {
	t.Helper()
	cmd := exec.Command(bin)
	err := cmd.Run()
	if err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			t.Fatalf("run %s: %v", bin, err)
		}
	}
	ws, ok := cmd.ProcessState.Sys().(syscall.WaitStatus)
	if ok && ws.Signaled() {
		return outcome{signal: ws.Signal(), signaled: true}
	}
	return outcome{status: cmd.ProcessState.ExitCode()}
}
