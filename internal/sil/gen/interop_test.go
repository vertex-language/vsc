package gen

import (
	"strings"
	"testing"
)

// The two attributes that name a symbol, and the ones that do not
// work and have to say so.
//
// The bug here was the quiet kind. All four parsed, none was read by
// anything, and each built a program that did not do what the source
// said: `@_cdecl` exported no symbol, `@_silgen_name` left a function
// that traps where the call should have been, `@objc` did nothing at
// all. A declaration attribute whose whole purpose is a symbol is
// either honoured or refused; there is no third answer that is not a
// lie.

// TestSilgenNameIsTheSymbol: the attribute decides the name, at the
// definition and at every call, because a symbol both ends do not
// agree on is not a symbol.
func TestSilgenNameIsTheSymbol(t *testing.T) {
	got, said := refusals(t, `
@_silgen_name("abs")
func c_abs(_ x: Int32) -> Int32
func main() -> Int32 { return c_abs(-5) }`)
	if said != "" {
		t.Fatalf("refused: %s", said)
	}
	if !strings.Contains(got, "function_ref @abs") {
		t.Errorf("the call does not name the symbol:\n%s", got)
	}
	// A declaration and not a definition: something else defines it,
	// which is the whole point. A body here would define the symbol
	// as a function that falls off its end.
	if strings.Contains(got, "@abs : $@convention(thin) (Int32) -> Int32 {") {
		t.Errorf("the symbol was defined here rather than declared:\n%s", got)
	}
}

// TestCDeclIsAThunk: `@_cdecl` adds a C entry point beside the
// function rather than renaming it, so both names are real -- Vertex
// calls it by its own and C by the other.
func TestCDeclIsAThunk(t *testing.T) {
	got, said := refusals(t, `
@_cdecl("vs_add")
func add(_ a: Int32, _ b: Int32) -> Int32 { return a + b }
func main() -> Int32 { return add(1, 2) }`)
	if said != "" {
		t.Fatalf("refused: %s", said)
	}
	if !strings.Contains(got, `[thunk] @vs_add : $@convention(c) (Int32, Int32) -> Int32`) {
		t.Errorf("no C entry point:\n%s", got)
	}
	// The function the source wrote keeps its own symbol, and the
	// call from main goes there rather than through the thunk.
	if !strings.Contains(got, "sil hidden [ossa] @$s4main3addys5Int32VAD_ADtF") {
		t.Errorf("the function lost its own symbol:\n%s", got)
	}
	if strings.Count(got, "function_ref @vs_add") != 0 {
		t.Errorf("a Vertex call went through the C thunk:\n%s", got)
	}
}

// TestInteropRefusals: what cannot be honoured says so. Each of these
// used to build.
func TestInteropRefusals(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"objc", `
class C { @objc func f() -> Int32 { return 1 } }
func main() -> Int32 { return C().f() }`, "'@objc'"},

		{"silgen name with no symbol", `
@_silgen_name
func f(_ x: Int32) -> Int32
func main() -> Int32 { return f(1) }`, "needs the symbol to use"},

		{"cdecl with no symbol", `
@_cdecl
func f(_ x: Int32) -> Int32 { return x }
func main() -> Int32 { return f(1) }`, "needs the symbol to export"},

		{"no body and no attribute", `
func f(_ x: Int32) -> Int32
func main() -> Int32 { return f(1) }`, "has no body"},

		{"cdecl over a value C cannot take", `
@_cdecl("vs_s")
func f(_ s: String) -> Int32 { return 0 }
func main() -> Int32 { return 0 }`, "cross in a register"},

		{"cdecl over a name already taken", `
@_cdecl("main")
func f() -> Int32 { return 0 }
func main() -> Int32 { return 0 }`, "both 'main'"},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, said := refusals(t, c.src)
			if said == "" {
				t.Fatalf("built it and said nothing")
			}
			if !strings.Contains(said, c.want) {
				t.Errorf("said %q, want it to mention %q", said, c.want)
			}
		})
	}
}
