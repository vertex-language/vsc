// Package core is the built-in module: what every program can see
// without importing anything.
//
// It is two things about the same declarations, from the two ends
// they are needed at.
//
// For the front end it is source. core.swift declares the operators
// on the primitive types, and the analyzer checks it the way it
// checks any other file — so `1 + 2` resolves to a declaration, and a
// call to one is a call rather than a rule buried in the checker.
//
// For the back end it is layout and implementation. An operator's
// declaration has no body, because its body is a machine instruction:
// Int's `+` is an add, and Layout and Lower below say which. Swift's
// own `+` is the same arrangement written differently — a
// @_transparent wrapper the first mandatory pass inlines away,
// leaving the builtin behind.
//
// # What is not here yet
//
// The primitive types themselves. `Int` is a types.Basic in the
// universe rather than a struct declared here, which is where Swift
// puts it. Declaring it would mean the checker resolving `Int.max`
// and `1.description` the ordinary way, and it would mean this
// package growing a body for each of them; both wait on a reason.
// Layout is the part the IR needs meanwhile, and it says what Swift's
// declaration says: an Int is a Builtin.Int64 in a field named
// _value.
//
// Precedence is the other absence. The groups and the operator-to-
// group table are built into the analyzer rather than declared here,
// which is a second source of truth for something Swift keeps in one
// place. Moving them is a change to a package that is finished and
// working, and it is worth doing when core declares the types too.
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
