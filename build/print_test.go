package build_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	vsc "github.com/vertex-language/vsc"
	"github.com/vertex-language/vsc/build"
)

// TestPrintMatchesSwift is the one test that compares output rather
// than an exit code.
//
// `print` is not this compiler's: what it reaches is
// `$ss5print_9separator10terminatoryypd_S2StF`, the standard
// library's own, and everything about the call has to be right for
// the text to come out right -- the array of `Any` the variadic list
// becomes, the metadata each element carries that says what is in it,
// the two defaults evaluated at the call, and the labels that decide
// which argument is which.
//
// So the same program is built twice, once by each compiler, and the
// two outputs are compared. Nothing here is asserted about the
// formatting: whatever swiftc prints is the answer.
//
// One line differs on purpose and is left out of the comparison: a
// struct declared in the program prints as `Point()` rather than
// `Point(x: 1, y: 2)`, because this compiler emits no reflection
// metadata and the runtime has no field names to read. That is what
// swiftc prints too when reflection metadata is turned off.
func TestPrintMatchesSwift(t *testing.T) {
	if runtime.GOARCH != "arm64" || runtime.GOOS != "darwin" {
		t.Skip("not on Apple Silicon")
	}
	swiftc, err := exec.LookPath("swiftc")
	if err != nil {
		t.Skip("no swiftc on PATH")
	}
	target, ok := build.Host()
	if !ok {
		t.Skip("no backend for this machine")
	}

	const program = `
func main() -> Int32 {
    print("hello")
    print("n =", 42)
    print(1, 2, 3, separator: "-")
    print("no newline, ", terminator: "")
    print("then one")
    print(true, 1.5, "mixed", 7)
    print()
    print("", terminator: "")
    let s = "held"
    print(s, s.count, separator: ": ")

    // Interpolation, which is the pieces and a concatenation here
    // where SILGen writes a DefaultStringInterpolation. Whether the
    // two agree is exactly what this test asks.
    let n: Int32 = 42
    print("hello \(s)")
    print("n = \(n), and one more is \(n + 1)")
    print("\(true) \(1.5) \(s)")
    print("\(s)")
    print("no holes at all")
    let joined = "a\(n)b"
    print(joined, joined.count)

    // An array whose elements are more than one register, walked and
    // read: what comes back out of one is a copy, and it is let go of
    // at the end of the iteration rather than after the loop.
    let names = ["ada", "grace", "alan"]
    print(names.count, names[1])
    for name in names { print(name, name.count) }
    var all = ""
    for name in names { all += name }
    print(all)
    return 0
}
`
	dir := t.TempDir()

	// swiftc's answer. Its top level is the program, so main() is
	// called rather than declared.
	swiftSrc := filepath.Join(dir, "oracle.swift")
	if err := os.WriteFile(swiftSrc, []byte(program+"\n_ = main()\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	oracleBin := filepath.Join(dir, "oracle")
	if out, err := exec.Command(swiftc, "-o", oracleBin, swiftSrc).CombinedOutput(); err != nil {
		t.Fatalf("swiftc: %v\n%s", err, out)
	}
	want, err := exec.Command(oracleBin).Output()
	if err != nil {
		t.Fatalf("running swiftc's program: %v", err)
	}

	// This compiler's. It links against a Swift object of its own so
	// that the standard library comes in.
	unit, diags := vsc.Compile([]vsc.Source{{Name: "p.swift", Text: []byte(program)}},
		vsc.Options{Module: "main", Target: target})
	if len(diags) > 0 {
		var b strings.Builder
		for _, d := range diags {
			b.WriteString("\n  ")
			b.WriteString(d.String())
		}
		t.Fatalf("this compiler refused the program:%s", b.String())
	}
	obj, err := build.Object(unit.VIR, build.Options{})
	if err != nil {
		t.Fatal(err)
	}
	objPath := filepath.Join(dir, "p.o")
	if err := os.WriteFile(objPath, obj, 0o644); err != nil {
		t.Fatal(err)
	}
	rt, err := build.Runtime(target)
	if err != nil {
		t.Fatal(err)
	}
	rtPath := filepath.Join(dir, "runtime.o")
	if err := os.WriteFile(rtPath, rt.Data, 0o644); err != nil {
		t.Fatal(err)
	}
	// An empty Swift object, so that the link brings in the standard
	// library: swiftc adds it for Swift inputs, and this program's
	// own object is not one.
	stubSrc := filepath.Join(dir, "stub.swift")
	if err := os.WriteFile(stubSrc, []byte("public func __linkSwift() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stubObj := filepath.Join(dir, "stub.o")
	if out, err := exec.Command(swiftc, "-parse-as-library", "-c",
		"-module-name", "Stub", "-o", stubObj, stubSrc).CombinedOutput(); err != nil {
		t.Fatalf("swiftc: %v\n%s", err, out)
	}

	binPath := filepath.Join(dir, "p")
	if out, err := exec.Command(swiftc, "-o", binPath, objPath, rtPath, stubObj).CombinedOutput(); err != nil {
		t.Fatalf("link: %v\n%s", err, out)
	}
	got, err := exec.Command(binPath).Output()
	if err != nil {
		t.Fatalf("running this compiler's program: %v", err)
	}

	if string(got) != string(want) {
		t.Errorf("output differs\n--- this compiler ---\n%s--- swiftc ---\n%s", got, want)
	}
}
