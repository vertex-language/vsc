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

// The C interop corpus: a C program that calls what this compiler
// built, and Vertex code that calls what clang built.
//
// tests/interop asks whether what this compiler builds is the same
// thing swiftc builds. This asks the same question of the other
// boundary, and it is a different question: there is no importer and
// no header here, so what crosses is a symbol and a register and
// nothing else. Two attributes name the symbol -- `@_cdecl` for one
// this compiler defines and `@_silgen_name` for one it does not --
// and the only way to find out whether either is true is to hand the
// object file to clang and run what comes out.
//
// Each directory holds two files:
//
//	library.swift  built by this compiler, as a module of its own
//	host.c         built by clang, and holds main
//
// The C program returns 42 when it is satisfied and the number of the
// check that failed otherwise, which is what tests/interop does and
// for the same reason: there is no oracle for a program that is half
// this compiler's, so it checks itself.
//
// clang drives the link. Nothing here needs the Swift runtime -- that
// is rather the point of the corpus, since a C caller has none -- so
// what goes on the command line is the object, the Vertex runtime,
// and the C file.
func TestCInteropCorpus(t *testing.T) {
	if runtime.GOARCH != "arm64" || runtime.GOOS != "darwin" {
		t.Skip("not on Apple Silicon")
	}
	clang, err := exec.LookPath("clang")
	if err != nil {
		t.Skip("no clang on PATH; there is no C to interoperate with")
	}
	target, ok := build.Host()
	if !ok {
		t.Skip("no backend for this machine")
	}

	dirs, err := filepath.Glob("../tests/cinterop/*")
	if err != nil || len(dirs) == 0 {
		t.Fatal("no cases found in tests/cinterop")
	}
	for _, dir := range dirs {
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			continue
		}
		t.Run(filepath.Base(dir), func(t *testing.T) {
			runCInteropCase(t, clang, target, dir)
		})
	}
}

func runCInteropCase(t *testing.T, clang string, target ir.Target, dir string) {
	t.Helper()
	work := t.TempDir()

	// The library, by this compiler. A module of its own rather than
	// main: it has no entry point, because the entry point is C's.
	src, err := os.ReadFile(filepath.Join(dir, "library.swift"))
	if err != nil {
		t.Fatal(err)
	}
	unit, diags := vsc.Compile([]vsc.Source{{Name: "library.swift", Text: src}},
		vsc.Options{Module: "vertexlib", Target: target})
	if len(diags) > 0 {
		var b strings.Builder
		for _, d := range diags {
			b.WriteString("\n  ")
			b.WriteString(d.String())
		}
		t.Fatalf("this compiler refused the library:%s", b.String())
	}
	obj, err := build.Object(unit.VIR, build.Options{})
	if err != nil {
		t.Fatal(err)
	}
	libObj := filepath.Join(work, "library.o")
	if err := os.WriteFile(libObj, obj, 0o644); err != nil {
		t.Fatal(err)
	}
	rt, err := build.Runtime(target)
	if err != nil {
		t.Fatal(err)
	}
	rtObj := filepath.Join(work, "runtime.o")
	if err := os.WriteFile(rtObj, rt.Data, 0o644); err != nil {
		t.Fatal(err)
	}

	bin := filepath.Join(work, "host")
	link := exec.Command(clang, "-o", bin, filepath.Join(dir, "host.c"), libObj, rtObj)
	if out, err := link.CombinedOutput(); err != nil {
		t.Fatalf("link: %v\n%s", err, out)
	}

	run := exec.Command(bin)
	_ = run.Run()
	if ws, ok := run.ProcessState.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		t.Fatalf("killed by %v", ws.Signal())
	}
	if got := run.ProcessState.ExitCode(); got != 42 {
		t.Errorf("exit status = %d, want 42 (the number is the check that failed)", got)
	}
}
