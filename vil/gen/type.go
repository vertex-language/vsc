package gen

import (
	"github.com/vertex-language/vsc/types"
	"github.com/vertex-language/vsc/vil"
)

// Type lowering: from what a program says a type is to what the IR
// has to move around.
//
// The formal type survives — a VIL type is the analyzer's type with
// an address bit — so the interesting part is the conventions, which
// are ownership crossing a call boundary. Swift's defaults, because
// they are the ones `swiftc -emit-silgen` prints:
//
//	trivial      no convention at all; there is nothing to own
//	borrowing    @guaranteed, and it is the default for a parameter
//	consuming    @owned
//	inout        @inout, and an address rather than a value
//	returning    @owned, since the caller receives what it must release

// lowerType is the VIL type of a formal type.
func lowerType(t types.Type) vil.Type {
	if t == nil {
		return vil.Object(types.Typ[types.Void])
	}
	return vil.Object(t)
}

// paramConvention is how a parameter is passed.
func paramConvention(p *types.Param, t vil.Type) vil.ParamConvention {
	switch p.Ownership {
	case types.InOut:
		return vil.ParamInout
	case types.Consuming:
		return vil.ParamOwned
	case types.Borrowing:
		return vil.ParamGuaranteed
	}
	// An existential is passed by address. Its type is not known, so
	// its size is not either -- what crosses the call is a pointer to
	// the five words it occupies, which is what @in_guaranteed means
	// and what swiftc's own signature for such a parameter says.
	// A variadic parameter is the array the arguments were collected
	// into, whatever the element type is -- `items: Any...` is passed
	// as an `[Any]`, in one register, and not by address.
	if isExistentialType(p.BodyType()) {
		return vil.ParamInGuaranteed
	}
	if t.Trivial() {
		return vil.ParamUnowned
	}
	// Swift's default: a parameter is borrowed for the call, and the
	// caller keeps it alive across it.
	return vil.ParamGuaranteed
}

// resultConvention is how a result comes back. A caller receives
// something it owns and must release, unless there is nothing to own.
func resultConvention(t vil.Type) vil.ResultConvention {
	// An existential is not returned at all: five words do not fit
	// the return registers, so the caller sets storage aside and the
	// callee fills it in. That is `@out` in SIL and sret in the
	// object file, and swiftc's own `makeOne` writes the buffer, the
	// metadata and the table through x8.
	if t.IsValid() && t.Formal() != nil && isExistentialType(t.Formal()) {
		return vil.ResultOut
	}
	if t.Trivial() {
		return vil.ResultUnowned
	}
	return vil.ResultOwned
}

// isExistentialType reports whether a value of this type is a value
// whose type arrives with it. It is existentialOf's answer without
// the existential, so that what is passed by address here and what is
// boxed in witness.go are the same set and cannot drift apart.
func isExistentialType(t types.Type) bool {
	_, ok := existentialOf(t)
	return ok
}

// byAddress reports whether a parameter of this convention crosses
// the call as a pointer rather than as a value.
//
// Two do. An existential is passed by address because there is no
// other way to hold one; an inout parameter is passed by address
// because the callee writes through it -- swiftc's own code for
// `swapped(_ a: inout Int32, _ b: inout Int32)` reads x0 and x1 and
// stores through both.
func byAddress(c vil.ParamConvention) bool {
	return c == vil.ParamInGuaranteed || c == vil.ParamInout
}
