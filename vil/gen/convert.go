package gen

import (
	"math"

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
	var args []*ast.CallArg
	if e.Args != nil {
		args = e.Args.Args
	}
	if len(args) != 1 || args[0].Label != nil {
		// `Int32(truncatingIfNeeded:)` and its neighbours are other
		// initializers with other rules, and none of them is this.
		return nil, false
	}
	from := g.typeOf(args[0].X)

	// One pointer read as another. Every unsafe pointer is the same
	// address, so this changes what the checker knows and nothing the
	// machine does. See pointer.go.
	if v, isPointer := g.pointerConvert(e, args[0].X, from, to); isPointer {
		return v, true
	}

	// A conversion that ends in a floating-point type cannot fail:
	// every integer and every narrower float has a value there, even
	// where it rounds. See floatConvert.
	if isFloat(to) {
		return g.floatConvert(e, args[0].X, from, to)
	}
	dst, ok := intRangeOf(to)
	if !ok {
		return nil, false
	}
	if isFloat(from) {
		return g.intFromFloat(e, args[0].X, from, to, dst)
	}
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

// floatConvert lowers `Double(x)` and `Float(x)`.
//
// Nothing can fail. Every integer has a value in a floating-point
// type, and so does every narrower float; what a wide integer loses
// is precision rather than range, and Swift's initializer rounds
// rather than trapping. So this is the conversion and nothing else,
// which is also all `swiftc -O` leaves behind.
//
// An integer narrower than a word is widened first, because that is
// what swiftc does: `Double(Int32)` is `sextOrBitCast_Int32_Int64`
// and then `sitofp_Int64_FPIEEE64`, and an unsigned source is
// zero-extended and converted with `uitofp`.
func (g *gen) floatConvert(e *ast.CallExpr, arg ast.Expr, from, to types.Type) (*vil.Value, bool) {
	v := g.rvalue(arg)
	if v == nil {
		return nil, true
	}
	raw := g.machine(v, from)
	if raw == nil {
		g.unsupported(e)
		return nil, true
	}
	dst := floatBuiltinName(to)
	result := lowerType(to)

	if isFloat(from) {
		src := floatBuiltinName(from)
		if src == dst {
			return g.blk.Struct(result, raw), true
		}
		verb := "fpext"
		if floatBits(to) < floatBits(from) {
			verb = "fptrunc"
		}
		out := g.blk.Builtin(verb+"_"+src+"_"+dst, vil.Object(builtinNamed(dst)), raw)
		return g.blk.Struct(result, out), true
	}

	src, ok := intRangeOf(from)
	if !ok {
		return nil, false
	}
	// A word first: swiftc converts from Int64 whatever the source's
	// own width is, and the extension says which half of the range
	// the value came from.
	if src.machine != "Int64" {
		verb := "sextOrBitCast"
		if !src.signed {
			verb = "zextOrBitCast"
		}
		raw = g.blk.Builtin(verb+"_"+src.machine+"_Int64",
			vil.Object(vil.BuiltinInt64), raw)
	}
	verb := "sitofp"
	if !src.signed {
		verb = "uitofp"
	}
	out := g.blk.Builtin(verb+"_Int64_"+dst, vil.Object(builtinNamed(dst)), raw)
	return g.blk.Struct(result, out), true
}

// isFloat reports whether a type is one of the two floating-point
// ones.
func isFloat(t types.Type) bool {
	b, ok := t.Underlying().(*types.Basic)
	return ok && b.Info()&types.IsFloat != 0
}

// floatBuiltinName is what SIL calls a floating-point type.
func floatBuiltinName(t types.Type) string {
	if b, ok := t.Underlying().(*types.Basic); ok && b.Kind() == types.Float {
		return "FPIEEE32"
	}
	return "FPIEEE64"
}

// floatBits is how wide one is.
func floatBits(t types.Type) int {
	if floatBuiltinName(t) == "FPIEEE32" {
		return 32
	}
	return 64
}

// intFromFloat lowers `Int32(d)`: truncation toward zero, trapping
// where the value has no place in the destination.
//
// Swift's initializer traps rather than wrapping, so the bounds are
// checked before the conversion. They are written as values of the
// source's own type, which is what makes the test exact: a signed
// destination of n bits holds [-2^(n-1), 2^(n-1)) and an unsigned one
// holds [0, 2^n), and both powers of two are exactly representable in
// binary floating point at every width here -- 2^63 and 2^64 included.
// So `lo <= d && d < hi` is the range and not an approximation of it.
//
// The test is written as two refusals of the negation rather than as
// one conjunction, which is also what catches NaN: a NaN compares
// false against every bound, so it fails the first test and traps
// there.
func (g *gen) intFromFloat(e *ast.CallExpr, arg ast.Expr, from, to types.Type,
	dst intRange) (*vil.Value, bool) {

	v := g.rvalue(arg)
	if v == nil {
		return nil, true
	}
	raw := g.machine(v, from)
	if raw == nil {
		g.unsupported(e)
		return nil, true
	}
	src := floatBuiltinName(from)
	srcT := vil.Object(builtinNamed(src))

	lo, hi := floatBounds(dst)
	loV := g.blk.FloatLiteral(srcT, floatLiteralBits(from, lo))
	hiV := g.blk.FloatLiteral(srcT, floatLiteralBits(from, hi))

	for _, check := range []struct {
		a, b *vil.Value
		msg  string
	}{
		{loV, raw, "Float value cannot be converted to " + typeName(to) +
			" because it is less than " + typeName(to) + ".min"},
		{raw, hiV, "Float value cannot be converted to " + typeName(to) +
			" because it is either infinite, NaN, or greater than " +
			typeName(to) + ".max"},
	} {
		ok := g.compareRaw("<=", check.a, check.b, from)
		if check.a == raw {
			ok = g.compareRaw("<", check.a, check.b, from)
		}
		if ok == nil {
			g.unsupported(e)
			return nil, true
		}
		bad := g.blk.Builtin("xor_Int1", vil.Object(vil.BuiltinInt1),
			ok, g.blk.IntegerLiteral(vil.Object(vil.BuiltinInt1), -1))
		g.blk.CondFail(bad, check.msg)
	}

	// The value fits, so what is left is the instruction. A
	// destination narrower than a word arrives in an i32 and is
	// narrowed after, which is the same shape the integer path has.
	verb := "fptosi"
	if !dst.signed {
		verb = "fptoui"
	}
	out := g.blk.Builtin(verb+"_"+src+"_"+dst.machine,
		vil.Object(builtinNamed(dst.machine)), raw)
	return g.blk.Struct(lowerType(to), out), true
}

// floatBounds is the half-open range a destination integer type
// holds, as the two numbers a comparison against the source's own
// type needs.
func floatBounds(dst intRange) (lo, hi float64) {
	if !dst.signed {
		return 0, math.Ldexp(1, dst.bits)
	}
	return -math.Ldexp(1, dst.bits-1), math.Ldexp(1, dst.bits-1)
}

// floatLiteralBits is a number written as the bit pattern of the
// source's own floating-point type, which is what a float literal
// carries.
func floatLiteralBits(from types.Type, v float64) int64 {
	if b, ok := from.Underlying().(*types.Basic); ok && b.Kind() == types.Float {
		return int64(math.Float32bits(float32(v)))
	}
	return int64(math.Float64bits(v))
}
