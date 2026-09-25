package lower

import "github.com/vertex-language/ir"

// Floating-point builtin instructions: arithmetic, comparisons and the
// one-instruction unary functions, for every float namespace VIR has --
// f32, f64, and the half floats f16 and bf16. Each namespace has the same
// verbs over its own value type, so each is written once, generically.

// floatNS is what every VIR float namespace provides.
type floatNS[V ir.Value] interface {
	Add(a, c V) V
	Sub(a, c V) V
	Mul(a, c V) V
	Div(a, c V) V
	Neg(a V) V
	Abs(a V) V
	Sqrt(a V) V
	Ceil(a V) V
	Floor(a V) V
	Trunc(a V) V
	Nearest(a V) V
	CopySign(a, c V) V
	Eq(a, c V) ir.I1
	Ne(a, c V) ir.I1
	Lt(a, c V) ir.I1
	Le(a, c V) ir.I1
}

func (c *fn) floatBuiltin(name, verb string, r repr, args []ir.Value) ([]ir.Value, error) {
	switch r.reg {
	case ir.TypeF64:
		return floatArith[ir.F64](c, c.b.F64, name, verb, args)
	case ir.TypeF16:
		return floatArith[ir.F16](c, c.b.F16(), name, verb, args)
	case ir.TypeBF16:
		return floatArith[ir.BF16](c, c.b.BF16(), name, verb, args)
	}
	return floatArith[ir.F32](c, c.b.F32, name, verb, args)
}

func floatArith[V ir.Value](c *fn, ns floatNS[V], name, verb string, args []ir.Value) ([]ir.Value, error) {
	a, aok := args[0].(V)
	if !aok {
		return nil, c.fail(ErrBuiltin, "builtin", name+": operand of the wrong float type")
	}
	if verb == "fneg" {
		return []ir.Value{ns.Neg(a)}, nil
	}
	if len(args) < 2 {
		return nil, c.fail(ErrBuiltin, "builtin", name+": too few operands")
	}
	b, bok := args[1].(V)
	if !bok {
		return nil, c.fail(ErrBuiltin, "builtin", name+": operand of the wrong float type")
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

// floatUnaryIn is floatUnary for one namespace.
func floatUnaryIn[V ir.Value](c *fn, ns floatNS[V], name, verb string, args []ir.Value) ([]ir.Value, bool, error) {
	a, ok := args[0].(V)
	if !ok {
		return nil, true, c.fail(ErrBuiltin, "builtin", name+": operand of the wrong float type")
	}
	switch verb {
	case "int_sqrt":
		return []ir.Value{ns.Sqrt(a)}, true, nil
	case "int_floor":
		return []ir.Value{ns.Floor(a)}, true, nil
	case "int_ceil":
		return []ir.Value{ns.Ceil(a)}, true, nil
	case "int_trunc":
		return []ir.Value{ns.Trunc(a)}, true, nil
	case "int_rint":
		return []ir.Value{ns.Nearest(a)}, true, nil
	case "int_fabs":
		return []ir.Value{ns.Abs(a)}, true, nil
	}
	b, ok := args[1].(V)
	if !ok {
		return nil, true, c.fail(ErrBuiltin, "builtin", name+": operand of the wrong float type")
	}
	return []ir.Value{ns.CopySign(a, b)}, true, nil
}
