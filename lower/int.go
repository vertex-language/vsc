package lower

import "github.com/vertex-language/ir"

// The integer builtins. There are two nearly identical bodies below
// because VIR has one Go type per register width on purpose: a verb
// the spec declines to give a width is a compile error here rather
// than a refusal at run time. The duplication is what that buys.

func (c *fn) intBuiltin(name, verb string, r repr, args []ir.Value) ([]ir.Value, error) {
	if r.reg == ir.TypeI64 {
		return c.int64Builtin(name, verb, args)
	}
	return c.int32Builtin(name, verb, r, args)
}

func (c *fn) int64Builtin(name, verb string, args []ir.Value) ([]ir.Value, error) {
	if len(args) < 2 {
		return nil, c.fail(ErrBuiltin, "builtin", name+": too few operands")
	}
	a, aok := args[0].(ir.I64)
	b, bok := args[1].(ir.I64)
	if !aok || !bok {
		return nil, c.fail(ErrBuiltin, "builtin", name+": operand is not an i64")
	}
	ns := c.b.I64

	switch verb {
	// Arithmetic that reports. VIR splits the value from the test, so
	// both come back and tuple_extract picks.
	case "sadd_with_overflow":
		return []ir.Value{ns.Add(a, b), ns.SAddO(a, b)}, nil
	case "uadd_with_overflow":
		return []ir.Value{ns.Add(a, b), ns.UAddO(a, b)}, nil
	case "ssub_with_overflow":
		return []ir.Value{ns.Sub(a, b), ns.SSubO(a, b)}, nil
	case "usub_with_overflow":
		// An unsigned subtraction overflows exactly when it borrows,
		// which is a < b. §L gives no usubo verb because there is
		// nothing for it to do that a comparison does not.
		return []ir.Value{ns.Sub(a, b), ns.ULt(a, b)}, nil
	case "smul_with_overflow":
		return []ir.Value{ns.Mul(a, b), ns.SMulO(a, b)}, nil
	case "umul_with_overflow":
		return []ir.Value{ns.Mul(a, b), ns.UMulO(a, b)}, nil

	case "sdiv":
		return []ir.Value{ns.SDiv(a, b)}, nil
	case "udiv":
		return []ir.Value{ns.UDiv(a, b)}, nil
	case "srem":
		return []ir.Value{ns.SRem(a, b)}, nil
	case "urem":
		return []ir.Value{ns.URem(a, b)}, nil

	case "and":
		return []ir.Value{ns.And(a, b)}, nil
	case "or":
		return []ir.Value{ns.Or(a, b)}, nil
	case "xor":
		return []ir.Value{ns.Xor(a, b)}, nil
	case "shl":
		return []ir.Value{ns.Shl(a, b)}, nil
	case "ashr":
		return []ir.Value{ns.SShr(a, b)}, nil
	case "lshr":
		return []ir.Value{ns.UShr(a, b)}, nil

	case "cmp_eq":
		return []ir.Value{ns.Eq(a, b)}, nil
	case "cmp_ne":
		return []ir.Value{ns.Ne(a, b)}, nil
	// There is no greater-than. Every target has one comparison and a
	// choice of which way round to read it, so §L gives lt and le and
	// the frontend swaps the operands.
	case "cmp_slt":
		return []ir.Value{ns.SLt(a, b)}, nil
	case "cmp_sle":
		return []ir.Value{ns.SLe(a, b)}, nil
	case "cmp_sgt":
		return []ir.Value{ns.SLt(b, a)}, nil
	case "cmp_sge":
		return []ir.Value{ns.SLe(b, a)}, nil
	case "cmp_ult":
		return []ir.Value{ns.ULt(a, b)}, nil
	case "cmp_ule":
		return []ir.Value{ns.ULe(a, b)}, nil
	case "cmp_ugt":
		return []ir.Value{ns.ULt(b, a)}, nil
	case "cmp_uge":
		return []ir.Value{ns.ULe(b, a)}, nil
	}
	return nil, c.fail(ErrBuiltin, "builtin", name)
}

