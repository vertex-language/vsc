package vsc_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/vertex-language/vsc/internal/cli"
)

// TestOverrideCalledFromAnotherModule: a module that declares no
// subclass of an imported class still reaches the overrides its
// instances have -- of a method, a computed property, and one read in
// a guard -- while a final class's members are called directly.
func TestOverrideCalledFromAnotherModule(t *testing.T) {
	root := t.TempDir()
	write := func(rel, text string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("vs.mod", "module github.com/x/m\n\nvertex 0.9\nplatform macos 13\n")
	write("lib/lib.vs", `package lib

public enum V { case u; case o(A) }
open class A {
    public init() {}
    open var C: bool { return false }
    open func Name() -> string { return "A" }
    open func Async() async -> string { return "A async" }
}
public final class B: A {
    public override init() { super.init() }
    public override var C: bool { return true }
    public override func Name() -> string { return "B" }
    public override func Async() async -> string { return "B async" }
    public func Only() -> string { return "final" }
}
public func Make() -> V { return .o(B()) }
`)
	write("use/use.vs", `package use

import "m/lib"

public func Guard(_ v: lib.V) -> string {
    guard case .o(let t) = v, t.C else { return "guard failed" }
    return "guard ok"
}
`)
	write("cmd/t/main.vs", `package main

import (
    "m/lib"
    "m/use"
)

func main() async -> int32 {
    let a: lib.A = lib.B()
    print(a.C, a.Name(), await a.Async())
    print(use.Guard(lib.Make()))
    print(lib.B().Only())
    return 0
}
`)
	t.Chdir(root)
	bin := filepath.Join(root, "t.bin")
	var stdout, stderr bytes.Buffer
	if code := cli.Run([]string{"build", "-offline", "-replace", "m=" + root, "-o", bin, "t"}, &stdout, &stderr); code != 0 {
		t.Fatalf("build: %s", stderr.String())
	}
	out, err := exec.Command(bin).CombinedOutput()
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	want := "true B B async\nguard ok\nfinal\n"
	if string(out) != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}
}
