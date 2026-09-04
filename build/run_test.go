package build_test

// The strongest check available: compile Swift, write the object,
// link it against a C caller with the native toolchain, and run it.
//
// Everything else in this repo says the output is well-formed. The
// parser agrees with swiftc, the checker agrees with swiftc, the
// ownership IR verifies, the VIR verifies. None of that says the
// program computes the right answer, and this does.
//
// Skipped rather than failed off Apple Silicon or with no clang: a
// missing toolchain is a fact about the machine and not a defect.

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	irverify "github.com/vertex-language/ir/verify"

	"github.com/vertex-language/vsc"
	"github.com/vertex-language/vsc/build"
)

func TestFibRuns(t *testing.T) {
	got := runSwift(t, "fib", `
public func fib(_ n: Int) -> Int {
  if n < 2 { return n }
  return fib(n - 1) + fib(n - 2)
}
`, `
#include <stdio.h>
long fib(long n) __asm__("$s3fib3fibyS2iF");
int main(void) {
    for (long i = 0; i < 10; i++) printf("%ld ", fib(i));
    return 0;
}
`)
	if want := "0 1 1 2 3 5 8 13 21 34 "; got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

// TestArithmeticRuns is the one that would catch a backend that
// selected the wrong instruction: every operator, on values chosen so
// that a sign error or a swapped operand changes the answer.
func TestArithmeticRuns(t *testing.T) {
	got := runSwift(t, "ar", `
public func add(_ a: Int, _ b: Int) -> Int { return a + b }
public func sub(_ a: Int, _ b: Int) -> Int { return a - b }
public func mul(_ a: Int, _ b: Int) -> Int { return a * b }
public func div(_ a: Int, _ b: Int) -> Int { return a / b }
public func rem(_ a: Int, _ b: Int) -> Int { return a % b }
public func neg(_ a: Int) -> Int { return -a }
public func inv(_ a: Int) -> Int { return ~a }
public func lt(_ a: Int, _ b: Int) -> Bool { return a < b }
public func gt(_ a: Int, _ b: Int) -> Bool { return a > b }
public func band(_ a: Int, _ b: Int) -> Int { return a & b }
public func bxor(_ a: Int, _ b: Int) -> Int { return a ^ b }
`, `
#include <stdio.h>
long add(long, long) __asm__("$s2ar3addyS2i_SitF");
long sub(long, long) __asm__("$s2ar3subyS2i_SitF");
long mul(long, long) __asm__("$s2ar3mulyS2i_SitF");
long xdiv(long, long) __asm__("$s2ar3divyS2i_SitF");
long xrem(long, long) __asm__("$s2ar3remyS2i_SitF");
long neg(long) __asm__("$s2ar3negyS2iF");
long inv(long) __asm__("$s2ar3invyS2iF");
_Bool lt(long, long) __asm__("$s2ar2ltySbSi_SitF");
_Bool gt(long, long) __asm__("$s2ar2gtySbSi_SitF");
long band(long, long) __asm__("$s2ar4bandyS2i_SitF");
long bxor(long, long) __asm__("$s2ar4bxoryS2i_SitF");
int main(void) {
    printf("%ld %ld %ld %ld %ld ", add(3,4), sub(3,4), mul(3,4), xdiv(-9,2), xrem(-9,2));
    printf("%ld %ld ", neg(5), inv(5));
    printf("%d %d ", lt(3,4), gt(3,4));
    printf("%ld %ld", band(12,10), bxor(12,10));
    return 0;
}
`)
	// -9/2 truncates toward zero, so it is -4 and the remainder is -1.
	// ~5 is -6. 12&10 is 8 and 12^10 is 6.
	if want := "7 -1 12 -4 -1 -5 -6 1 0 8 6"; got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

// TestOverflowTraps: the check Swift promises is not decoration. An
// addition that overflows has to stop the program, and the only way to
// know it does is to run one.
func TestOverflowTraps(t *testing.T) {
	bin := buildSwift(t, "ov", `
public func add(_ a: Int, _ b: Int) -> Int { return a + b }
`, `
#include <stdio.h>
long add(long, long) __asm__("$s2ov3addyS2i_SitF");
int main(void) {
    printf("%ld\n", add(0x7fffffffffffffffL, 1));
    return 0;
}
`)
	err := exec.Command(bin).Run()
	if err == nil {
		t.Fatal("the addition overflowed and the program carried on")
	}
	if !strings.Contains(err.Error(), "trace/BPT trap") && !strings.Contains(err.Error(), "signal") {
		t.Errorf("the program failed, but not by trapping: %v", err)
	}
}

// --- the harness ---

func runSwift(t *testing.T, module, swift, mainC string) string {
	t.Helper()
	out, err := exec.Command(buildSwift(t, module, swift, mainC)).CombinedOutput()
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	return string(out)
}

// buildSwift compiles the Swift, writes the object, links it against
// the C and returns the executable.
func buildSwift(t *testing.T, module, swift, mainC string) string {
	t.Helper()
	if runtime.GOARCH != "arm64" || runtime.GOOS != "darwin" {
		t.Skip("not on Apple Silicon; skipping the link-and-run check")
	}
	clang, err := exec.LookPath("clang")
	if err != nil {
		t.Skip("no clang on PATH; skipping the link-and-run check")
	}
	target, ok := build.Host()
	if !ok {
		t.Skip("no backend for this machine")
	}

	u, diags := vsc.Compile([]vsc.Source{{
		Name: module + ".swift", Text: []byte(swift),
	}}, vsc.Options{Module: module, Target: target})
	for _, d := range diags {
		t.Fatalf("%s", d)
	}
	if err := irverify.Module(u.VIR); err != nil {
		t.Fatalf("verify: %v", err)
	}

	obj, err := build.Object(u.VIR, build.Options{})
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	objPath := filepath.Join(dir, module+".o")
	if err := os.WriteFile(objPath, obj, 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(dir, "main.c")
	if err := os.WriteFile(mainPath, []byte(mainC), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, module)
	if out, err := exec.Command(clang, "-o", bin, mainPath, objPath).CombinedOutput(); err != nil {
		t.Fatalf("link: %v\n%s", err, out)
	}
	return bin
}
