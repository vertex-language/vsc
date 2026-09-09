package core

import (
	_ "embed"
	"sync"

	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/parser"
	"github.com/vertex-language/vsc/token"
	"github.com/vertex-language/vsc/types"
)

//go:embed core.swift
var source string

// Source is the built-in module's text.
func Source() string { return source }

var (
	once sync.Once
	file *ast.File
	unit *token.File
	errs []token.Diagnostic
)

// Files parses the built-in module and returns it, with the
// token.File its positions resolve through and whatever parsing it
// had to say — which should be nothing, and there is a test.
//
// Parsed once and shared: it is the same text every time, and a
// compilation that read it twice would pay twice for one answer.
func Files() (*ast.File, *token.File, []token.Diagnostic) {
	once.Do(func() {
		unit = token.NewFile("core.swift", []byte(source))
		file, errs = parser.ParseFile(unit, 0)
	})
	return file, unit, errs
}

// Layout says how a value of a primitive type is represented: the
// field that holds it, and the builtin type that field is.
//
// `struct_extract %0, #Int._value` is the instruction this exists
// for. An Int is one machine word inside a struct, and the IR has to
// reach through the struct to the word.
func Layout(t types.Type) (field, machine string, ok bool) {
	b, isBasic := underlyingBasic(t)
	if !isBasic {
		return "", "", false
	}
	switch b.Kind() {
	case types.Bool:
		return "_value", "Int1", true
	case types.Int8, types.UInt8:
		return "_value", "Int8", true
	case types.Int16, types.UInt16:
		return "_value", "Int16", true
	case types.Int32, types.UInt32:
		return "_value", "Int32", true
	case types.Int, types.Int64, types.UInt, types.UInt64:
		return "_value", "Int64", true
	case types.Float:
		return "_value", "FPIEEE32", true
	case types.Double:
		return "_value", "FPIEEE64", true
	}
	return "", "", false
}

// A Builtin is the machine instruction behind an operator, and what
// the caller has to know about it.
type Builtin struct {
	// Name is the instruction, spelled as SIL spells it:
	// "sadd_with_overflow_Int64", "cmp_slt_Int64", "fmul_FPIEEE64".
	Name string
	// Overflows says it returns a value and an overflow bit, which
	// the caller must trap on. Swift's checked `+` is this and a
	// cond_fail.
	Overflows bool
	// Result is the builtin type it produces: the operand's own for
	// arithmetic, Int1 for a comparison.
	Result string
}

// An Extern is a standard-library function behind an operator whose
// implementation is not a machine instruction.
//
// String's operators are the ones that are not. Concatenation
// allocates, and comparison walks two sequences of bytes that may not
// even be encoded the same way; neither is an instruction, and both
// are already written -- in the standard library, at the symbols
// below, which is where swiftc's own code for `a + b` goes.
type Extern struct {
	// Symbol is the mangled name libswiftCore exports.
	Symbol string
	// Negate says the answer is the negation of what the symbol
	// returns. Swift's `!=` on String is the generic one an Equatable
	// extension declares, which would need the metadata and the
	// witness table to call; the negation of `==` is the same answer
	// and is one instruction.
	Negate bool
	// Owned says the result is a value the caller has to let go of.
	Owned bool
}

// LowerExtern says which standard-library function implements an
// operator on a type, or reports that none does.
//
// The symbols are read back from what swiftc emits for the same
// source rather than remembered, the way the builtin table is.
func LowerExtern(op string, operand types.Type) (Extern, bool) {
	b, ok := underlyingBasic(operand)
	if !ok || b.Kind() != types.String {
		return Extern{}, false
	}
	switch op {
	case "+":
		return Extern{Symbol: "$sSS1poiyS2S_SStFZ", Owned: true}, true
	case "==":
		return Extern{Symbol: "$sSS2eeoiySbSS_SStFZ"}, true
	case "!=":
		return Extern{Symbol: "$sSS2eeoiySbSS_SStFZ", Negate: true}, true
	case "<":
		return Extern{Symbol: "$sSS1loiySbSS_SStFZ"}, true
	}
	return Extern{}, false
}

