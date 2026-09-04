package parser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vertex-language/vsc/token"
)

// The tests in this file check the parser against Swift itself rather
// than against this repository's idea of Swift.
//
// Two oracles, both on the host:
//
//	the SDK's module interfaces — 50MB of Swift that Apple's own
//	compiler generated and reads back, covering every corner of the
//	language a public API can reach;
//
//	swiftc, which says whether a file is Swift.
//
// Both are skipped where they are not installed, so the suite still
// runs on a machine with no toolchain. Neither is a substitute for
// tests/, which is the corpus this front end owns.

// sdkRoots returns the SDK directories to search for module
// interfaces, newest first, or nil if there is no toolchain.
func sdkRoots(t *testing.T) []string {
	t.Helper()
	out, err := exec.Command("xcrun", "--show-sdk-path").Output()
	if err != nil {
		return nil
	}
	sdk := strings.TrimSpace(string(out))
	if sdk == "" {
		return nil
	}
	if testing.Short() {
		return []string{sdk}
	}
	// The default SDK's parent holds every installed one, and each
	// ships interfaces built by a different compiler version. The
	// command line tools keep their own set beside Xcode's.
	var roots []string
	for _, dir := range []string{
		filepath.Dir(sdk),
		"/Library/Developer/CommandLineTools/SDKs",
	} {
		found, err := filepath.Glob(filepath.Join(dir, "*.sdk"))
		if err == nil {
			roots = append(roots, found...)
		}
	}
	if len(roots) == 0 {
		return []string{sdk}
	}
	return roots
}

// TestSDKInterfaces parses every .swiftinterface in every installed
// SDK. A module interface is Swift source: written by the compiler,
// read back by it, and full of the spellings a hand-written corpus
// does not reach — underscored modifiers, access levels on imports,
// value generics, compilation conditions on single declarations. None
// of it may produce a diagnostic.
func TestSDKInterfaces(t *testing.T) {
	roots := sdkRoots(t)
	if len(roots) == 0 {
		t.Skip("no SDK: install the Xcode command line tools to run this")
	}
	var files []string
	for _, root := range roots {
		filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err == nil && !d.IsDir() && strings.HasSuffix(path, ".swiftinterface") {
				files = append(files, path)
			}
			return nil
		})
	}
	if len(files) == 0 {
		t.Skip("no module interfaces in the installed SDKs")
	}

	bad := 0
	for _, name := range files {
		src, err := os.ReadFile(name)
		if err != nil {
			continue
		}
		f := token.NewFile(name, src)
		if _, diags := ParseFile(f, 0); len(diags) > 0 {
			bad++
			if bad <= 20 {
				t.Errorf("%s", diags[0].Print(f))
			}
		}
	}
	if bad > 0 {
		t.Errorf("%d of %d module interfaces did not parse", bad, len(files))
	} else {
		t.Logf("%d module interfaces parsed clean", len(files))
	}
}

// swiftcAccepts reports whether Swift's own parser accepts src.
func swiftcAccepts(t *testing.T, swiftc, src string) bool {
	t.Helper()
	name := filepath.Join(t.TempDir(), "case.swift")
	if err := os.WriteFile(name, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return exec.Command(swiftc, "-frontend", "-parse", name).Run() == nil
}

// accepts reports whether this parser accepts src.
func accepts(src string) bool {
	f := token.NewFile("case.swift", []byte(src))
	_, diags := ParseFile(f, 0)
	for _, d := range diags {
		if d.Severity == token.Error {
			return false
		}
	}
	return true
}

// TestSwiftcAgreement runs the corpus and a table of malformed
// sources past both parsers and compares the verdicts. Agreeing on
// what is Swift means agreeing about what is not: a parser that
// accepts everything would pass every corpus test in the suite.
func TestSwiftcAgreement(t *testing.T) {
	swiftc, err := exec.LookPath("swiftc")
	if err != nil {
		t.Skip("no swiftc on PATH")
	}

	// Sources Swift rejects. Each is a place this parser could be
	// permissive without any corpus noticing.
	rejected := []string{
		"let x = ",
		"func f( {}",
		"struct S { let }",
		"if true { } else",
		"let x: = 1",
		"func f() -> {}",
		"let x = 1 +",
		"enum E { case }",
		"let x = [1, 2",
		"switch x { case 1 }",
		"func f(a: Int b: Int) {}",
		"let 1x = 2",
		"guard true else",
		"for x { }",
		`let x = "abc`,
		"extension {}",
		"typealias = Int",
		"func f(inout x: Int) {}",
		"struct S: nonisolated(unsafe) P {}",
		"let t: (a Int) -> Void = { _ in }",
		"class C { deinit(x: Int) {} }",
		"protocol P { func f() -> }",
	}
	for _, src := range rejected {
		if swiftcAccepts(t, swiftc, src) {
			t.Errorf("swiftc accepts %q: the case belongs in tests/, not here", src)
			continue
		}
		if accepts(src) {
			t.Errorf("accepted a source Swift rejects: %q", src)
		}
	}

	// The corpus, the other way round: everything in tests/ is Swift,
	// and this is what keeps it honest as it grows.
	files, _ := filepath.Glob("../tests/*.swift")
	for _, name := range files {
		src, err := os.ReadFile(name)
		if err != nil {
			continue
		}
		if !swiftcAccepts(t, swiftc, string(src)) {
			t.Errorf("%s: swiftc rejects it, so it is not Swift", filepath.Base(name))
		}
	}
}
