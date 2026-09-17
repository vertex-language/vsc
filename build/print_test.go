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
// `print` is the Vertex runtime's, and everything about the call has to
// be right for the text to come out right -- the array of `Any` the
// variadic list becomes, the metadata each element carries that says
// what is in it, the two defaults evaluated at the call, and the labels
// that decide which argument is which -- and then the runtime has to
// describe each value exactly as Swift's standard library does.
//
// So the same program is built twice, once by each compiler, and the
// two outputs are compared. Nothing here is asserted about the
// formatting: whatever swiftc prints is the answer.
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
struct Point { var x: Int32; var y: Double }
struct Flags { var on: Bool; var level: UInt8; var p: Point }
struct Named { var name: String; var n: Int32 }
struct Tagged { var id: Int32; var tags: [Int32] }
struct Wide { var a: String; var b: String; var n: Int }
class Holder { var n: Int32; init() { n = 1 } }
class Tracker {
    var name: String
    init(_ name: String) { self.name = name }
    deinit { print("deinit", name) }
}

// An object's deinit runs where its last reference goes, before what it
// owns is let go of.
func scoped() {
    let first = Tracker("first, and long enough to be stored")
    let alias = first
    print("made", alias.name)
    let second = Tracker("second")
    print(second.name)
}

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

    // Structs, by their fields: a nested one qualified by its module,
    // and one holding a String, which is quoted and escaped and whose
    // copy into the existential retains it.
    print(Point(x: 1, y: 2.5))
    print(Flags(on: true, level: 7, p: Point(x: -3, y: 1e20)))
    let named = Named(name: "a \"quoted\"\tname that is long enough to allocate", n: 3)
    print(named, "\(named)")
    let pair = [named, Named(name: "e\u{301}", n: 4)]
    print(pair.count, pair[1].name == "é")

    // Optionals, arrays and class instances, which the runtime
    // describes from metadata this module emits for them.
    let some: Int32? = 3
    let none: Int? = nil
    print(some, none, [1, 2, 3], ["x", "y\"z"], [[1.5], []])
    print(Tagged(id: 7, tags: [1, 2]))
    let holder = Holder()
    print(holder, [holder], "\(holder)")

    // Wider than an existential's buffer, so boxed.
    let wide = Wide(a: "first, and long enough to allocate", b: "second", n: 3)
    print(wide, [wide, wide].count, "\(wide)")
    scoped()
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
	// Named main, as this compiler names the program's module, so that a
	// nested struct is qualified the same way by both.
	if out, err := exec.Command(swiftc, "-module-name", "main", "-o", oracleBin, swiftSrc).CombinedOutput(); err != nil {
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
	// Linked by this package alone: print is the Vertex runtime's, so
	// there is no Swift library to bring in and no toolchain to ask.
	exe, err := build.Executable([]build.Input{{Name: "p.o", Data: obj}},
		build.LinkOptions{Target: target})
	if err != nil {
		t.Fatal(err)
	}
	binPath := filepath.Join(dir, "p")
	if err := os.WriteFile(binPath, exe, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := exec.Command(binPath).Output()
	if err != nil {
		t.Fatalf("running this compiler's program: %v", err)
	}

	if string(got) != string(want) {
		t.Errorf("output differs\n--- this compiler ---\n%s--- swiftc ---\n%s", got, want)
	}
}
