package build_test

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"testing"

	"github.com/vertex-language/ir"
	vsc "github.com/vertex-language/vsc"
	"github.com/vertex-language/vsc/build"
	"github.com/vertex-language/vsc/iface"
)

// The interop corpus: programs this compiler builds that call Swift
// libraries swiftc builds.
//
// tests/compiler asks whether a program this compiler builds by itself
// does what it says. This asks the other question -- whether what it
// builds is the same thing swiftc builds -- and it asks it the only
// way that cannot be fudged, by putting both compilers' output in one
// process. The symbol asked of the linker has to be the symbol swiftc
// defined, and the registers the arguments go in have to be the ones
// swiftc's code reads them from.
//
// The libraries use the parts of Swift this compiler does not have:
// String, Array, Dictionary, Optional, Codable, Foundation's Calendar
// and JSON. That is the point rather than a limitation. A compiler
// does not have to implement a library to call it -- it has to agree
// with it about names and registers, and nothing short of running the
// two together shows that it does.
//
// Each directory holds three files:
//
//	library.swift          built by swiftc, with -parse-as-library
//	<Module>.vertexinterface  what this compiler is told about it
//	program.swift          built by this compiler, and run
//
// The interface is written by hand rather than emitted, because
// swiftc emits one only under -enable-library-evolution, which is a
// different ABI -- the resilient one, where a field is read through a
// getter call. So the interface is a claim about the library, and the
// test is whether the claim is true: if it says something swiftc did
// not build, the link fails.
//
// A program returns 42 when it is satisfied, and the number of the
// check that failed otherwise. There is no oracle to compare against
// here -- the program is this compiler's alone -- so it checks itself,
// which is the convention tests/compiler uses for the same reason.
func TestInteropCorpus(t *testing.T) {
	if runtime.GOARCH != "arm64" || runtime.GOOS != "darwin" {
		t.Skip("not on Apple Silicon")
	}
	swiftc, err := exec.LookPath("swiftc")
	if err != nil {
		t.Skip("no swiftc on PATH; there is no Swift to interoperate with")
	}
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("no clang on PATH")
	}
	target, ok := build.Host()
	if !ok {
		t.Skip("no backend for this machine")
	}

	dirs, err := filepath.Glob("../tests/interop/*")
	if err != nil || len(dirs) == 0 {
		t.Fatal("no cases found in tests/interop")
	}
	for _, dir := range dirs {
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			continue
		}
		t.Run(filepath.Base(dir), func(t *testing.T) {
			runInteropCase(t, swiftc, target, dir)
		})
	}
}

func runInteropCase(t *testing.T, swiftc string, target ir.Target, dir string) {
	t.Helper()
	work := t.TempDir()
	libObjs := buildLibraries(t, swiftc, dir, work)

	// The program, by this compiler.
	src, err := os.ReadFile(filepath.Join(dir, "program.swift"))
	if err != nil {
		t.Fatal(err)
	}
	unit, diags := vsc.Compile([]vsc.Source{{Name: "program.swift", Text: src}},
		vsc.Options{Module: "main", Target: target, ImportPaths: []string{work}})
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
	appObj := filepath.Join(work, "program.o")
	if err := os.WriteFile(appObj, obj, 0o644); err != nil {
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

	// swiftc drives the link: a library that uses the standard
	// library needs it and the Swift runtime on the command line, and
	// swiftc is what knows where they are.
	bin := filepath.Join(work, "prog")
	link := exec.Command(swiftc, append([]string{"-o", bin, appObj, rtObj}, libObjs...)...)
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

// interfaceIn finds the one interface in a case directory and reads
// the module name out of its header.
// buildLibraries builds every Swift library in a case and puts its
// interface where an import will look for it, returning the objects
// to link.
//
// A case is usually one library, and then the file is `library.swift`
// beside its interface. A case about what happens across two modules
// is two, named `1-Base.swift` and `2-Top.swift`: the number is the
// order they have to be built in, because one imports the other, and
// the name after it is the module. That order is the point of such a
// case, so it is in the filename rather than in a manifest.
func buildLibraries(t *testing.T, swiftc, dir, work string) []string {
	t.Helper()
	sources, err := filepath.Glob(filepath.Join(dir, "*.swift"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(sources)

	// Every interface first: a module built second may import the
	// first, and swiftc looks for it the same way this compiler does.
	interfaces, err := filepath.Glob(filepath.Join(dir, "*"+iface.Extension))
	if err != nil || len(interfaces) == 0 {
		t.Fatalf("%s: no interface", dir)
	}
	for _, path := range interfaces {
		text, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(work, filepath.Base(path)), text, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	var objs []string
	for _, src := range sources {
		base := filepath.Base(src)
		if base == "program.swift" {
			continue
		}
		module := moduleOfSource(t, base, dir)
		obj := filepath.Join(work, module+".o")
		cmd := exec.Command(swiftc, "-parse-as-library", "-c", "-I", work,
			"-module-name", module, "-o", obj, src)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("swiftc could not build %s: %v\n%s", module, err, out)
		}
		// So that the next library can import this one.
		mod := exec.Command(swiftc, "-emit-module", "-I", work, "-module-name", module,
			"-o", filepath.Join(work, module+".swiftmodule"), src)
		if out, err := mod.CombinedOutput(); err != nil {
			t.Fatalf("swiftc could not emit %s's module: %v\n%s", module, err, out)
		}
		objs = append(objs, obj)
	}
	if len(objs) == 0 {
		t.Fatalf("%s: no library to build", dir)
	}
	return objs
}

// moduleOfSource is the module a library file belongs to: the name
// after the ordering number, or -- for a case with one library --
// whatever its single interface says.
func moduleOfSource(t *testing.T, base, dir string) string {
	name := strings.TrimSuffix(base, ".swift")
	if i := strings.Index(name, "-"); i > 0 && name[:i] != "" {
		return name[i+1:]
	}
	_, module := interfaceIn(t, dir)
	return module
}

func interfaceIn(t *testing.T, dir string) (path, module string) {
	t.Helper()
	found, err := filepath.Glob(filepath.Join(dir, "*"+iface.Extension))
	if err != nil || len(found) != 1 {
		t.Fatalf("%s: want exactly one %s file, found %d", dir, iface.Extension, len(found))
	}
	f, err := os.Open(found[0])
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	const marker = "// vertex-module-name:"
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, marker) {
			return found[0], strings.TrimSpace(strings.TrimPrefix(line, marker))
		}
	}
	// An interface swiftc emitted has no such line -- it says
	// `-module-name` in its own header instead, and the compiler
	// takes the name from the file rather than from either. So does
	// this, which is what lets a case hand over a real interface
	// unedited.
	base := strings.TrimSuffix(filepath.Base(found[0]), iface.Extension)
	if base == "" {
		t.Fatalf("%s: no module name in the interface and none in its name", found[0])
	}
	return found[0], base
}
