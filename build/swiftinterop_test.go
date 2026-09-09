package build_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	vsc "github.com/vertex-language/vsc"
	"github.com/vertex-language/vsc/build"
	"github.com/vertex-language/vsc/iface"
)

// TestCallsRealSwift links a program this compiler built against a
// library swiftc built, and runs it.
//
// Every other oracle test compares two compilers' answers. This one
// puts their output in the same process: the symbol this compiler
// asks the linker for has to be the symbol swiftc defined, and the
// registers it puts the arguments in have to be the ones swiftc's
// code reads them from. Nothing here is compared -- it either links
// and runs or it does not.
//
// It is what forced word substitutions. swiftc does not spell a name
// twice: inside `sumPair(_ p: Pair)` the type is a back-reference to
// the word the function's own name already wrote, so a compiler that
// spells `4Pair` asks for a symbol nobody defined. The mangling is
// not a detail that can be approximately right.
func TestCallsRealSwift(t *testing.T) {
	if runtime.GOARCH != "arm64" || runtime.GOOS != "darwin" {
		t.Skip("not on Apple Silicon")
	}
	swiftc, err := exec.LookPath("swiftc")
	if err != nil {
		t.Skip("no swiftc on PATH")
	}
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("no clang on PATH")
	}
	target, ok := build.Host()
	if !ok {
		t.Skip("no backend")
	}
	dir := t.TempDir()

	// The library, in Swift, built by Swift.
	const librarySwift = `
public struct Pair {
    public var a: Int32
    public var b: Int32
}
public func triple(_ n: Int32) -> Int32 { return n * 3 }
public func add(_ a: Int32, _ b: Int32) -> Int32 { return a + b }
public func makePair(_ a: Int32, _ b: Int32) -> Pair { return Pair(a: a, b: b) }
public func sumPair(_ p: Pair) -> Int32 { return p.a + p.b }

public struct Quad { public var a: Int; public var b: Int; public var c: Int; public var d: Int }
public func makeQuad() -> Quad { return Quad(a: 1, b: 2, c: 3, d: 4) }

public struct Wide {
    public var a: Int; public var b: Int; public var c: Int
    public var d: Int; public var e: Int; public var f: Int
}
public func makeWide() -> Wide { return Wide(a: 1, b: 2, c: 3, d: 4, e: 5, f: 6) }
public func sumWide(_ w: Wide) -> Int { return w.a + w.b + w.c + w.d + w.e + w.f }
`
	libSrc := filepath.Join(dir, "SwiftLib.swift")
	if err := os.WriteFile(libSrc, []byte(librarySwift), 0o644); err != nil {
		t.Fatal(err)
	}
	libObj := filepath.Join(dir, "SwiftLib.o")
	if out, err := exec.Command(swiftc, "-parse-as-library", "-c",
		"-module-name", "SwiftLib", "-o", libObj, libSrc).CombinedOutput(); err != nil {
		t.Fatalf("swiftc: %v\n%s", err, out)
	}

	// Its interface, in this compiler's dialect. Written by hand
	// because swiftc's own is only emitted under library evolution,
	// which is a different ABI -- see the iface package.
	const interfaceText = `// vertex-interface-format-version: 1.0
// vertex-module-name: SwiftLib

public struct Pair {
  public var a: Int32
  public var b: Int32
}

public func triple(_ n: Int32) -> Int32

public func add(_ a: Int32, _ b: Int32) -> Int32

public func makePair(_ a: Int32, _ b: Int32) -> Pair

public func sumPair(_ p: Pair) -> Int32

public struct Quad {
  public var a: Int
  public var b: Int
  public var c: Int
  public var d: Int
}

public struct Wide {
  public var a: Int
  public var b: Int
  public var c: Int
  public var d: Int
  public var e: Int
  public var f: Int
}

public func makeQuad() -> Quad

public func makeWide() -> Wide

public func sumWide(_ w: Wide) -> Int
`
	ifPath := filepath.Join(dir, "SwiftLib"+iface.Extension)
	if err := os.WriteFile(ifPath, []byte(interfaceText), 0o644); err != nil {
		t.Fatal(err)
	}

	// The program, in Vertex, built by this compiler.
	app, diags := vsc.Compile([]vsc.Source{{Name: "app.swift", Text: []byte(`
import SwiftLib

func main() -> Int32 {
    if add(triple(12), 6) != 42 { return 91 }

    // A struct crosses the boundary in registers: eight bytes is one
    // of them, and swiftc reads the halves back out of it.
    let p = makePair(20, 15)
    if p.a != 20 { return 92 }
    if p.b != 15 { return 93 }
    if sumPair(p) != 35 { return 94 }

    // Four words come back in x0 to x3 -- swiftc's own convention,
    // which is not C's, and which is why this needed the backend to
    // stop capping a return at two registers.
    let q = makeQuad()
    if q.a + q.b + q.c + q.d != 10 { return 95 }
    if q.d != 4 { return 96 }

    // Six words do not fit in registers at all: the caller sets
    // storage aside and passes its address, and a parameter of that
    // width is a pointer to it.
    let w = makeWide()
    if sumWide(w) != 21 { return 97 }
    if w.f != 6 { return 98 }

    return sumPair(makePair(20, 15)) + 7
}
`)}}, vsc.Options{Module: "main", Target: target, ImportPaths: []string{dir}})
	for _, d := range diags {
		t.Fatalf("app: %v", d)
	}
	obj, err := build.Object(app.VIR, build.Options{})
	if err != nil {
		t.Fatal(err)
	}
	appObj := filepath.Join(dir, "app.o")
	if err := os.WriteFile(appObj, obj, 0o644); err != nil {
		t.Fatal(err)
	}
	rt, err := build.Runtime(target)
	if err != nil {
		t.Fatal(err)
	}
	rtObj := filepath.Join(dir, "rt.o")
	if err := os.WriteFile(rtObj, rt.Data, 0o644); err != nil {
		t.Fatal(err)
	}

	bin := filepath.Join(dir, "prog")
	if out, err := exec.Command("clang", "-o", bin, appObj, libObj, rtObj).CombinedOutput(); err != nil {
		t.Fatalf("link against swiftc's object: %v\n%s", err, out)
	}
	cmd := exec.Command(bin)
	_ = cmd.Run()
	if got := cmd.ProcessState.ExitCode(); got != 42 {
		t.Errorf("exit status = %d, want 42", got)
	}
}
