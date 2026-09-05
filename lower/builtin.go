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
	// A conversion names two types rather than one -- the source in
	// the verb and the destination after it -- so it cannot be read
	// by the "verb plus width" split below.
	if out, ok, err := c.convertBuiltin(name, args); ok {
		return out, err
	}
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

// convertBuiltin translates a width conversion, and reports whether
// the name was one.
//
// Three verbs, and which one gen chose already answers the signedness
// question: sext for a signed source, zext for an unsigned one, trunc
// for a narrowing. What is left here is the register class, and VIR
// has only two integer classes -- i32 and i64. Int8 and Int16 are not
// register types; a value of one lives in an i32 already extended to
// its own signedness, which is why a conversion between any two of
// Int8, Int16 and Int32 moves no bits at all.
//
// It moves no bits safely because gen has already emitted the range
// check: by the time the value reaches here it fits the destination,
// so the register holds what the destination wants.
func (c *fn) convertBuiltin(name string, args []ir.Value) ([]ir.Value, bool, error) {
	verb, src, dst, ok := splitConvert(name)
	if !ok {
		return nil, false, nil
	}
	if len(args) != 1 {
		return nil, true, c.fail(ErrBuiltin, "builtin", name+": a conversion takes one operand")
	}
	from, fok := builtinRepr(src)
	to, tok := builtinRepr(dst)
	if !fok || !tok {
		return nil, true, c.fail(ErrBuiltin, "builtin", name)
	}

	switch {
	case from.reg == to.reg:
		// Same register class: Int8, Int16 and Int32 share one, and
		// the value already fits.
		return []ir.Value{args[0]}, true, nil

	case from.reg == ir.TypeI64 && to.reg == ir.TypeI32:
		a, ok := args[0].(ir.I64)
		if !ok {
			return nil, true, c.fail(ErrBuiltin, "builtin", name+": operand is not an i64")
		}
		return []ir.Value{c.b.I32.WrapI64(a)}, true, nil

	case from.reg == ir.TypeI32 && to.reg == ir.TypeI64:
		a, ok := args[0].(ir.I32)
		if !ok {
			return nil, true, c.fail(ErrBuiltin, "builtin", name+": operand is not an i32")
		}
		if verb == "zextOrBitCast" {
			return []ir.Value{c.b.I64.ZExtI32(a)}, true, nil
		}
		return []ir.Value{c.b.I64.SExtI32(a)}, true, nil
	}
	return nil, true, c.fail(ErrBuiltin, "builtin", name)
}

// splitConvert reads a conversion's verb and its two types.
func splitConvert(name string) (verb, src, dst string, ok bool) {
	for _, v := range [...]string{"truncOrBitCast_", "sextOrBitCast_", "zextOrBitCast_"} {
		if !strings.HasPrefix(name, v) {
			continue
		}
		rest := name[len(v):]
		i := strings.LastIndexByte(rest, '_')
		if i < 0 {
			return "", "", "", false
		}
		return strings.TrimSuffix(v, "_"), rest[:i], rest[i+1:], true
	}
	return "", "", "", false
}
