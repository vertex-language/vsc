package gen

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/parser"
	"github.com/vertex-language/vsc/token"
	"github.com/vertex-language/vsc/vil"
	"github.com/vertex-language/vsc/vil/text"
	"github.com/vertex-language/vsc/vil/verify"
)

// lower parses, checks and lowers a program, failing on any
// diagnostic from either phase — a program that does not check is not
// a program this package promises anything about.
func lower(t *testing.T, src string) *vil.Module {
	t.Helper()
	f := token.NewFile("t.swift", []byte(src))
	file, diags := parser.ParseFile(f, 0)
	for _, d := range diags {
		t.Fatalf("parse: %s", d.Print(f))
	}
	info, checks := analyzer.Check([]*ast.File{file})
	for _, d := range checks {
		t.Fatalf("check: %s", d.Print(f))
	}
	m, gens := File("t", file, info)
	for _, d := range gens {
		t.Fatalf("gen: %s", d.Print(f))
	}
	return m
}

// TestLoweredModulesVerify is the contract this package owes the
// next one: what it emits is sound by the rules vil/verify holds.
// Every program here is one the checker accepts and the generator
// covers.
func TestLoweredModulesVerify(t *testing.T) {
	programs := []struct{ name, src string }{
		{"borrowed parameter", `
final class Box { var n: Int = 0 }
func borrows(_ b: Box) -> Int { return b.n }
`},
		{"a let that keeps a reference", `
final class Box { var n: Int = 0 }
func keeps(_ b: Box) -> Box {
    let kept = b
    return kept
}
`},
		{"a var, written and read", `
final class Box { var n: Int = 0 }
func reads(_ b: Box) -> Int {
    var total = 0
    total = b.n
    return total
}
`},
		{"a branch that returns from both arms", `
final class Box { var n: Int = 0 }
func picks(_ b: Box, _ flag: Bool) -> Box {
    if flag { return b }
    return b
}
`},
		{"a branch that returns from one arm", `
final class Box { var n: Int = 0 }
func maybe(_ b: Box, _ flag: Bool) -> Int {
    if flag { return 0 }
    return b.n
}
`},
		{"a let of a class, unused", `
final class Box { var n: Int = 0 }
func drops(_ b: Box) -> Int {
    let kept = b
    return b.n
}
`},
		{"a call", `
func inner(_ n: Int) -> Int { return n }
func outer(_ n: Int) -> Int { return inner(n) }
`},
		{"arithmetic", `
func add(_ a: Int, _ b: Int) -> Int { return a + b }
`},
		{"comparison", `
func lt(_ a: Int, _ b: Int) -> Bool { return a < b }
`},
		{"floating point", `
func scale(_ a: Double, _ b: Double) -> Double { return a * b }
`},
		{"fib, from the README", `
func fib(_ n: Int) -> Int {
    if n <= 1 { return n }
    return fib(n - 1) + fib(n - 2)
}
`},
		{"a function with no result", `
final class Box { var n: Int = 0 }
func nothing(_ b: Box) {
    let kept = b
}
`},
	}

	for _, p := range programs {
		t.Run(p.name, func(t *testing.T) {
			m := lower(t, p.src)
			if err := verify.Module(m); err != nil {
				t.Errorf("%v\n\n%s", err, text.String(m))
			}
		})
	}
}

