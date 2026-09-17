package build_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	vsc "github.com/vertex-language/vsc"
	"github.com/vertex-language/vsc/build"
)

// TestInterfaceDeclaresACFunction: a module whose interface declares C
// functions by `@_silgen_name` and no body, which is what a package's C
// target looks like to the Swift targets that import it. The program
// calls through the import, clang builds the C, and this package's own
// linker joins the two objects.
//
// Two things are under test. The attribute is read out of the
// interface's text -- it used to be read against the calling file's,
// which panicked -- and an object clang wrote links beside one this
// compiler wrote.
func TestInterfaceDeclaresACFunction(t *testing.T) {
	if runtime.GOARCH != "arm64" || runtime.GOOS != "darwin" {
		t.Skip("not on Apple Silicon")
	}
	clang, err := exec.LookPath("clang")
	if err != nil {
		t.Skip("no clang on PATH")
	}
	target, ok := build.Host()
	if !ok {
		t.Skip("no backend for this machine")
	}
	dir := t.TempDir()
	iface := `// swift-module-flags: -module-name CMath
@_silgen_name("cmath_scale")
public func cmath_scale(_ x: Int32) -> Int32

@_silgen_name("cmath_sum")
public func cmath_sum(_ a: Int32, _ b: Int32) -> Int32
`
	if err := os.WriteFile(filepath.Join(dir, "CMath.vinterface"), []byte(iface), 0o644); err != nil {
		t.Fatal(err)
	}
	csrc := filepath.Join(dir, "cmath.c")
	if err := os.WriteFile(csrc, []byte("int cmath_scale(int x) { return x * 3; }\nint cmath_sum(int a, int b) { return a + b; }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// The deployment target this package links for, which clang would
	// otherwise take from the SDK and the linker refuse as newer.
	cobj := filepath.Join(dir, "cmath.o")
	if out, err := exec.Command(clang, "-c", "-mmacosx-version-min=11.0", "-o", cobj, csrc).CombinedOutput(); err != nil {
		t.Fatalf("clang: %v\n%s", err, out)
	}
	cdata, err := os.ReadFile(cobj)
	if err != nil {
		t.Fatal(err)
	}

	program := `import CMath

func main() -> Int32 {
    print(cmath_sum(cmath_scale(10), 12))
    return 0
}
`
	unit, diags := vsc.Compile([]vsc.Source{{Name: "main.swift", Text: []byte(program)}},
		vsc.Options{Module: "main", Target: target, ImportPaths: []string{dir}})
	if len(diags) > 0 {
		t.Fatalf("refused: %v", diags)
	}
	obj, err := build.Object(unit.VIR, build.Options{})
	if err != nil {
		t.Fatal(err)
	}
	exe, err := build.Executable([]build.Input{{Name: "main.o", Data: obj}, {Name: "cmath.o", Data: cdata}},
		build.LinkOptions{Target: target})
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "app")
	if err := os.WriteFile(bin, exe, 0o755); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(bin).CombinedOutput()
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	if string(out) != "42\n" {
		t.Errorf("printed %q, want %q", out, "42\n")
	}
}

// TestCInterfaceMatchesSwift imports a header with vcx and compares each
// declaration with the one Swift's own importer writes for it:
// swift-synthesize-interface prints a C module as Swift sees it, so it is
// the answer for every function this importer declares. What the importer
// leaves out has to be named, and a function Swift does not import either
// -- a variadic one -- must not appear.
func TestCInterfaceMatchesSwift(t *testing.T) {
	if runtime.GOARCH != "arm64" || runtime.GOOS != "darwin" {
		t.Skip("not on Apple Silicon")
	}
	swiftc, err := exec.LookPath("swiftc")
	if err != nil {
		t.Skip("no swiftc on PATH")
	}
	synthesize := filepath.Join(filepath.Dir(swiftc), "swift-synthesize-interface")
	if _, err := os.Stat(synthesize); err != nil {
		if resolved, err := exec.Command("xcrun", "--find", "swift-synthesize-interface").Output(); err == nil {
			synthesize = strings.TrimSpace(string(resolved))
		} else {
			t.Skip("no swift-synthesize-interface")
		}
	}
	sdk, err := exec.Command("xcrun", "--show-sdk-path").Output()
	if err != nil {
		t.Skip("no SDK")
	}
	target, ok := build.Host()
	if !ok {
		t.Skip("no backend for this machine")
	}
	dir := t.TempDir()
	header := `#pragma once
#include <stdbool.h>

#ifdef __cplusplus
extern "C" {
#endif

int scalar_int(int value, unsigned count);
long scalar_long(long a, unsigned long b);
long long scalar_wide(long long a, unsigned long long b);
short scalar_short(short a, unsigned short b);
char scalar_char(char c, signed char s, unsigned char u);
double scalar_double(double d, float f);
bool scalar_bool(bool flag);
void scalar_void(void);
void scalar_takes(int in, int where);
const char *pointer_name(int which);
void pointer_fill(char *buf, const void *from, void *to, const unsigned char *bytes);
int variadic_sum(int count, ...);
static inline int inline_twice(int x) { return x * 2; }

#ifdef __cplusplus
}
#endif
`
	if err := os.WriteFile(filepath.Join(dir, "Scalars.h"), []byte(header), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "module.modulemap"), []byte("module Scalars {\n  header \"Scalars.h\"\n  export *\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	text, skipped, err := build.CInterface(build.CImport{Module: "Scalars", Headers: dir, Target: target})
	if err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(synthesize, "-module-name", "Scalars", "-I", dir,
		"-target", "arm64-apple-macosx14.0", "-sdk", strings.TrimSpace(string(sdk))).CombinedOutput()
	if err != nil {
		t.Fatalf("swift-synthesize-interface: %v\n%s", err, out)
	}

	swiftDecls := funcSignatures(string(out))
	vscDecls := funcSignatures(string(text))
	for name, sig := range vscDecls {
		if swiftDecls[name] != sig {
			t.Errorf("%s:\n  vsc:   %s\n  swift: %s", name, sig, swiftDecls[name])
		}
	}
	for _, want := range []string{"scalar_int", "scalar_long", "scalar_wide", "scalar_short", "scalar_char",
		"scalar_double", "scalar_bool", "scalar_void", "scalar_takes", "pointer_name", "pointer_fill"} {
		if _, ok := vscDecls[want]; !ok {
			t.Errorf("%s was not imported; skipped: %v", want, skipped)
		}
	}
	if _, ok := vscDecls["variadic_sum"]; ok {
		t.Error("a variadic function was imported, which Swift does not do")
	}
	named := strings.Join(skipped, "\n")
	for _, left := range []string{"variadic_sum", "inline_twice"} {
		if !strings.Contains(named, left) {
			t.Errorf("%s was left out without being named: %v", left, skipped)
		}
	}
}

// funcSignatures is each `public func` line by the function's name, with the
// parameter names taken out: they are the header's, and carry no meaning at
// a call.
func funcSignatures(text string) map[string]string {
	out := map[string]string{}
	param := regexp.MustCompile(`_ [A-Za-z_][A-Za-z0-9_]*: `)
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "public func ") {
			continue
		}
		name := strings.TrimPrefix(line, "public func ")
		if i := strings.IndexByte(name, '('); i >= 0 {
			name = name[:i]
		}
		out[name] = param.ReplaceAllString(line, "_: ")
	}
	return out
}
