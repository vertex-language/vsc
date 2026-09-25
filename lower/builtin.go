package lower

import (
	"strings"

	"github.com/vertex-language/ir"
	"github.com/vertex-language/vsc/stdlib"
)

// builtin translates a Builtin.* instruction into VIR operations.
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
	// How many elements an array's storage holds: the word after the
	// header, see stdlib/ABI.md. Read inline so that a subscript is a
	// compare and a load rather than a call.
	if verb == "vertexArrayCount" {
		storage, ok := args[0].(ir.Ptr)
		if len(args) != 1 || !ok || r.reg != ir.TypeI64 {
			return nil, c.fail(ErrBuiltin, "builtin", name+": operand is not an array's storage")
		}
		return []ir.Value{c.b.I64.Load(c.b.Ptr.Add(storage, c.b.I64.Const(stdlib.ArrayCountWord)))}, nil
	}
	switch r.reg {
	case ir.TypeI32, ir.TypeI64:
		if out, ok, err := c.intUnary(name, verb, r, args); ok {
			return out, err
		}
	case ir.TypeF32, ir.TypeF64, ir.TypeF16, ir.TypeBF16:
		if out, ok, err := c.floatUnary(name, verb, r, args); ok {
			return out, err
		}
	}
	switch r.reg {
	case ir.TypeI1:
		return c.boolBuiltin(name, verb, args)
	case ir.TypeI32, ir.TypeI64:
		return c.intBuiltin(name, verb, r, args)
	case ir.TypeF32, ir.TypeF64, ir.TypeF16, ir.TypeBF16:
		return c.floatBuiltin(name, verb, r, args)
	case ir.TypePtr:
		return c.ptrBuiltin(name, verb, args)
	}
	return nil, c.fail(ErrBuiltin, "builtin", name)
}

