package lower

import "github.com/vertex-language/ir"

// The one-operand instructions the core declares with @_builtin: the bit
// counts and byte swap on integers, and the roundings, square root and
// absolute value on floats. Each is one IR instruction, as Swift's
// Builtin.int_ctpop_Int64 and Builtin.int_sqrt_FPIEEE64 are one LLVM
// intrinsic.

// intUnary lowers a one-operand integer builtin, and reports whether name
// was one. An Int8 or Int16 is held in a 32-bit register, so its bits are
// counted in that width only: the register's upper bits are not the value's.
func (c *fn) intUnary(name, verb string, r repr, args []ir.Value) ([]ir.Value, bool, error) {
	switch verb {
	case "int_ctpop", "int_ctlz", "int_cttz", "int_bswap":
	default:
		return nil, false, nil
	}
	if len(args) < 1 {
		return nil, true, c.fail(ErrBuiltin, "builtin", name+": too few operands")
	}
	if r.reg == ir.TypeI64 {
		a, ok := args[0].(ir.I64)
		if !ok {
			return nil, true, c.fail(ErrBuiltin, "builtin", name+": operand is not an i64")
		}
		ns := c.b.I64
		switch verb {
		case "int_ctpop":
			return []ir.Value{ns.Popcnt(a)}, true, nil
		case "int_ctlz":
			return []ir.Value{ns.Clz(a)}, true, nil
		case "int_cttz":
			return []ir.Value{ns.Ctz(a)}, true, nil
		default:
			return []ir.Value{ns.Bswap(a)}, true, nil
		}
	}
	a, ok := args[0].(ir.I32)
	if !ok {
		return nil, true, c.fail(ErrBuiltin, "builtin", name+": operand is not an i32")
	}
	ns := c.b.I32
	if !r.narrow() {
		switch verb {
		case "int_ctpop":
			return []ir.Value{ns.Popcnt(a)}, true, nil
		case "int_ctlz":
			return []ir.Value{ns.Clz(a)}, true, nil
		case "int_cttz":
			return []ir.Value{ns.Ctz(a)}, true, nil
		default:
			return []ir.Value{ns.Bswap(a)}, true, nil
		}
	}
	w := int64(r.width)
	bits := c.narrow(a, r.width, false) // the value's own bits, zero above them
	switch verb {
	case "int_ctpop":
		return []ir.Value{ns.Popcnt(bits)}, true, nil
	case "int_ctlz":
		// Counted in 32 bits, of which the top 32-w are not the value's.
		return []ir.Value{ns.Sub(ns.Clz(bits), ns.Const(32-w))}, true, nil
	case "int_cttz":
		// A bit just above the value stops the count at w for zero.
		return []ir.Value{ns.Ctz(ns.Or(bits, ns.Const(int64(1)<<w)))}, true, nil
	default:
		if w == 8 {
			return []ir.Value{bits}, true, nil
		}
		// The two bytes, swapped, are the top half of the 32-bit swap.
		return []ir.Value{ns.UShr(ns.Bswap(bits), ns.Const(32-w))}, true, nil
	}
}

// floatUnary lowers a one-operand or sign-copying float builtin, and
// reports whether name was one. int_rint rounds to nearest, ties to even.
func (c *fn) floatUnary(name, verb string, r repr, args []ir.Value) ([]ir.Value, bool, error) {
	switch verb {
	case "int_sqrt", "int_floor", "int_ceil", "int_trunc", "int_rint", "int_fabs", "int_copysign":
	default:
		return nil, false, nil
	}
	want := 1
	if verb == "int_copysign" {
		want = 2
	}
	if len(args) < want {
		return nil, true, c.fail(ErrBuiltin, "builtin", name+": too few operands")
	}
	switch r.reg {
	case ir.TypeF64:
		return floatUnaryIn[ir.F64](c, c.b.F64, name, verb, args)
	case ir.TypeF16:
		return floatUnaryIn[ir.F16](c, c.b.F16(), name, verb, args)
	case ir.TypeBF16:
		return floatUnaryIn[ir.BF16](c, c.b.BF16(), name, verb, args)
	}
	return floatUnaryIn[ir.F32](c, c.b.F32, name, verb, args)
}
