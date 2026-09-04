package lower

import "github.com/vertex-language/ir"

// The floating-point builtins. Swift's names carry the ordering rule
// in them -- oeq is ordered and equal, une is unordered or unequal --
// and VIR's float comparisons are IEEE, which is the same thing said
// once. So oeq is Eq, une is Ne, and a NaN answers false to every
// ordered question either way.

func (c *fn) floatBuiltin(name, verb string, r repr, args []ir.Value) ([]ir.Value, error) {
	if r.reg == ir.TypeF64 {
		return c.float64Builtin(name, verb, args)
	}
	return c.float32Builtin(name, verb, args)
}

func (c *fn) float64Builtin(name, verb string, args []ir.Value) ([]ir.Value, error) {
	ns := c.b.F64
	a, aok := args[0].(ir.F64)
	if !aok {
		return nil, c.fail(ErrBuiltin, "builtin", name+": operand is not an f64")
	}
	if verb == "fneg" {
		return []ir.Value{ns.Neg(a)}, nil
	}
	if len(args) < 2 {
		return nil, c.fail(ErrBuiltin, "builtin", name+": too few operands")
	}
	b, bok := args[1].(ir.F64)
	if !bok {
		return nil, c.fail(ErrBuiltin, "builtin", name+": operand is not an f64")
	}
	switch verb {
	case "fadd":
		return []ir.Value{ns.Add(a, b)}, nil
	case "fsub":
		return []ir.Value{ns.Sub(a, b)}, nil
	case "fmul":
		return []ir.Value{ns.Mul(a, b)}, nil
	case "fdiv":
		return []ir.Value{ns.Div(a, b)}, nil
	case "fcmp_oeq":
		return []ir.Value{ns.Eq(a, b)}, nil
	case "fcmp_une":
		return []ir.Value{ns.Ne(a, b)}, nil
	case "fcmp_olt":
		return []ir.Value{ns.Lt(a, b)}, nil
	case "fcmp_ole":
		return []ir.Value{ns.Le(a, b)}, nil
	case "fcmp_ogt":
		return []ir.Value{ns.Lt(b, a)}, nil
	case "fcmp_oge":
		return []ir.Value{ns.Le(b, a)}, nil
	}
	return nil, c.fail(ErrBuiltin, "builtin", name)
}

func (c *fn) float32Builtin(name, verb string, args []ir.Value) ([]ir.Value, error) {
	ns := c.b.F32
	a, aok := args[0].(ir.F32)
	if !aok {
		return nil, c.fail(ErrBuiltin, "builtin", name+": operand is not an f32")
	}
	if verb == "fneg" {
		return []ir.Value{ns.Neg(a)}, nil
	}
	if len(args) < 2 {
		return nil, c.fail(ErrBuiltin, "builtin", name+": too few operands")
	}
	b, bok := args[1].(ir.F32)
	if !bok {
		return nil, c.fail(ErrBuiltin, "builtin", name+": operand is not an f32")
	}
	switch verb {
	case "fadd":
		return []ir.Value{ns.Add(a, b)}, nil
	case "fsub":
		return []ir.Value{ns.Sub(a, b)}, nil
	case "fmul":
		return []ir.Value{ns.Mul(a, b)}, nil
	case "fdiv":
		return []ir.Value{ns.Div(a, b)}, nil
	case "fcmp_oeq":
		return []ir.Value{ns.Eq(a, b)}, nil
	case "fcmp_une":
		return []ir.Value{ns.Ne(a, b)}, nil
	case "fcmp_olt":
		return []ir.Value{ns.Lt(a, b)}, nil
	case "fcmp_ole":
		return []ir.Value{ns.Le(a, b)}, nil
	case "fcmp_ogt":
		return []ir.Value{ns.Lt(b, a)}, nil
	case "fcmp_oge":
		return []ir.Value{ns.Le(b, a)}, nil
	}
	return nil, c.fail(ErrBuiltin, "builtin", name)
}
