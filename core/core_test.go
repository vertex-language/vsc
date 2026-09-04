package core

import (
	"strings"
	"testing"

	"github.com/vertex-language/vsc/types"
)

// TestSourceParses is the one thing this package must never get
// wrong. Everything downstream reads these declarations, and a
// compiler whose own built-in module does not parse has nothing to
// say about anyone else's program.
func TestSourceParses(t *testing.T) {
	file, unit, diags := Files()
	if file == nil {
		t.Fatal("core.swift did not parse at all")
	}
	for _, d := range diags {
		t.Errorf("core.swift: %s", d.Print(unit))
	}
	if len(file.Stmts) == 0 {
		t.Error("core.swift declares nothing")
	}
}

// TestLayout holds the primitives to what they are made of. These are
// the numbers `struct_extract %0, #Int._value` depends on, and they
// are Swift's.
func TestLayout(t *testing.T) {
	cases := []struct {
		typ            types.Type
		field, machine string
	}{
		{types.Typ[types.Bool], "_value", "Int1"},
		{types.Typ[types.Int8], "_value", "Int8"},
		{types.Typ[types.Int16], "_value", "Int16"},
		{types.Typ[types.Int32], "_value", "Int32"},
		{types.Typ[types.Int], "_value", "Int64"},
		{types.Typ[types.Int64], "_value", "Int64"},
		{types.Typ[types.UInt], "_value", "Int64"},
		{types.Typ[types.Float], "_value", "FPIEEE32"},
		{types.Typ[types.Double], "_value", "FPIEEE64"},
	}
	for _, c := range cases {
		field, machine, ok := Layout(c.typ)
		if !ok || field != c.field || machine != c.machine {
			t.Errorf("Layout(%s) = %q, %q, %v; want %q, %q, true",
				c.typ, field, machine, ok, c.field, c.machine)
		}
	}
	// A type with no machine representation says so rather than
	// guessing one.
	if _, _, ok := Layout(types.Typ[types.String]); ok {
		t.Error("String has no single machine word")
	}
}

// TestLower holds the operator table to the instruction names swiftc
// prints. Every one of these was read back from `swiftc -emit-sil`
// rather than remembered: it is the table both compilers have to
// agree on for a lowered function to be comparable at all.
func TestLower(t *testing.T) {
	cases := []struct {
		op        string
		typ       types.Type
		name      string
		overflows bool
		result    string
	}{
		{"+", types.Typ[types.Int], "sadd_with_overflow_Int64", true, "Int64"},
		{"-", types.Typ[types.Int], "ssub_with_overflow_Int64", true, "Int64"},
		{"*", types.Typ[types.Int], "smul_with_overflow_Int64", true, "Int64"},
		{"/", types.Typ[types.Int], "sdiv_Int64", false, "Int64"},
		{"%", types.Typ[types.Int], "srem_Int64", false, "Int64"},
		{"+", types.Typ[types.UInt], "uadd_with_overflow_Int64", true, "Int64"},
		{"/", types.Typ[types.UInt], "udiv_Int64", false, "Int64"},
		{"==", types.Typ[types.Int], "cmp_eq_Int64", false, "Int1"},
		{"!=", types.Typ[types.Int], "cmp_ne_Int64", false, "Int1"},
		{"<", types.Typ[types.Int], "cmp_slt_Int64", false, "Int1"},
		{">=", types.Typ[types.Int], "cmp_sge_Int64", false, "Int1"},
		{"<", types.Typ[types.UInt], "cmp_ult_Int64", false, "Int1"},
		{"+", types.Typ[types.Double], "fadd_FPIEEE64", false, "FPIEEE64"},
		{"*", types.Typ[types.Double], "fmul_FPIEEE64", false, "FPIEEE64"},
		{"<", types.Typ[types.Double], "fcmp_olt_FPIEEE64", false, "Int1"},
		{"<=", types.Typ[types.Double], "fcmp_ole_FPIEEE64", false, "Int1"},
		{"&&", types.Typ[types.Bool], "and_Int1", false, "Int1"},
		{"!=", types.Typ[types.Bool], "xor_Int1", false, "Int1"},
		{"<<", types.Typ[types.Int32], "shl_Int32", false, "Int32"},
		{">>", types.Typ[types.Int32], "ashr_Int32", false, "Int32"},
	}
	for _, c := range cases {
		got, ok := Lower(c.op, c.typ)
		if !ok {
			t.Errorf("Lower(%q, %s) found nothing", c.op, c.typ)
			continue
		}
		if got.Name != c.name || got.Overflows != c.overflows || got.Result != c.result {
			t.Errorf("Lower(%q, %s) = %+v; want %s overflows=%v result=%s",
				c.op, c.typ, got, c.name, c.overflows, c.result)
		}
	}

	// An operator with no machine instruction behind it says so.
	if _, ok := Lower("+", types.Typ[types.String]); ok {
		t.Error("String concatenation is not one instruction")
	}
	if _, ok := Lower("???", types.Typ[types.Int]); ok {
		t.Error("an operator nothing implements should not resolve")
	}
}

// TestDeclaresTheOperators is a check on the source rather than the
// tables: every operator the lowering table knows how to implement
// should be declared, or a program that writes it will not check.
func TestDeclaresTheOperators(t *testing.T) {
	src := Source()
	for _, op := range []string{"+", "-", "*", "/", "%", "==", "!=", "<", "<=", ">", ">="} {
		if !strings.Contains(src, "func "+op+" (lhs: Int, rhs: Int)") {
			t.Errorf("core.swift does not declare %q on Int", op)
		}
	}
}
