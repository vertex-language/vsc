package core

import (
	_ "embed"
	"sync"

	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/parser"
	"github.com/vertex-language/vsc/stdlib"
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

// Files parses and returns the embedded built-in module, parsing it once and caching the result.
func Files() (*ast.File, *token.File, []token.Diagnostic) {
	once.Do(func() {
		unit = token.NewFile("core.swift", []byte(source))
		file, errs = parser.ParseFile(unit, 0)
	})
	return file, unit, errs
}

// Layout returns the underlying primitive field name and machine type representation for t.
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

// A Builtin describes a low-level machine instruction used to implement an operator.
type Builtin struct {
	// Name is the SIL instruction name (e.g. "sadd_with_overflow_Int64").
	Name string
	// Overflows indicates the instruction returns an overflow flag to trap on.
	Overflows bool
	// Result is the resulting machine type.
	Result string
}

// An Extern describes a runtime function implementing an operator.
type Extern struct {
	// Symbol is the runtime function symbol.
	Symbol string
	// Negate indicates whether the result should be negated.
	Negate bool
	// Owned indicates whether the result value requires cleanup.
	Owned bool
}

// Wrapping reports whether op is a wrapping arithmetic operator (&+, &-, &*).
func Wrapping(op string) bool {
	switch op {
	case "&+", "&-", "&*":
		return true
	}
	return false
}

// LowerExtern returns the runtime function implementing op for the given operand type.
func LowerExtern(op string, operand types.Type) (Extern, bool) {
	b, ok := underlyingBasic(operand)
	if !ok || b.Kind() != types.String {
		return Extern{}, false
	}
	switch op {
	case "+":
		return Extern{Symbol: stdlib.StringConcat, Owned: true}, true
	case "==":
		return Extern{Symbol: stdlib.StringEqual}, true
	case "!=":
		return Extern{Symbol: stdlib.StringEqual, Negate: true}, true
	case "<":
		return Extern{Symbol: stdlib.StringLess}, true
	}
	return Extern{}, false
}

// A Member is a property of a primitive type whose implementation is
// a standard-library call.
//
// String and Array have no fields to read: what a String is made of
// is Swift's business, and what an Array holds is behind a reference
// this compiler never looks inside. So `s.count` and `a.count` are
// A Member describes a runtime function implementing a property read.
type Member struct {
	// Symbol is the runtime function that reads it.
	Symbol string
	// Result is what it hands back.
	Result types.Type
	// Element is the generic element type metadata required by the call, if any.
	Element types.Type
}

// LowerMember returns the runtime function reading a property of a primitive type.
func LowerMember(t types.Type, name string) (Member, bool) {
	if a, ok := t.Underlying().(*types.Array); ok {
		switch name {
		case "count":
			return Member{
				Symbol:  stdlib.ArrayCount,
				Result:  types.Typ[types.Int],
				Element: a.Elem,
			}, true
		case "isEmpty":
			return Member{
				Symbol:  stdlib.ArrayIsEmpty,
				Result:  types.Typ[types.Bool],
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
		return Member{Symbol: stdlib.StringCount, Result: types.Typ[types.Int]}, true
	case "isEmpty":
		return Member{Symbol: stdlib.StringIsEmpty, Result: types.Typ[types.Bool]}, true
	case "description":
		return Member{Symbol: stdlib.StringDescription, Result: types.Typ[types.String]}, true
	case "debugDescription":
		return Member{Symbol: stdlib.StringDebugDescription, Result: types.Typ[types.String]}, true
	}
	return Member{}, false
}

// LowerStaticMember returns the runtime function reading a static property of a built-in type.
func LowerStaticMember(t types.Type, name string) (Member, bool) {
	e, ok := t.Underlying().(*types.Enum)
	if !ok || e.Name != "CommandLine" {
		return Member{}, false
	}
	switch name {
	case "arguments":
		return Member{
			Symbol: stdlib.CommandLineArguments,
			Result: &types.Array{Elem: types.Typ[types.String]},
		}, true
	}
	return Member{}, false
}

// LowerTask returns the runtime function a member of the built-in Task
// type is: its initializer starts a task, and its static functions wait.
// Task has no body in core.swift; each of these is a call into
// stdlib/runtime/task.cpp.
func LowerTask(name string) (string, bool) {
	switch name {
	case "init":
		return stdlib.TaskStart, true
	case "value":
		return stdlib.TaskJoin, true
	case "sleep":
		return stdlib.TaskSleep, true
	case "yield":
		return stdlib.TaskYield, true
	}
	return "", false
}

// Subscript returns the runtime function implementing subscript access for type t.
func Subscript(t types.Type) (Member, bool) {
	a, ok := t.Underlying().(*types.Array)
	if !ok {
		return Member{}, false
	}
	return Member{Symbol: stdlib.ArrayElement, Result: a.Elem, Element: a.Elem}, true
}

// Lower returns the built-in machine instruction implementing an operator on a type.
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
	// Wrapping operators (&+, &-, &*) map to the same base instruction.
	switch op {
	case "&+":
		op = "+"
	case "&-":
		op = "-"
	case "&*":
		op = "*"
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
	// Shift operators (<<, >>) require branching logic and are handled in SILGen.
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

// floatBuiltin returns the ordered comparison or arithmetic instruction for float operations.
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

// A Step represents one built-in instruction in the expansion of a prefix operator.
type Step struct {
	// Name is the builtin, and Result the machine type it produces.
	Name, Result string
	// Overflows indicates the builtin returns an overflow flag; Reports says the flag is checked.
	Overflows, Reports bool
	// HasConst indicates whether the expansion supplies a second constant operand.
	HasConst  bool
	Const     int64
	ConstLeft bool
}

// LowerPrefix returns the sequence of builtins a prefix operator expands to.
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
