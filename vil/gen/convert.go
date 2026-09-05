package gen

import (
	"github.com/vertex-language/vsc/ast"
	"github.com/vertex-language/vsc/core"
	"github.com/vertex-language/vsc/types"
	"github.com/vertex-language/vsc/vil"
)

// Numeric conversions.
//
// `Int32(n)` is a call to an initializer the standard library declares,
// and there is no standard library here -- but what swiftc -O leaves
// once it has inlined that initializer is a shape this compiler can
// emit directly, and it is the same decision literal() and construct()
// already take for the same reason.
//
// For `Int32(n)` where n is an Int, the whole of it:
//
//	%2 = struct_extract %0, #Int._value
//	%4 = builtin "cmp_slt_Int64"(%2, %3) : $Builtin.Int1
//	cond_fail %4, "Not enough bits to represent a signed value"
//	%7 = builtin "cmp_slt_Int64"(%6, %2) : $Builtin.Int1
//	cond_fail %7, "Not enough bits to represent the passed value"
//	%9 = builtin "truncOrBitCast_Int64_Int32"(%2) : $Builtin.Int32
//	%10 = struct $Int32 (%9)
//
// Two facts in that output are the ones worth having. A conversion
// that cannot represent its argument traps rather than wrapping, which
// is what makes `Int32(someInt)` different from C's cast -- and the
// message says which end it fell off. And a widening conversion emits
// no check at all, because none can fail.
//
// Which checks are needed is not a table of type pairs but a question
// about ranges: the low check is emitted when the destination's
// minimum is above the source's, the high check when its maximum is
// below. That answers signed-to-unsigned, unsigned-to-signed, widening
// and narrowing without enumerating them, and it is why `Int(int32)`
// is a sign-extension and nothing else.

// convert lowers `T(x)` where both are integer types, and reports
// whether it was one.
func (g *gen) convert(e *ast.CallExpr, to types.Type) (*vil.Value, bool) {
	dst, ok := intRangeOf(to)
	if !ok {
		return nil, false
	}
	var args []*ast.CallArg
	if e.Args != nil {
		args = e.Args.Args
	}
	if len(args) != 1 || args[0].Label != nil {
		// `Int32(truncatingIfNeeded:)` and its neighbours are other
		// initializers with other rules, and none of them is this.
		return nil, false
	}
	from := g.info.Types[args[0].X]
	src, ok := intRangeOf(from)
	if !ok {
		return nil, false
	}

	v := g.rvalue(args[0].X)
	if v == nil {
		return nil, true
	}
	raw := g.machine(v, from)

	// The checks, each emitted only where it can fail.
	if src.min < dst.min {
		lo := g.blk.IntegerLiteral(vil.Object(builtinFor(from)), dst.min)
		bad := g.compareRaw("<", raw, lo, from)
		if bad == nil {
			g.unsupported(e)
			return nil, true
		}
		msg := "Not enough bits to represent a signed value"
		if !dst.signed {
			// The destination has no negative half at all, which is a
			// different mistake and swiftc says so.
			msg = "Negative value is not representable"
		}
		g.blk.CondFail(bad, msg)
	}
	if src.max > dst.max {
		hi := g.blk.IntegerLiteral(vil.Object(builtinFor(from)), int64(dst.max))
		bad := g.compareRaw("<", hi, raw, from)
		if bad == nil {
			g.unsupported(e)
			return nil, true
		}
		g.blk.CondFail(bad, "Not enough bits to represent the passed value")
	}

	// The value now fits, so the conversion is a change of width and
	// nothing more. Named the way SILGen names it: the source in the
	// verb, the destination after it.
	out := raw
	if src.machine != dst.machine {
		verb := "truncOrBitCast"
		if dst.bits > src.bits {
			verb = "sextOrBitCast"
			if !src.signed {
				verb = "zextOrBitCast"
			}
		}
		out = g.blk.Builtin(verb+"_"+src.machine+"_"+dst.machine,
			vil.Object(builtinFor(to)), raw)
	}
	return g.blk.Struct(lowerType(to), out), true
}

// compareRaw is compare on values already taken out of their structs,
// which is what a conversion has: the bounds are literals of the
// source's machine type rather than values the program wrote.
func (g *gen) compareRaw(op string, a, b *vil.Value, t types.Type) *vil.Value {
	bi, ok := core.Lower(op, t)
	if !ok {
		return nil
	}
	return g.blk.Builtin(bi.Name, vil.Object(builtinNamed(bi.Result)), a, b)
}

// intRange is what a conversion needs to know about an integer type:
// the range it can hold, and the machine type it is held in.
type intRange struct {
	min     int64
	max     uint64
	bits    int
	signed  bool
	machine string
}

func intRangeOf(t types.Type) (intRange, bool) {
	if t == nil {
		return intRange{}, false
	}
	b, ok := t.Underlying().(*types.Basic)
	if !ok || b.Info()&types.IsInteger == 0 {
		return intRange{}, false
	}
	signed := b.Info()&types.IsUnsigned == 0
	bits := 0
	switch b.Kind() {
	case types.Int8, types.UInt8:
		bits = 8
	case types.Int16, types.UInt16:
		bits = 16
	case types.Int32, types.UInt32:
		bits = 32
	case types.Int64, types.UInt64, types.Int, types.UInt:
		// Int and UInt are the word, and every target with a backend
		// here has a 64-bit one.
		bits = 64
	default:
		return intRange{}, false
	}
	info := intRange{bits: bits, signed: signed}
	if signed {
		info.min = -(int64(1) << (bits - 1))
		info.max = (uint64(1) << (bits - 1)) - 1
	} else {
		info.min = 0
		if bits == 64 {
			info.max = ^uint64(0)
		} else {
			info.max = (uint64(1) << bits) - 1
		}
	}
	// The machine type a value of it is held in, which is what names
	// the builtin.
	switch bits {
	case 8:
		info.machine = "Int8"
	case 16:
		info.machine = "Int16"
	case 32:
		info.machine = "Int32"
	default:
		info.machine = "Int64"
	}
	return info, true
}