// A Member is a property of a primitive type whose implementation is
// a standard-library call.
//
// String and Array have no fields to read: what a String is made of
// is Swift's business, and what an Array holds is behind a reference
// this compiler never looks inside. So `s.count` and `a.count` are
// calls, at the symbols libswiftCore exports for them.
type Member struct {
	// Symbol is the getter libswiftCore exports.
	Symbol string
	// Result is what it hands back.
	Result types.Type
	// Element is the type whose metadata the call carries, where the
	// getter is a generic one. Array's is; String's is not.
	Element types.Type
}

// LowerMember says which standard-library function reads a property
// of a primitive type, or reports that none does.
//
// Only the ones libswiftCore actually exports. `isEmpty` is not here
// because it is inlinable and has no symbol to call -- `count == 0`
// is the same question with an answer.
func LowerMember(t types.Type, name string) (Member, bool) {
	if a, ok := t.Underlying().(*types.Array); ok {
		switch name {
		case "count":
			return Member{
				Symbol:  "$sSa5countSivg",
				Result:  types.Typ[types.Int],
				Element: a.Elem,
			}, true
		}
		return Member{}, false
	}
	b, ok := underlyingBasic(t)
	if !ok || b.Kind() != types.String {
		return Member{}, false
	}
	switch name {
	case "count":
		return Member{Symbol: "$sSS5countSivg", Result: types.Typ[types.Int]}, true
	}
	return Member{}, false
}

// Subscript is the standard-library getter behind `a[i]`, and reports
// whether the type has one.
//
// Array's is generic and returns its element indirectly:
// `<T> (Int, @guaranteed Array<T>) -> @out T`. That is not a fact
// about how wide an element is -- swiftc passes a four-byte slot in
// x8 for an [Int32] -- it is what a generic result convention is.
func Subscript(t types.Type) (Member, bool) {
	a, ok := t.Underlying().(*types.Array)
	if !ok {
		return Member{}, false
	}
	return Member{Symbol: "$sSayxSicig", Result: a.Elem, Element: a.Elem}, true
}

// Lower says which machine instruction implements an operator on a
// type, or reports that none does.
//
// The names are Swift's, read back from `swiftc -emit-sil` rather
// than remembered: this is the table both compilers must agree on for
// a lowered function to be comparable at all.
func Lower(op string, operand types.Type) (Builtin, bool) {
	b, ok := underlyingBasic(operand)
	if !ok {
		return Builtin{}, false
	}
	_, machine, ok := Layout(operand)
	if !ok {
		return Builtin{}, false
	}

	info := b.Info()
	switch {
	case info&types.IsFloat != 0:
		return floatBuiltin(op, machine)
	case info&types.IsBoolean != 0:
		return boolBuiltin(op)
	case info&types.IsInteger != 0:
		return intBuiltin(op, machine, info&types.IsUnsigned != 0)
	}
	return Builtin{}, false
}

func intBuiltin(op, machine string, unsigned bool) (Builtin, bool) {
	s := "s"
	if unsigned {
		s = "u"
	}
	switch op {
	case "+":
		return Builtin{s + "add_with_overflow_" + machine, true, machine}, true
	case "-":
		return Builtin{s + "sub_with_overflow_" + machine, true, machine}, true
	case "*":
		return Builtin{s + "mul_with_overflow_" + machine, true, machine}, true
	case "/":
		return Builtin{s + "div_" + machine, false, machine}, true
	case "%":
		return Builtin{s + "rem_" + machine, false, machine}, true
	case "&":
		return Builtin{"and_" + machine, false, machine}, true
	case "|":
		return Builtin{"or_" + machine, false, machine}, true
	case "^":
		return Builtin{"xor_" + machine, false, machine}, true
	case "<<":
		return Builtin{"shl_" + machine, false, machine}, true
	case ">>":
		if unsigned {
			return Builtin{"lshr_" + machine, false, machine}, true
		}
		return Builtin{"ashr_" + machine, false, machine}, true
	case "==":
		return Builtin{"cmp_eq_" + machine, false, "Int1"}, true
	case "!=":
		return Builtin{"cmp_ne_" + machine, false, "Int1"}, true
	case "<":
		return Builtin{"cmp_" + s + "lt_" + machine, false, "Int1"}, true
	case "<=":
		return Builtin{"cmp_" + s + "le_" + machine, false, "Int1"}, true
	case ">":
		return Builtin{"cmp_" + s + "gt_" + machine, false, "Int1"}, true
	case ">=":
		return Builtin{"cmp_" + s + "ge_" + machine, false, "Int1"}, true
	}
	return Builtin{}, false
}

