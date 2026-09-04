package lower

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vertex-language/ir"
	irtext "github.com/vertex-language/ir/text"
	irverify "github.com/vertex-language/ir/verify"
	"github.com/vertex-language/vsc/analyzer"
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/parser"
	"github.com/vertex-language/vsc/token"
	"github.com/vertex-language/vsc/vil"
	"github.com/vertex-language/vsc/vil/gen"
	"github.com/vertex-language/vsc/vil/pass"
)

// target is the one these tests lower for. Nothing here depends on
// which it is except the width of a word.
var target = ir.AArch64MacOS

// TestFib is the whole compiler in one test: Swift in, VIR out.
//
// The symbol is the mangled one, because that is what a linker will
// be asked for. It is worth reading the expected text rather than
// trusting it. The
// comparison became a branch, the arithmetic became an add and a
// separate test of whether it overflowed, the trap the language
// promises became a block that traps, and the recursion became a
// call. Nothing of Swift is left in it.
func TestFib(t *testing.T) {
	got := vir(t, "testdata/fib.swift")
	want := `internal func @$s1t3fibyS2iF(%a0 i64) i64 {
@entry:
  %0 = i64.const 2
  %1 = i64.slt %a0, %0
  brif %1, @bb1, @bb2

@bb1:
  return %a0

@bb2:
  br @bb3

@bb3:
  %2 = i64.const 1
  %3 = i64.sub %a0, %2
  %4 = i64.ssubo %a0, %2
  brif %4, @trap, @cont1

@trap:
  trap

@cont1:
  %5 = call @$s1t3fibyS2iF(%3)
  %6 = i64.const 2
  %7 = i64.sub %a0, %6
  %8 = i64.ssubo %a0, %6
  brif %8, @trap, @cont2

@cont2:
  %9 = call @$s1t3fibyS2iF(%7)
  %10 = i64.add %5, %9
  %11 = i64.saddo %5, %9
  brif %11, @trap, @cont3

@cont3:
  return %10
}
`
	if body(got) != want {
		t.Errorf("got:\n%s\nwant:\n%s", body(got), want)
	}
}

