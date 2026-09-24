package lower

import "github.com/vertex-language/ir"

// Integer builtin lowerings for 64-bit and 32-bit register widths.

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
	// The high word of a double-width product: multipliedFullWidth.
	case "int_smulhi":
		return []ir.Value{ns.SMulHi(a, b)}, nil
	case "int_umulhi":
		return []ir.Value{ns.UMulHi(a, b)}, nil
	// Arithmetic with overflow reporting.
	case "sadd_with_overflow":
		return []ir.Value{ns.Add(a, b), ns.SAddO(a, b)}, nil
	case "uadd_with_overflow":
		return []ir.Value{ns.Add(a, b), ns.UAddO(a, b)}, nil
	case "ssub_with_overflow":
		return []ir.Value{ns.Sub(a, b), ns.SSubO(a, b)}, nil
	case "usub_with_overflow":
		// Unsigned subtraction overflows on borrow (a < b).
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
	// Greater-than variants swap operands for less-than comparisons.
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

	// checked clamps narrow integer arithmetic to the declared width and detects overflow.
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

	// Bitwise operations preserve range; only shift requires narrowing.
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

// narrow clamps an i32 value to the given bit width (sign-extending or zero-extending).
func (c *fn) narrow(v ir.I32, width uint, signed bool) ir.I32 {
	ns := c.b.I32
	if !signed {
		return ns.And(v, ns.Const(int64(1)<<width-1))
	}
	shift := ns.Const(int64(32 - width))
	return ns.SShr(ns.Shl(v, shift), shift)
}