// The float comparisons are the ordered ones: an unordered operand —
// a NaN — compares false, which is what the `o` in the name says.
func floatBuiltin(op, machine string) (Builtin, bool) {
	switch op {
	case "+":
		return Builtin{"fadd_" + machine, false, machine}, true
	case "-":
		return Builtin{"fsub_" + machine, false, machine}, true
	case "*":
		return Builtin{"fmul_" + machine, false, machine}, true
	case "/":
		return Builtin{"fdiv_" + machine, false, machine}, true
	case "==":
		return Builtin{"fcmp_oeq_" + machine, false, "Int1"}, true
	case "!=":
		return Builtin{"fcmp_une_" + machine, false, "Int1"}, true
	case "<":
		return Builtin{"fcmp_olt_" + machine, false, "Int1"}, true
	case "<=":
		return Builtin{"fcmp_ole_" + machine, false, "Int1"}, true
	case ">":
		return Builtin{"fcmp_ogt_" + machine, false, "Int1"}, true
	case ">=":
		return Builtin{"fcmp_oge_" + machine, false, "Int1"}, true
	}
	return Builtin{}, false
}

func boolBuiltin(op string) (Builtin, bool) {
	switch op {
	case "&&", "&":
		return Builtin{"and_Int1", false, "Int1"}, true
	case "||", "|":
		return Builtin{"or_Int1", false, "Int1"}, true
	case "==":
		return Builtin{"cmp_eq_Int1", false, "Int1"}, true
	case "!=", "^":
		return Builtin{"xor_Int1", false, "Int1"}, true
	}
	return Builtin{}, false
}

func underlyingBasic(t types.Type) (*types.Basic, bool) {
	if t == nil {
		return nil, false
	}
	b, ok := t.Underlying().(*types.Basic)
	return b, ok
}

// A Step is one builtin in the expansion of a prefix operator.
//
// A prefix operator is not always one instruction. Swift writes `-x`
// as a subtraction from zero and `~x` as `(0 - x) - 1`, both of them
// through the same overflow-reporting builtin that binary subtraction
// uses -- with the reporting turned off for `~`, since inverting the
// bits of the smallest value is not an error.
type Step struct {
	// Name is the builtin, and Result the machine type it produces.
	Name, Result string
	// Overflows says the builtin returns a value and a flag, and
	// Reports says the flag is checked. A step can overflow without
	// reporting: that is Swift's wrapping arithmetic.
	Overflows, Reports bool
	// HasConst says the expansion supplies a second operand. Without
	// one the builtin takes the operand alone, which is how negation
	// of a floating-point value is written.
	HasConst  bool
	Const     int64
	ConstLeft bool
}

// LowerPrefix gives the builtins a prefix operator expands to, in
// order, each taking the result of the one before it. An operator
// that does nothing -- unary plus -- expands to no steps at all,
// which is not a failure.
func LowerPrefix(op string, operand types.Type) ([]Step, bool) {
	b, ok := underlyingBasic(operand)
	if !ok {
		return nil, false
	}
	_, machine, ok := Layout(operand)
	if !ok {
		return nil, false
	}
	if op == "+" {
		return nil, true
	}

	info := b.Info()
	switch {
	case info&types.IsBoolean != 0:
		if op != "!" {
			return nil, false
		}
		// All bits set, and there is one bit.
		return []Step{{Name: "xor_Int1", Result: "Int1", HasConst: true, Const: -1}}, true

	case info&types.IsFloat != 0:
		if op != "-" {
			return nil, false
		}
		return []Step{{Name: "fneg_" + machine, Result: machine}}, true

	case info&types.IsInteger != 0:
		s := "s"
		if info&types.IsUnsigned != 0 {
			s = "u"
		}
		sub := s + "sub_with_overflow_" + machine
		switch op {
		case "-":
			return []Step{{
				Name: sub, Result: machine,
				Overflows: true, Reports: true,
				HasConst: true, Const: 0, ConstLeft: true,
			}}, true
		case "~":
			// Two's complement: ~x is -x - 1, and neither half of it
			// can fail, so neither half reports.
			return []Step{{
				Name: sub, Result: machine,
				Overflows: true,
				HasConst:  true, Const: 0, ConstLeft: true,
			}, {
				Name: sub, Result: machine,
				Overflows: true,
				HasConst:  true, Const: 1,
			}}, true
		}
	}
	return nil, false
}