// TestBorrowedParameter is the whole pipeline against one function,
// printed. Every instruction here is one swiftc emits for the same
// program, in the same order.
func TestBorrowedParameter(t *testing.T) {
	m := lower(t, `
final class Box { var n: Int = 0 }
func borrows(_ b: Box) -> Int { return b.n }
`)
	want := `sil hidden [ossa] @borrows : $@convention(thin) (@guaranteed Box) -> Int {
bb0(%0 : @guaranteed $Box):
  debug_value %0, let, name "b", argno 1
  %1 = ref_element_addr %0, #Box.n
  %2 = begin_access [read] [dynamic] %1
  %3 = load [trivial] %2
  end_access %2
  return %3
} // end sil function 'borrows'
`
	if got := funcText(t, m, "borrows"); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// TestOwnershipIsEmitted covers the reason the IR exists: a binding
// that keeps a reference copies it and destroys it, and returning it
// copies again inside a borrow.
func TestOwnershipIsEmitted(t *testing.T) {
	m := lower(t, `
final class Box { var n: Int = 0 }
func keeps(_ b: Box) -> Box {
    let kept = b
    return kept
}
`)
	want := `sil hidden [ossa] @keeps : $@convention(thin) (@guaranteed Box) -> @owned Box {
bb0(%0 : @guaranteed $Box):
  debug_value %0, let, name "b", argno 1
  %1 = copy_value %0
  %2 = move_value [lexical] [var_decl] %1
  debug_value %2, let, name "kept"
  %3 = begin_borrow %2
  %4 = copy_value %3
  end_borrow %3
  destroy_value %2
  return %4
} // end sil function 'keeps'
`
	if got := funcText(t, m, "keeps"); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// TestArithmeticIsBuiltins covers what core made possible: an
// operator resolves to a declaration, and the declaration's body is a
// machine instruction. Every instruction below is one swiftc emits
// for the same source, in the same order.
func TestArithmeticIsBuiltins(t *testing.T) {
	m := lower(t, `
func add(_ a: Int, _ b: Int) -> Int { return a + b }
`)
	want := `sil hidden [ossa] @add : $@convention(thin) (Int, Int) -> Int {
bb0(%0 : $Int, %1 : $Int):
  debug_value %0, let, name "a", argno 1
  debug_value %1, let, name "b", argno 2
  %2 = struct_extract %0, #Int._value
  %3 = struct_extract %1, #Int._value
  %4 = integer_literal $Builtin.Int1, -1
  %5 = builtin "sadd_with_overflow_Int64"(%2, %3, %4) : $(Builtin.Int64, Builtin.Int1)
  %6 = tuple_extract %5, 0
  %7 = tuple_extract %5, 1
  cond_fail %7, "arithmetic overflow"
  %8 = struct $Int (%6)
  return %8
} // end sil function 'add'
`
	if got := funcText(t, m, "add"); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// TestComparisonIsABuiltin: no overflow check, and the result is a
// bit wrapped in a Bool.
func TestComparisonIsABuiltin(t *testing.T) {
	m := lower(t, `
func lt(_ a: Int, _ b: Int) -> Bool { return a < b }
`)
	want := `sil hidden [ossa] @lt : $@convention(thin) (Int, Int) -> Bool {
bb0(%0 : $Int, %1 : $Int):
  debug_value %0, let, name "a", argno 1
  debug_value %1, let, name "b", argno 2
  %2 = struct_extract %0, #Int._value
  %3 = struct_extract %1, #Int._value
  %4 = builtin "cmp_slt_Int64"(%2, %3) : $Builtin.Int1
  %5 = struct $Bool (%4)
  return %5
} // end sil function 'lt'
`
	if got := funcText(t, m, "lt"); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// The corpus, and the oracle.
//
// testdata holds programs, each naming in its first line the function
// to compare. Every one is lowered by this package and by swiftc, and
// after the harness normalizes the two — symbols, value numbers, the
// def-use comments — they must be the same text.
//
// What is in the corpus is what matches today, which is the ownership
// machinery: borrowed and owned parameters, bindings that keep
// references, calls, struct and class members, functions with no
// result. That is the part worth holding still, because it is the
// part that is hard to get right and quiet when it is wrong.
//
// What is not in it, and why: anything with an operator or a literal.
// In raw SIL both are calls into the standard library — `a + b` is an
// apply of Int's `+`, `0` is an apply of its literal initializer —
// and this compiler emits the form those calls leave behind once the
// first mandatory pass has inlined them. So the two differ at the raw
// stage by exactly the calls, and at the canonical stage by the
// ownership form and whatever else the optimizer did. Closing that is
// core/ declaring the primitive types, which is written down in
// core's own documentation as the next thing it needs.

// TestAgreesWithSwiftc runs the corpus past both compilers.
func TestAgreesWithSwiftc(t *testing.T) {
	swiftc, err := exec.LookPath("swiftc")
	if err != nil {
		t.Skip("no swiftc on PATH")
	}
	for _, path := range corpus(t) {
		t.Run(filepath.Base(path), func(t *testing.T) {
			src, fn := program(t, path)
			ours := text.Normalize(funcText(t, lower(t, src), fn))
			theirs := text.Normalize(swiftSIL(t, swiftc, src, fn))
			if ours != theirs {
				t.Errorf("VIL and SIL disagree.\n--- vil\n%s\n--- sil\n%s", ours, theirs)
			}
		})
	}
}

// TestCorpusVerifies holds the same programs to the ownership rules,
// with or without a toolchain to compare against. Matching swiftc and
// being sound are different claims, and this is the one that does not
// need swiftc installed to make.
func TestCorpusVerifies(t *testing.T) {
	for _, path := range corpus(t) {
		t.Run(filepath.Base(path), func(t *testing.T) {
			src, _ := program(t, path)
			m := lower(t, src)
			if err := verify.Module(m); err != nil {
				t.Errorf("%v\n\n%s", err, text.String(m))
			}
		})
	}
}

// corpus is every program in testdata.
func corpus(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob("testdata/*.swift")
	if err != nil || len(files) == 0 {
		t.Skip("no corpus")
	}
	return files
}

// program reads one corpus file: its source, and the function its
// first line names.
func program(t *testing.T, path string) (src, fn string) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	src = string(b)
	first, _, _ := strings.Cut(src, "\n")
	name, ok := strings.CutPrefix(strings.TrimSpace(first), "// vil:")
	if !ok {
		t.Fatalf("%s: the first line must name the function to compare, as `// vil: name`", path)
	}
	return src, strings.TrimSpace(name)
}

// swiftSIL runs swiftc over the same source and returns the one
// function's SIL.
func swiftSIL(t *testing.T, swiftc, src, fn string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "t.swift")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(swiftc, "-emit-silgen", path).Output()
	if err != nil {
		t.Skipf("swiftc: %v", err)
	}

	var keep []string
	in := false
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "sil ") && strings.Contains(line, fn) {
			in = true
		}
		if in {
			keep = append(keep, line)
			if strings.HasPrefix(line, "} // end sil function") {
				break
			}
		}
	}
	if len(keep) == 0 {
		t.Skipf("swiftc emitted no function matching %q", fn)
	}
	return strings.Join(keep, "\n") + "\n"
}

func funcText(t *testing.T, m *vil.Module, name string) string {
	t.Helper()
	f := m.Lookup(name)
	if f == nil {
		t.Fatalf("no function %q in the module:\n%s", name, text.String(m))
	}
	var b strings.Builder
	if err := text.Func(&b, f); err != nil {
		t.Fatal(err)
	}
	return b.String()
}
