package build_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/vertex-language/ir"

	vsc "github.com/vertex-language/vsc"
	"github.com/vertex-language/vsc/build"
)

// The corpus: tests/NNN-*.swift, one small thing per file, numbered in
// the order they climb (see tests/README.md).
//
// Every file is built twice -- once by this compiler, once by swiftc --
// run, and the stdout and the way each run ended compared. Nothing here
// writes down an expected value: swiftc's answer is the oracle, and a
// disagreement with it is a bug in vsc by definition.
//
// Each file is a main.swift of top-level code, and both compilers are
// given it unchanged.

// outcome is how a process ended.
type outcome struct {
	status   int
	signal   syscall.Signal
	signaled bool
	timedOut bool
	stdout   string
}

func (o outcome) String() string {
	switch {
	case o.timedOut:
		return "timed out after 10s"
	case o.signaled:
		return "killed by " + o.signal.String() + " (signal " + strconv.Itoa(int(o.signal)) + ")"
	}
	return "exit " + strconv.Itoa(o.status)
}

func TestCorpus(t *testing.T) {
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

	files, err := filepath.Glob("../tests/*.swift")
	if err != nil || len(files) == 0 {
		t.Fatal("no programs found in tests/*.swift")
	}
	sort.Strings(files)

	for _, file := range files {
		t.Run(strings.TrimSuffix(filepath.Base(file), ".swift"), func(t *testing.T) {
			src, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			want := runSwiftc(t, swiftc, src)
			got := runVsc(t, target, src)
			if got.String() != want.String() {
				t.Errorf("vsc's build ended with %s; swiftc's with %s", got, want)
			}
			// A program killed by a trap may lose what it buffered, and
			// the two runtimes buffer differently, so output is compared
			// where both ran to the end.
			if !got.signaled && !want.signaled && got.stdout != want.stdout {
				t.Errorf("output differs from swiftc's\n--- vsc ---\n%s\n--- swiftc ---\n%s", clip(got.stdout), clip(want.stdout))
			}
		})
	}
}

// runVsc compiles the program with this compiler and runs it.
func runVsc(t *testing.T, target ir.Target, src []byte) outcome {
	t.Helper()
	u, diags := vsc.Compile([]vsc.Source{{Name: "main.swift", Text: src}},
		vsc.Options{Module: "main", Target: target})
	if len(diags) > 0 {
		// A refusal is a failure: every rung is Swift, and the ladder
		// is what the compiler has to build.
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
func runSwiftc(t *testing.T, swiftc string, src []byte) outcome {
	t.Helper()
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "main.swift")
	if err := os.WriteFile(srcPath, src, 0o644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "oracle")
	// The module is main, as vsc's is: a type's qualified name has it.
	cmd := exec.Command(swiftc, "-module-name", "main", "-o", bin, srcPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("swiftc rejected the program: %v\n%s", err, out)
	}
	return run(t, bin)
}

// run executes a built program and reports how it ended, with a time
// limit and a cap on its output: a miscompiled loop must fail its own
// test, not take the whole run down with it. A trap is an outcome like
// any other: `UInt8(300)` is supposed to kill the process, and a
// compiler that returned a number instead would be wrong in a way an
// exit status alone would hide.
func run(t *testing.T, bin string) outcome {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin)
	var stdout capped
	cmd.Stdout = &stdout
	err := cmd.Run()
	if ctx.Err() != nil {
		return outcome{timedOut: true, stdout: stdout.String()}
	}
	if err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			t.Fatalf("run %s: %v", bin, err)
		}
	}
	ws, ok := cmd.ProcessState.Sys().(syscall.WaitStatus)
	if ok && ws.Signaled() {
		return outcome{signal: ws.Signal(), signaled: true, stdout: stdout.String()}
	}
	return outcome{status: cmd.ProcessState.ExitCode(), stdout: stdout.String()}
}

// capped is a buffer that keeps the first megabyte written to it.
type capped struct{ bytes.Buffer }

func (c *capped) Write(p []byte) (int, error) {
	if room := 1<<20 - c.Len(); room < len(p) {
		if room > 0 {
			c.Buffer.Write(p[:room])
		}
		return len(p), nil
	}
	return c.Buffer.Write(p)
}

// clip shortens a long output for a failure message.
func clip(s string) string {
	const max = 4000
	if len(s) > max {
		return s[:max] + "\n... (" + strconv.Itoa(len(s)-max) + " more bytes)"
	}
	return s
}