// ptrBuiltin translates address equality comparisons.
func (c *fn) ptrBuiltin(name, verb string, args []ir.Value) ([]ir.Value, error) {
	// Where an array keeps its elements, past the storage's header: see
	// stdlib/ABI.md. The empty array's storage has that place too.
	if verb == "vertexArrayElements" {
		storage, ok := args[0].(ir.Ptr)
		if len(args) != 1 || !ok {
			return nil, c.fail(ErrBuiltin, "builtin", name+": operand is not an array's storage")
		}
		return []ir.Value{c.b.Ptr.Add(storage, c.b.I64.Const(stdlib.ArrayElements))}, nil
	}
	// An object's dynamic type: the metadata its header's metadata word
	// points at names it. See stdlib.HeapMetadataType.
	if verb == "vertexObjectType" {
		object, ok := args[0].(ir.Ptr)
		if len(args) != 1 || !ok {
			return nil, c.fail(ErrBuiltin, "builtin", name+": operand is not an object")
		}
		heap := c.b.Ptr.Load(object)
		return []ir.Value{c.b.Ptr.Load(c.b.Ptr.Add(heap, c.b.I64.Const(stdlib.HeapMetadataType)))}, nil
	}
	if len(args) < 2 {
		return nil, c.fail(ErrBuiltin, "builtin", name+": too few operands")
	}
	a, aok := args[0].(ir.Ptr)
	b, bok := args[1].(ir.Ptr)
	if !aok || !bok {
		return nil, c.fail(ErrBuiltin, "builtin", name+": operand is not an address")
	}
	switch verb {
	case "cmp_eq":
		return []ir.Value{c.b.Ptr.Eq(a, b)}, nil
	case "cmp_ne":
		return []ir.Value{c.b.Ptr.Ne(a, b)}, nil
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

// convertBuiltin translates width conversion builtins (sext, zext, trunc, fp/int conversions).
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

	// Float/integer representation conversion.
	if out, ok, err := c.floatConvertBuiltin(verb, from, to, args[0]); ok {
		return out, true, err
	}

	// A narrow integer's register holds its bits and whatever the last
	// operation left above them -- sign or zero, by that operation's
	// signedness -- so a conversion out of one extends from its width,
	// and one into one keeps its width's worth.
	switch {
	case from.reg == to.reg:
		// Same register class: Int8, Int16 and Int32 share one.
		a, isI32 := args[0].(ir.I32)
		if !isI32 {
			return []ir.Value{args[0]}, true, nil
		}
		switch {
		case verb == "sextOrBitCast" && from.narrow():
			return []ir.Value{c.narrow(a, from.width, true)}, true, nil
		case verb == "zextOrBitCast" && from.narrow():
			return []ir.Value{c.narrow(a, from.width, false)}, true, nil
		case verb == "truncOrBitCast" && to.narrow():
			return []ir.Value{c.narrow(a, to.width, true)}, true, nil
		}
		return []ir.Value{a}, true, nil

	case from.reg == ir.TypeI64 && to.reg == ir.TypeI32:
		a, ok := args[0].(ir.I64)
		if !ok {
			return nil, true, c.fail(ErrBuiltin, "builtin", name+": operand is not an i64")
		}
		w := c.b.I32.WrapI64(a)
		if to.narrow() {
			w = c.narrow(w, to.width, true)
		}
		return []ir.Value{w}, true, nil

	case from.reg == ir.TypeI32 && to.reg == ir.TypeI64:
		a, ok := args[0].(ir.I32)
		if !ok {
			return nil, true, c.fail(ErrBuiltin, "builtin", name+": operand is not an i32")
		}
		if from.narrow() {
			a = c.narrow(a, from.width, verb != "zextOrBitCast")
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
	for _, v := range [...]string{
		"truncOrBitCast_", "sextOrBitCast_", "zextOrBitCast_",
		"sitofp_", "uitofp_", "fptosi_", "fptoui_",
		"fpext_", "fptrunc_", "bitcast_",
	} {
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

// floatConvertBuiltin translates float-to-int and int-to-float conversions.
func (c *fn) floatConvertBuiltin(verb string, from, to repr, a ir.Value) ([]ir.Value, bool, error) {
	fail := func() ([]ir.Value, bool, error) {
		return nil, true, c.fail(ErrBuiltin, "builtin", verb+": operand of the wrong class")
	}
	switch verb {
	case "sitofp", "uitofp":
		n, ok := a.(ir.I64)
		if !ok {
			return fail()
		}
		signed := verb == "sitofp"
		switch to.reg {
		case ir.TypeF64:
			if signed {
				return []ir.Value{c.b.F64.SCvtI64(n)}, true, nil
			}
			return []ir.Value{c.b.F64.UCvtI64(n)}, true, nil
		case ir.TypeF32:
			if signed {
				return []ir.Value{c.b.F32.SCvtI64(n)}, true, nil
			}
			return []ir.Value{c.b.F32.UCvtI64(n)}, true, nil
		case ir.TypeF16:
			if signed {
				return []ir.Value{c.b.F16().SCvtI64(n)}, true, nil
			}
			return []ir.Value{c.b.F16().UCvtI64(n)}, true, nil
		case ir.TypeBF16:
			if signed {
				return []ir.Value{c.b.BF16().SCvtI64(n)}, true, nil
			}
			return []ir.Value{c.b.BF16().UCvtI64(n)}, true, nil
		}
		return fail()

	case "bitcast":
		// Within one class nothing moves: a metatype read as Any.Type.
		if from.reg == to.reg && !from.narrow() && !to.narrow() {
			return []ir.Value{a}, true, nil
		}
		// The same bits read as the other class: a Double's bitPattern.
		switch v := a.(type) {
		case ir.F64:
			if to.reg == ir.TypeI64 {
				return []ir.Value{c.b.I64.BitcastF64(v)}, true, nil
			}
		case ir.F32:
			if to.reg == ir.TypeI32 {
				return []ir.Value{c.b.I32.BitcastF32(v)}, true, nil
			}
		case ir.I64:
			if to.reg == ir.TypeF64 {
				return []ir.Value{c.b.F64.BitcastI64(v)}, true, nil
			}
		case ir.I32:
			if to.reg == ir.TypeF32 && !from.narrow() {
				return []ir.Value{c.b.F32.BitcastI32(v)}, true, nil
			}
			// A half's encoding from a UInt16, held in an i32.
			if to.reg == ir.TypeF16 {
				return []ir.Value{c.b.F16().BitcastI32(v)}, true, nil
			}
			if to.reg == ir.TypeBF16 {
				return []ir.Value{c.b.BF16().BitcastI32(v)}, true, nil
			}
		case ir.F16:
			if to.reg == ir.TypeI32 {
				return []ir.Value{c.b.I32.BitcastF16(v)}, true, nil
			}
		case ir.BF16:
			if to.reg == ir.TypeI32 {
				return []ir.Value{c.b.I32.BitcastBF16(v)}, true, nil
			}
		}
		return fail()

	case "fpext", "fptrunc":
		// Between any two float namespaces: widening is exact, narrowing
		// rounds once. f16 and bf16 are the same width and neither holds
		// the other, so either verb may name that pair.
		if v, ok := c.floatToFloat(a, to.reg); ok {
			return []ir.Value{v}, true, nil
		}
		return fail()

	case "fptosi", "fptoui":
		// Truncation toward zero into destination integer register.
		signed := verb == "fptosi"
		switch f := a.(type) {
		case ir.F64:
			switch to.reg {
			case ir.TypeI64:
				if signed {
					return []ir.Value{c.b.I64.SCvtF64(f)}, true, nil
				}
				return []ir.Value{c.b.I64.UCvtF64(f)}, true, nil
			case ir.TypeI32:
				if signed {
					return []ir.Value{c.b.I32.SCvtF64(f)}, true, nil
				}
				return []ir.Value{c.b.I32.UCvtF64(f)}, true, nil
			}
		case ir.F32:
			switch to.reg {
			case ir.TypeI64:
				if signed {
					return []ir.Value{c.b.I64.SCvtF32(f)}, true, nil
				}
				return []ir.Value{c.b.I64.UCvtF32(f)}, true, nil
			case ir.TypeI32:
				if signed {
					return []ir.Value{c.b.I32.SCvtF32(f)}, true, nil
				}
				return []ir.Value{c.b.I32.UCvtF32(f)}, true, nil
			}
		case ir.F16:
			switch to.reg {
			case ir.TypeI64:
				if signed {
					return []ir.Value{c.b.I64.SCvtF16(f)}, true, nil
				}
				return []ir.Value{c.b.I64.UCvtF16(f)}, true, nil
			case ir.TypeI32:
				if signed {
					return []ir.Value{c.b.I32.SCvtF16(f)}, true, nil
				}
				return []ir.Value{c.b.I32.UCvtF16(f)}, true, nil
			}
		case ir.BF16:
			switch to.reg {
			case ir.TypeI64:
				if signed {
					return []ir.Value{c.b.I64.SCvtBF16(f)}, true, nil
				}
				return []ir.Value{c.b.I64.UCvtBF16(f)}, true, nil
			case ir.TypeI32:
				if signed {
					return []ir.Value{c.b.I32.SCvtBF16(f)}, true, nil
				}
				return []ir.Value{c.b.I32.UCvtBF16(f)}, true, nil
			}
		}
		return fail()
	}
	return nil, false, nil
}

// floatToFloat converts between two float namespaces.
func (c *fn) floatToFloat(a ir.Value, to ir.RegType) (ir.Value, bool) {
	b := c.b
	switch v := a.(type) {
	case ir.F32:
		switch to {
		case ir.TypeF64:
			return b.F64.FCvtF32(v), true
		case ir.TypeF16:
			return b.F16().FCvtF32(v), true
		case ir.TypeBF16:
			return b.BF16().FCvtF32(v), true
		}
	case ir.F64:
		switch to {
		case ir.TypeF32:
			return b.F32.FCvtF64(v), true
		case ir.TypeF16:
			return b.F16().FCvtF64(v), true
		case ir.TypeBF16:
			return b.BF16().FCvtF64(v), true
		}
	case ir.F16:
		switch to {
		case ir.TypeF32:
			return b.F32.FCvtF16(v), true
		case ir.TypeF64:
			return b.F64.FCvtF16(v), true
		case ir.TypeBF16:
			return b.BF16().FCvtF16(v), true
		}
	case ir.BF16:
		switch to {
		case ir.TypeF32:
			return b.F32.FCvtBF16(v), true
		case ir.TypeF64:
			return b.F64.FCvtBF16(v), true
		case ir.TypeF16:
			return b.F16().FCvtBF16(v), true
		}
	}
	return nil, false
}
