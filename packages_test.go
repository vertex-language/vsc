package vsc_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vertex-language/vsc"
	"github.com/vertex-language/vsc/iface"
	"github.com/vertex-language/vsc/token"
)

// A fakePackages stands in for the fetching resolver, so that what is
// under test is which paths the compiler hands it and which it answers
// from disk -- not the network.
type fakePackages struct {
	local map[string]string
	dirs  map[string]string
	errs  map[string]error
	asked []string // what Fetch was asked for
	near  []string // what Local was asked for
}

func (p *fakePackages) Local(path, fromDir string) (string, error) {
	p.near = append(p.near, path)
	return p.local[path], nil
}

func (p *fakePackages) Fetch(path string) (string, error) {
	p.asked = append(p.asked, path)
	if err := p.errs[path]; err != nil {
		return "", err
	}
	return p.dirs[path], nil
}

// writePackage writes a folder of source and returns where it is.
func writePackage(t *testing.T, dir, name, body string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	src := "package " + name + "\n" + body
	if err := os.WriteFile(filepath.Join(dir, name+".vs"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestLocalAnswerComesFirst: the resolver's local answer -- a -replace, or
// the checkout the importing file is in -- beats a folder under a search
// root and a fetch, so a package's own tests build against it as it is.
func TestLocalAnswerComesFirst(t *testing.T) {
	root := t.TempDir()
	// A folder the search root would find, answering a different number, so
	// which one was compiled is visible in the program.
	writePackage(t, filepath.Join(root, "net", "tcp"), "tcp", "public func answer() -> Int32 { return 1 }\n")
	checkout := writePackage(t, filepath.Join(t.TempDir(), "tcp"), "tcp", "public func answer() -> Int32 { return 42 }\n")

	p := &fakePackages{
		local: map[string]string{"net/tcp": checkout},
		dirs:  map[string]string{"net/tcp": "/should/not/be/used"},
	}
	u, diags := compile(t, `
import "net/tcp"
func main() -> Int32 { return tcp.answer() }
`, vsc.Options{PackagePaths: []string{root}, Packages: p})
	for _, d := range diags {
		t.Fatalf("compile: %v", d)
	}
	if len(p.asked) != 0 {
		t.Errorf("resolver was asked to fetch %v, though it had the package locally", p.asked)
	}
	if len(u.Packages) != 1 || u.Packages[0].Dir != checkout {
		t.Fatalf("compiled %+v, want the local %s", u.Packages, checkout)
	}
}

// TestFolderBeatsFetchingForAnOrdinaryPath: a folder under a search root
// is used rather than one downloaded over it, whatever the path looks like.
func TestFolderBeatsFetchingForAnOrdinaryPath(t *testing.T) {
	root := t.TempDir()
	local := writePackage(t, filepath.Join(root, "util", "text"), "text", "public func n() -> Int32 { return 7 }\n")

	p := &fakePackages{dirs: map[string]string{"util/text": "/should/not/be/used"}}
	u, diags := compile(t, `
import "util/text"
func main() -> Int32 { return text.n() }
`, vsc.Options{PackagePaths: []string{root}, Packages: p})
	for _, d := range diags {
		t.Fatalf("compile: %v", d)
	}
	if len(p.asked) != 0 {
		t.Errorf("resolver was asked for %v, though the folder was there", p.asked)
	}
	if len(u.Packages) != 1 || u.Packages[0].Dir != local {
		t.Fatalf("compiled %+v, want %s", u.Packages, local)
	}
}

// TestFetchedWhenNoFolderHasIt is what makes `vsc run` work on a machine
// that has never seen the package.
func TestFetchedWhenNoFolderHasIt(t *testing.T) {
	fetched := writePackage(t, filepath.Join(t.TempDir(), "thing"), "thing", "public func n() -> Int32 { return 9 }\n")
	p := &fakePackages{dirs: map[string]string{"github.com/you/thing": fetched}}
	u, diags := compile(t, `
import "github.com/you/thing"
func main() -> Int32 { return thing.n() }
`, vsc.Options{Packages: p})
	for _, d := range diags {
		t.Fatalf("compile: %v", d)
	}
	if len(u.Packages) != 1 || u.Packages[0].Dir != fetched {
		t.Fatalf("compiled %+v, want %s", u.Packages, fetched)
	}
}

// TestRelativePathIsNeverFetched keeps "./util" meaning the folder beside
// the file, whatever a resolver would say about it.
func TestRelativePathIsNeverFetched(t *testing.T) {
	p := &fakePackages{dirs: map[string]string{"./util": "/somewhere"}}
	_, diags := compile(t, `
import "./util"
func main() -> Int32 { return 0 }
`, vsc.Options{Packages: p})
	if len(diags) == 0 {
		t.Fatal("a relative import of a folder that is not there was accepted")
	}
	if len(p.asked) != 0 || len(p.near) != 0 {
		t.Errorf("resolver was asked for %v and %v, though the path is relative", p.asked, p.near)
	}
}

// TestNoResolverStillReportsTheMissingPackage: compiling with nothing to
// fetch through is the ordinary case for a library, and the message is
// the one it always was.
func TestNoResolverStillReportsTheMissingPackage(t *testing.T) {
	_, diags := compile(t, `
import "net/tcp"
func main() -> Int32 { return 0 }
`, vsc.Options{})
	if len(diags) == 0 {
		t.Fatal("an import of a package that is not there was accepted")
	}
	if !strings.Contains(diags[0].Message, "no such package 'net/tcp'") {
		t.Errorf("reported %q", diags[0].Message)
	}
}

// TestAFetchThatFailsIsReported: with no folder and no checkout, what the
// fetch said is the diagnostic, not a generic "no such package".
func TestAFetchThatFailsIsReported(t *testing.T) {
	p := &fakePackages{errs: map[string]error{"util/text": os.ErrNotExist}}
	_, diags := compile(t, `
import "util/text"
func main() -> Int32 { return 0 }
`, vsc.Options{PackagePaths: []string{t.TempDir()}, Packages: p})
	if len(diags) == 0 {
		t.Fatal("an import nothing could find was accepted")
	}
	if !strings.Contains(diags[0].Message, os.ErrNotExist.Error()) {
		t.Errorf("reported %q, want the fetch's error", diags[0].Message)
	}
	if len(p.asked) != 1 || p.asked[0] != "util/text" {
		t.Errorf("resolver was asked %v, want [util/text]", p.asked)
	}
}

// TestInlinableRunsInAnotherModulesKernel: a kernel calls a package's
// @inlinable function, and the device compile has its body -- which a
// kernel cannot get any other way, since a device cannot call into
// another module's object code. A function that is not @inlinable is
// refused, with what to do about it.
func TestInlinableRunsInAnotherModulesKernel(t *testing.T) {
	root := t.TempDir()
	writePackage(t, filepath.Join(root, "acc", "ops"), "ops", `
@inlinable public func twice(_ x: Float) -> Float { return x + x }
public func plain(_ x: Float) -> Float { return x }
public struct Pair {
    public let a: Float
    public let b: Float
    @inlinable public init(a: Float, b: Float) { self.a = a; self.b = b }
    @inlinable public func sum() -> Float { return a + b }
}
@inlinable public func viaPlain(_ x: Float) -> Float { return plain(x) }
`)
	opts := vsc.Options{PackagePaths: []string{root}, Packages: &fakePackages{}}
	_, diags := compile(t, `
import "gpu"
import "acc/ops"
func k(_ y: gpu.MutableSpan<float32>) kernel { y[gpu.Index.x] = ops.twice(ops.Pair(a: y[gpu.Index.x], b: 1).sum()) }
func main() -> Int32 { return Int32(ops.twice(1) + ops.Pair(a: 1, b: 2).sum()) }
`, opts)
	for _, d := range diags {
		if d.Severity == token.Error {
			t.Fatalf("compile: %v", d)
		}
	}
	_, diags = compile(t, `
import "gpu"
import "acc/ops"
func k(_ y: gpu.MutableSpan<float32>) kernel { y[gpu.Index.x] = ops.viaPlain(y[gpu.Index.x]) }
func main() -> Int32 { return 0 }
`, opts)
	var msg string
	for _, d := range diags {
		msg += d.String()
	}
	if !strings.Contains(msg, "Mark it @inlinable") {
		t.Fatalf("a kernel reaching a non-inlinable function of another module: got %q", msg)
	}
}

// TestInterfaceKeepsInlinableBodies: an interface is the module's public
// face with the bodies taken out -- except an @inlinable function's,
// which is the client's to compile.
func TestInterfaceKeepsInlinableBodies(t *testing.T) {
	u, diags := compile(t, `
@inlinable public func twice(_ x: Int32) -> Int32 { return x + x }
public func hidden(_ x: Int32) -> Int32 { return x * 3 }
public struct Box {
    public let v: Int32
    @inlinable public init(_ v: Int32) { self.v = v }
    public init(other: Int32) { self.v = other * 5 }
    @inlinable public func doubled() -> Int32 { return v * 2 }
}
`, vsc.Options{Module: "lib", Stop: vsc.Checked})
	for _, d := range diags {
		t.Fatalf("compile: %v", d)
	}
	var b strings.Builder
	if err := iface.Print(&b, iface.Module{Name: "lib", Files: u.Files, Units: u.Positions, Info: u.Info}); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if !strings.Contains(out, "@inlinable public func twice(_ x: Int32) -> Int32 { return x + x }") {
		t.Errorf("the inlinable body is missing:\n%s", out)
	}
	for _, want := range []string{
		"@inlinable public init(_ v: Int32) { self.v = v }",
		"@inlinable public func doubled() -> Int32 { return v * 2 }",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "other * 5") {
		t.Errorf("a body that is not @inlinable was written:\n%s", out)
	}
	if strings.Contains(out, "x * 3") {
		t.Errorf("a body that is not @inlinable was written:\n%s", out)
	}
}

// TestGenericKernelFromAnotherModule: a package's @inlinable generic
// function launches its @inlinable generic kernel over a protocol the
// package declares and conforms float32 to; the client specializes both.
func TestGenericKernelFromAnotherModule(t *testing.T) {
	root := t.TempDir()
	writePackage(t, filepath.Join(root, "acc", "ops"), "ops", `
import "gpu"
public protocol Scalable: Numeric { static func Twice(_ x: Self) -> Self }
extension float32: Scalable { @inlinable public static func Twice(_ x: float32) -> float32 { return x + x } }
@inlinable public func _k<T: Scalable>(_ y: gpu.MutableSpan<T>, _ a: T) kernel { y[gpu.Index.x] = T.Twice(y[gpu.Index.x]) * a }
@inlinable public func Apply<T: Scalable>(_ b: gpu.Buffer<T>, _ a: T) async throws { try await _k.Launch(b, a, over: b.count) }
`)
	_, diags := compile(t, `
import "gpu"
import "acc/ops"
func main() async throws {
    let b = try await gpu.CPU().Upload([float32(1), 2])
    try await ops.Apply(b, 3)
}
`, vsc.Options{PackagePaths: []string{root}, Packages: &fakePackages{}})
	for _, d := range diags {
		if d.Severity == token.Error {
			t.Fatalf("compile: %v", d)
		}
	}
}

// TestInterfaceCarriesProtocolsAndExtensions: what a client needs of a
// module's protocols -- the protocol, the conformances extensions give,
// the packages they come from -- is in its interface.
func TestInterfaceCarriesProtocolsAndExtensions(t *testing.T) {
	u, diags := compile(t, `
import "gpu"
public protocol Shape { static func Sides() -> Int32 }
extension float32: Shape { @inlinable public static func Sides() -> Int32 { return 3 } }
`, vsc.Options{Module: "lib", Stop: vsc.Checked})
	for _, d := range diags {
		t.Fatalf("compile: %v", d)
	}
	var b strings.Builder
	if err := iface.Print(&b, iface.Module{Name: "lib", Files: u.Files, Units: u.Positions, Info: u.Info}); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, want := range []string{
		`import "gpu"`,
		"public protocol Shape { static func Sides() -> Int32 }",
		"extension float32: Shape {",
		"@inlinable public static func Sides() -> Int32 { return 3 }",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q:\n%s", want, out)
		}
	}
}
