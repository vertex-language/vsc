package lower

import (
	"strings"

	"github.com/vertex-language/ir"
)

// builtin translates a Builtin.* call. The name carries both the
// operation and the width it happens at -- sadd_with_overflow_Int64,
// cmp_slt_Int32, fcmp_ole_FPIEEE64 -- which is why the width is read
// from the name here rather than from the operand's type.
//
// A with_overflow builtin produces two registers, the result and the
// overflow bit, and takes a third operand Swift uses to say whether
// the overflow should be reported. VIR splits the arithmetic from the
// test, so both are emitted and the caller's tuple_extract picks one.
func (c *fn) builtin(name string, args []ir.Value) ([]ir.Value, error) {
	verb, width, ok := splitBuiltin(name)
	if !ok {
		return nil, c.fail(ErrBuiltin, "builtin", name)
	}
	r, ok := builtinRepr(width)
	if !ok {
		return nil, c.fail(ErrBuiltin, "builtin", name)
	}
	switch r.reg {
	case ir.TypeI1:
		return c.boolBuiltin(name, verb, args)
	case ir.TypeI32, ir.TypeI64:
		return c.intBuiltin(name, verb, r, args)
	case ir.TypeF32, ir.TypeF64:
		return c.floatBuiltin(name, verb, r, args)
	}
	return nil, c.fail(ErrBuiltin, "builtin", name)
}

// splitBuiltin separates a builtin's verb from the type it names. The
// type is the last underscore-separated word, and every builtin this
// compiler emits carries one.
func splitBuiltin(name string) (verb, width string, ok bool) {
	i := strings.LastIndexByte(name, '_')
	if i < 0 {
		return "", "", false
	}
	return name[:i], name[i+1:], true
}

func (c *fn) boolBuiltin(name, verb string, args []ir.Value) ([]ir.Value, error) {
	ns := c.b.I1
	a, aok := args[0].(ir.I1)
	if !aok {
		return nil, c.fail(ErrBuiltin, "builtin", name+": operand is not an i1")
	}
	if verb == "int_not" || verb == "not" {
		return []ir.Value{ns.Not(a)}, nil
	}
	b, bok := args[1].(ir.I1)
	if !bok {
		return nil, c.fail(ErrBuiltin, "builtin", name+": operand is not an i1")
	}
	switch verb {
	case "and":
		return []ir.Value{ns.And(a, b)}, nil
	case "or":
		return []ir.Value{ns.Or(a, b)}, nil
	case "xor":
		return []ir.Value{ns.Xor(a, b)}, nil
	case "cmp_eq":
		return []ir.Value{ns.Xor(ns.Xor(a, b), ns.Const(true))}, nil
	case "cmp_ne":
		return []ir.Value{ns.Xor(a, b)}, nil
	}
	return nil, c.fail(ErrBuiltin, "builtin", name)
}