func (c *fn) int32Builtin(name, verb string, r repr, args []ir.Value) ([]ir.Value, error) {
	if len(args) < 2 {
		return nil, c.fail(ErrBuiltin, "builtin", name+": too few operands")
	}
	a, aok := args[0].(ir.I32)
	b, bok := args[1].(ir.I32)
	if !aok || !bok {
		return nil, c.fail(ErrBuiltin, "builtin", name+": operand is not an i32")
	}
	ns := c.b.I32
	signed := verb[0] == 's' || verb == "ashr" || (len(verb) > 4 && verb[:4] == "cmp_" && verb[4] == 's')

	// checked emits an operation whose result must be brought back
	// into the declared width, and reports overflow as the fact that
	// bringing it back changed it. At the full width of the register
	// the arithmetic verbs answer for themselves.
	checked := func(raw ir.I32, wide func() ir.I1) []ir.Value {
		if !r.narrow() {
			return []ir.Value{raw, wide()}
		}
		fit := c.narrow(raw, r.width, signed)
		return []ir.Value{fit, ns.Ne(raw, fit)}
	}
	plain := func(raw ir.I32) []ir.Value {
		if !r.narrow() {
			return []ir.Value{raw}
		}
		return []ir.Value{c.narrow(raw, r.width, signed)}
	}

	switch verb {
	case "sadd_with_overflow":
		return checked(ns.Add(a, b), func() ir.I1 { return ns.SAddO(a, b) }), nil
	case "uadd_with_overflow":
		return checked(ns.Add(a, b), func() ir.I1 { return ns.UAddO(a, b) }), nil
	case "ssub_with_overflow":
		return checked(ns.Sub(a, b), func() ir.I1 { return ns.SSubO(a, b) }), nil
	case "usub_with_overflow":
		return checked(ns.Sub(a, b), func() ir.I1 { return ns.ULt(a, b) }), nil
	case "smul_with_overflow":
		return checked(ns.Mul(a, b), func() ir.I1 { return ns.SMulO(a, b) }), nil
	case "umul_with_overflow":
		return checked(ns.Mul(a, b), func() ir.I1 { return ns.UMulO(a, b) }), nil

	case "sdiv":
		return plain(ns.SDiv(a, b)), nil
	case "udiv":
		return plain(ns.UDiv(a, b)), nil
	case "srem":
		return plain(ns.SRem(a, b)), nil
	case "urem":
		return plain(ns.URem(a, b)), nil

	// Bitwise operations on operands that are already in range give a
	// result in range, so only the shift needs bringing back.
	case "and":
		return []ir.Value{ns.And(a, b)}, nil
	case "or":
		return []ir.Value{ns.Or(a, b)}, nil
	case "xor":
		return []ir.Value{ns.Xor(a, b)}, nil
	case "shl":
		return plain(ns.Shl(a, b)), nil
	case "ashr":
		return []ir.Value{ns.SShr(a, b)}, nil
	case "lshr":
		return []ir.Value{ns.UShr(a, b)}, nil

	case "cmp_eq":
		return []ir.Value{ns.Eq(a, b)}, nil
	case "cmp_ne":
		return []ir.Value{ns.Ne(a, b)}, nil
	case "cmp_slt":
		return []ir.Value{ns.SLt(a, b)}, nil
	case "cmp_sle":
		return []ir.Value{ns.SLe(a, b)}, nil
	case "cmp_sgt":
		return []ir.Value{ns.SLt(b, a)}, nil
	case "cmp_sge":
		return []ir.Value{ns.SLe(b, a)}, nil
	case "cmp_ult":
		return []ir.Value{ns.ULt(a, b)}, nil
	case "cmp_ule":
		return []ir.Value{ns.ULe(a, b)}, nil
	case "cmp_ugt":
		return []ir.Value{ns.ULt(b, a)}, nil
	case "cmp_uge":
		return []ir.Value{ns.ULe(b, a)}, nil
	}
	return nil, c.fail(ErrBuiltin, "builtin", name)
}

// narrow brings a value back into the range of a type held in a wider
// register. Int8 and Int16 live in i32 because §2 makes those widths
// storage-only, and the invariant this package keeps is that such a
// register always holds a value already extended from its own width.
func (c *fn) narrow(v ir.I32, width uint, signed bool) ir.I32 {
	ns := c.b.I32
	if !signed {
		return ns.And(v, ns.Const(int64(1)<<width-1))
	}
	shift := ns.Const(int64(32 - width))
	return ns.SShr(ns.Shl(v, shift), shift)
}