// TestNarrowWidths is the one place a Swift type does not fit its
// register. VIR has no i8, so an Int8 is held in an i32 that is always
// sign-extended from its own width -- and the arithmetic has to put it
// back there, which is also how the overflow is detected.
func TestNarrowWidths(t *testing.T) {
	got := body(vir(t, "testdata/narrow.swift"))
	for _, want := range []string{
		"i32.add %a0, %a1",
		"i32.shl", "i32.sshr", // back into range
		"i32.ne", // and the fact that it moved is the overflow
		"@trap",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

// TestUnsignedSubtractBorrows: §L gives no usubo verb, because an
// unsigned subtraction overflows exactly when it borrows and that is
// a comparison every target already has.
func TestUnsignedSubtractBorrows(t *testing.T) {
	got := body(vir(t, "testdata/unsigned.swift"))
	if !strings.Contains(got, "i64.ult %a0, %a1") {
		t.Errorf("the borrow is not a comparison:\n%s", got)
	}
	if strings.Contains(got, "usubo") {
		t.Errorf("a verb the spec does not have:\n%s", got)
	}
}

// TestGreaterThanSwapsItsOperands: there is no greater-than either.
// Swift asks whether a is at least b; VIR is asked whether b is at
// most a, which is the same question read the other way round.
func TestGreaterThanSwapsItsOperands(t *testing.T) {
	got := body(vir(t, "testdata/float.swift"))
	if !strings.Contains(got, "f64.le %a1, %a0") {
		t.Errorf("the operands were not swapped:\n%s", got)
	}
}

func TestBitwise(t *testing.T) {
	got := body(vir(t, "testdata/bitwise.swift"))
	for _, want := range []string{"i32.and %a0, %a1", "i32.xor %a0, %a1", "i32.or"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

// TestRefusesTheOwnershipForm. A module that still has copy_value in
// it has retains that have not been written down, and lowering it
// would drop them.
func TestRefusesTheOwnershipForm(t *testing.T) {
	m := vil.NewModule("t", vil.StageRaw)
	if _, err := Module(m, target); !errors.Is(err, ErrStage) {
		t.Errorf("a raw module was accepted: %v", err)
	}
	m = vil.NewModule("t", vil.StageCanonical)
	if _, err := Module(m, target); !errors.Is(err, ErrStage) {
		t.Errorf("a canonical module was accepted: %v", err)
	}
}

// TestRefusalsNameWhatTheyRefuse. This package cannot yet lower a
// struct of more than one field, and what matters about that is that
// it says so: an unlowerable program must not become a lowered one
// that is wrong.
func TestRefusalsNameWhatTheyRefuse(t *testing.T) {
	for _, path := range []string{
		"../vil/gen/testdata/07-struct-member.swift",
		"../vil/gen/testdata/14-struct-field.swift",
	} {
		t.Run(filepath.Base(path), func(t *testing.T) {
			m := lowered(t, path)
			out, err := Module(m, target)
			if err == nil {
				t.Fatalf("lowered a program it cannot lower:\n%s", dump(t, out))
			}
			var e *Error
			if !errors.As(err, &e) {
				t.Fatalf("the refusal is not an *Error: %v", err)
			}
			if e.Func == "" {
				t.Errorf("the refusal does not say where: %v", err)
			}
			if !errors.Is(err, ErrType) && !errors.Is(err, ErrUnsupported) {
				t.Errorf("the refusal has no reason: %v", err)
			}
		})
	}
}

// TestCorpusLowers takes the programs vil/gen is held to and requires
// that what this package produces from them is a module ir/verify
// accepts. Two of them it refuses instead, which the test above covers.
func TestCorpusLowers(t *testing.T) {
	refused := map[string]bool{
		"07-struct-member.swift": true,
		"14-struct-field.swift":  true,
	}
	files, err := filepath.Glob("../vil/gen/testdata/*.swift")
	if err != nil || len(files) == 0 {
		t.Skip("no corpus")
	}
	for _, path := range files {
		name := filepath.Base(path)
		t.Run(name, func(t *testing.T) {
			out, err := Module(lowered(t, path), target)
			if refused[name] {
				if err == nil {
					t.Error("lowered a program listed as refused")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := irverify.Module(out); err != nil {
				t.Errorf("%v\n\n%s", err, dump(t, out))
			}
		})
	}
}

// TestOwnLoweredCorpusVerifies does the same for this package's own
// programs, which are arithmetic rather than ownership.
func TestOwnLoweredCorpusVerifies(t *testing.T) {
	files, _ := filepath.Glob("testdata/*.swift")
	for _, path := range files {
		t.Run(filepath.Base(path), func(t *testing.T) {
			out, err := Module(lowered(t, path), target)
			if err != nil {
				t.Fatal(err)
			}
			if err := irverify.Module(out); err != nil {
				t.Errorf("%v\n\n%s", err, dump(t, out))
			}
		})
	}
}

// --- helpers ---

// lowered takes a source file all the way to a VIL module this
// package will accept.
func lowered(t *testing.T, path string) *vil.Module {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	f := token.NewFile(path, src)
	file, diags := parser.ParseFile(f, 0)
	for _, d := range diags {
		t.Fatalf("parse: %s", d.Print(f))
	}
	info, checks := analyzer.Check([]*ast.File{file})
	for _, d := range checks {
		t.Fatalf("check: %s", d.Print(f))
	}
	m, gens := gen.File("t", file, info)
	for _, d := range gens {
		t.Fatalf("gen: %s", d.Print(f))
	}
	if err := pass.Mandatory(m); err != nil {
		t.Fatalf("mandatory: %v", err)
	}
	if err := pass.LowerOwnership(m); err != nil {
		t.Fatal(err)
	}
	return m
}

func vir(t *testing.T, path string) *ir.Module {
	t.Helper()
	out, err := Module(lowered(t, path), target)
	if err != nil {
		t.Fatal(err)
	}
	if err := irverify.Module(out); err != nil {
		t.Fatalf("%v\n\n%s", err, dump(t, out))
	}
	return out
}

func dump(t *testing.T, m *ir.Module) string {
	t.Helper()
	var b strings.Builder
	if err := irtext.Print(&b, m); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// body is the module's text from the first function on, so that a
// golden test is about the program and not about the target's layout
// block.
func body(m *ir.Module) string {
	var b strings.Builder
	if err := irtext.Print(&b, m); err != nil {
		return err.Error()
	}
	s := b.String()
	if i := strings.Index(s, "internal func"); i >= 0 {
		return s[i:]
	}
	return s
}
