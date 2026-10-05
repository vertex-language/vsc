package analyzer

import (
	"testing"

	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/parser"
	"github.com/vertex-language/vsc/token"
)

// TestAccessOf: a declaration that says nothing is internal, and every
// spelling that says something is read. The linkage of every symbol
// the compiler emits depends on this, so it is worth its own test.
func TestAccessOf(t *testing.T) {
	for _, c := range []struct {
		src  string
		want Access
	}{
		{"public func f() {}", Public},
		{"open func f() {}", Open},
		{"package func f() {}", Package},
		{"internal func f() {}", Internal},
		{"fileprivate func f() {}", FilePrivate},
		{"private func f() {}", Private},
		{"func f() {}", Internal},
		{"@inlinable public func f() {}", Public},
		{"public static func f() {}", Public},
	} {
		t.Run(c.src, func(t *testing.T) {
			sym := declaredFunc(t, c.src)
			if got := sym.Access(); got != c.want {
				t.Errorf("%s is %v, want %v", c.src, got, c.want)
			}
		})
	}
}

// declaredFunc checks one source line and returns the function it
// declared.
func declaredFunc(t *testing.T, src string) *FuncSymbol {
	t.Helper()
	f := token.NewFile("t.swift", []byte(src))
	file, diags := parser.ParseFile(f, 0)
	for _, d := range diags {
		t.Fatalf("parse: %s", d.Print(f))
	}
	info, checks := Check([]*ast.File{file})
	for _, d := range checks {
		t.Fatalf("check: %s", d.Print(f))
	}
	for _, sym := range info.Defs {
		if fs, ok := sym.(*FuncSymbol); ok && fs.Name() == "f" {
			return fs
		}
	}
	t.Fatalf("no function f declared by %q", src)
	return nil
}
